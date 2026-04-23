package auth

import (
	"context"
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	magiclink "github.com/teslashibe/magiclink-auth-go"

	"github.com/teslashibe/agent-setup/backend/internal/apperrors"
)

type Middleware struct {
	magicSvc       *magiclink.Service
	authSvc        *Service
	devBypassEmail string
}

func NewMiddleware(magicSvc *magiclink.Service, authSvc *Service) *Middleware {
	return &Middleware{magicSvc: magicSvc, authSvc: authSvc}
}

// EnableDevBypass turns on the unauthenticated-request fallback for the given
// email. When configured, any request that arrives without an Authorization
// header is authenticated as the user behind that email, looked up via
// authSvc. The user MUST already exist (pre-create it at boot via
// UpsertIdentity + EnsurePersonalTeam) — this hot path stays a single keyed
// SELECT so it doesn't slow every request. Pass an empty string to disable
// (the default).
//
// LOCAL DEV ONLY. The server logs a loud warning at boot when this is set.
func (m *Middleware) EnableDevBypass(email string) {
	m.devBypassEmail = strings.ToLower(strings.TrimSpace(email))
}

func (m *Middleware) RequireAuth() fiber.Handler {
	return m.authenticator(func(c *fiber.Ctx) string {
		if h := strings.TrimSpace(c.Get("Authorization")); h != "" {
			return h
		}
		if t := strings.TrimSpace(c.Query("token")); t != "" {
			return "Bearer " + t
		}
		return ""
	})
}

// RequirePathAuth authenticates a request whose JWT lives in the named path
// segment (e.g. /mcp/u/:token/v1). Used by the per-user MCP endpoint:
// Anthropic Managed Agents' BetaManagedAgentsURLMCPServerParams does not let
// us inject custom auth headers, so we encode the per-user JWT into the URL
// path the agent is configured with.
//
// Falls back to RequireAuth-style header/query auth so the same handler can
// be hit by tools that do support headers (curl, Postman, our own tests).
func (m *Middleware) RequirePathAuth(paramName string) fiber.Handler {
	return m.authenticator(func(c *fiber.Ctx) string {
		if t := strings.TrimSpace(c.Params(paramName)); t != "" {
			return "Bearer " + t
		}
		if h := strings.TrimSpace(c.Get("Authorization")); h != "" {
			return h
		}
		if t := strings.TrimSpace(c.Query("token")); t != "" {
			return "Bearer " + t
		}
		return ""
	})
}

func (m *Middleware) authenticator(extract func(c *fiber.Ctx) string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		header := extract(c)
		if header == "" {
			if user, ok := m.devBypassUser(c.UserContext()); ok {
				apperrors.SetUserID(c, user.ID)
				apperrors.SetUserEmail(c, user.Email)
				apperrors.SetUserDisplayName(c, user.Name)
				return c.Next()
			}
			return apperrors.ErrUnauthorized
		}
		userID, claims, err := m.magicSvc.AuthenticateBearer(c.UserContext(), header)
		if err != nil {
			return apperrors.ErrUnauthorized
		}
		user, err := m.authSvc.GetUser(c.UserContext(), userID)
		if err != nil {
			return err
		}
		apperrors.SetUserID(c, userID)
		apperrors.SetUserEmail(c, user.Email)
		apperrors.SetUserDisplayName(c, user.Name)
		c.Locals("auth_claims", claims)
		return c.Next()
	}
}

// devBypassUser returns the bypass user when AUTH_DEV_BYPASS_EMAIL is set
// and the user already exists in the DB; otherwise (no bypass configured,
// or user missing) returns ok=false. Errors other than "not found" propagate
// as ok=false too — fall through to the standard 401 to avoid masking real
// problems behind a misleading 200.
func (m *Middleware) devBypassUser(ctx context.Context) (User, bool) {
	if m.devBypassEmail == "" {
		return User{}, false
	}
	user, err := m.authSvc.GetByEmail(ctx, m.devBypassEmail)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return User{}, false
		}
		return User{}, false
	}
	return user, true
}
