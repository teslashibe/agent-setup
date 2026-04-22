package mcp

import "context"

// userIDKey is the unexported context key the MCP server uses to thread the
// authenticated user ID into per-request client constructors. Plug-in
// platforms that need user-scoped state (e.g. internal/notifications) pull
// it back out via UserIDFromContext.
type userIDKey struct{}

// withUserID stores userID on ctx using the package-private key. Called by
// Server.CallTool just before the per-request NewClient factory runs.
func withUserID(ctx context.Context, userID string) context.Context {
	if userID == "" {
		return ctx
	}
	return context.WithValue(ctx, userIDKey{}, userID)
}

// UserIDFromContext returns the authenticated user ID stored on ctx by the
// MCP server, or "" when missing. Safe to call from any PlatformBinding.NewClient
// callback that needs to scope per-request state to the calling user.
//
// Bindings that require a user ID should treat "" as a configuration error
// (the server only fails to set it for unauthenticated transports, which
// agent-setup does not currently expose).
func UserIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(userIDKey{}).(string)
	return v
}

// WithUserIDForTest is a test-only escape hatch that lets external test
// packages (e.g. internal/mcp/platforms) build a context that
// UserIDFromContext will read back. Production code MUST NOT call this —
// the server stamps the user id via the unexported withUserID helper as
// part of CallTool.
func WithUserIDForTest(ctx context.Context, userID string) context.Context {
	return withUserID(ctx, userID)
}
