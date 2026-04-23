package brand

import (
	"testing"
)

// resetRegistry snapshots the existing personas map, clears it for
// the duration of the test, and restores the snapshot via t.Cleanup.
// This lets the registry tests run in isolation while preserving any
// init()-registered personas a fork's brand file added at package
// load — the snapshot survives the wipe and is reinstated when the
// test returns.
func resetRegistry(t *testing.T) {
	t.Helper()
	mu.Lock()
	saved := make(map[ID]Persona, len(personas))
	for k, v := range personas {
		saved[k] = v
	}
	personas = map[ID]Persona{}
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		personas = saved
		mu.Unlock()
	})
}

func TestPromptForBrand_UnknownReturnsFalse(t *testing.T) {
	resetRegistry(t)
	if got, ok := PromptForBrand("nope"); ok || got != "" {
		t.Fatalf("PromptForBrand(unknown) = (%q, %v), want (\"\", false)", got, ok)
	}
}

func TestPromptForBrand_EmptyIDReturnsFalse(t *testing.T) {
	resetRegistry(t)
	Register("acme", Persona{SystemPrompt: "you are acme"})
	if got, ok := PromptForBrand(""); ok || got != "" {
		t.Fatalf("PromptForBrand(\"\") = (%q, %v), want (\"\", false)", got, ok)
	}
}

func TestPromptForBrand_RegisteredCaseInsensitive(t *testing.T) {
	resetRegistry(t)
	Register("acme", Persona{SystemPrompt: "you are acme"})
	for _, in := range []string{"acme", "ACME", "  Acme  "} {
		got, ok := PromptForBrand(in)
		if !ok {
			t.Fatalf("PromptForBrand(%q) ok=false, want true", in)
		}
		if got != "you are acme" {
			t.Fatalf("PromptForBrand(%q) = %q, want %q", in, got, "you are acme")
		}
	}
}

func TestPromptForBrand_EmptySystemPromptReturnsFalse(t *testing.T) {
	resetRegistry(t)
	Register("acme", Persona{ToolAllowlist: []string{"x"}})
	if got, ok := PromptForBrand("acme"); ok || got != "" {
		t.Fatalf("PromptForBrand(empty-prompt) = (%q, %v), want (\"\", false)", got, ok)
	}
}

func TestAllowlistForBrand_RegisteredReturnsCopy(t *testing.T) {
	resetRegistry(t)
	original := []string{"a", "b"}
	Register("acme", Persona{SystemPrompt: "p", ToolAllowlist: original})
	got, ok := AllowlistForBrand("acme")
	if !ok || len(got) != 2 {
		t.Fatalf("AllowlistForBrand(acme) = (%v, %v), want 2-elem slice", got, ok)
	}
	got[0] = "mutated"
	got2, _ := AllowlistForBrand("acme")
	if got2[0] == "mutated" {
		t.Fatalf("AllowlistForBrand returned shared slice — caller mutation leaked into registry")
	}
}

func TestAllowlistForBrand_EmptyReturnsFalse(t *testing.T) {
	resetRegistry(t)
	Register("acme", Persona{SystemPrompt: "p"})
	if got, ok := AllowlistForBrand("acme"); ok || got != nil {
		t.Fatalf("AllowlistForBrand(no-allowlist) = (%v, %v), want (nil, false)", got, ok)
	}
}

func TestRegistered_ListsBrands(t *testing.T) {
	resetRegistry(t)
	Register("alpha", Persona{SystemPrompt: "a"})
	Register("beta", Persona{SystemPrompt: "b"})
	got := Registered()
	if len(got) != 2 {
		t.Fatalf("Registered() len=%d, want 2 (got %v)", len(got), got)
	}
}

func TestRegister_Replaces(t *testing.T) {
	resetRegistry(t)
	Register("acme", Persona{SystemPrompt: "v1"})
	Register("acme", Persona{SystemPrompt: "v2"})
	got, ok := PromptForBrand("acme")
	if !ok || got != "v2" {
		t.Fatalf("PromptForBrand(acme) after re-register = (%q, %v), want (\"v2\", true)", got, ok)
	}
}
