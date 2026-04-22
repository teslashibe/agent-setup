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
	cfg         config.Config
	store       *Store
	client      anthropic.Client
	provisioner *Provisioner
}

// NewService constructs the agent service.
//
// The (cfg.AnthropicAgentID, cfg.AnthropicEnvID) pair is the historical
// shared-agent fallback used when no per-user [Provisioner] has been
// installed via [Service.UseProvisioner]. With MCP integration the
// recommended setup is to install a Provisioner so each user gets their own
// Agent + Environment + MCP toolset.
func NewService(cfg config.Config, store *Store) (*Service, error) {
	if cfg.AnthropicAPIKey == "" {
		return nil, errors.New("ANTHROPIC_API_KEY is required")
	}
	return &Service{
		cfg:    cfg,
		store:  store,
		client: anthropic.NewClient(option.WithAPIKey(cfg.AnthropicAPIKey)),
	}, nil
}

// UseProvisioner installs a per-user agent provisioner. When set,
// CreateSession lazily provisions an Anthropic Agent + Environment for the
// user (idempotent) and uses those IDs instead of the static cfg values.
func (s *Service) UseProvisioner(p *Provisioner) { s.provisioner = p }

// Client returns the underlying Anthropic client. Useful for the
// provisioner constructor in main.go to avoid re-instantiating the client.
func (s *Service) Client() anthropic.Client { return s.client }

func (s *Service) Store() *Store { return s.store }

// CreateSession provisions an Anthropic Managed Agent session and stores the
// mapping in our database, scoped to the active team. When a per-user
// [Provisioner] is installed it is used to look up (or lazily create) that
// user's Agent + Environment IDs. Otherwise the shared cfg.AnthropicAgentID /
// cfg.AnthropicEnvID pair is used.
func (s *Service) CreateSession(ctx context.Context, teamID, userID, title string) (Session, error) {
	agentID := s.cfg.AnthropicAgentID
	envID := s.cfg.AnthropicEnvID
	if s.provisioner != nil {
		ua, err := s.provisioner.EnsureForUser(ctx, userID)
		if err != nil {
			return Session{}, fmt.Errorf("ensure user agent: %w", err)
		}
		agentID, envID = ua.AgentID, ua.EnvironmentID
	}
	if agentID == "" || envID == "" {
		return Session{}, errors.New("agent: no agent provisioned — set ANTHROPIC_AGENT_ID/ANTHROPIC_ENVIRONMENT_ID or install a Provisioner")
	}

	antSess, err := s.client.Beta.Sessions.New(ctx, anthropic.BetaSessionNewParams{
		Agent:         anthropic.BetaSessionNewParamsAgentUnion{OfString: anthropic.String(agentID)},
		EnvironmentID: envID,
	})
	if err != nil {
		return Session{}, fmt.Errorf("create anthropic session: %w", err)
	}
	if strings.TrimSpace(title) == "" {
		title = "New chat"
	}
	return s.store.CreateSession(ctx, teamID, userID, title, antSess.ID)
}

// Run streams agent events for a user message. The Anthropic event stream is
// opened before sending the message to avoid a race condition.
func (s *Service) Run(ctx context.Context, sess Session, userText string) (<-chan Event, error) {
	events := make(chan Event, 32)
	go func() {
		defer close(events)

		stream := s.client.Beta.Sessions.Events.StreamEvents(ctx, sess.AnthropicSessionID,
			anthropic.BetaSessionEventStreamParams{})
		defer stream.Close()

		if err := s.sendUserMessage(ctx, sess.AnthropicSessionID, userText); err != nil {
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

func (s *Service) sendUserMessage(ctx context.Context, sessionID, text string) error {
	tb := anthropic.BetaManagedAgentsTextBlockParam{
		Type: anthropic.BetaManagedAgentsTextBlockTypeText,
		Text: text,
	}
	msg := anthropic.BetaManagedAgentsUserMessageEventParams{
		Type:    anthropic.BetaManagedAgentsUserMessageEventParamsTypeUserMessage,
		Content: []anthropic.BetaManagedAgentsUserMessageEventParamsContentUnion{{OfText: &tb}},
	}
	_, err := s.client.Beta.Sessions.Events.Send(ctx, sessionID, anthropic.BetaSessionEventSendParams{
		Events: []anthropic.BetaManagedAgentsEventParamsUnion{{OfUserMessage: &msg}},
	})
	return err
}

// History fetches event history from Anthropic and reconstructs it as
// conversation turns for the client.
func (s *Service) History(ctx context.Context, anthropicSessionID string) ([]Message, error) {
	var (
		messages  = []Message{}
		assistant *Message
	)
	flush := func() {
		if assistant != nil && len(assistant.Content) > 0 {
			messages = append(messages, *assistant)
		}
		assistant = nil
	}
	appendAssistant := func(b Block) {
		if assistant == nil {
			assistant = &Message{Role: "assistant"}
		}
		assistant.Content = append(assistant.Content, b)
	}

	iter := s.client.Beta.Sessions.Events.ListAutoPaging(ctx, anthropicSessionID,
		anthropic.BetaSessionEventListParams{Order: anthropic.BetaSessionEventListParamsOrderAsc})

	for iter.Next() {
		u := iter.Current()
		switch u.Type {
		case "user.message":
			flush()
			var blocks []Block
			for _, c := range u.AsUserMessage().Content {
				if c.Type == "text" && c.Text != "" {
					blocks = append(blocks, Block{Type: "text", Text: c.Text})
				}
			}
			if len(blocks) > 0 {
				messages = append(messages, Message{Role: "user", Content: blocks})
			}
		case "agent.message":
			for _, b := range u.AsAgentMessage().Content {
				if b.Text != "" {
					appendAssistant(Block{Type: "text", Text: b.Text})
				}
			}
		case "agent.tool_use":
			ev := u.AsAgentToolUse()
			appendAssistant(Block{Type: "tool_use", ID: ev.ID, Name: ev.Name})
		case "agent.tool_result":
			appendAssistant(Block{Type: "tool_result", ToolID: u.AsAgentToolResult().ToolUseID})
		}
	}
	flush()
	return messages, iter.Err()
}

func translateStreamEvent(u anthropic.BetaManagedAgentsStreamSessionEventsUnion) *Event {
	switch u.Type {
	case "agent.message":
		var text strings.Builder
		for _, b := range u.AsAgentMessage().Content {
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
		return &Event{Type: "error", Error: u.AsSessionError().Error.RawJSON()}
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
