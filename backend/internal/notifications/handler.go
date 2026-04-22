package notifications

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/teslashibe/agent-setup/backend/internal/apperrors"
)

// Handler exposes the REST surface for notification ingest and (optional)
// query. Callers mount it on an authenticated Fiber group.
type Handler struct {
	svc *Service
}

// NewHandler wires a Handler around a Service.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Mount installs the notification routes on the given (already-authenticated)
// Fiber router. ingestLimiter, when non-nil, is applied to the high-volume
// POST /batch route only — read endpoints are unbounded because the agent
// itself drives them at a low pace.
//
//	api := app.Group("/api", authMW.RequireAuth())
//	notifications.NewHandler(svc).Mount(api, batchLimiter)
//
// Routes:
//
//	POST /api/notifications/batch  - device flush ingest
//	GET  /api/notifications        - paginated list (debug + future log UI)
//	GET  /api/notifications/apps   - per-app summary (mobile settings stat)
func (h *Handler) Mount(api fiber.Router, ingestLimiter fiber.Handler) {
	g := api.Group("/notifications")
	if ingestLimiter != nil {
		g.Post("/batch", ingestLimiter, h.ingestBatch)
	} else {
		g.Post("/batch", h.ingestBatch)
	}
	g.Get("/", h.listEvents)
	g.Get("/apps", h.listApps)
}

func (h *Handler) ingestBatch(c *fiber.Ctx) error {
	userID := apperrors.UserID(c)
	if userID == "" {
		return apperrors.ErrUnauthorized
	}
	var in BatchInput
	if err := c.BodyParser(&in); err != nil {
		return apperrors.New(http.StatusBadRequest, "invalid request body")
	}
	res, err := h.svc.IngestBatch(c.UserContext(), userID, in)
	if err != nil {
		return apperrors.New(http.StatusInternalServerError, "ingest failed: "+err.Error())
	}
	return c.JSON(res)
}

func (h *Handler) listEvents(c *fiber.Ctx) error {
	userID := apperrors.UserID(c)
	if userID == "" {
		return apperrors.ErrUnauthorized
	}
	opts := ListOpts{
		AppPackage: strings.TrimSpace(c.Query("app")),
		Limit:      atoiOrZero(c.Query("limit")),
	}
	if since := strings.TrimSpace(c.Query("since")); since != "" {
		t, err := time.Parse(time.RFC3339, since)
		if err != nil {
			return apperrors.New(http.StatusBadRequest, "invalid 'since' (want RFC3339)")
		}
		opts.Since = &t
	}
	if until := strings.TrimSpace(c.Query("until")); until != "" {
		t, err := time.Parse(time.RFC3339, until)
		if err != nil {
			return apperrors.New(http.StatusBadRequest, "invalid 'until' (want RFC3339)")
		}
		opts.Until = &t
	}
	q := strings.TrimSpace(c.Query("q"))
	var (
		events []Event
		err    error
	)
	if q == "" {
		events, err = h.svc.List(c.UserContext(), userID, opts)
	} else {
		events, err = h.svc.Search(c.UserContext(), userID, q, opts)
	}
	if err != nil {
		return apperrors.New(http.StatusInternalServerError, "list failed: "+err.Error())
	}
	if events == nil {
		events = []Event{}
	}
	return c.JSON(fiber.Map{"events": events, "count": len(events)})
}

func (h *Handler) listApps(c *fiber.Ctx) error {
	userID := apperrors.UserID(c)
	if userID == "" {
		return apperrors.ErrUnauthorized
	}
	var since, until *time.Time
	if s := strings.TrimSpace(c.Query("since")); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return apperrors.New(http.StatusBadRequest, "invalid 'since' (want RFC3339)")
		}
		since = &t
	}
	if u := strings.TrimSpace(c.Query("until")); u != "" {
		t, err := time.Parse(time.RFC3339, u)
		if err != nil {
			return apperrors.New(http.StatusBadRequest, "invalid 'until' (want RFC3339)")
		}
		until = &t
	}
	apps, err := h.svc.ListApps(c.UserContext(), userID, since, until)
	if err != nil {
		return apperrors.New(http.StatusInternalServerError, "list apps failed: "+err.Error())
	}
	if apps == nil {
		apps = []AppSummary{}
	}
	return c.JSON(fiber.Map{"apps": apps, "count": len(apps)})
}

func atoiOrZero(raw string) int {
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return n
}
