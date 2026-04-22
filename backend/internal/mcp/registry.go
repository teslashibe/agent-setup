// Package mcp implements the agent-setup MCP server: a per-user, per-platform
// HTTP+JSON-RPC endpoint that exposes the tool surfaces of every registered
// platform package (linkedin-go/mcp, x-go/mcp, …) to an Anthropic Managed
// Agent.
//
// The server is intentionally a thin shell:
//
//   - Tool definitions come from each platform's mcp/ subpackage via
//     mcptool.Provider.
//   - Per-user authentication is shared with the rest of the API
//     (Bearer JWT issued by magiclink-auth-go).
//   - Per-user, per-platform credentials are looked up from the
//     credentials.Service and passed as the opaque client argument to each
//     tool's Invoke.
//   - Response shaping (truncation, pagination caps, byte caps) is applied
//     uniformly by ResponseShaper rather than being per-tool concerns.
//
// Drift prevention: each platform's mcp_test.go enforces that every exported
// *Client method is wrapped or explicitly excluded. Adding a new platform
// to agent-setup is a single line in cmd/server/main.go.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/teslashibe/mcptool"
)

// PlatformBinding wires a single platform's MCP provider to the host.
//
//   - Provider is the package's mcptool.Provider implementation (typically a
//     zero-value linkmcp.Provider{}).
//   - NewClient receives the user's decrypted credential JSON and returns
//     the concrete *Client value the provider's tools expect. It is called
//     at most once per (user, platform) per request and the result is
//     cached for the duration of the request.
//   - ValidateCredential is optional; when present, the credential service
//     calls it before persisting the credential so users get fast feedback
//     on bad cookie blobs.
type PlatformBinding struct {
	Provider           mcptool.Provider
	NewClient          func(ctx context.Context, credential json.RawMessage) (any, error)
	ValidateCredential func(credential json.RawMessage) error
}

// Platform returns the platform identifier for this binding (delegates to
// the embedded provider).
func (b PlatformBinding) Platform() string { return b.Provider.Platform() }

// Registry is the read-only set of platform bindings the MCP server hosts.
// Build one with NewRegistry and pass it to NewServer.
type Registry struct {
	mu       sync.RWMutex
	bindings map[string]PlatformBinding
	tools    map[string]toolEntry // tool name → entry
}

type toolEntry struct {
	platform string
	tool     mcptool.Tool
}

// NewRegistry constructs a Registry from a slice of bindings. Returns an
// error on duplicate platform IDs, duplicate tool names across providers, or
// any per-provider validation failure (mcptool.ValidateTools).
func NewRegistry(bindings ...PlatformBinding) (*Registry, error) {
	r := &Registry{
		bindings: map[string]PlatformBinding{},
		tools:    map[string]toolEntry{},
	}
	for _, b := range bindings {
		if b.Provider == nil {
			return nil, errors.New("mcp: PlatformBinding.Provider must not be nil")
		}
		p := b.Platform()
		if p == "" {
			return nil, errors.New("mcp: PlatformBinding has empty platform")
		}
		if _, dup := r.bindings[p]; dup {
			return nil, fmt.Errorf("mcp: duplicate platform binding %q", p)
		}
		tools := b.Provider.Tools()
		if err := mcptool.ValidateTools(tools); err != nil {
			return nil, fmt.Errorf("mcp: validate %s tools: %w", p, err)
		}
		for _, t := range tools {
			if existing, dup := r.tools[t.Name]; dup {
				return nil, fmt.Errorf("mcp: tool name %q registered by both %s and %s",
					t.Name, existing.platform, p)
			}
			r.tools[t.Name] = toolEntry{platform: p, tool: t}
		}
		r.bindings[p] = b
	}
	return r, nil
}

// Platforms returns the platform IDs in alphabetical order.
func (r *Registry) Platforms() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.bindings))
	for p := range r.bindings {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// Tools returns every registered tool, sorted by name.
func (r *Registry) Tools() []mcptool.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]mcptool.Tool, 0, len(r.tools))
	for _, e := range r.tools {
		out = append(out, e.tool)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Lookup returns the platform and tool for a given tool name.
func (r *Registry) Lookup(name string) (platform string, tool mcptool.Tool, ok bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.tools[name]
	return e.platform, e.tool, ok
}

// Binding returns the binding for a given platform.
func (r *Registry) Binding(platform string) (PlatformBinding, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	b, ok := r.bindings[platform]
	return b, ok
}

// Bindings returns all bindings (in unspecified order).
func (r *Registry) Bindings() []PlatformBinding {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]PlatformBinding, 0, len(r.bindings))
	for _, b := range r.bindings {
		out = append(out, b)
	}
	return out
}
