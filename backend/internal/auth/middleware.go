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
	return func(c *fiber.Ctx) error {
		header := strings.TrimSpace(c.Get("Authorization"))
		if header == "" {
			if t := strings.TrimSpace(c.Query("token")); t != "" {
				header = "Bearer " + t
			}
		}
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
