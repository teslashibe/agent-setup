// provision creates the Anthropic Agent and Environment resources needed to run
// Claude Managed Agents. Run once and store the printed IDs in backend/.env.
//
//	make managed-agents-provision
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	apiKey := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
	if apiKey == "" {
		log.Fatal("ANTHROPIC_API_KEY is required in backend/.env")
	}
	systemPrompt := os.Getenv("AGENT_SYSTEM_PROMPT")
	if systemPrompt == "" {
		systemPrompt = "You are a helpful assistant."
	}

	client := anthropic.NewClient(option.WithAPIKey(apiKey))
	ctx := context.Background()

	fmt.Println("Creating Anthropic Agent...")
	agent, err := client.Beta.Agents.New(ctx, anthropic.BetaAgentNewParams{
		Name: "agent-setup",
		Model: anthropic.BetaManagedAgentsModelConfigParams{
			ID: anthropic.BetaManagedAgentsModelClaudeSonnet4_5_20250929,
		},
		System: anthropic.String(systemPrompt),
		Tools: []anthropic.BetaAgentNewParamsToolUnion{{
			OfAgentToolset20260401: &anthropic.BetaManagedAgentsAgentToolset20260401Params{
				Type: anthropic.BetaManagedAgentsAgentToolset20260401ParamsTypeAgentToolset20260401,
			},
		}},
	})
	if err != nil {
		log.Fatalf("create agent: %v", err)
	}
	fmt.Printf("Agent: %s (version %d)\n", agent.ID, agent.Version)

	fmt.Println("Creating Anthropic Environment...")
	env, err := client.Beta.Environments.New(ctx, anthropic.BetaEnvironmentNewParams{
		Name: "agent-setup-env",
		Config: anthropic.BetaCloudConfigParams{
			Networking: anthropic.BetaCloudConfigParamsNetworkingUnion{
				OfUnrestricted: &anthropic.BetaUnrestrictedNetworkParam{},
			},
		},
	})
	if err != nil {
		log.Fatalf("create environment: %v", err)
	}
	fmt.Printf("Environment: %s\n", env.ID)

	fmt.Println("\nAdd these to backend/.env:")
	fmt.Printf("ANTHROPIC_AGENT_ID=%s\n", agent.ID)
	fmt.Printf("ANTHROPIC_ENVIRONMENT_ID=%s\n", env.ID)
}
