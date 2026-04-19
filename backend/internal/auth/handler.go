package auth

import (
	"github.com/gofiber/fiber/v2"

	"github.com/teslashibe/agent-setup/backend/internal/apperrors"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) GetMe(c *fiber.Ctx) error {
	user, err := h.svc.GetUser(c.UserContext(), apperrors.UserID(c))
	if err != nil {
		return err
	}
	return c.JSON(user)
}
