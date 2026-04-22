// codegen-demo is a tiny CLI that proves the local Claude Code wiring works
// end-to-end. It loads CODEGEN_* env vars, builds an Agent, runs it against
// the current directory with the prompt provided on argv, and prints the
// captured output.
//
// Usage:
//
//	go run ./cmd/codegen-demo "Summarise this directory in one paragraph."
//
// Prerequisites: `claude` on PATH and `claude login` already done (or
// CODEGEN_AGENT=generic with CODEGEN_COMMAND set to your CLI of choice).
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"

	"github.com/teslashibe/agent-setup/backend/internal/codegen"
)

func main() {
	_ = godotenv.Load(".env", "backend/.env")

	if len(os.Args) < 2 {
		log.Fatalf("usage: %s <prompt> [workDir]", filepath.Base(os.Args[0]))
	}
	prompt := os.Args[1]
	workDir, _ := os.Getwd()
	if len(os.Args) >= 3 {
		workDir = os.Args[2]
	}

	agent, err := codegen.LoadFromEnv()
	if err != nil {
		log.Fatalf("load agent: %v", err)
	}

	res, err := agent.Run(context.Background(), prompt, workDir)
	if err != nil {
		log.Fatalf("%s failed (exit=%d): %v\n--- output (tail) ---\n%s",
			agent.Name(), res.ExitCode, err, tail(res.Output, 4000))
	}

	fmt.Printf("--- %s (%s) ---\n%s\n",
		agent.Name(), res.Duration.Round(1e6), res.Output)
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
