// Package codegen wires github.com/teslashibe/codegen-go into the agent-setup
// template as an OPTIONAL local-execution path that runs the Claude Code CLI
// (or any other "prompt-on-stdin, edit-files-in-cwd" agent) inside this
// process's working directory.
//
// This is complementary to — not a replacement for — the Anthropic Managed
// Agents path used by the `internal/agent` package. Use this shim when you
// need the agent to:
//
//   - Run on this machine (filesystem access, on-prem, off-platform tools).
//   - Drive a different CLI (Codex, Aider, OpenHands, Cline, your own).
//   - Operate inside a long-running process under your direct control.
//
// Configuration is read from CODEGEN_* environment variables; see
// LoadFromEnv. Defaults match codegen-go upstream.
package codegen

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	cgg "github.com/teslashibe/codegen-go"
)

// Re-exports so callers can keep using `codegen.X` without juggling two
// import paths.
type (
	Agent  = cgg.Agent
	Config = cgg.Config
	Result = cgg.Result
)

// New constructs an Agent directly from cfg. Thin alias around cgg.NewAgent.
func New(cfg Config) (Agent, error) { return cgg.NewAgent(cfg) }

// LoadFromEnv builds an Agent from CODEGEN_* environment variables:
//
//	CODEGEN_AGENT             "claude-code" (default) | "generic"
//	CODEGEN_MODEL             optional --model override forwarded to claude
//	CODEGEN_TIMEOUT           Go duration (default 30m)
//	CODEGEN_MAX_OUTPUT_BYTES  cap on captured stdout+stderr (default 10 MiB)
//	CODEGEN_COMMAND           binary for the generic CLI agent
//	CODEGEN_ARGS              comma-separated argv prepended to the generic CLI
//
// Returns an error if CODEGEN_AGENT is unrecognised or if generic mode is
// selected without CODEGEN_COMMAND.
func LoadFromEnv() (Agent, error) {
	cfg := Config{
		Type:           getEnv("CODEGEN_AGENT", "claude-code"),
		Model:          strings.TrimSpace(os.Getenv("CODEGEN_MODEL")),
		Timeout:        getDurationEnv("CODEGEN_TIMEOUT", cgg.DefaultTimeout),
		MaxOutputBytes: getIntEnv("CODEGEN_MAX_OUTPUT_BYTES", cgg.DefaultMaxOutputBytes),
		Command:        strings.TrimSpace(os.Getenv("CODEGEN_COMMAND")),
		Args:           splitCSV(os.Getenv("CODEGEN_ARGS")),
	}
	if cfg.Type == "generic" && cfg.Command == "" {
		return nil, fmt.Errorf("codegen: CODEGEN_AGENT=generic requires CODEGEN_COMMAND")
	}
	return cgg.NewAgent(cfg)
}

func getEnv(key, fallback string) string {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return fallback
	}
	return strings.TrimSpace(v)
}

func getIntEnv(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}

func getDurationEnv(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return d
}

func splitCSV(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
