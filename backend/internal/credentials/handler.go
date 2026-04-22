package credentials

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/teslashibe/agent-setup/backend/internal/apperrors"
)

// Handler exposes the credential CRUD endpoints used by the mobile/web
// settings screen.
type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Mount wires the handler into a Fiber router. Caller is expected to apply
// auth middleware to the parent group.
//
// Routes follow the spec in agent-setup#6:
//
//	GET    /platforms                                — list platform connections
//	GET    /platforms/:platform/credentials          — connection metadata (no plaintext)
//	POST   /platforms/:platform/credentials          — upsert credential
//	PUT    /platforms/:platform/credentials          — alias of POST (idempotent)
//	DELETE /platforms/:platform/credentials          — disconnect
func (h *Handler) Mount(r fiber.Router) {
	r.Get("/platforms", h.list)
	r.Get("/platforms/:platform/credentials", h.get)
	r.Post("/platforms/:platform/credentials", h.set)
	r.Put("/platforms/:platform/credentials", h.set)
	r.Delete("/platforms/:platform/credentials", h.delete)
}

func (h *Handler) list(c *fiber.Ctx) error {
	userID, err := requireUser(c)
	if err != nil {
		return err
	}
	conns, err := h.svc.List(c.UserContext(), userID)
	if err != nil {
		return err
	}
	platforms := h.svc.Platforms()
	connected := map[string]ConnectionSummary{}
	for _, conn := range conns {
		connected[conn.Platform] = conn
	}
	type platformStatus struct {
		Platform  string             `json:"platform"`
		Connected bool               `json:"connected"`
		Summary   *ConnectionSummary `json:"summary,omitempty"`
	}
	out := make([]platformStatus, 0, len(platforms))
	for _, p := range platforms {
		ps := platformStatus{Platform: p}
		if s, ok := connected[p]; ok {
			ps.Connected = true
			s := s
			ps.Summary = &s
		}
		out = append(out, ps)
	}
	for _, conn := range conns {
		if _, known := indexOf(platforms, conn.Platform); !known {
			conn := conn
			out = append(out, platformStatus{Platform: conn.Platform, Connected: true, Summary: &conn})
		}
	}
	return c.JSON(fiber.Map{"platforms": out})
}

type setRequest struct {
	Label      string          `json:"label"`
	Credential json.RawMessage `json:"credential"`
}

func (h *Handler) set(c *fiber.Ctx) error {
	userID, err := requireUser(c)
	if err != nil {
		return err
	}
	platform := strings.TrimSpace(strings.ToLower(c.Params("platform")))
	if platform == "" {
		return apperrors.New(http.StatusBadRequest, "platform is required")
	}
	var req setRequest
	if err := c.BodyParser(&req); err != nil {
		return apperrors.New(http.StatusBadRequest, "invalid request body")
	}
	if len(req.Credential) == 0 {
		return apperrors.New(http.StatusBadRequest, "credential is required")
	}
	conn, err := h.svc.Set(c.UserContext(), userID, platform, strings.TrimSpace(req.Label), req.Credential)
	if err != nil {
		return apperrors.New(http.StatusBadRequest, err.Error())
	}
	return c.JSON(conn)
}

// get returns the connection metadata for a single platform — never the
// plaintext credential. Useful for the Settings UI to show "Connected since
// X, last used Y" without round-tripping the entire list.
func (h *Handler) get(c *fiber.Ctx) error {
	userID, err := requireUser(c)
	if err != nil {
		return err
	}
	platform := strings.TrimSpace(strings.ToLower(c.Params("platform")))
	if platform == "" {
		return apperrors.New(http.StatusBadRequest, "platform is required")
	}
	conns, err := h.svc.List(c.UserContext(), userID)
	if err != nil {
		return err
	}
	for _, conn := range conns {
		if conn.Platform == platform {
			return c.JSON(conn)
		}
	}
	return apperrors.New(http.StatusNotFound, "no credential for platform")
}

func (h *Handler) delete(c *fiber.Ctx) error {
	userID, err := requireUser(c)
	if err != nil {
		return err
	}
	platform := strings.TrimSpace(strings.ToLower(c.Params("platform")))
	if platform == "" {
		return apperrors.New(http.StatusBadRequest, "platform is required")
	}
	if err := h.svc.Delete(c.UserContext(), userID, platform); err != nil {
		if errors.Is(err, ErrNotFound) {
			return apperrors.New(http.StatusNotFound, "credential not found")
		}
		return err
	}
	return c.SendStatus(http.StatusNoContent)
}

func requireUser(c *fiber.Ctx) (string, error) {
	id := apperrors.UserID(c)
	if id == "" {
		return "", apperrors.ErrUnauthorized
	}
	return id, nil
}

func indexOf(haystack []string, needle string) (int, bool) {
	for i, v := range haystack {
		if v == needle {
			return i, true
		}
	}
	return -1, false
}
