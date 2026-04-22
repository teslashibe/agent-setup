package apperrors

import (
	"errors"
	"net/http"

	"github.com/gofiber/fiber/v2"
)

type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return e.Message }

func New(status int, message string) *Error {
	return &Error{Status: status, Message: message}
}

var (
	ErrUnauthorized = New(http.StatusUnauthorized, "unauthorized")
	ErrNotFound     = New(http.StatusNotFound, "not found")
	ErrBadRequest   = New(http.StatusBadRequest, "bad request")
	ErrForbidden    = New(http.StatusForbidden, "forbidden")

	// Team / membership errors.
	ErrTeamNotFound         = New(http.StatusNotFound, "team not found")
	ErrNotTeamMember        = New(http.StatusForbidden, "not a member of this team")
	ErrInsufficientRole     = New(http.StatusForbidden, "your role does not permit this action")
	ErrCannotRemoveOwner    = New(http.StatusBadRequest, "owner cannot be removed; transfer ownership first")
	ErrCannotLeavePersonal  = New(http.StatusBadRequest, "cannot leave your personal team")
	ErrCannotDeletePersonal = New(http.StatusBadRequest, "cannot delete your personal team")
	ErrSeatLimitReached     = New(http.StatusBadRequest, "team has reached its seat limit")
	ErrAlreadyMember        = New(http.StatusConflict, "user is already a member of this team")
	ErrOwnerExists          = New(http.StatusConflict, "team already has an owner")
	ErrPersonalTeamExists   = New(http.StatusConflict, "user already has a personal team")
	ErrFeatureDisabled      = New(http.StatusNotFound, "feature is not enabled in this deployment")

	// Invite errors.
	ErrInviteNotFound        = New(http.StatusNotFound, "invite not found")
	ErrInviteExpired         = New(http.StatusGone, "invite expired")
	ErrInviteAlreadyAccepted = New(http.StatusConflict, "invite already accepted")
	ErrInviteRevoked         = New(http.StatusGone, "invite revoked")
	ErrInvitePending         = New(http.StatusConflict, "an active invite for this email already exists")
	ErrEmailMismatch         = New(http.StatusForbidden, "this invite was sent to a different email")
)

// FiberHandler is the centralized Fiber error handler. Wire it via
// fiber.Config{ErrorHandler: apperrors.FiberHandler}.
func FiberHandler(c *fiber.Ctx, err error) error {
	var appErr *Error
	if errors.As(err, &appErr) {
		return c.Status(appErr.Status).JSON(fiber.Map{"error": appErr.Message})
	}
	return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "internal server error"})
}

const userIDKey = "user_id"

// SetUserID is called by the auth middleware after a successful JWT check.
func SetUserID(c *fiber.Ctx, id string) { c.Locals(userIDKey, id) }

// UserID returns the authenticated user's ID. Safe to call only inside
// routes guarded by RequireAuth — the middleware guarantees the value is set.
func UserID(c *fiber.Ctx) string {
	id, _ := c.Locals(userIDKey).(string)
	return id
}
