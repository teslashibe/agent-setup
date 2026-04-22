package notificationsmcp

import (
	"context"
	"time"

	"github.com/teslashibe/mcptool"

	"github.com/teslashibe/agent-setup/backend/internal/notifications"
)

// ActionsInput is the typed input for notifications_pending_actions.
type ActionsInput struct {
	Since           string `json:"since,omitempty" jsonschema:"description=RFC3339 lower bound on captured_at"`
	Until           string `json:"until,omitempty" jsonschema:"description=RFC3339 upper bound on captured_at"`
	ReplyWindowHrs  int    `json:"reply_window_hours,omitempty" jsonschema:"description=hours after which an unanswered message becomes 'follow_up',minimum=1,maximum=72,default=2"`
	Limit           int    `json:"limit,omitempty" jsonschema:"description=cap on returned action items,minimum=1,maximum=200,default=50"`
}

func runActions(ctx context.Context, c *Client, in ActionsInput) (any, error) {
	if err := c.requireUser(); err != nil {
		return nil, err
	}
	opts := notifications.ActionOpts{
		ReplyWindowHrs: in.ReplyWindowHrs,
		Limit:          in.Limit,
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
	return c.Svc.PendingActions(ctx, c.UserID, opts)
}

var actionTools = []mcptool.Tool{
	mcptool.Define[*Client, ActionsInput](
		"notifications_pending_actions",
		"Heuristic candidate to-dos extracted from notifications: missed calls, time-sensitive keywords, questions",
		"PendingActions",
		runActions,
	),
}
