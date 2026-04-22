package notificationsmcp

import (
	"errors"

	"github.com/teslashibe/agent-setup/backend/internal/notifications"
)

// Client is the per-request handle every notifications_* tool receives via
// mcptool.Define[*Client, …]. The MCP server constructs one per (user,
// request) by reading the authenticated user ID off the request context
// (mcp.UserIDFromContext) — see platforms.Notifications in platforms.go.
//
// The Client intentionally holds *only* the things the tool handlers need:
// a service handle and the user ID. Cross-request state lives on the
// service itself.
type Client struct {
	Svc    *notifications.Service
	UserID string
}

// ErrMissingUserID is returned when the platform binding is wired without a
// user ID. Should never happen in production — the MCP server always sets
// it from the JWT — but the check turns a class of latent bugs into a
// loud error.
var ErrMissingUserID = errors.New("notifications: missing authenticated user ID on context")

func (c *Client) requireUser() error {
	if c == nil || c.UserID == "" {
		return ErrMissingUserID
	}
	return nil
}
