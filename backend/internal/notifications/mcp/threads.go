package notificationsmcp

import (
	"context"
	"time"

	"github.com/teslashibe/mcptool"

	"github.com/teslashibe/agent-setup/backend/internal/notifications"
)

// ThreadsInput is the typed input for notifications_threads.
type ThreadsInput struct {
	Since      string `json:"since,omitempty" jsonschema:"description=RFC3339 lower bound on captured_at"`
	Until      string `json:"until,omitempty" jsonschema:"description=RFC3339 upper bound on captured_at"`
	AppPackage string `json:"app_package,omitempty" jsonschema:"description=Restrict to a single app package id"`
	GroupBy    string `json:"group_by,omitempty" jsonschema:"description=Cluster key,enum=contact,enum=app,default=contact"`
	Limit      int    `json:"limit,omitempty" jsonschema:"description=cap on returned threads,minimum=1,maximum=200,default=50"`
}

func runThreads(ctx context.Context, c *Client, in ThreadsInput) (any, error) {
	if err := c.requireUser(); err != nil {
		return nil, err
	}
	opts := notifications.ThreadOpts{
		AppPackage: in.AppPackage,
		GroupBy:    in.GroupBy,
		Limit:      in.Limit,
	}
	if in.Since != "" {
		t, err := time.Parse(time.RFC3339, in.Since)
		if err != nil {
			return nil, &mcptool.Error{Code: "invalid_input", Message: "invalid 'since' (want RFC3339): " + err.Error()}
		}
		opts.Since = &t
	}
	if in.Until != "" {
		t, err := time.Parse(time.RFC3339, in.Until)
		if err != nil {
			return nil, &mcptool.Error{Code: "invalid_input", Message: "invalid 'until' (want RFC3339): " + err.Error()}
		}
		opts.Until = &t
	}
	return c.Svc.GroupThreads(ctx, c.UserID, opts)
}

var threadTools = []mcptool.Tool{
	mcptool.Define[*Client, ThreadsInput](
		"notifications_threads",
		"Group notifications into conversation-like threads by app + contact (or by app) for the rollup view",
		"GroupThreads",
		runThreads,
	),
}
