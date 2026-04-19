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
