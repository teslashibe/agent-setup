// Package brand encodes brand personas the per-user agent can adopt.
//
// The template ships a thin registry — no concrete personas — so a fork
// can register its brand identity in an `init()` on its own file (e.g.
// `internal/brand/myco.go`) without touching this file. The registry
// is consulted by `cmd/server/main.go` via the BRAND env var:
//
//	prompt, ok := brand.PromptForBrand(cfg.Brand)
//	opts := agent.ProvisionerOptions{}
//	if ok {
//	    opts.SystemPrompt = prompt
//	}
//
// Personas are intentionally Go code (not flat files) so changes are
// version-controlled and reviewed alongside the agent loop.
package brand

import (
	"strings"
	"sync"
)

// ID identifies a brand persona this build supports. The empty ID
// means "no brand — use the catch-all default prompt the template
// ships with" (e.g. the notifications system prompt, or no prompt).
type ID string

const IDNone ID = ""

// Persona is a single brand's system-prompt + tool-restriction bundle.
// SystemPrompt is the per-user agent's persistent persona (passed to
// Anthropic's Managed Agent API at provision time). ToolAllowlist is
// the deliberately small set of MCP tool names the agent is permitted
// to call — empty means "no restriction beyond the MCP server's own
// platform gating".
type Persona struct {
	SystemPrompt  string
	ToolAllowlist []string
}

// Register adds (or replaces) the persona for the given ID. Forks
// call this from an `init()` in their own brand file. Identifier
// matching is case-insensitive and whitespace-trimmed.
//
// Calling Register from main code instead of init() also works —
// the registry is plain mutex-guarded state — but the conventional
// pattern is init() so the registry is fully populated before
// PromptForBrand is consulted in main.go.
func Register(id ID, p Persona) {
	mu.Lock()
	defer mu.Unlock()
	if personas == nil {
		personas = map[ID]Persona{}
	}
	personas[normalize(string(id))] = p
}

// PromptForBrand returns the system prompt for the given brand
// identifier (case-insensitive, whitespace-trimmed). Returns
// ok=false when the identifier is unknown OR when the registered
// persona has an empty SystemPrompt — callers should fall back to
// the template default and log a warning rather than failing
// startup.
func PromptForBrand(id string) (prompt string, ok bool) {
	mu.RLock()
	defer mu.RUnlock()
	p, found := personas[normalize(id)]
	if !found || p.SystemPrompt == "" {
		return "", false
	}
	return p.SystemPrompt, true
}

// AllowlistForBrand returns the MVP MCP tool allowlist for the
// brand, if any. Reserved for the registry-composition layer that
// brand-aware mounts can use to filter platform tools at the MCP
// server. Today the brand prompt itself self-restricts; tool-level
// allowlisting at the server is a follow-up.
func AllowlistForBrand(id string) (tools []string, ok bool) {
	mu.RLock()
	defer mu.RUnlock()
	p, found := personas[normalize(id)]
	if !found || len(p.ToolAllowlist) == 0 {
		return nil, false
	}
	out := make([]string, len(p.ToolAllowlist))
	copy(out, p.ToolAllowlist)
	return out, true
}

// Registered returns the sorted list of registered brand identifiers
// (lowercased). Useful for `--list-brands` style CLI affordances and
// for logging at startup so operators can sanity-check that their
// fork's init() ran.
func Registered() []ID {
	mu.RLock()
	defer mu.RUnlock()
	ids := make([]ID, 0, len(personas))
	for id := range personas {
		ids = append(ids, id)
	}
	return ids
}

func normalize(id string) ID {
	return ID(strings.ToLower(strings.TrimSpace(id)))
}

var (
	mu       sync.RWMutex
	personas map[ID]Persona
)
