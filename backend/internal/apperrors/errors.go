package apperrors

import (
	"errors"
	"net/http"

	"github.com/gofiber/fiber/v2"
)

type AppError struct {
	Status  int
	Message string
}

func (e *AppError) Error() string { return e.Message }

func New(status int, message string) *AppError {
	return &AppError{Status: status, Message: message}
}

var (
	ErrUnauthorized = New(http.StatusUnauthorized, "unauthorized")
	ErrNotFound     = New(http.StatusNotFound, "not found")
	ErrBadRequest   = New(http.StatusBadRequest, "bad request")
	ErrForbidden    = New(http.StatusForbidden, "forbidden")
)

func Handle(c *fiber.Ctx, err error) error {
	if err == nil {
		return nil
	}
	var appErr *AppError
	if errors.As(err, &appErr) {
		return c.Status(appErr.Status).JSON(fiber.Map{"error": appErr.Message})
	}
	return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": "internal server error"})
}

const userIDKey = "user_id"

func SetUserID(c *fiber.Ctx, id string)  { c.Locals(userIDKey, id) }

func CurrentUserID(c *fiber.Ctx) (string, error) {
	id, ok := c.Locals(userIDKey).(string)
	if !ok || id == "" {
		return "", ErrUnauthorized
	}
	return id, nil
}
