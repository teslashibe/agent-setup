package uploads

import (
	"bytes"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	svc, err := NewService(Config{
		SigningKey:   "0123456789abcdef0123456789abcdef",
		BaseURL:      "https://chat.example.com",
		Dir:          dir,
		SignedURLTTL: time.Hour,
		RetainFor:    24 * time.Hour,
		MaxBytes:     1024,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func TestSaveAndVerifyAndOpen(t *testing.T) {
	svc := newTestService(t)
	body := []byte("\x89PNG\r\n\x1a\nfake")
	up, err := svc.Save("user-1", "cat.png", "image/png", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if up.ID == "" || up.Size != int64(len(body)) || up.MimeType != "image/png" || up.OriginalName != "cat.png" {
		t.Fatalf("metadata drift: %+v", up)
	}

	signed := svc.SignedURL(up.ID, up.CreatedAt)
	parsed, err := url.Parse(signed)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	q := parsed.Query()
	f, meta, err := svc.VerifyAndOpen(up.ID, q.Get("exp"), q.Get("sig"), up.CreatedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("VerifyAndOpen: %v", err)
	}
	defer f.Close()
	got, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("blob round-trip drift: got %v, want %v", got, body)
	}
	if meta.OwnerUserID != "user-1" {
		t.Errorf("owner drift: %s", meta.OwnerUserID)
	}
}

func TestVerifyExpiredAndBadSig(t *testing.T) {
	svc := newTestService(t)
	up, err := svc.Save("user-1", "x.jpg", "image/jpeg", bytes.NewReader([]byte("hi")))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	parsed, _ := url.Parse(svc.SignedURL(up.ID, up.CreatedAt))
	q := parsed.Query()

	// Past the expiry window — should be ErrExpired even with a valid signature.
	_, _, err = svc.VerifyAndOpen(up.ID, q.Get("exp"), q.Get("sig"), up.CreatedAt.Add(2*time.Hour))
	if !errors.Is(err, ErrExpired) {
		t.Errorf("want ErrExpired, got %v", err)
	}

	// Tamper with the signature.
	_, _, err = svc.VerifyAndOpen(up.ID, q.Get("exp"), "deadbeef", up.CreatedAt)
	if !errors.Is(err, ErrBadSignature) {
		t.Errorf("want ErrBadSignature, got %v", err)
	}

	// Tamper with the exp (so the signature won't match the recomputed one).
	_, _, err = svc.VerifyAndOpen(up.ID, "9999999999", q.Get("sig"), up.CreatedAt)
	if !errors.Is(err, ErrBadSignature) {
		t.Errorf("want ErrBadSignature for tampered exp, got %v", err)
	}
}

func TestSaveRejectsOversize(t *testing.T) {
	svc := newTestService(t) // MaxBytes = 1024
	huge := bytes.Repeat([]byte{'A'}, 2048)
	_, err := svc.Save("user-1", "huge.png", "image/png", bytes.NewReader(huge))
	if !errors.Is(err, ErrTooLarge) {
		t.Errorf("want ErrTooLarge, got %v", err)
	}

	// Confirm the partial blob was cleaned up — only stale entries
	// should be from this and prior tests; count files in svc.cfg.Dir.
	entries, _ := os.ReadDir(svc.cfg.Dir)
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == "" {
			t.Errorf("leftover blob after oversize reject: %s", e.Name())
		}
	}
}

func TestSaveRejectsNonImageMime(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.Save("user-1", "doc.pdf", "application/pdf", bytes.NewReader([]byte("%PDF")))
	if err == nil || !strings.Contains(err.Error(), "not accepted") {
		t.Errorf("want not-accepted error, got %v", err)
	}
}

func TestVerifyMissingBlob(t *testing.T) {
	svc := newTestService(t)
	_, _, err := svc.VerifyAndOpen("never-existed", "9999999999", "deadbeef", time.Now())
	if !errors.Is(err, ErrBadSignature) {
		// Bad-sig is correct because we don't want to leak existence
		// before signature check passes.
		t.Errorf("want ErrBadSignature for unknown id with bad sig, got %v", err)
	}
}
