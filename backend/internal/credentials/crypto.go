// Package credentials manages per-user, per-platform authentication blobs
// for MCP-exposed services.
//
// Credentials live in the platform_credentials table, encrypted at rest with
// AES-GCM using the key in CREDENTIALS_ENCRYPTION_KEY (32 bytes; supplied as
// 64-char hex or 44-char base64). One row per (user, platform); reconnecting
// the same platform replaces the existing row.
package credentials

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

// Cipher encrypts and decrypts opaque credential blobs with AES-256-GCM.
// Construct via NewCipher; the zero value is invalid.
type Cipher struct {
	aead cipher.AEAD
}

// NewCipher parses a 32-byte AES-256 key supplied as either a 64-char hex
// string or a base64 (standard or URL, with or without padding) string and
// returns a ready-to-use Cipher.
func NewCipher(key string) (*Cipher, error) {
	raw, err := decodeKey(key)
	if err != nil {
		return nil, err
	}
	if len(raw) != 32 {
		return nil, fmt.Errorf("credentials: key must be 32 bytes; got %d", len(raw))
	}
	block, err := aes.NewCipher(raw)
	if err != nil {
		return nil, fmt.Errorf("credentials: aes.NewCipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("credentials: cipher.NewGCM: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// Seal encrypts plaintext with a fresh random nonce and returns nonce||ciphertext.
func (c *Cipher) Seal(plaintext []byte) ([]byte, error) {
	if c == nil || c.aead == nil {
		return nil, errors.New("credentials: nil cipher")
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("credentials: nonce: %w", err)
	}
	ct := c.aead.Seal(nil, nonce, plaintext, nil)
	out := make([]byte, 0, len(nonce)+len(ct))
	out = append(out, nonce...)
	out = append(out, ct...)
	return out, nil
}

// Open decrypts payload (nonce||ciphertext) and returns the plaintext.
func (c *Cipher) Open(payload []byte) ([]byte, error) {
	if c == nil || c.aead == nil {
		return nil, errors.New("credentials: nil cipher")
	}
	ns := c.aead.NonceSize()
	if len(payload) < ns+c.aead.Overhead() {
		return nil, errors.New("credentials: payload too short")
	}
	nonce, ct := payload[:ns], payload[ns:]
	pt, err := c.aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("credentials: decrypt: %w", err)
	}
	return pt, nil
}

func decodeKey(key string) ([]byte, error) {
	if len(key) == 0 {
		return nil, errors.New("credentials: CREDENTIALS_ENCRYPTION_KEY is empty")
	}
	if raw, err := hex.DecodeString(key); err == nil && len(raw) == 32 {
		return raw, nil
	}
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding,
		base64.RawStdEncoding,
		base64.URLEncoding,
		base64.RawURLEncoding,
	} {
		if raw, err := enc.DecodeString(key); err == nil {
			return raw, nil
		}
	}
	return nil, errors.New("credentials: key must be 64 hex chars or base64-encoded 32 bytes")
}
