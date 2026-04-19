package auth

import (
	"github.com/gofiber/fiber/v2"

	"github.com/teslashibe/agent-setup/backend/internal/apperrors"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) GetMe(c *fiber.Ctx) error {
	userID, err := apperrors.CurrentUserID(c)
	if err != nil {
		return apperrors.Handle(c, err)
	}
	user, err := h.svc.GetUser(c.UserContext(), userID)
	if err != nil {
		return apperrors.Handle(c, err)
	}
	return c.JSON(user)
}
