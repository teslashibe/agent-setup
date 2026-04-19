package auth

import "testing"

func TestDisplayName(t *testing.T) {
	cases := []struct {
		name, email, input, want string
	}{
		{"explicit name wins", "person@example.com", "Taylor", "Taylor"},
		{"falls back to email local part", "person@example.com", "", "person"},
		{"falls back to generic user", "", "", "User"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := displayName(tc.input, tc.email); got != tc.want {
				t.Fatalf("displayName(%q, %q) = %q, want %q", tc.input, tc.email, got, tc.want)
			}
		})
	}
}
