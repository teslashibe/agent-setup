package invites

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/teslashibe/agent-setup/backend/internal/apperrors"
	"github.com/teslashibe/agent-setup/backend/internal/teams"
)

// Handler exposes invite-related routes. Mount returns a small package of
// routes mounted on different parents because public + authed flows live
// next to each other:
//
//   - /api/teams/:teamID/invites/* (auth + RequireTeamFromParam)
//   - GET  /api/invites/:token         (PUBLIC — invite preview)
//   - GET  /api/invites/preview?token  (PUBLIC — query alias for the above)
//   - POST /api/invites/:token/accept  (auth required)
//   - POST /api/invites/accept         (auth required, body {token} alias)
//   - GET  /invites/accept             (PUBLIC; deep-links into mobile app)
type Handler struct {
	svc      *Service
	mw       *teams.Middleware
	limiters Limiters
}

// Limiters lets the caller wire fiber rate-limit middlewares specific to
// invite create / accept (per spec — admin invite cap, IP-bound accept cap).
// Either may be nil, in which case the route is mounted unlimited.
type Limiters struct {
	Create fiber.Handler
	Accept fiber.Handler
}

func NewHandler(svc *Service, mw *teams.Middleware) *Handler {
	return &Handler{svc: svc, mw: mw}
}

// WithLimiters attaches rate-limit middlewares. Returns the handler for
// chaining so wiring stays one-line in main.go.
func (h *Handler) WithLimiters(l Limiters) *Handler {
	h.limiters = l
	return h
}

// MountTeamRoutes attaches the per-team invite routes under
// `/api/teams/:teamID`. Requires that the parent group already enforced
// auth + team membership.
func (h *Handler) MountTeamRoutes(api fiber.Router) {
	api.Get("/invites", h.ListActive)
	create := []fiber.Handler{teams.RequireRole(teams.RoleAdmin)}
	if h.limiters.Create != nil {
		create = append(create, h.limiters.Create)
	}
	create = append(create, h.Create)
	api.Post("/invites", create...)
	api.Post("/invites/:inviteID/resend", teams.RequireRole(teams.RoleAdmin), h.Resend)
	api.Delete("/invites/:inviteID", teams.RequireRole(teams.RoleAdmin), h.Revoke)
}

// MountAuthAPIRoutes attaches accept routes under `/api/invites`. Caller is
// responsible for ensuring auth.RequireAuth is on the parent group.
func (h *Handler) MountAuthAPIRoutes(api fiber.Router) {
	g := api.Group("/invites")
	if h.limiters.Accept != nil {
		g.Use(h.limiters.Accept)
	}
	// Spec-shaped: POST /api/invites/:token/accept
	g.Post("/:token/accept", h.AcceptByPath)
	// Legacy alias: POST /api/invites/accept with body {token}
	g.Post("/accept", h.Accept)
}

// MountPublicAPIRoutes attaches the public preview routes under `/api/invites`.
// Mount on `app` directly (NOT the auth-protected `/api` group) so the
// invite landing page can call them before the user signs in.
func (h *Handler) MountPublicAPIRoutes(app fiber.Router) {
	g := app.Group("/api/invites")
	if h.limiters.Accept != nil {
		// Same per-IP throttle protects token-enumeration via preview.
		g.Use(h.limiters.Accept)
	}
	// Order matters: literal "/preview" must come before "/:token" or the
	// param route swallows the alias path with token = "preview".
	g.Get("/preview", h.Preview)
	g.Get("/:token", h.PreviewByPath)
}

// MountPublicRoutes attaches the public deep-link landing under `/invites`.
func (h *Handler) MountPublicRoutes(app fiber.Router, mobileScheme string) {
	app.Get("/invites/accept", h.LandingPage(mobileScheme))
}

func (h *Handler) ListActive(c *fiber.Ctx) error {
	list, err := h.svc.ListActive(c.UserContext(), apperrors.TeamID(c))
	if err != nil {
		return err
	}
	// Strip the token from each invite — only the email recipient should
	// ever see it. Admins can resend to push the link out again.
	redacted := make([]teams.Invite, len(list))
	for i, inv := range list {
		inv.Token = ""
		redacted[i] = inv
	}
	return c.JSON(fiber.Map{"invites": redacted})
}

type createReq struct {
	Email string     `json:"email"`
	Role  teams.Role `json:"role"`
}

func (h *Handler) Create(c *fiber.Ctx) error {
	var req createReq
	if err := c.BodyParser(&req); err != nil {
		return apperrors.New(fiber.StatusBadRequest, "invalid request body")
	}
	if req.Role == "" {
		req.Role = teams.RoleMember
	}
	inv, err := h.svc.CreateAndSend(
		c.UserContext(),
		apperrors.TeamID(c),
		apperrors.UserID(c),
		req.Email,
		req.Role,
	)
	if err != nil {
		return err
	}
	// Strip the token from the response — only the email recipient needs it.
	inv.Token = ""
	return c.Status(fiber.StatusCreated).JSON(inv)
}

func (h *Handler) Resend(c *fiber.Ctx) error {
	inviteID := strings.TrimSpace(c.Params("inviteID"))
	if inviteID == "" {
		return apperrors.New(fiber.StatusBadRequest, "invite id required")
	}
	inv, err := h.svc.Resend(c.UserContext(), apperrors.TeamID(c), apperrors.UserID(c), inviteID)
	if err != nil {
		return err
	}
	inv.Token = ""
	return c.JSON(inv)
}

func (h *Handler) Revoke(c *fiber.Ctx) error {
	inviteID := strings.TrimSpace(c.Params("inviteID"))
	if inviteID == "" {
		return apperrors.New(fiber.StatusBadRequest, "invite id required")
	}
	if err := h.svc.Revoke(c.UserContext(), apperrors.TeamID(c), inviteID); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) Preview(c *fiber.Ctx) error {
	token := strings.TrimSpace(c.Query("token"))
	if token == "" {
		return apperrors.New(fiber.StatusBadRequest, "token is required")
	}
	preview, err := h.svc.Preview(c.UserContext(), token)
	if err != nil {
		return err
	}
	return c.JSON(preview)
}

// PreviewByPath is the spec-shaped twin of Preview but takes the token from
// the URL path so external integrators can hit `GET /api/invites/:token`.
// Public endpoint — no auth, intentionally — so the mobile invite landing
// can render the preview before forcing sign-in.
func (h *Handler) PreviewByPath(c *fiber.Ctx) error {
	token := strings.TrimSpace(c.Params("token"))
	if token == "" {
		return apperrors.New(fiber.StatusBadRequest, "token is required")
	}
	preview, err := h.svc.Preview(c.UserContext(), token)
	if err != nil {
		return err
	}
	return c.JSON(preview)
}

// AcceptByPath mirrors the spec route `POST /api/invites/:token/accept`. The
// token lives in the URL so a recipient who forwarded the email link can
// still complete acceptance after clicking through; the email-match check
// inside AcceptByToken is what stops a third party from claiming the seat.
func (h *Handler) AcceptByPath(c *fiber.Ctx) error {
	token := strings.TrimSpace(c.Params("token"))
	if token == "" {
		return apperrors.New(fiber.StatusBadRequest, "token is required")
	}
	team, role, err := h.svc.AcceptByToken(c.UserContext(), apperrors.UserID(c), token)
	if err != nil {
		return err
	}
	return c.JSON(teams.Membership{Team: team, Role: role})
}

type acceptReq struct {
	Token string `json:"token"`
}

func (h *Handler) Accept(c *fiber.Ctx) error {
	var req acceptReq
	if err := c.BodyParser(&req); err != nil {
		return apperrors.New(fiber.StatusBadRequest, "invalid request body")
	}
	token := strings.TrimSpace(req.Token)
	if token == "" {
		token = strings.TrimSpace(c.Query("token"))
	}
	if token == "" {
		return apperrors.New(fiber.StatusBadRequest, "token is required")
	}
	team, role, err := h.svc.AcceptByToken(c.UserContext(), apperrors.UserID(c), token)
	if err != nil {
		return err
	}
	return c.JSON(teams.Membership{Team: team, Role: role})
}

// LandingPage serves a tiny HTML page that:
//
//  1. Tries to deep-link into the mobile app at <scheme>://invites/accept?token=...
//  2. Falls back to a "open the app or sign in" message for browsers without
//     the app installed.
//
// The actual invite acceptance happens after the user signs in (via mobile or
// web) — this page never touches the database, so it's safe to render
// without authentication.
func (h *Handler) LandingPage(mobileScheme string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := strings.TrimSpace(c.Query("token"))
		if token == "" {
			return apperrors.New(fiber.StatusBadRequest, "token is required")
		}
		preview, err := h.svc.Preview(c.UserContext(), token)
		if err != nil {
			return c.Status(fiber.StatusGone).Type("html").SendString(landingErrorHTML(err))
		}
		c.Set("Content-Type", "text/html; charset=utf-8")
		return c.SendString(landingHTML(preview, token, mobileScheme))
	}
}
