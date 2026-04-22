package platforms

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/teslashibe/agent-setup/backend/internal/mcp"
	"github.com/teslashibe/agent-setup/backend/internal/notifications"
	notificationsmcp "github.com/teslashibe/agent-setup/backend/internal/notifications/mcp"
)

// TestNotificationsPluginShape pins the wire-up of the opt-in
// notifications platform. Even though it isn't in All() (it's appended
// conditionally in cmd/server/main.go) the construction must obey the
// same invariants as the other plugins, otherwise mcp.NewRegistry will
// reject it at server boot.
func TestNotificationsPluginShape(t *testing.T) {
	svc := notifications.NewService(nil, notifications.ServiceConfig{})
	plugin := Notifications(svc)

	if plugin.Binding.Provider == nil {
		t.Fatal("Notifications plugin has nil Provider")
	}
	if got := plugin.Binding.Provider.Platform(); got != "notifications" {
		t.Errorf("Provider.Platform() = %q; want notifications", got)
	}
	if !plugin.Binding.NoCredentials {
		t.Error("Notifications binding must set NoCredentials=true (data is pushed by the device)")
	}
	if plugin.Binding.NewClient == nil {
		t.Fatal("NewClient is required")
	}
	if plugin.Validator == nil {
		t.Fatal("Validator is required even for credential-less platforms")
	}
	if got := plugin.Validator.Platform(); got != "notifications" {
		t.Errorf("Validator.Platform() = %q; want notifications", got)
	}
}

// TestNotificationsNewClientRequiresUserID demonstrates the safety check
// that prevents a tool from running without an authenticated caller.
// Without mcp.UserIDFromContext set, NewClient must error.
func TestNotificationsNewClientRequiresUserID(t *testing.T) {
	svc := notifications.NewService(nil, notifications.ServiceConfig{})
	plugin := Notifications(svc)

	_, err := plugin.Binding.NewClient(context.Background(), json.RawMessage(`null`))
	if err == nil {
		t.Fatal("NewClient must error when ctx has no userID")
	}
}

// TestNotificationsNewClientPropagatesUser verifies the happy path: when
// the MCP server stamps a userID on the context, NewClient returns a
// notificationsmcp.Client wired to that user.
func TestNotificationsNewClientPropagatesUser(t *testing.T) {
	svc := notifications.NewService(nil, notifications.ServiceConfig{})
	plugin := Notifications(svc)

	ctx := mcpUserCtx("user_xyz")
	c, err := plugin.Binding.NewClient(ctx, nil)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	client, ok := c.(*notificationsmcp.Client)
	if !ok {
		t.Fatalf("NewClient returned %T; want *notificationsmcp.Client", c)
	}
	if client.UserID != "user_xyz" {
		t.Errorf("client.UserID = %q; want user_xyz", client.UserID)
	}
	if client.Svc != svc {
		t.Errorf("client.Svc not the one we passed in")
	}
}

// TestNotificationsRegistryComposes proves the plugin can be added to a
// registry alongside All() — i.e. there are no duplicate platform names
// or tool names. This mirrors the production wiring in cmd/server/main.go.
func TestNotificationsRegistryComposes(t *testing.T) {
	svc := notifications.NewService(nil, notifications.ServiceConfig{})
	plugins := append(All(), Notifications(svc))
	bindings := make([]mcp.PlatformBinding, 0, len(plugins))
	for _, p := range plugins {
		bindings = append(bindings, p.Binding)
	}
	registry, err := mcp.NewRegistry(bindings...)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	if got := len(registry.Platforms()); got != len(plugins) {
		t.Errorf("registry has %d platforms; want %d", got, len(plugins))
	}
	// Confirm the 5 notification tools made it through.
	want := map[string]bool{
		"notifications_list":            false,
		"notifications_search":          false,
		"notifications_threads":         false,
		"notifications_apps":            false,
		"notifications_pending_actions": false,
	}
	for _, tool := range registry.Tools() {
		if _, expected := want[tool.Name]; expected {
			want[tool.Name] = true
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("registry missing tool %q after appending Notifications plugin", name)
		}
	}
}

// mcpUserCtx is a tiny helper mirroring the production server's call to
// mcp.withUserID. We use the package-internal helper through the exported
// UserIDFromContext path: build a context that satisfies
// mcp.UserIDFromContext == userID.
func mcpUserCtx(userID string) context.Context {
	return mcp.WithUserIDForTest(context.Background(), userID)
}
