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
			return apperrors.Handle(c, apperrors.ErrUnauthorized)
		}
		userID, claims, err := m.magicSvc.AuthenticateBearer(c.UserContext(), header)
		if err != nil {
			return apperrors.Handle(c, apperrors.ErrUnauthorized)
		}
		if _, err := m.authSvc.GetUser(c.UserContext(), userID); err != nil {
			return apperrors.Handle(c, err)
		}
		apperrors.SetUserID(c, userID)
		c.Locals("auth_claims", claims)
		return c.Next()
	}
}
