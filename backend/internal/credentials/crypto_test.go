package credentials_test

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"testing"

	"github.com/teslashibe/agent-setup/backend/internal/credentials"
)

func newKey(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		t.Fatal(err)
	}
	return k
}

func TestCipher_RoundTrip_Hex(t *testing.T) {
	t.Parallel()
	k := newKey(t)
	c, err := credentials.NewCipher(hex.EncodeToString(k))
	if err != nil {
		t.Fatal(err)
	}
	pt := []byte(`{"li_at":"AQEDA...","csrf":"ajax:..."}`)
	ct, err := c.Seal(pt)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Open(ct)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, pt) {
		t.Errorf("round trip mismatch")
	}
}

func TestCipher_RoundTrip_Base64Variants(t *testing.T) {
	t.Parallel()
	k := newKey(t)
	encs := map[string]*base64.Encoding{
		"std":     base64.StdEncoding,
		"raw_std": base64.RawStdEncoding,
		"url":     base64.URLEncoding,
		"raw_url": base64.RawURLEncoding,
	}
	for name, enc := range encs {
		t.Run(name, func(t *testing.T) {
			c, err := credentials.NewCipher(enc.EncodeToString(k))
			if err != nil {
				t.Fatalf("NewCipher: %v", err)
			}
			ct, err := c.Seal([]byte("hi"))
			if err != nil {
				t.Fatal(err)
			}
			pt, err := c.Open(ct)
			if err != nil {
				t.Fatal(err)
			}
			if string(pt) != "hi" {
				t.Errorf("got %q", pt)
			}
		})
	}
}

func TestCipher_RandomNonce(t *testing.T) {
	t.Parallel()
	c, _ := credentials.NewCipher(hex.EncodeToString(newKey(t)))
	a, _ := c.Seal([]byte("same"))
	b, _ := c.Seal([]byte("same"))
	if bytes.Equal(a, b) {
		t.Error("two seals of identical plaintext produced identical ciphertext (nonce reuse)")
	}
}

func TestCipher_TamperedCiphertextFails(t *testing.T) {
	t.Parallel()
	c, _ := credentials.NewCipher(hex.EncodeToString(newKey(t)))
	ct, _ := c.Seal([]byte("payload"))
	ct[len(ct)-1] ^= 0x01
	if _, err := c.Open(ct); err == nil {
		t.Error("expected error on tampered ciphertext")
	}
}

func TestNewCipher_RejectsBadKey(t *testing.T) {
	t.Parallel()
	cases := []string{
		"",
		"not hex or base64 !!! @@@ ###",
		hex.EncodeToString(make([]byte, 16)),
	}
	for _, k := range cases {
		if _, err := credentials.NewCipher(k); err == nil {
			t.Errorf("NewCipher(%q) should have errored", k)
		}
	}
}

func TestCipher_OpenRejectsShortPayload(t *testing.T) {
	t.Parallel()
	c, _ := credentials.NewCipher(hex.EncodeToString(newKey(t)))
	if _, err := c.Open([]byte("short")); err == nil {
		t.Error("expected error on short payload")
	}
}
