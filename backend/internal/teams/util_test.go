package teams

import (
	"strings"
	"testing"
)

func TestRoleAtLeast(t *testing.T) {
	cases := []struct {
		actor, min Role
		want       bool
	}{
		{RoleOwner, RoleOwner, true},
		{RoleOwner, RoleAdmin, true},
		{RoleOwner, RoleMember, true},
		{RoleAdmin, RoleOwner, false},
		{RoleAdmin, RoleAdmin, true},
		{RoleAdmin, RoleMember, true},
		{RoleMember, RoleOwner, false},
		{RoleMember, RoleAdmin, false},
		{RoleMember, RoleMember, true},
		{Role(""), RoleMember, false},
		{Role("garbage"), RoleMember, false},
	}
	for _, tc := range cases {
		if got := tc.actor.AtLeast(tc.min); got != tc.want {
			t.Errorf("Role(%q).AtLeast(%q) = %v, want %v", tc.actor, tc.min, got, tc.want)
		}
	}
}

func TestRoleValid(t *testing.T) {
	for _, r := range []Role{RoleOwner, RoleAdmin, RoleMember} {
		if !r.Valid() {
			t.Errorf("expected %q to be valid", r)
		}
	}
	for _, r := range []Role{Role(""), Role("OWNER"), Role("guest")} {
		if r.Valid() {
			t.Errorf("expected %q to be invalid", r)
		}
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"":                    "",
		"  ":                  "",
		"Acme Corp":           "acme-corp",
		"Bob's Workspace":     "bob-s-workspace",
		"   leading-trailing": "leading-trailing",
		"!!! Special !!!":     "special",
		"UPPER 123":           "upper-123",
		"a__b__c":             "a-b-c",
		"---dashes---":        "dashes",
	}
	for in, want := range cases {
		got := slugify(in)
		if got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}

	long := strings.Repeat("a", 100)
	if got := slugify(long); len(got) > 48 {
		t.Errorf("slugify did not cap length: got %d", len(got))
	}
}

func TestPersonalTeamName(t *testing.T) {
	cases := []struct {
		display, email, want string
	}{
		{"Alice", "alice@example.com", "Alice's Workspace"},
		{"", "bob@example.com", "bob's Workspace"},
		{"   ", "carol@x.io", "carol's Workspace"},
		{"", "", "Personal Workspace"},
	}
	for _, tc := range cases {
		got := personalTeamName(tc.display, tc.email)
		if got != tc.want {
			t.Errorf("personalTeamName(%q, %q) = %q, want %q", tc.display, tc.email, got, tc.want)
		}
	}
}

func TestNewInviteToken(t *testing.T) {
	a, err := NewInviteToken()
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewInviteToken()
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("expected unique invite tokens")
	}
	if len(a) < 40 {
		t.Fatalf("invite token surprisingly short: %q (%d bytes)", a, len(a))
	}
	if strings.ContainsAny(a, "+/=") {
		t.Fatalf("invite token contains non-URL-safe characters: %q", a)
	}
}

func TestInviteActive(t *testing.T) {
	now := SuggestUpdatedAt()
	expired := Invite{ExpiresAt: now.Add(-1)}
	if expired.Active(now) {
		t.Fatal("expected expired invite to be inactive")
	}

	live := Invite{ExpiresAt: now.Add(60)}
	if !live.Active(now) {
		t.Fatal("expected live invite to be active")
	}
}
