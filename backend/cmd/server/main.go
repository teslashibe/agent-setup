package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	magiclink "github.com/teslashibe/magiclink-auth-go"
	"github.com/teslashibe/magiclink-auth-go/resend"

	"github.com/teslashibe/agent-setup/backend/internal/agent"
	"github.com/teslashibe/agent-setup/backend/internal/apperrors"
	"github.com/teslashibe/agent-setup/backend/internal/auth"
	"github.com/teslashibe/agent-setup/backend/internal/config"
	"github.com/teslashibe/agent-setup/backend/internal/credentials"
	"github.com/teslashibe/agent-setup/backend/internal/invites"
	mcppkg "github.com/teslashibe/agent-setup/backend/internal/mcp"
	"github.com/teslashibe/agent-setup/backend/internal/mcp/platforms"
	"github.com/teslashibe/agent-setup/backend/internal/teams"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()
	ctx := context.Background()

	pool, err := newPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	authSvc := auth.NewService(pool)
	teamsSvc := teams.NewService(teams.NewStore(pool), cfg.TeamsDefaultMaxSeats)
	magicSvc, codeStore, err := newMagicLinkService(cfg, pool, authSvc, teamsSvc)
	if err != nil {
		log.Fatalf("magiclink: %v", err)
	}

	var invitesSvc *invites.Service
	if cfg.TeamsEnabled {
		invitesSvc = invites.NewService(invites.Config{
			AppName:   "Claude Agent Go",
			AppURL:    cfg.AppURL,
			FromName:  cfg.TeamsInviteFromName,
			InviteTTL: cfg.TeamsInviteTTL,
		}, teamsSvc, authSvc, newInviteEmailSender(cfg))
	}

	agentSvc, err := agent.NewService(cfg, agent.NewStore(pool))
	if err != nil {
		log.Fatalf("agent: %v", err)
	}

	app := fiber.New(fiber.Config{
		AppName:           "Claude Agent Go",
		StreamRequestBody: true,
		ErrorHandler:      apperrors.FiberHandler,
	})
	app.Use(recover.New(), logger.New(), cors.New(cors.Config{
		AllowOrigins: cfg.CORSAllowedOrigins,
		AllowMethods: "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
	}))

	app.Get("/health", func(c *fiber.Ctx) error { return c.JSON(fiber.Map{"status": "ok"}) })
	app.Post("/auth/magic-link", sendMagicLinkHandler(magicSvc, codeStore, invitesSvc))
	app.Post("/auth/verify", verifyCodeHandler(magicSvc, codeStore, invitesSvc))
	app.Get("/auth/verify", verifyLinkHandler(magicSvc, codeStore, invitesSvc))
	app.Post("/auth/login", devLoginHandler(magicSvc, authSvc, teamsSvc))

	authMW := auth.NewMiddleware(magicSvc, authSvc)
	teamMW := teams.NewMiddleware(teamsSvc)
	api := app.Group("/api", authMW.RequireAuth())
	api.Get("/me", auth.NewHandler(authSvc).GetMe)

	if cfg.TeamsEnabled {
		teams.NewHandler(teamsSvc, teamMW).Mount(api)

		// Per-route rate limits per spec §"Rate limits":
		//   - invite create  : per (team, hour) — slows admin email spam.
		//   - preview/accept : per IP, per minute — slows token enumeration.
		inviteCreateLimiter := limiter.New(limiter.Config{
			Max:        cfg.TeamsInviteRateLimit,
			Expiration: cfg.TeamsInviteRateWindow,
			KeyGenerator: func(c *fiber.Ctx) string {
				return "invite-create:" + apperrors.TeamID(c)
			},
			LimitReached: func(c *fiber.Ctx) error {
				return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
					"error": "invite rate limit exceeded for this team — try again later",
				})
			},
		})
		acceptLimiter := limiter.New(limiter.Config{
			Max:        cfg.TeamsAcceptRateLimit,
			Expiration: cfg.TeamsAcceptRateWindow,
			KeyGenerator: func(c *fiber.Ctx) string { return "invite-accept:" + c.IP() },
			LimitReached: func(c *fiber.Ctx) error {
				return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
					"error": "too many invite attempts — please slow down",
				})
			},
		})

		invitesH := invites.NewHandler(invitesSvc, teamMW).WithLimiters(invites.Limiters{
			Create: inviteCreateLimiter,
			Accept: acceptLimiter,
		})

		// Authenticated accept routes live under /api/invites (auth group).
		invitesH.MountAuthAPIRoutes(api)

		// Per-team invite routes share /api/teams/:teamID's RequireTeamFromParam
		// so admins can list / create / revoke invites for the team they own.
		teamScoped := api.Group("/teams/:teamID", teamMW.RequireTeamFromParam("teamID"))
		invitesH.MountTeamRoutes(teamScoped)

		// PUBLIC: preview routes mounted on `app` directly so unauthenticated
		// landing pages (mobile + web) can render the invite before sign-in.
		invitesH.MountPublicAPIRoutes(app)

		// Public HTML landing page that deep-links into the mobile app.
		invitesH.MountPublicRoutes(app, cfg.MobileAppScheme)
	}

	// Agent routes are team-scoped. RequireTeam reads X-Team-ID (or falls
	// back to the caller's personal team) and stamps team_id + team_role.
	agentGroup := api.Group("", teamMW.RequireTeam())
	agent.NewHandler(agentSvc).Mount(agentGroup, limiter.New(limiter.Config{
		Max:        cfg.AgentRunRateLimit,
		Expiration: cfg.AgentRunRateWindow,
		KeyGenerator: func(c *fiber.Ctx) string {
			if id, ok := c.Locals("user_id").(string); ok && id != "" {
				return "run:" + id
			}
			return "run:" + c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(
				fiber.Map{"error": "rate limit exceeded — please wait before sending another message"})
		},
	}))

	if err := mountMCP(app, api, authMW, cfg, pool, magicSvc, agentSvc); err != nil {
		log.Fatalf("mcp: %v", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	errCh := make(chan error, 1)
	go func() { errCh <- app.Listen(":" + cfg.Port) }()

	select {
	case err := <-errCh:
		if err != nil {
			log.Fatalf("listen: %v", err)
		}
	case sig := <-sigCh:
		log.Printf("shutdown: %s", sig)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := app.ShutdownWithContext(shutdownCtx); err != nil {
			log.Fatalf("shutdown: %v", err)
		}
	}
}

func newPool(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	if cfg.MaxConns == 4 {
		cfg.MaxConns = 20
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

func newInviteEmailSender(cfg config.Config) invites.EmailSender {
	if strings.TrimSpace(cfg.ResendAPIKey) == "" {
		return nil
	}
	return resend.New(cfg.ResendAPIKey, cfg.AuthEmailFrom)
}

// mountMCP wires the MCP server (registry, transport, credentials handler)
// and installs a per-user agent provisioner on agentSvc when
// CREDENTIALS_ENCRYPTION_KEY is configured. When the key is missing the
// helper logs a warning and returns nil so local-dev workflows that don't
// need MCP can still come up.
func mountMCP(
	app *fiber.App,
	api fiber.Router,
	authMW *auth.Middleware,
	cfg config.Config,
	pool *pgxpool.Pool,
	magicSvc *magiclink.Service,
	agentSvc *agent.Service,
) error {
	if cfg.CredentialsEncryptionKey == "" {
		log.Printf("mcp: CREDENTIALS_ENCRYPTION_KEY not set — MCP routes and per-user provisioner disabled")
		return nil
	}
	cipher, err := credentials.NewCipher(cfg.CredentialsEncryptionKey)
	if err != nil {
		return fmt.Errorf("credentials cipher: %w", err)
	}

	plugins := platforms.All()
	validators := make([]credentials.Validator, 0, len(plugins))
	bindings := make([]mcppkg.PlatformBinding, 0, len(plugins))
	for _, pl := range plugins {
		validators = append(validators, pl.Validator)
		bindings = append(bindings, pl.Binding)
	}

	credSvc := credentials.NewService(credentials.NewStore(pool), cipher, validators...)
	credentials.NewHandler(credSvc).Mount(api)

	registry, err := mcppkg.NewRegistry(bindings...)
	if err != nil {
		return fmt.Errorf("mcp registry: %w", err)
	}
	mcpSrv := mcppkg.NewServer(registry, credSvc, mcppkg.ResponseShaper{
		MaxItemsPerPage:  cfg.MCPMaxItemsPerPage,
		MaxStringLen:     cfg.MCPMaxStringLen,
		MaxResponseBytes: cfg.MCPMaxResponseBytes,
	})
	transport := mcppkg.NewTransport(mcpSrv)

	transport.MountHealth(app.Group("/mcp"))

	mcpUser := app.Group("/mcp/u/:token", authMW.RequirePathAuth("token"))
	transport.Mount(mcpUser)

	mcpAPI := api.Group("/mcp")
	transport.Mount(mcpAPI)

	endpointFn, err := newMCPEndpointFn(cfg, pool, magicSvc)
	if err != nil {
		return fmt.Errorf("mcp endpoint factory: %w", err)
	}
	provisioner, err := agent.NewProvisioner(cfg, agentSvc.Client(), pool, endpointFn, agent.ProvisionerOptions{})
	if err != nil {
		return fmt.Errorf("agent provisioner: %w", err)
	}
	agentSvc.UseProvisioner(provisioner)

	log.Printf("mcp: %d tools across %d platforms registered", len(registry.Tools()), len(registry.Platforms()))
	return nil
}

// newMCPEndpointFn returns the per-user MCP URL factory used by the
// Provisioner. We mint a fresh, long-lived JWT per user (subject = the
// user's identity_key) and embed it in the URL path; the MCP transport
// validates the JWT via auth.RequirePathAuth.
//
// The URL format is `<MCPPublicURL>/mcp/u/<token>/v1`. MCPPublicURL falls
// back to AppURL.
func newMCPEndpointFn(cfg config.Config, pool *pgxpool.Pool, magicSvc *magiclink.Service) (agent.MCPEndpointFn, error) {
	base := strings.TrimRight(firstNonBlank(cfg.MCPPublicURL, cfg.AppURL), "/")
	if base == "" {
		return nil, fmt.Errorf("MCP_PUBLIC_URL or APP_URL must be set so Anthropic agents can reach the MCP server")
	}
	if u, err := url.Parse(base); err != nil || u.Scheme == "" {
		return nil, fmt.Errorf("invalid MCP base URL %q", base)
	}
	const userQuery = `SELECT identity_key, email, name FROM users WHERE id = $1`
	return func(ctx context.Context, userID string) (string, error) {
		var identity, email, name string
		if err := pool.QueryRow(ctx, userQuery, userID).Scan(&identity, &email, &name); err != nil {
			return "", fmt.Errorf("lookup user %s: %w", userID, err)
		}
		token, err := magicSvc.IssueToken(magiclink.Claims{
			Subject:     identity,
			Email:       email,
			DisplayName: name,
		})
		if err != nil {
			return "", fmt.Errorf("issue MCP token: %w", err)
		}
		return base + "/mcp/u/" + url.PathEscape(token) + "/v1", nil
	}, nil
}

func firstNonBlank(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func devLoginHandler(magicSvc *magiclink.Service, authSvc *auth.Service, teamsSvc *teams.Service) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var req struct {
			Email string `json:"email"`
			Name  string `json:"name"`
		}
		if err := c.BodyParser(&req); err != nil {
			return apperrors.New(http.StatusBadRequest, "invalid request body")
		}
		email := strings.ToLower(strings.TrimSpace(req.Email))
		if email == "" {
			return apperrors.New(http.StatusBadRequest, "email is required")
		}
		identity := "dev|" + email
		res, err := authSvc.UpsertIdentity(c.UserContext(), identity, email, strings.TrimSpace(req.Name))
		if err != nil {
			return err
		}
		user := res.User
		if _, err := teamsSvc.EnsurePersonalTeam(c.UserContext(), user.ID, user.Name, user.Email); err != nil {
			return err
		}
		token, err := magicSvc.IssueToken(magiclink.Claims{
			Subject:     identity,
			Email:       email,
			DisplayName: user.Name,
		})
		if err != nil {
			return err
		}
		return c.JSON(magiclink.AuthResult{JWT: token, UserID: user.ID, Email: user.Email, DisplayName: user.Name})
	}
}
