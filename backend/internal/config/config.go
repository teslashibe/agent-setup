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

	AnthropicAPIKey    string
	AnthropicAgentID   string
	AnthropicEnvID     string

	AgentRunRateLimit  int
	AgentRunRateWindow time.Duration

	TeamsEnabled         bool
	TeamsDefaultMaxSeats int
	TeamsInviteTTL       time.Duration
	TeamsInviteFromName  string
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

		AnthropicAPIKey:  strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")),
		AnthropicAgentID: strings.TrimSpace(os.Getenv("ANTHROPIC_AGENT_ID")),
		AnthropicEnvID:   strings.TrimSpace(os.Getenv("ANTHROPIC_ENVIRONMENT_ID")),

		AgentRunRateLimit:  getEnvInt("AGENT_RUN_RATE_LIMIT", 10),
		AgentRunRateWindow: time.Duration(getEnvInt("AGENT_RUN_RATE_WINDOW_SECONDS", 60)) * time.Second,

		TeamsEnabled:         getEnvBool("TEAMS_ENABLED", true),
		TeamsDefaultMaxSeats: getEnvInt("TEAMS_DEFAULT_MAX_SEATS", 25),
		TeamsInviteTTL:       time.Duration(getEnvInt("TEAMS_INVITE_TTL_HOURS", 168)) * time.Hour,
		TeamsInviteFromName:  getEnv("TEAMS_INVITE_FROM_NAME", "Agent App"),
	}
}

func getEnv(key, fallback string) string {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		return fallback
	}
	return strings.TrimSpace(v)
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

func getEnvBool(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return v
}
