package codegen

import (
	"strings"
	"testing"
)

func TestLoadFromEnv_Defaults(t *testing.T) {
	t.Setenv("CODEGEN_AGENT", "")
	t.Setenv("CODEGEN_COMMAND", "")
	t.Setenv("CODEGEN_ARGS", "")

	a, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv: %v", err)
	}
	if a.Name() != "claude-code" {
		t.Fatalf("Name() = %q, want claude-code", a.Name())
	}
}

func TestLoadFromEnv_Generic(t *testing.T) {
	t.Setenv("CODEGEN_AGENT", "generic")
	t.Setenv("CODEGEN_COMMAND", "echo")
	t.Setenv("CODEGEN_ARGS", "hi, there")

	a, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv: %v", err)
	}
	if a.Name() != "generic" {
		t.Fatalf("Name() = %q, want generic", a.Name())
	}
}

func TestLoadFromEnv_GenericMissingCommand(t *testing.T) {
	t.Setenv("CODEGEN_AGENT", "generic")
	t.Setenv("CODEGEN_COMMAND", "")

	_, err := LoadFromEnv()
	if err == nil {
		t.Fatal("expected error when generic agent has no command")
	}
	if !strings.Contains(err.Error(), "CODEGEN_COMMAND") {
		t.Fatalf("error %q should mention CODEGEN_COMMAND", err.Error())
	}
}

func TestSplitCSV(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"a", []string{"a"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{"a, b , c ", []string{"a", "b", "c"}},
		{"a,,b", []string{"a", "b"}},
	}
	for _, tc := range tests {
		got := splitCSV(tc.in)
		if len(got) != len(tc.want) {
			t.Fatalf("splitCSV(%q) = %v, want %v", tc.in, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("splitCSV(%q)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
			}
		}
	}
}
