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
	magiclink "github.com/teslashibe/magiclink-auth-go"
	"github.com/teslashibe/magiclink-auth-go/fiberadapter"

	"github.com/teslashibe/agent-setup/backend/internal/agent"
	"github.com/teslashibe/agent-setup/backend/internal/apperrors"
	"github.com/teslashibe/agent-setup/backend/internal/auth"
	"github.com/teslashibe/agent-setup/backend/internal/bootstrap"
)

func main() {
	ctx := context.Background()

	core, err := bootstrap.Init(ctx)
	if err != nil {
		log.Fatalf("bootstrap: %v", err)
	}
	defer core.Pool.Close()

	authSvc := auth.NewService(core.Pool)
	magicSvc, err := newMagicLinkService(core.Cfg, core.Pool, authSvc)
	if err != nil {
		log.Fatalf("magiclink: %v", err)
	}

	agentSvc, err := agent.NewService(core.Cfg, agent.NewStore(core.Pool))
	if err != nil {
		log.Fatalf("agent: %v", err)
	}

	authMW := auth.NewMiddleware(magicSvc, authSvc)
	authHandler := auth.NewHandler(authSvc)
	agentHandler := agent.NewHandler(agentSvc)

	app := fiber.New(fiber.Config{
		AppName:           "Claude Agent Go API",
		StreamRequestBody: true,
	})
	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: core.Cfg.CORSAllowedOrigins,
		AllowMethods: "GET,POST,PUT,PATCH,DELETE,OPTIONS",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
	}))

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	app.Post("/auth/magic-link", fiberadapter.SendHandler(magicSvc))
	app.Post("/auth/verify", fiberadapter.VerifyCodeHandler(magicSvc))
	app.Get("/auth/verify", fiberadapter.VerifyLinkHandler(magicSvc))
	app.Post("/auth/login", devLoginHandler(magicSvc, authSvc))

	runLimiter := limiter.New(limiter.Config{
		Max:        core.Cfg.AgentRunRateLimit,
		Expiration: core.Cfg.AgentRunRateWindow,
		KeyGenerator: func(c *fiber.Ctx) string {
			if id, ok := c.Locals("user_id").(string); ok && id != "" {
				return "run:" + id
			}
			return "run:" + c.IP()
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "rate limit exceeded — please wait before sending another message",
			})
		},
	})

	api := app.Group("/api", authMW.RequireAuth())
	api.Get("/me", authHandler.GetMe)
	agentHandler.Mount(api, runLimiter)

	errCh := make(chan error, 1)
	go func() { errCh <- app.Listen(":" + core.Cfg.Port) }()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case listenErr := <-errCh:
		if listenErr != nil {
			log.Fatalf("listen: %v", listenErr)
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
		identityKey := fmt.Sprintf("dev|%s", email)
		user, err := authSvc.UpsertIdentity(c.UserContext(), identityKey, email, strings.TrimSpace(r.Name))
		if err != nil {
			return apperrors.Handle(c, err)
		}
		token, err := magicSvc.IssueToken(magiclink.Claims{
			Subject:     identityKey,
			Email:       email,
			DisplayName: user.Name,
		})
		if err != nil {
			return apperrors.Handle(c, err)
		}
		return c.JSON(magiclink.AuthResult{
			JWT:         token,
			UserID:      user.ID,
			Email:       user.Email,
			DisplayName: user.Name,
		})
	}
}
