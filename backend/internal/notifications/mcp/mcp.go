// Package notificationsmcp exposes the per-user notification corpus to
// the Claude agent as a set of MCP tools. Unlike the other platform-bound
// providers in agent-setup (which wrap external scrapers), this provider
// queries the local notification_events hypertable directly through a
// per-request notifications.Service.
//
// Registration is gated on cfg.NotificationsEnabled in cmd/server/main.go;
// when off, the provider is never appended to the platform list and the
// agent never sees these tools in tools/list.
package notificationsmcp

import "github.com/teslashibe/mcptool"

// Provider implements mcptool.Provider for the internal notifications
// platform. Zero value is ready to use.
type Provider struct{}

// Platform returns "notifications". Tool names are prefixed accordingly.
func (Provider) Platform() string { return "notifications" }

// Tools returns every notifications_* MCP tool exposed by this provider.
// Order here is purely cosmetic; the registry sorts by name.
func (Provider) Tools() []mcptool.Tool {
	out := make([]mcptool.Tool, 0, 5)
	out = append(out, listTools...)
	out = append(out, searchTools...)
	out = append(out, threadTools...)
	out = append(out, appTools...)
	out = append(out, actionTools...)
	return out
}
