package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"

	"github.com/teslashibe/agent-setup/backend/internal/apperrors"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Mount(r fiber.Router, runLimiter fiber.Handler) {
	r.Post("/agent/sessions", h.CreateSession)
	r.Get("/agent/sessions", h.ListSessions)
	r.Get("/agent/sessions/:id", h.GetSession)
	r.Delete("/agent/sessions/:id", h.DeleteSession)
	r.Get("/agent/sessions/:id/messages", h.ListMessages)
	r.Post("/agent/sessions/:id/run", runLimiter, h.Run)
}

func (h *Handler) CreateSession(c *fiber.Ctx) error {
	var req struct{ Title string `json:"title"` }
	if err := c.BodyParser(&req); err != nil {
		return apperrors.ErrBadRequest
	}
	sess, err := h.svc.CreateSession(c.UserContext(), apperrors.UserID(c), req.Title)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(sess)
}

func (h *Handler) ListSessions(c *fiber.Ctx) error {
	sessions, err := h.svc.Store().ListSessions(c.UserContext(), apperrors.UserID(c))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"sessions": sessions})
}

func (h *Handler) GetSession(c *fiber.Ctx) error {
	sess, err := h.svc.Store().GetSession(c.UserContext(), apperrors.UserID(c), c.Params("id"))
	if err != nil {
		return err
	}
	return c.JSON(sess)
}

func (h *Handler) DeleteSession(c *fiber.Ctx) error {
	if err := h.svc.Store().DeleteSession(c.UserContext(), apperrors.UserID(c), c.Params("id")); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) ListMessages(c *fiber.Ctx) error {
	sess, err := h.svc.Store().GetSession(c.UserContext(), apperrors.UserID(c), c.Params("id"))
	if err != nil {
		return err
	}
	messages, err := h.svc.History(c.UserContext(), sess.AnthropicSessionID)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"messages": messages})
}

func (h *Handler) Run(c *fiber.Ctx) error {
	sess, err := h.svc.Store().GetSession(c.UserContext(), apperrors.UserID(c), c.Params("id"))
	if err != nil {
		return err
	}
	var req struct{ Message string `json:"message"` }
	if err := c.BodyParser(&req); err != nil {
		return apperrors.ErrBadRequest
	}
	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		return apperrors.New(fiber.StatusBadRequest, "message is required")
	}

	ctx := c.UserContext()
	events, err := h.svc.Run(ctx, sess, msg)
	if err != nil {
		return err
	}

	if sess.Title == "" || sess.Title == "New chat" {
		title := msg
		if len(title) > 60 {
			title = title[:60]
		}
		_ = h.svc.Store().UpdateTitle(ctx, sess.ID, title)
	}

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("X-Accel-Buffering", "no")
	c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
		for ev := range events {
			payload, _ := json.Marshal(ev)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, payload)
			w.Flush()
		}
	}))
	return nil
}
