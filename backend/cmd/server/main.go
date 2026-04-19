package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
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
	"github.com/teslashibe/magiclink-auth-go/fiberadapter"

	"github.com/teslashibe/agent-setup/backend/internal/agent"
	"github.com/teslashibe/agent-setup/backend/internal/apperrors"
	"github.com/teslashibe/agent-setup/backend/internal/auth"
	"github.com/teslashibe/agent-setup/backend/internal/config"
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
	magicSvc, err := newMagicLinkService(cfg, pool, authSvc)
	if err != nil {
		log.Fatalf("magiclink: %v", err)
	}
	agentSvc, err := agent.NewService(cfg, agent.NewStore(pool))
	if err != nil {
		log.Fatalf("agent: %v", err)
	}

	app := fiber.New(fiber.Config{AppName: "Claude Agent Go", StreamRequestBody: true})
	app.Use(recover.New(), logger.New(), cors.New(cors.Config{
		AllowOrigins: cfg.CORSAllowedOrigins,
		AllowMethods: "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
	}))

	app.Get("/health", func(c *fiber.Ctx) error { return c.JSON(fiber.Map{"status": "ok"}) })
	app.Post("/auth/magic-link", fiberadapter.SendHandler(magicSvc))
	app.Post("/auth/verify", fiberadapter.VerifyCodeHandler(magicSvc))
	app.Get("/auth/verify", fiberadapter.VerifyLinkHandler(magicSvc))
	app.Post("/auth/login", devLoginHandler(magicSvc, authSvc))

	authMW := auth.NewMiddleware(magicSvc, authSvc)
	api := app.Group("/api", authMW.RequireAuth())
	api.Get("/me", auth.NewHandler(authSvc).GetMe)
	agent.NewHandler(agentSvc).Mount(api, limiter.New(limiter.Config{
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

func devLoginHandler(magicSvc *magiclink.Service, authSvc *auth.Service) fiber.Handler {
	type req struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	return func(c *fiber.Ctx) error {
		var r req
		if err := c.BodyParser(&r); err != nil {
			return apperrors.Handle(c, apperrors.New(http.StatusBadRequest, "invalid request body"))
		}
		email := strings.ToLower(strings.TrimSpace(r.Email))
		if email == "" {
			return apperrors.Handle(c, apperrors.New(http.StatusBadRequest, "email is required"))
		}
		user, err := authSvc.UpsertIdentity(c.UserContext(), fmt.Sprintf("dev|%s", email), email, strings.TrimSpace(r.Name))
		if err != nil {
			return apperrors.Handle(c, err)
		}
		token, err := magicSvc.IssueToken(magiclink.Claims{
			Subject:     fmt.Sprintf("dev|%s", email),
			Email:       email,
			DisplayName: user.Name,
		})
		if err != nil {
			return apperrors.Handle(c, err)
		}
		return c.JSON(magiclink.AuthResult{JWT: token, UserID: user.ID, Email: user.Email, DisplayName: user.Name})
	}
}
