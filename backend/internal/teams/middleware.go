package teams

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/teslashibe/agent-setup/backend/internal/apperrors"
)

const headerName = "X-Team-ID"

// Middleware exposes RequireTeam and RequireRole. It expects to be mounted
// downstream of auth.Middleware.RequireAuth so the user is already resolved.
type Middleware struct{ svc *Service }

func NewMiddleware(svc *Service) *Middleware { return &Middleware{svc: svc} }

// RequireTeam resolves the active team from the X-Team-ID header (or, if not
// present, the user's personal team) and stores team_id + team_role in locals.
// Returns 403 if the header references a team the caller does not belong to.
func (m *Middleware) RequireTeam() fiber.Handler {
	return func(c *fiber.Ctx) error {
		uid := apperrors.UserID(c)
		if uid == "" {
			return apperrors.ErrUnauthorized
		}
		requested := strings.TrimSpace(c.Get(headerName))
		membership, err := m.svc.ResolveActive(
			c.UserContext(), uid, requested,
			apperrors.UserDisplayName(c), apperrors.UserEmail(c),
		)
		if err != nil {
			return err
		}
		apperrors.SetTeamID(c, membership.Team.ID)
		apperrors.SetTeamRole(c, string(membership.Role))
		return c.Next()
	}
}

// RequireTeamFromParam resolves the active team from a URL parameter (e.g.
// :teamID) instead of the header. Use for /api/teams/:teamID/* routes where
// the team is unambiguously identified by the path.
func (m *Middleware) RequireTeamFromParam(param string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		uid := apperrors.UserID(c)
		if uid == "" {
			return apperrors.ErrUnauthorized
		}
		teamID := strings.TrimSpace(c.Params(param))
		if teamID == "" {
			return apperrors.New(fiber.StatusBadRequest, "team id is required")
		}
		membership, err := m.svc.Store().GetMembership(c.UserContext(), teamID, uid)
		if err != nil {
			return err
		}
		apperrors.SetTeamID(c, membership.Team.ID)
		apperrors.SetTeamRole(c, string(membership.Role))
		return c.Next()
	}
}

// RequireRole asserts the caller's resolved role is at least min. Use after
// RequireTeam / RequireTeamFromParam.
func RequireRole(min Role) fiber.Handler {
	return func(c *fiber.Ctx) error {
		role := Role(apperrors.TeamRole(c))
		if !role.Valid() {
			return apperrors.ErrUnauthorized
		}
		if !role.AtLeast(min) {
			return apperrors.ErrInsufficientRole
		}
		return c.Next()
	}
}
