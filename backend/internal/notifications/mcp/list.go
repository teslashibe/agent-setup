package notificationsmcp

import (
	"context"
	"time"

	"github.com/teslashibe/mcptool"

	"github.com/teslashibe/agent-setup/backend/internal/notifications"
)

// ListInput is the typed input for notifications_list.
type ListInput struct {
	Since      string `json:"since,omitempty" jsonschema:"description=RFC3339 lower bound on captured_at (e.g. 2026-04-22T00:00:00Z)"`
	Until      string `json:"until,omitempty" jsonschema:"description=RFC3339 upper bound on captured_at"`
	AppPackage string `json:"app_package,omitempty" jsonschema:"description=Restrict to a single app package id (e.g. com.whatsapp)"`
	Limit      int    `json:"limit,omitempty" jsonschema:"description=cap on returned events,minimum=1,maximum=200,default=50"`
}

func runList(ctx context.Context, c *Client, in ListInput) (any, error) {
	if err := c.requireUser(); err != nil {
		return nil, err
	}
	opts, err := buildListOpts(in.Since, in.Until, in.AppPackage, in.Limit)
	if err != nil {
		return nil, err
	}
	return c.Svc.List(ctx, c.UserID, opts)
}

// buildListOpts is shared between list and search so time-range parsing
// stays in one place.
func buildListOpts(since, until, app string, limit int) (notifications.ListOpts, error) {
	opts := notifications.ListOpts{AppPackage: app, Limit: limit}
	if since != "" {
		t, err := time.Parse(time.RFC3339, since)
		if err != nil {
			return opts, &mcptool.Error{Code: "invalid_input", Message: "invalid 'since' (want RFC3339): " + err.Error()}
		}
		opts.Since = &t
	}
	if until != "" {
		t, err := time.Parse(time.RFC3339, until)
		if err != nil {
			return opts, &mcptool.Error{Code: "invalid_input", Message: "invalid 'until' (want RFC3339): " + err.Error()}
		}
		opts.Until = &t
	}
	return opts, nil
}

var listTools = []mcptool.Tool{
	mcptool.Define[*Client, ListInput](
		"notifications_list",
		"List the user's captured notifications in reverse chronological order; primary tool for daily rollups",
		"List",
		runList,
	),
}
