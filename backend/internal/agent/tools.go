package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Tool is the interface every agent tool implements.
//
// Tools are invoked by the model via tool_use blocks. The Execute method's
// return value is JSON-marshaled and sent back as the tool_result.
type Tool interface {
	Name() string
	Description() string
	InputSchema() map[string]any
	Execute(ctx context.Context, input json.RawMessage) (any, error)
}

// Registry holds the tools available to the agent.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

func NewRegistry(tools ...Tool) *Registry {
	r := &Registry{tools: make(map[string]Tool)}
	for _, t := range tools {
		r.Register(t)
	}
	return r
}

func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
}

func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) All() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	return out
}

// DefaultRegistry returns a registry pre-populated with built-in tools.
// Add your own tools here or call Register at startup.
func DefaultRegistry() *Registry {
	return NewRegistry(
		&getCurrentTimeTool{},
	)
}

// getCurrentTimeTool is a trivial example tool. Replace / extend with real ones.
type getCurrentTimeTool struct{}

func (t *getCurrentTimeTool) Name() string { return "get_current_time" }

func (t *getCurrentTimeTool) Description() string {
	return "Returns the current UTC time in RFC3339 format. Optionally accepts an IANA timezone name."
}

func (t *getCurrentTimeTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"timezone": map[string]any{
				"type":        "string",
				"description": "Optional IANA timezone, e.g. 'America/New_York'. Defaults to UTC.",
			},
		},
	}
}

func (t *getCurrentTimeTool) Execute(_ context.Context, input json.RawMessage) (any, error) {
	var args struct {
		Timezone string `json:"timezone"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &args); err != nil {
			return nil, fmt.Errorf("invalid input: %w", err)
		}
	}

	loc := time.UTC
	if args.Timezone != "" {
		l, err := time.LoadLocation(args.Timezone)
		if err != nil {
			return nil, fmt.Errorf("invalid timezone %q: %w", args.Timezone, err)
		}
		loc = l
	}
	now := time.Now().In(loc)
	return map[string]any{
		"timezone": loc.String(),
		"time":     now.Format(time.RFC3339),
	}, nil
}
