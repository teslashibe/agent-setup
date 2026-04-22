package teams

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/teslashibe/agent-setup/backend/internal/apperrors"
)

// Handler exposes /api/teams/* routes. Mount via Mount(router, mw) where mw
// supplies RequireTeamFromParam (so the routes that take :teamID benefit
// from membership resolution + role checks).
type Handler struct {
	svc *Service
	mw  *Middleware
}

func NewHandler(svc *Service, mw *Middleware) *Handler {
	return &Handler{svc: svc, mw: mw}
}

// Mount wires the routes under the given parent. Caller is responsible for
// ensuring auth.RequireAuth is already applied to the parent group.
func (h *Handler) Mount(api fiber.Router) {
	teams := api.Group("/teams")
	teams.Get("/", h.List)
	teams.Post("/", h.Create)

	t := teams.Group("/:teamID", h.mw.RequireTeamFromParam("teamID"))
	t.Get("/", h.Get)
	t.Patch("/", RequireRole(RoleAdmin), h.UpdateName)
	t.Delete("/", RequireRole(RoleOwner), h.Delete)
	t.Post("/leave", h.Leave)
	t.Post("/transfer-ownership", RequireRole(RoleOwner), h.TransferOwnership)

	t.Get("/members", h.ListMembers)
	t.Patch("/members/:userID", RequireRole(RoleAdmin), h.ChangeMemberRole)
	t.Delete("/members/:userID", RequireRole(RoleAdmin), h.RemoveMember)
}

func (h *Handler) List(c *fiber.Ctx) error {
	memberships, err := h.svc.ListForUser(c.UserContext(), apperrors.UserID(c))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"teams": memberships})
}

type createReq struct {
	Name string `json:"name"`
}

func (h *Handler) Create(c *fiber.Ctx) error {
	var req createReq
	if err := c.BodyParser(&req); err != nil {
		return apperrors.New(fiber.StatusBadRequest, "invalid request body")
	}
	team, err := h.svc.Create(c.UserContext(), apperrors.UserID(c), req.Name)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(Membership{Team: team, Role: RoleOwner})
}

func (h *Handler) Get(c *fiber.Ctx) error {
	team, err := h.svc.Get(c.UserContext(), apperrors.TeamID(c))
	if err != nil {
		return err
	}
	return c.JSON(Membership{Team: team, Role: Role(apperrors.TeamRole(c))})
}

type updateNameReq struct {
	Name string `json:"name"`
}

func (h *Handler) UpdateName(c *fiber.Ctx) error {
	var req updateNameReq
	if err := c.BodyParser(&req); err != nil {
		return apperrors.New(fiber.StatusBadRequest, "invalid request body")
	}
	team, err := h.svc.UpdateName(c.UserContext(), apperrors.UserID(c), apperrors.TeamID(c), req.Name)
	if err != nil {
		return err
	}
	return c.JSON(Membership{Team: team, Role: Role(apperrors.TeamRole(c))})
}

func (h *Handler) Delete(c *fiber.Ctx) error {
	if err := h.svc.Delete(c.UserContext(), apperrors.UserID(c), apperrors.TeamID(c)); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) Leave(c *fiber.Ctx) error {
	if err := h.svc.Leave(c.UserContext(), apperrors.UserID(c), apperrors.TeamID(c)); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type transferReq struct {
	UserID string `json:"user_id"`
}

func (h *Handler) TransferOwnership(c *fiber.Ctx) error {
	var req transferReq
	if err := c.BodyParser(&req); err != nil {
		return apperrors.New(fiber.StatusBadRequest, "invalid request body")
	}
	if strings.TrimSpace(req.UserID) == "" {
		return apperrors.New(fiber.StatusBadRequest, "user_id is required")
	}
	if err := h.svc.TransferOwnership(c.UserContext(), apperrors.UserID(c), apperrors.TeamID(c), req.UserID); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) ListMembers(c *fiber.Ctx) error {
	members, err := h.svc.ListMembers(c.UserContext(), apperrors.UserID(c), apperrors.TeamID(c))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"members": members})
}

type changeRoleReq struct {
	Role Role `json:"role"`
}

func (h *Handler) ChangeMemberRole(c *fiber.Ctx) error {
	var req changeRoleReq
	if err := c.BodyParser(&req); err != nil {
		return apperrors.New(fiber.StatusBadRequest, "invalid request body")
	}
	target := strings.TrimSpace(c.Params("userID"))
	if target == "" {
		return apperrors.New(fiber.StatusBadRequest, "user id is required")
	}
	if err := h.svc.ChangeRole(c.UserContext(), apperrors.UserID(c), apperrors.TeamID(c), target, req.Role); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) RemoveMember(c *fiber.Ctx) error {
	target := strings.TrimSpace(c.Params("userID"))
	if target == "" {
		return apperrors.New(fiber.StatusBadRequest, "user id is required")
	}
	if err := h.svc.RemoveMember(c.UserContext(), apperrors.UserID(c), apperrors.TeamID(c), target); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}
