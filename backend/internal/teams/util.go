package teams

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/base64"
	"regexp"
	"strings"
)

var (
	slugStripper  = regexp.MustCompile(`[^a-z0-9]+`)
	slugTrimmer   = regexp.MustCompile(`(^-+)|(-+$)`)
	collapseDashes = regexp.MustCompile(`-+`)
)

// slugify produces a lowercase, hyphenated slug that satisfies the
// teams_slug_check regex (`^[a-z0-9][a-z0-9-]*$`).
func slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = slugStripper.ReplaceAllString(s, "-")
	s = collapseDashes.ReplaceAllString(s, "-")
	s = slugTrimmer.ReplaceAllString(s, "")
	if len(s) > 48 {
		s = strings.TrimRight(s[:48], "-")
	}
	return s
}

// randomShortToken returns 6 lowercase alphanumeric characters; suitable as
// a slug suffix for disambiguation.
func randomShortToken() (string, error) {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return strings.ToLower(strings.TrimRight(
		base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf[:]),
		"=",
	))[:6], nil
}

// NewInviteToken returns a 32-byte URL-safe random token suitable for use as
// the invite link's `?token=` value.
func NewInviteToken() (string, error) {
	var buf [32]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf[:]), nil
}
