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

// Mount registers the agent routes under the provided fiber.Router.
// All routes assume the parent router has already enforced authentication.
// runLimiter is applied only to the /run endpoint to guard Anthropic spend.
func (h *Handler) Mount(r fiber.Router, runLimiter ...fiber.Handler) {
	r.Post("/agent/sessions", h.CreateSession)
	r.Get("/agent/sessions", h.ListSessions)
	r.Get("/agent/sessions/:id", h.GetSession)
	r.Get("/agent/sessions/:id/messages", h.ListMessages)
	if len(runLimiter) > 0 && runLimiter[0] != nil {
		r.Post("/agent/sessions/:id/run", runLimiter[0], h.Run)
	} else {
		r.Post("/agent/sessions/:id/run", h.Run)
	}
}

type createSessionRequest struct {
	Title        string  `json:"title"`
	SystemPrompt *string `json:"system_prompt"`
	Model        *string `json:"model"`
}

func (h *Handler) CreateSession(c *fiber.Ctx) error {
	userID, err := httputil.CurrentUserID(c)
	if err != nil {
		return apperrors.Handle(c, err)
	}

	var req createSessionRequest
	if err := c.BodyParser(&req); err != nil {
		return apperrors.Handle(c, apperrors.ErrBadRequest)
	}

	sess, err := h.svc.Store().CreateSession(c.UserContext(), userID, req.Title, req.SystemPrompt, req.Model)
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
	sessions, err := h.svc.Store().ListSessions(c.UserContext(), userID, 50)
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

func (h *Handler) ListMessages(c *fiber.Ctx) error {
	userID, err := httputil.CurrentUserID(c)
	if err != nil {
		return apperrors.Handle(c, err)
	}
	if _, err := h.svc.Store().GetSession(c.UserContext(), userID, c.Params("id")); err != nil {
		return apperrors.Handle(c, err)
	}
	msgs, err := h.svc.Store().ListMessages(c.UserContext(), c.Params("id"))
	if err != nil {
		return apperrors.Handle(c, err)
	}
	if msgs == nil {
		msgs = []Message{}
	}
	return c.JSON(fiber.Map{"messages": msgs})
}

type runRequest struct {
	Message string `json:"message"`
}

// Run handles POST /agent/sessions/:id/run and streams events as SSE.
func (h *Handler) Run(c *fiber.Ctx) error {
	userID, err := httputil.CurrentUserID(c)
	if err != nil {
		return apperrors.Handle(c, err)
	}

	sessionID := c.Params("id")
	sess, err := h.svc.Store().GetSession(c.UserContext(), userID, sessionID)
	if err != nil {
		return apperrors.Handle(c, err)
	}

	var req runRequest
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
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, payload); err != nil {
				return
			}
			if err := w.Flush(); err != nil {
				return
			}
		}
	}))

	return nil
}
