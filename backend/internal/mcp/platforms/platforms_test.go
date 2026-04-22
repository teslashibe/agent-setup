package platforms

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/teslashibe/agent-setup/backend/internal/mcp"
)

// TestRegistryComposes verifies that every plugin returned by All() carries
// a non-nil Provider/NewClient/Validator and that the resulting registry
// builds without duplicate platform/tool collisions. This is the single
// most important integration test in the agent-setup MCP layer — adding a
// new platform that breaks any of these invariants fails CI immediately.
func TestRegistryComposes(t *testing.T) {
	plugins := All()
	if len(plugins) == 0 {
		t.Fatal("All() returned no plugins")
	}
	bindings := make([]mcp.PlatformBinding, 0, len(plugins))
	for i, p := range plugins {
		if p.Binding.Provider == nil {
			t.Errorf("plugin %d (%s) has nil Provider", i, p.Validator.Platform())
		}
		if p.Binding.NewClient == nil {
			t.Errorf("plugin %d (%s) has nil NewClient", i, p.Validator.Platform())
		}
		if p.Validator == nil {
			t.Errorf("plugin %d has nil Validator", i)
			continue
		}
		if p.Binding.Provider.Platform() != p.Validator.Platform() {
			t.Errorf("plugin %d platform mismatch: provider=%q validator=%q",
				i, p.Binding.Provider.Platform(), p.Validator.Platform())
		}
		bindings = append(bindings, p.Binding)
	}

	registry, err := mcp.NewRegistry(bindings...)
	if err != nil {
		t.Fatalf("NewRegistry(...): %v", err)
	}

	platformsList := registry.Platforms()
	if got := len(platformsList); got != len(plugins) {
		t.Errorf("registry has %d platforms; want %d", got, len(plugins))
	}

	tools := registry.Tools()
	if len(tools) == 0 {
		t.Fatal("registry has no tools — provider wiring is broken")
	}

	for _, tool := range tools {
		if tool.Name == "" {
			t.Errorf("tool with empty name in registry: %+v", tool)
		}
		if tool.Description == "" {
			t.Errorf("tool %s missing description", tool.Name)
		}
		if tool.InputSchema == nil {
			t.Errorf("tool %s missing input schema", tool.Name)
		}
		if tool.Invoke == nil {
			t.Errorf("tool %s missing Invoke", tool.Name)
		}
	}
}

// TestValidatorRejectsBadCredentials makes sure each platform's validator
// returns an error for an empty credential blob (the most common user
// mistake — pasting nothing into the settings UI). The two scoring
// platforms (xviral, redditviral) accept any credential by design.
func TestValidatorRejectsBadCredentials(t *testing.T) {
	allowEmpty := map[string]bool{
		"xviral":      true,
		"redditviral": true,
		"codegen":     true,
	}
	for _, p := range All() {
		platform := p.Validator.Platform()
		err := p.Validator.Validate(json.RawMessage(`{}`))
		switch {
		case allowEmpty[platform] && err != nil:
			t.Errorf("%s: validator rejected empty credential (allow-empty platform): %v", platform, err)
		case !allowEmpty[platform] && err == nil:
			t.Errorf("%s: validator accepted empty credential", platform)
		}
	}
}

// TestNewClientReportsMeaningfulErrors checks that calling NewClient with
// an empty credential blob returns an error whose message names the
// missing field, so users see a useful message in the chat UI when they
// haven't connected a platform.
func TestNewClientReportsMeaningfulErrors(t *testing.T) {
	allowEmpty := map[string]bool{
		"xviral":      true,
		"redditviral": true,
		"codegen":     true,
	}
	ctx := context.Background()
	for _, p := range All() {
		platform := p.Binding.Provider.Platform()
		_, err := p.Binding.NewClient(ctx, json.RawMessage(`{}`))
		if allowEmpty[platform] {
			if err != nil {
				t.Errorf("%s: NewClient on empty cred should succeed; got %v", platform, err)
			}
			continue
		}
		if err == nil {
			t.Errorf("%s: NewClient on empty cred should error", platform)
			continue
		}
		if !strings.Contains(err.Error(), platform) {
			t.Errorf("%s: error message %q should mention platform name", platform, err.Error())
		}
	}
}
