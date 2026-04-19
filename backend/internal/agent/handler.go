package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"

	"github.com/teslashibe/agent-setup/backend/internal/apperrors"
	"github.com/teslashibe/agent-setup/backend/internal/httputil"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Mount(r fiber.Router, runLimiter ...fiber.Handler) {
	r.Post("/agent/sessions", h.CreateSession)
	r.Get("/agent/sessions", h.ListSessions)
	r.Get("/agent/sessions/:id", h.GetSession)
	r.Delete("/agent/sessions/:id", h.DeleteSession)
	r.Get("/agent/sessions/:id/messages", h.ListMessages)
	if len(runLimiter) > 0 && runLimiter[0] != nil {
		r.Post("/agent/sessions/:id/run", runLimiter[0], h.Run)
	} else {
		r.Post("/agent/sessions/:id/run", h.Run)
	}
}

func (h *Handler) CreateSession(c *fiber.Ctx) error {
	userID, err := httputil.CurrentUserID(c)
	if err != nil {
		return apperrors.Handle(c, err)
	}
	var req struct {
		Title string `json:"title"`
	}
	if err := c.BodyParser(&req); err != nil {
		return apperrors.Handle(c, apperrors.ErrBadRequest)
	}
	sess, err := h.svc.CreateSession(c.UserContext(), userID, req.Title)
	if err != nil {
		return apperrors.Handle(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(sess)
}

func (h *Handler) ListSessions(c *fiber.Ctx) error {
	userID, err := httputil.CurrentUserID(c)
	if err != nil {
		return apperrors.Handle(c, err)
	}
	sessions, err := h.svc.Store().ListSessions(c.UserContext(), userID)
	if err != nil {
		return apperrors.Handle(c, err)
	}
	if sessions == nil {
		sessions = []Session{}
	}
	return c.JSON(fiber.Map{"sessions": sessions})
}

func (h *Handler) GetSession(c *fiber.Ctx) error {
	userID, err := httputil.CurrentUserID(c)
	if err != nil {
		return apperrors.Handle(c, err)
	}
	sess, err := h.svc.Store().GetSession(c.UserContext(), userID, c.Params("id"))
	if err != nil {
		return apperrors.Handle(c, err)
	}
	return c.JSON(sess)
}

func (h *Handler) DeleteSession(c *fiber.Ctx) error {
	userID, err := httputil.CurrentUserID(c)
	if err != nil {
		return apperrors.Handle(c, err)
	}
	if err := h.svc.Store().DeleteSession(c.UserContext(), userID, c.Params("id")); err != nil {
		return apperrors.Handle(c, err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) ListMessages(c *fiber.Ctx) error {
	userID, err := httputil.CurrentUserID(c)
	if err != nil {
		return apperrors.Handle(c, err)
	}
	sess, err := h.svc.Store().GetSession(c.UserContext(), userID, c.Params("id"))
	if err != nil {
		return apperrors.Handle(c, err)
	}
	messages, err := h.svc.History(c.UserContext(), sess.AnthropicSessionID)
	if err != nil {
		return apperrors.Handle(c, err)
	}
	if messages == nil {
		messages = []Message{}
	}
	return c.JSON(fiber.Map{"messages": messages})
}

func (h *Handler) Run(c *fiber.Ctx) error {
	userID, err := httputil.CurrentUserID(c)
	if err != nil {
		return apperrors.Handle(c, err)
	}
	sess, err := h.svc.Store().GetSession(c.UserContext(), userID, c.Params("id"))
	if err != nil {
		return apperrors.Handle(c, err)
	}
	var req struct {
		Message string `json:"message"`
	}
	if err := c.BodyParser(&req); err != nil {
		return apperrors.Handle(c, apperrors.ErrBadRequest)
	}
	if strings.TrimSpace(req.Message) == "" {
		return apperrors.Handle(c, apperrors.New(fiber.StatusBadRequest, "message is required"))
	}

	ctx := c.UserContext()
	events, err := h.svc.Run(ctx, sess, req.Message)
	if err != nil {
		return apperrors.Handle(c, err)
	}

	// Auto-title: if session is still "New chat", update from first message.
	if sess.Title == "New chat" || sess.Title == "" {
		title := req.Message
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
			payload, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, payload)
			w.Flush()
		}
	}))

	return nil
}
