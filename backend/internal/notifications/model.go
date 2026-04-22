// Package notifications stores and queries device-captured notification
// events ingested from the Expo NotificationListenerService module on
// Android. The Claude agent reads this data through the MCP tools defined
// in subpackage notifications/mcp to produce daily communication rollups
// across SMS, WhatsApp, email, Zillow, etc.
//
// The package is opt-in at runtime: callers gate route mounting and MCP
// registration on cfg.NotificationsEnabled. The migration ships in every
// fork; the table simply stays empty when the feature is off.
package notifications

import "time"

// Event is a single notification observation, stored in the
// notification_events hypertable.
type Event struct {
	ID         int64     `json:"id"`
	UserID     string    `json:"user_id"`
	AppPackage string    `json:"app_package"`
	AppLabel   string    `json:"app_label"`
	Title      string    `json:"title,omitempty"`
	Content    string    `json:"content,omitempty"`
	Category   string    `json:"category,omitempty"`
	CapturedAt time.Time `json:"captured_at"`
	CreatedAt  time.Time `json:"created_at"`
}

// EventInput is the per-event payload accepted by POST /api/notifications/batch.
// It is the device's view: no server-assigned IDs, no user_id (taken from JWT),
// no created_at (assigned at insert time).
type EventInput struct {
	AppPackage string    `json:"app_package"`
	AppLabel   string    `json:"app_label"`
	Title      string    `json:"title,omitempty"`
	Content    string    `json:"content,omitempty"`
	Category   string    `json:"category,omitempty"`
	CapturedAt time.Time `json:"captured_at"`
}

// BatchInput wraps a slice of EventInput. The mobile app flushes its local
// SQLite buffer in batches (default every 5 minutes) so the server side
// keeps the round-trip count low.
type BatchInput struct {
	Events []EventInput `json:"events"`
}

// BatchResult is the response shape from POST /api/notifications/batch. We
// return the count of accepted (non-duplicate) rows so the mobile client
// can surface a "X notifications captured today" stat.
type BatchResult struct {
	Accepted int `json:"accepted"`
}

// Thread is a contact-or-app-grouped cluster of notifications used by the
// notifications_threads MCP tool to give the agent a conversation view.
type Thread struct {
	Contact      string    `json:"contact"`
	AppLabel     string    `json:"app_label"`
	AppPackage   string    `json:"app_package"`
	MessageCount int       `json:"message_count"`
	FirstAt      time.Time `json:"first_at"`
	LastAt       time.Time `json:"last_at"`
	Preview      string    `json:"preview,omitempty"`
}

// AppSummary is the aggregate per-app view used by notifications_apps and
// the GET /api/notifications/apps REST endpoint that powers the mobile
// settings screen's "captured apps" count.
type AppSummary struct {
	AppPackage string    `json:"app_package"`
	AppLabel   string    `json:"app_label"`
	Count      int       `json:"count"`
	LastAt     time.Time `json:"last_at"`
}

// ActionItem is a heuristic-extracted candidate task surfaced by the
// notifications_pending_actions MCP tool. The agent ranks and presents
// these in the "What needs attention" half of the rollup.
type ActionItem struct {
	Priority   string    `json:"priority"`
	Summary    string    `json:"summary"`
	Contact    string    `json:"contact,omitempty"`
	AppLabel   string    `json:"app_label"`
	AppPackage string    `json:"app_package"`
	CapturedAt time.Time `json:"captured_at"`
	Reason     string    `json:"reason"`
	EventID    int64     `json:"event_id"`
}

// ListOpts is the shared time-range / app / pagination filter struct used
// across List and Search.
type ListOpts struct {
	Since      *time.Time
	Until      *time.Time
	AppPackage string
	Limit      int
}

// ThreadOpts narrows GroupThreads to a time range and optional app, with
// configurable grouping (currently "contact" or "app").
type ThreadOpts struct {
	Since      *time.Time
	Until      *time.Time
	AppPackage string
	GroupBy    string
	Limit      int
}

// ActionOpts narrows PendingActions to a time range with a configurable
// reply-window threshold for the "unanswered" heuristic.
type ActionOpts struct {
	Since           *time.Time
	Until           *time.Time
	ReplyWindowHrs  int
	Limit           int
}
