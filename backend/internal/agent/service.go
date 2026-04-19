package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/teslashibe/agent-setup/backend/internal/config"
)

// Service runs Claude agent loops, persisting every turn to TimescaleDB
// and streaming events back to the caller.
type Service struct {
	cfg      config.Config
	store    *Store
	registry *Registry
	client   anthropic.Client
}

func NewService(cfg config.Config, store *Store, registry *Registry) (*Service, error) {
	if cfg.AnthropicAPIKey == "" {
		return nil, errors.New("ANTHROPIC_API_KEY is required")
	}
	if registry == nil {
		registry = DefaultRegistry()
	}

	client := anthropic.NewClient(option.WithAPIKey(cfg.AnthropicAPIKey))

	return &Service{
		cfg:      cfg,
		store:    store,
		registry: registry,
		client:   client,
	}, nil
}

func (s *Service) Store() *Store      { return s.store }
func (s *Service) Registry() *Registry { return s.registry }

// Run executes one user turn against an existing session.
//
// It loads the session's prior messages, appends the user message, then enters
// the tool-use loop. For each iteration it calls Messages.New, persists the
// assistant turn, executes any requested tools, persists their results as a
// follow-up user turn, and repeats until the model stops calling tools or the
// max iteration count is hit. Events are pushed to the returned channel as
// they happen; the channel is closed when the run completes.
func (s *Service) Run(ctx context.Context, sess Session, userText string) (<-chan Event, error) {
	prior, err := s.store.ListMessages(ctx, sess.ID)
	if err != nil {
		return nil, fmt.Errorf("load history: %w", err)
	}

	history := make([]anthropic.MessageParam, 0, len(prior)+1)
	for _, m := range prior {
		mp, err := messageParamFromStored(m)
		if err != nil {
			return nil, fmt.Errorf("decode stored message %s: %w", m.ID, err)
		}
		history = append(history, mp)
	}

	userMsg := anthropic.NewUserMessage(anthropic.NewTextBlock(userText))
	userContent, err := json.Marshal(userMsg.Content)
	if err != nil {
		return nil, fmt.Errorf("marshal user content: %w", err)
	}
	if _, err := s.store.AppendMessage(ctx, sess.ID, "user", userContent, nil, nil); err != nil {
		return nil, fmt.Errorf("persist user message: %w", err)
	}
	history = append(history, userMsg)

	events := make(chan Event, 32)

	go func() {
		defer close(events)
		s.runLoop(ctx, sess, history, events)
	}()

	return events, nil
}

func (s *Service) runLoop(ctx context.Context, sess Session, history []anthropic.MessageParam, events chan<- Event) {
	model := s.cfg.AnthropicModel
	if sess.Model != nil && *sess.Model != "" {
		model = *sess.Model
	}
	systemPrompt := s.cfg.AgentSystemPrompt
	if sess.SystemPrompt != nil && *sess.SystemPrompt != "" {
		systemPrompt = *sess.SystemPrompt
	}

	tools := s.buildTools()

	for iter := 0; iter < s.cfg.AgentMaxIterations; iter++ {
		params := anthropic.MessageNewParams{
			Model:     anthropic.Model(model),
			MaxTokens: int64(s.cfg.AnthropicMaxTokens),
			Messages:  history,
			Tools:     tools,
		}
		if systemPrompt != "" {
			params.System = []anthropic.TextBlockParam{{Text: systemPrompt}}
		}

		resp, err := s.client.Messages.New(ctx, params)
		if err != nil {
			emit(events, Event{Type: "error", Error: err.Error()})
			return
		}

		assistantParam := resp.ToParam()
		assistantJSON, mErr := json.Marshal(assistantParam.Content)
		if mErr != nil {
			emit(events, Event{Type: "error", Error: "marshal assistant content: " + mErr.Error()})
			return
		}
		stopReason := string(resp.StopReason)
		usage := &Usage{
			InputTokens:  int(resp.Usage.InputTokens),
			OutputTokens: int(resp.Usage.OutputTokens),
		}
		if _, err := s.store.AppendMessage(ctx, sess.ID, "assistant", assistantJSON, &stopReason, usage); err != nil {
			emit(events, Event{Type: "error", Error: "persist assistant: " + err.Error()})
			return
		}

		for _, block := range resp.Content {
			switch v := block.AsAny().(type) {
			case anthropic.TextBlock:
				if v.Text != "" {
					emit(events, Event{Type: "text", Text: v.Text})
				}
			case anthropic.ToolUseBlock:
				input := v.JSON.Input.Raw()
				emit(events, Event{
					Type:   "tool_use",
					Tool:   v.Name,
					ToolID: v.ID,
					Input:  json.RawMessage(input),
				})
			}
		}

		emit(events, Event{Type: "usage", Usage: usage})

		history = append(history, assistantParam)

		toolResults := []anthropic.ContentBlockParamUnion{}
		for _, block := range resp.Content {
			tu, ok := block.AsAny().(anthropic.ToolUseBlock)
			if !ok {
				continue
			}
			result, isErr := s.executeTool(ctx, tu)
			resultJSON, _ := json.Marshal(result)

			emit(events, Event{
				Type:    "tool_result",
				Tool:    tu.Name,
				ToolID:  tu.ID,
				Output:  resultJSON,
				IsError: isErr,
			})

			toolResults = append(toolResults, anthropic.NewToolResultBlock(tu.ID, string(resultJSON), isErr))
		}

		if len(toolResults) == 0 {
			emit(events, Event{Type: "done"})
			_ = s.store.TouchSession(ctx, sess.ID)
			return
		}

		toolMsg := anthropic.NewUserMessage(toolResults...)
		toolJSON, mErr := json.Marshal(toolMsg.Content)
		if mErr != nil {
			emit(events, Event{Type: "error", Error: "marshal tool content: " + mErr.Error()})
			return
		}
		if _, err := s.store.AppendMessage(ctx, sess.ID, "user", toolJSON, nil, nil); err != nil {
			emit(events, Event{Type: "error", Error: "persist tool results: " + err.Error()})
			return
		}
		history = append(history, toolMsg)
	}

	emit(events, Event{Type: "error", Error: "max tool iterations reached"})
}

func (s *Service) executeTool(ctx context.Context, tu anthropic.ToolUseBlock) (any, bool) {
	tool, ok := s.registry.Get(tu.Name)
	if !ok {
		return map[string]string{"error": "unknown tool: " + tu.Name}, true
	}
	out, err := tool.Execute(ctx, json.RawMessage(tu.JSON.Input.Raw()))
	if err != nil {
		log.Printf("tool %q error: %v", tu.Name, err)
		return map[string]string{"error": err.Error()}, true
	}
	return out, false
}

func (s *Service) buildTools() []anthropic.ToolUnionParam {
	all := s.registry.All()
	out := make([]anthropic.ToolUnionParam, 0, len(all))
	for _, t := range all {
		schema := t.InputSchema()
		props, _ := schema["properties"].(map[string]any)
		tp := anthropic.ToolParam{
			Name:        t.Name(),
			Description: anthropic.String(t.Description()),
			InputSchema: anthropic.ToolInputSchemaParam{Properties: props},
		}
		out = append(out, anthropic.ToolUnionParam{OfTool: &tp})
	}
	return out
}

func emit(ch chan<- Event, ev Event) {
	select {
	case ch <- ev:
	default:
		// Channel buffer full or closed; drop the event rather than block the loop.
	}
}

// messageParamFromStored reconstructs a MessageParam from a persisted row.
// We stored the content blocks as raw JSON, so the SDK's JSON tags handle
// re-hydration directly.
func messageParamFromStored(m Message) (anthropic.MessageParam, error) {
	var blocks []anthropic.ContentBlockParamUnion
	if err := json.Unmarshal(m.Content, &blocks); err != nil {
		return anthropic.MessageParam{}, err
	}
	return anthropic.MessageParam{
		Role:    anthropic.MessageParamRole(m.Role),
		Content: blocks,
	}, nil
}
