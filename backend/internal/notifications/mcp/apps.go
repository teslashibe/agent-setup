package notificationsmcp

import (
	"context"
	"time"

	"github.com/teslashibe/mcptool"
)

// AppsInput is the typed input for notifications_apps.
type AppsInput struct {
	Since string `json:"since,omitempty" jsonschema:"description=RFC3339 lower bound on captured_at"`
	Until string `json:"until,omitempty" jsonschema:"description=RFC3339 upper bound on captured_at"`
}

func runApps(ctx context.Context, c *Client, in AppsInput) (any, error) {
	if err := c.requireUser(); err != nil {
		return nil, err
	}
	var since, until *time.Time
	if in.Since != "" {
		t, err := time.Parse(time.RFC3339, in.Since)
		if err != nil {
			return nil, &mcptool.Error{Code: "invalid_input", Message: "invalid 'since' (want RFC3339): " + err.Error()}
		}
		since = &t
	}
	if in.Until != "" {
		t, err := time.Parse(time.RFC3339, in.Until)
		if err != nil {
			return nil, &mcptool.Error{Code: "invalid_input", Message: "invalid 'until' (want RFC3339): " + err.Error()}
		}
		until = &t
	}
	return c.Svc.ListApps(ctx, c.UserID, since, until)
}

var appTools = []mcptool.Tool{
	mcptool.Define[*Client, AppsInput](
		"notifications_apps",
		"Summarise which apps sent notifications in the time range (count + last seen). Use to scope the rollup",
		"ListApps",
		runApps,
	),
}
