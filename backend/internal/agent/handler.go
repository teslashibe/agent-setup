package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"

	"github.com/teslashibe/agent-setup/backend/internal/apperrors"
	"github.com/teslashibe/agent-setup/backend/internal/teams"
)

// Handler exposes /api/agent/* under a /api group that has already resolved
// the active team via teams.Middleware.RequireTeam, so handlers can safely
// read team_id and team_role from locals.
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
	var req struct {
		Title string `json:"title"`
	}
	if err := c.BodyParser(&req); err != nil {
		return apperrors.ErrBadRequest
	}
	sess, err := h.svc.CreateSession(c.UserContext(), apperrors.TeamID(c), apperrors.UserID(c), req.Title)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(sess)
}

// ListSessions returns sessions in the active team. Members always see only
// their own sessions; admins/owners default to "mine" but can pass ?scope=all
// to see every session in the team.
func (h *Handler) ListSessions(c *fiber.Ctx) error {
	teamID := apperrors.TeamID(c)
	userID := apperrors.UserID(c)
	role := teams.Role(apperrors.TeamRole(c))

	scope := strings.ToLower(strings.TrimSpace(c.Query("scope")))
	switch scope {
	case "", "mine":
		sessions, err := h.svc.Store().ListSessionsInTeam(c.UserContext(), teamID, userID)
		if err != nil {
			return err
		}
		return c.JSON(fiber.Map{"sessions": sessions, "scope": "mine"})
	case "all":
		if !role.AtLeast(teams.RoleAdmin) {
			return apperrors.ErrInsufficientRole
		}
		sessions, err := h.svc.Store().ListSessionsInTeam(c.UserContext(), teamID, "")
		if err != nil {
			return err
		}
		return c.JSON(fiber.Map{"sessions": sessions, "scope": "all"})
	default:
		return apperrors.New(fiber.StatusBadRequest, "scope must be 'mine' or 'all'")
	}
}

func (h *Handler) GetSession(c *fiber.Ctx) error {
	sess, err := h.lookup(c)
	if err != nil {
		return err
	}
	return c.JSON(sess)
}

// DeleteSession allows the session owner to delete their own session, or any
// admin+ to delete a session in the team. Members deleting another member's
// session get 403 even if they could read it via /sessions?scope=mine (which
// they couldn't anyway, since lookup() rejects the read first).
func (h *Handler) DeleteSession(c *fiber.Ctx) error {
	sess, err := h.lookup(c)
	if err != nil {
		return err
	}
	role := teams.Role(apperrors.TeamRole(c))
	if sess.UserID != apperrors.UserID(c) && !role.AtLeast(teams.RoleAdmin) {
		return apperrors.ErrInsufficientRole
	}
	if err := h.svc.Store().DeleteSessionInTeam(c.UserContext(), apperrors.TeamID(c), sess.ID); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) ListMessages(c *fiber.Ctx) error {
	sess, err := h.lookup(c)
	if err != nil {
		return err
	}
	messages, err := h.svc.History(c.UserContext(), sess.AnthropicSessionID)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"messages": messages})
}

// Run streams the assistant's reply. Only the session owner may run a session
// (admins can read but not impersonate).
func (h *Handler) Run(c *fiber.Ctx) error {
	sess, err := h.lookup(c)
	if err != nil {
		return err
	}
	if sess.UserID != apperrors.UserID(c) {
		return apperrors.ErrInsufficientRole
	}

	var req struct {
		Message string `json:"message"`
	}
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

// lookup loads a session in the active team and enforces the read rule:
// owner of the session, or any admin+ in the team.
func (h *Handler) lookup(c *fiber.Ctx) (Session, error) {
	teamID := apperrors.TeamID(c)
	userID := apperrors.UserID(c)
	role := teams.Role(apperrors.TeamRole(c))

	sess, err := h.svc.Store().GetSessionInTeam(c.UserContext(), teamID, c.Params("id"))
	if err != nil {
		return Session{}, err
	}
	if sess.UserID != userID && !role.AtLeast(teams.RoleAdmin) {
		// Members trying to peek at another member's session get 404, not 403,
		// so we don't leak existence.
		return Session{}, apperrors.ErrNotFound
	}
	return sess, nil
}
