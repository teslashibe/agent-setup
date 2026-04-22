package notificationsmcp

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/teslashibe/mcptool"

	"github.com/teslashibe/agent-setup/backend/internal/notifications"
)

// TestProviderToolNames pins the exact set of notifications_* tools
// exposed to Claude. Adding or removing a tool must come with a deliberate
// edit here so the system prompt + provisioning stay in sync.
func TestProviderToolNames(t *testing.T) {
	p := Provider{}
	got := toolNames(p.Tools())
	want := []string{
		"notifications_apps",
		"notifications_list",
		"notifications_pending_actions",
		"notifications_search",
		"notifications_threads",
	}
	sort.Strings(got)
	if len(got) != len(want) {
		t.Fatalf("Provider tool count: got %d %v; want %d %v", len(got), got, len(want), want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("tool[%d]: got %q; want %q", i, got[i], want[i])
		}
	}
}

// TestProviderTools_HavePlatformPrefix enforces the agent-setup convention
// that every tool name starts with its provider platform string. This
// keeps tool listings predictable for the agent.
func TestProviderTools_HavePlatformPrefix(t *testing.T) {
	p := Provider{}
	platform := p.Platform()
	if platform == "" {
		t.Fatal("Provider.Platform() returned empty string")
	}
	for _, tool := range p.Tools() {
		if !strings.HasPrefix(tool.Name, platform+"_") {
			t.Errorf("tool %q must be prefixed with %q_", tool.Name, platform)
		}
		if tool.Description == "" {
			t.Errorf("tool %q has empty description", tool.Name)
		}
		if tool.InputSchema == nil {
			t.Errorf("tool %q has nil InputSchema", tool.Name)
		}
		if tool.Invoke == nil {
			t.Errorf("tool %q has nil Invoke", tool.Name)
		}
	}
}

// TestProviderTools_PassValidation runs each tool through mcptool's
// validator so any malformed input schema or missing field is caught at
// build time rather than at runtime when Claude calls the tool.
func TestProviderTools_PassValidation(t *testing.T) {
	p := Provider{}
	if err := mcptool.ValidateTools(p.Tools()); err != nil {
		t.Fatalf("Provider tools failed mcptool validation: %v", err)
	}
}

// TestRequireUser verifies the safety check that prevents accidental
// cross-user data leaks: every tool must call requireUser() before
// touching the service. We test this on the Client directly.
func TestRequireUser(t *testing.T) {
	cases := []struct {
		name    string
		client  *Client
		wantErr bool
	}{
		{name: "nil_client", client: nil, wantErr: true},
		{name: "empty_user_id", client: &Client{UserID: ""}, wantErr: true},
		{name: "valid_user_id", client: &Client{UserID: "user_123"}, wantErr: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.client.requireUser()
			if tc.wantErr && err == nil {
				t.Errorf("requireUser() should error for %s", tc.name)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("requireUser() should succeed for %s; got %v", tc.name, err)
			}
		})
	}
}

// TestBuildListOpts_Validation pins the RFC3339 contract for since/until
// inputs. Bad timestamps are returned as a typed mcptool.Error with code
// "invalid_input" so the agent surfaces a helpful message rather than a
// generic 500.
func TestBuildListOpts_Validation(t *testing.T) {
	t.Run("valid_inputs", func(t *testing.T) {
		opts, err := buildListOpts("2026-04-22T00:00:00Z", "2026-04-22T23:59:59Z", "com.whatsapp", 50)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if opts.Since == nil || opts.Until == nil {
			t.Fatal("expected since+until populated")
		}
		if opts.AppPackage != "com.whatsapp" {
			t.Errorf("AppPackage: got %q; want com.whatsapp", opts.AppPackage)
		}
		if opts.Limit != 50 {
			t.Errorf("Limit: got %d; want 50", opts.Limit)
		}
	})
	t.Run("bad_since", func(t *testing.T) {
		_, err := buildListOpts("not a date", "", "", 0)
		if err == nil {
			t.Fatal("expected error for bad since")
		}
		me, ok := err.(*mcptool.Error)
		if !ok {
			t.Fatalf("expected *mcptool.Error; got %T", err)
		}
		if me.Code != "invalid_input" {
			t.Errorf("error code: got %q; want invalid_input", me.Code)
		}
	})
	t.Run("bad_until", func(t *testing.T) {
		_, err := buildListOpts("", "still not a date", "", 0)
		if err == nil {
			t.Fatal("expected error for bad until")
		}
		me, ok := err.(*mcptool.Error)
		if !ok {
			t.Fatalf("expected *mcptool.Error; got %T", err)
		}
		if me.Code != "invalid_input" {
			t.Errorf("error code: got %q; want invalid_input", me.Code)
		}
	})
}

// TestNoUserShortcircuit confirms that calling any tool with a Client that
// has no UserID returns ErrMissingUserID without touching the service.
// This is our defence-in-depth against missing context propagation.
func TestNoUserShortcircuit(t *testing.T) {
	emptyClient := &Client{Svc: notifications.NewService(nil, notifications.ServiceConfig{}), UserID: ""}
	ctx := context.Background()

	if _, err := runList(ctx, emptyClient, ListInput{}); err == nil {
		t.Error("runList should error on empty UserID")
	}
	if _, err := runSearch(ctx, emptyClient, SearchInput{Query: "anything"}); err == nil {
		t.Error("runSearch should error on empty UserID")
	}
	if _, err := runThreads(ctx, emptyClient, ThreadsInput{}); err == nil {
		t.Error("runThreads should error on empty UserID")
	}
	if _, err := runApps(ctx, emptyClient, AppsInput{}); err == nil {
		t.Error("runApps should error on empty UserID")
	}
	if _, err := runActions(ctx, emptyClient, ActionsInput{}); err == nil {
		t.Error("runActions should error on empty UserID")
	}
}

func toolNames(ts []mcptool.Tool) []string {
	out := make([]string, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.Name)
	}
	return out
}
