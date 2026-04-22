package mcp

import (
	"context"
	"testing"
)

// TestUserIDFromContext_RoundTrip verifies the canonical plumbing path:
// withUserID stores a user id and UserIDFromContext reads it back. This
// is the tight loop every credential-less platform binding (e.g.
// notifications) depends on, so the test is deliberately small and stable.
func TestUserIDFromContext_RoundTrip(t *testing.T) {
	ctx := withUserID(context.Background(), "user_abc")
	if got := UserIDFromContext(ctx); got != "user_abc" {
		t.Errorf("UserIDFromContext returned %q; want %q", got, "user_abc")
	}
}

// TestUserIDFromContext_MissingReturnsEmpty pins the contract that callers
// can safely call UserIDFromContext on any context (including nil) and
// get back a zero value — never a panic.
func TestUserIDFromContext_MissingReturnsEmpty(t *testing.T) {
	if got := UserIDFromContext(context.Background()); got != "" {
		t.Errorf("UserIDFromContext on bare context returned %q; want empty", got)
	}
	if got := UserIDFromContext(nil); got != "" { //nolint:staticcheck // intentional nil
		t.Errorf("UserIDFromContext(nil) returned %q; want empty", got)
	}
}

// TestWithUserID_EmptyIsNoop guarantees we don't pollute ctx with empty
// user-id values that would later confuse the "did the server set a user"
// check inside platform bindings.
func TestWithUserID_EmptyIsNoop(t *testing.T) {
	ctx := withUserID(context.Background(), "")
	if got := UserIDFromContext(ctx); got != "" {
		t.Errorf("withUserID(\"\") leaked %q into context; want empty", got)
	}
}
