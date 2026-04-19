package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/teslashibe/agent-setup/backend/internal/config"
)

type Service struct {
	cfg    config.Config
	store  *Store
	client anthropic.Client
}

func NewService(cfg config.Config, store *Store) (*Service, error) {
	if cfg.AnthropicAPIKey == "" {
		return nil, errors.New("ANTHROPIC_API_KEY is required")
	}
	if cfg.AnthropicAgentID == "" || cfg.AnthropicEnvID == "" {
		return nil, errors.New("ANTHROPIC_AGENT_ID and ANTHROPIC_ENVIRONMENT_ID are required — run: make managed-agents-provision")
	}
	return &Service{
		cfg:    cfg,
		store:  store,
		client: anthropic.NewClient(option.WithAPIKey(cfg.AnthropicAPIKey)),
	}, nil
}

func (s *Service) Store() *Store { return s.store }

// CreateSession provisions an Anthropic Managed Agent session and stores the
// mapping in our database.
func (s *Service) CreateSession(ctx context.Context, userID, title string) (Session, error) {
	antSess, err := s.client.Beta.Sessions.New(ctx, anthropic.BetaSessionNewParams{
		Agent: anthropic.BetaSessionNewParamsAgentUnion{
			OfString: anthropic.String(s.cfg.AnthropicAgentID),
		},
		EnvironmentID: s.cfg.AnthropicEnvID,
	})
	if err != nil {
		return Session{}, fmt.Errorf("create anthropic session: %w", err)
	}
	if strings.TrimSpace(title) == "" {
		title = "New chat"
	}
	return s.store.CreateSession(ctx, userID, title, antSess.ID)
}

// Run streams agent events for a user message. The Anthropic event stream is
// opened before sending the message to avoid a race condition, then events are
// translated and emitted on the returned channel.
func (s *Service) Run(ctx context.Context, sess Session, userText string) (<-chan Event, error) {
	events := make(chan Event, 32)
	go func() {
		defer close(events)

		stream := s.client.Beta.Sessions.Events.StreamEvents(ctx, sess.AnthropicSessionID,
			anthropic.BetaSessionEventStreamParams{})
		defer stream.Close()

		textBlock := anthropic.BetaManagedAgentsTextBlockParam{
			Type: anthropic.BetaManagedAgentsTextBlockTypeText,
			Text: userText,
		}
		userMsg := anthropic.BetaManagedAgentsUserMessageEventParams{
			Type: anthropic.BetaManagedAgentsUserMessageEventParamsTypeUserMessage,
			Content: []anthropic.BetaManagedAgentsUserMessageEventParamsContentUnion{
				{OfText: &textBlock},
			},
		}
		if _, err := s.client.Beta.Sessions.Events.Send(ctx, sess.AnthropicSessionID,
			anthropic.BetaSessionEventSendParams{
				Events: []anthropic.BetaManagedAgentsEventParamsUnion{
					{OfUserMessage: &userMsg},
				},
			}); err != nil {
			emit(events, Event{Type: "error", Error: err.Error()})
			return
		}

		for stream.Next() {
			ev := translateStreamEvent(stream.Current())
			if ev == nil {
				continue
			}
			emit(events, *ev)
			if ev.Type == "done" || ev.Type == "error" {
				return
			}
		}
		if err := stream.Err(); err != nil {
			emit(events, Event{Type: "error", Error: err.Error()})
		}
	}()
	return events, nil
}

// History fetches the full event history for a session from Anthropic and
// reconstructs it as conversation turns for the client.
func (s *Service) History(ctx context.Context, anthropicSessionID string) ([]Message, error) {
	var messages []Message
	var assistantMsg *Message

	flushAssistant := func() {
		if assistantMsg != nil && len(assistantMsg.Content) > 0 {
			messages = append(messages, *assistantMsg)
			assistantMsg = nil
		}
	}

	iter := s.client.Beta.Sessions.Events.ListAutoPaging(ctx, anthropicSessionID,
		anthropic.BetaSessionEventListParams{
			Order: anthropic.BetaSessionEventListParamsOrderAsc,
		})

	for iter.Next() {
		u := iter.Current()
		switch u.Type {
		case "user.message":
			flushAssistant()
			ev := u.AsUserMessage()
			var blocks []Block
			for _, c := range ev.Content {
				if c.Type == "text" && c.Text != "" {
					blocks = append(blocks, Block{Type: "text", Text: c.Text})
				}
			}
			if len(blocks) > 0 {
				messages = append(messages, Message{Role: "user", Content: blocks})
			}

		case "agent.message":
			if assistantMsg == nil {
				assistantMsg = &Message{Role: "assistant"}
			}
			ev := u.AsAgentMessage()
			for _, b := range ev.Content {
				if b.Text != "" {
					assistantMsg.Content = append(assistantMsg.Content, Block{Type: "text", Text: b.Text})
				}
			}

		case "agent.tool_use":
			if assistantMsg == nil {
				assistantMsg = &Message{Role: "assistant"}
			}
			ev := u.AsAgentToolUse()
			assistantMsg.Content = append(assistantMsg.Content, Block{
				Type: "tool_use", ID: ev.ID, Name: ev.Name,
			})

		case "agent.tool_result":
			if assistantMsg == nil {
				assistantMsg = &Message{Role: "assistant"}
			}
			ev := u.AsAgentToolResult()
			assistantMsg.Content = append(assistantMsg.Content, Block{
				Type: "tool_result", ToolID: ev.ToolUseID,
			})
		}
	}
	flushAssistant()

	if err := iter.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}

func translateStreamEvent(u anthropic.BetaManagedAgentsStreamSessionEventsUnion) *Event {
	switch u.Type {
	case "agent.message":
		ev := u.AsAgentMessage()
		var text strings.Builder
		for _, b := range ev.Content {
			text.WriteString(b.Text)
		}
		if text.Len() == 0 {
			return nil
		}
		return &Event{Type: "text", Text: text.String()}

	case "agent.tool_use":
		ev := u.AsAgentToolUse()
		return &Event{Type: "tool_use", Tool: ev.Name, ToolID: ev.ID}

	case "agent.tool_result":
		ev := u.AsAgentToolResult()
		return &Event{Type: "tool_result", ToolID: ev.ToolUseID, IsError: ev.IsError}

	case "session.status_idle":
		return &Event{Type: "done"}

	case "session.error":
		ev := u.AsSessionError()
		return &Event{Type: "error", Error: ev.Error.RawJSON()}

	case "session.status_terminated":
		return &Event{Type: "error", Error: "session terminated"}
	}
	return nil
}

func emit(ch chan<- Event, ev Event) {
	select {
	case ch <- ev:
	default:
	}
}
