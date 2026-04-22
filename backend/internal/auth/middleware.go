package auth

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	magiclink "github.com/teslashibe/magiclink-auth-go"

	"github.com/teslashibe/agent-setup/backend/internal/apperrors"
)

type Middleware struct {
	magicSvc *magiclink.Service
	authSvc  *Service
}

func NewMiddleware(magicSvc *magiclink.Service, authSvc *Service) *Middleware {
	return &Middleware{magicSvc: magicSvc, authSvc: authSvc}
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
