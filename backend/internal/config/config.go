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

	// TeamsInviteRateLimit caps invite-creation attempts per (team, hour).
	// Spec: "10 invites / hour per team".
	TeamsInviteRateLimit  int
	TeamsInviteRateWindow time.Duration

	// TeamsAcceptRateLimit caps invite preview + accept attempts per IP.
	// Spec: "5 attempts / minute per IP to slow token enumeration".
	TeamsAcceptRateLimit  int
	TeamsAcceptRateWindow time.Duration

	// CredentialsEncryptionKey is the 32-byte AES-256 key used to encrypt
	// platform_credentials.credential at rest. Required when at least one
	// MCP-exposing platform is configured. Provide as a 64-char hex string
	// or a 44-char URL-safe base64 (with padding) string. In production,
	// inject via a sealed Kubernetes Secret.
	CredentialsEncryptionKey string

	// MCPPublicURL is the externally-reachable origin Anthropic-managed
	// agents will use to reach this server's MCP endpoint. Defaults to
	// AppURL. Override only when AppURL points at a non-public hostname
	// (e.g. when the API is split behind two ingresses).
	MCPPublicURL string

	// MCPDefaultPageLimit is the default value applied when a tool input
	// omits a `limit` field. Defaults to 10 (per spec; biases towards
	// minimal token usage in the common case).
	MCPDefaultPageLimit int

	// MCPMaxItemsPerPage caps the size of any list response returned from
	// an MCP tool, regardless of what the underlying scraper returned.
	// Defaults to 50.
	MCPMaxItemsPerPage int

	// MCPMaxStringLen truncates any string field in a tool result to this
	// many runes. Applied recursively. Default 800 (per spec).
	MCPMaxStringLen int

	// MCPMaxResponseBytes caps the total compact-JSON byte size of any
	// tool response. Set to 0 to disable. Default 32 KiB.
	MCPMaxResponseBytes int

	// NotificationsEnabled gates the Android notification capture feature.
	// When true, the backend mounts /api/notifications/* routes, registers
	// the notifications_* MCP tools (5 of them) on the per-user agent, and
	// extends the system prompt with rollup instructions. The mobile app's
	// capture screen and provider also light up.
	//
	// Defaults to false so existing forks pick up the new migration without
	// any runtime behaviour change. Set to true (and run the standard
	// migrations) to enable.
	NotificationsEnabled bool

	// NotificationsIngestRateLimit caps POST /api/notifications/batch
	// uploads per user per window. At the default 5-minute device flush
	// cadence the configured 60/minute gives 12x headroom.
	NotificationsIngestRateLimit  int
	NotificationsIngestRateWindow time.Duration

	// NotificationsDefaultPageSize is applied when MCP/REST callers omit a
	// limit. NotificationsMaxPageSize is the hard cap.
	NotificationsDefaultPageSize int
	NotificationsMaxPageSize     int

	// NotificationsReplyWindowHrs is the unanswered-message threshold used
	// by the notifications_pending_actions classifier.
	NotificationsReplyWindowHrs int

	// Brand selects the agent persona baked into per-user provisioned
	// agents. When empty the catch-all template prompt is used. Recognized
	// values are whatever a fork registers in `internal/brand` (e.g.
	// "seafret"). Unknown values log a warning and fall through to the
	// default.
	Brand string

	// AuthDevBypassEmail, when non-empty, makes the auth middleware
	// short-circuit any request that arrives without an Authorization
	// header / token query / token path-segment by treating it as if the
	// configured email had logged in via the dev login flow. Intended for
	// local development only — gives curl/Postman/scripts a way to hit
	// authenticated routes with no JWT plumbing. The user record (and
	// personal team) for this email is pre-created at server boot so the
	// first request is fast.
	//
	// LEAVE BLANK in any deployed environment. The server logs a loud
	// boot-time warning when this is set.
	AuthDevBypassEmail string
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

		TeamsInviteRateLimit:  getEnvInt("TEAMS_INVITE_RATE_LIMIT", 10),
		TeamsInviteRateWindow: time.Duration(getEnvInt("TEAMS_INVITE_RATE_WINDOW_SECONDS", 3600)) * time.Second,
		TeamsAcceptRateLimit:  getEnvInt("TEAMS_ACCEPT_RATE_LIMIT", 5),
		TeamsAcceptRateWindow: time.Duration(getEnvInt("TEAMS_ACCEPT_RATE_WINDOW_SECONDS", 60)) * time.Second,

		CredentialsEncryptionKey: strings.TrimSpace(os.Getenv("CREDENTIALS_ENCRYPTION_KEY")),
		MCPPublicURL:             strings.TrimSpace(os.Getenv("MCP_PUBLIC_URL")),
		MCPDefaultPageLimit:      getEnvInt("MCP_DEFAULT_PAGE_LIMIT", 10),
		MCPMaxItemsPerPage:       getEnvInt("MCP_MAX_ITEMS_PER_PAGE", 50),
		MCPMaxStringLen:          getEnvInt("MCP_MAX_STRING_LEN", 800),
		MCPMaxResponseBytes:      getEnvInt("MCP_MAX_RESPONSE_BYTES", 32*1024),

		NotificationsEnabled:          getEnvBool("NOTIFICATIONS_ENABLED", false),
		NotificationsIngestRateLimit:  getEnvInt("NOTIFICATIONS_INGEST_RATE_LIMIT", 60),
		NotificationsIngestRateWindow: time.Duration(getEnvInt("NOTIFICATIONS_INGEST_RATE_WINDOW_SECONDS", 60)) * time.Second,
		NotificationsDefaultPageSize:  getEnvInt("NOTIFICATIONS_DEFAULT_PAGE_SIZE", 50),
		NotificationsMaxPageSize:      getEnvInt("NOTIFICATIONS_MAX_PAGE_SIZE", 200),
		NotificationsReplyWindowHrs:   getEnvInt("NOTIFICATIONS_ACTION_REPLY_WINDOW_HOURS", 2),

		Brand:              strings.TrimSpace(os.Getenv("BRAND")),
		AuthDevBypassEmail: strings.ToLower(strings.TrimSpace(os.Getenv("AUTH_DEV_BYPASS_EMAIL"))),
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
