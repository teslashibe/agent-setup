package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port               string
	DatabaseURL        string
	JWTSecret          string
	AppURL             string
	CORSAllowedOrigins string
	ResendAPIKey       string
	AuthEmailFrom      string
	MobileAppScheme    string

	AnthropicAPIKey     string
	AnthropicModel      string
	AnthropicMaxTokens  int
	AgentMaxIterations  int
	AgentSystemPrompt   string
	AgentRunRateLimit   int
	AgentRunRateWindow  time.Duration
}

func Load() Config {
	return Config{
		Port:               getEnv("PORT", "8080"),
		DatabaseURL:        getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5434/agent_app?sslmode=disable"),
		JWTSecret:          getEnv("JWT_SECRET", "dev-secret-change-me-please-use-a-long-random-value"),
		AppURL:             getEnv("APP_URL", "http://localhost:8080"),
		CORSAllowedOrigins: getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000,http://localhost:8081"),
		ResendAPIKey:       strings.TrimSpace(os.Getenv("RESEND_API_KEY")),
		AuthEmailFrom:      getEnv("AUTH_EMAIL_FROM", "Agent App <onboarding@example.com>"),
		MobileAppScheme:    getEnv("MOBILE_APP_SCHEME", "agentapp"),

		AnthropicAPIKey:    strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")),
		AnthropicModel:     getEnv("ANTHROPIC_MODEL", "claude-sonnet-4-5-20250929"),
		AnthropicMaxTokens: getEnvInt("ANTHROPIC_MAX_TOKENS", 4096),
		AgentMaxIterations: getEnvInt("AGENT_MAX_TOOL_ITERATIONS", 10),
		AgentSystemPrompt:  getEnv("AGENT_SYSTEM_PROMPT", "You are a helpful assistant."),
		AgentRunRateLimit:  getEnvInt("AGENT_RUN_RATE_LIMIT", 10),
		AgentRunRateWindow: time.Duration(getEnvInt("AGENT_RUN_RATE_WINDOW_SECONDS", 60)) * time.Second,
	}
}

func getEnv(key, fallback string) string {
	value, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func getEnvInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
}
