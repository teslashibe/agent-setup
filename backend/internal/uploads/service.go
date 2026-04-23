// Package uploads stores chat-attached binaries (currently just images
// for image-style platform posts) and serves them back over a signed,
// short-lived URL.
//
// The flow:
//
//  1. Mobile client POSTs a multipart file to /api/uploads (auth
//     required). Service.Save writes the bytes to disk and returns
//     a stable id + signed URL.
//  2. Mobile shows the image inline in the chat bubble using the
//     signed URL (works in <Image> without needing the user JWT —
//     the signature stands in).
//  3. The agent quotes that URL into its outgoing tool call (e.g.
//     reddit_submit_image). The MCP tool fetches the URL, which
//     hits GET /api/uploads/:id?sig=...&exp=..., verifies the
//     signature, and streams the bytes back.
//
// Signed URLs are HMAC-SHA256 over `id|exp` using a key derived from
// CREDENTIALS_ENCRYPTION_KEY. We deliberately reuse that key (vs.
// adding a new env var) because: (a) it's already required for the
// MCP routes the uploads endpoint feeds into, (b) anyone with that
// key has full access to the credential store anyway, so leaking
// signed-URL forgery doesn't expand the blast radius.
//
// Storage is plain disk under cfg.Dir (default $TMPDIR/agent-uploads).
// A periodic janitor goroutine deletes files older than cfg.RetainFor
// — separate from the signed-URL TTL because we want some grace
// period after a URL expires, both for debugging and for the (rare)
// case where the agent races us between minting the URL and using it.
package uploads

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// MaxUploadBytes caps a single upload at 10 MiB. The downstream
// platform clients (e.g. Reddit's image post endpoint) accept up to
// 20 MiB, but capping lower keeps chat-side memory pressure
// manageable and gives an obvious error before the operator wastes
// time on an upload that would fail at the platform step anyway.
const MaxUploadBytes int64 = 10 * 1024 * 1024

// Common MIME types we accept. Not strictly enforced here (we let the
// Save call accept any image/* type), but documented for callers.
var defaultAcceptedMimePrefixes = []string{"image/"}

// Config tunes a Service. All fields have sane zero-value fallbacks
// applied in NewService — the only required field is SigningKey.
type Config struct {
	// SigningKey is the secret HMAC key used to sign download URLs.
	// Required. Pass the same string the credentials cipher uses
	// (cfg.CredentialsEncryptionKey) so we don't introduce a new
	// secret to rotate.
	SigningKey string

	// BaseURL is the externally-reachable origin the signed URL is
	// rooted at, e.g. https://app.example.com. The path
	// /api/uploads/{id}?... is appended. Required.
	BaseURL string

	// Dir is the on-disk directory uploads live in. Defaults to
	// $TMPDIR/agent-uploads. Created on demand with mode 0700.
	Dir string

	// SignedURLTTL is how long a freshly-minted signed URL stays
	// valid. Defaults to 1h — long enough to survive a chat round
	// trip plus a few user re-reads, short enough that a leaked
	// URL ages out before it can be widely shared.
	SignedURLTTL time.Duration

	// RetainFor is how long uploads stay on disk before the janitor
	// deletes them. Must be >= SignedURLTTL. Defaults to 24h.
	RetainFor time.Duration

	// AcceptedMimePrefixes restricts the set of mimetypes Save will
	// accept. Defaults to image/* only — we don't currently have a
	// feature that needs to upload anything else.
	AcceptedMimePrefixes []string

	// MaxBytes caps a single upload. Defaults to MaxUploadBytes.
	MaxBytes int64
}

// Upload is the metadata for a stored upload. Persisted alongside the
// blob bytes as <id>.json so we can render Content-Type on the GET
// without sniffing.
type Upload struct {
	ID           string    `json:"id"`
	OwnerUserID  string    `json:"owner_user_id"`
	OriginalName string    `json:"original_name"`
	MimeType     string    `json:"mime_type"`
	Size         int64     `json:"size"`
	CreatedAt    time.Time `json:"created_at"`
}

// Service owns disk storage and signed-URL minting/verification.
type Service struct {
	cfg     Config
	keyHash []byte
}

// NewService constructs a Service from cfg. It creates cfg.Dir if it
// doesn't exist and starts the janitor goroutine.
func NewService(cfg Config) (*Service, error) {
	if strings.TrimSpace(cfg.SigningKey) == "" {
		return nil, errors.New("uploads: SigningKey is required")
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, errors.New("uploads: BaseURL is required")
	}
	if cfg.Dir == "" {
		cfg.Dir = filepath.Join(os.TempDir(), "agent-uploads")
	}
	if cfg.SignedURLTTL <= 0 {
		cfg.SignedURLTTL = time.Hour
	}
	if cfg.RetainFor <= 0 {
		cfg.RetainFor = 24 * time.Hour
	}
	if cfg.RetainFor < cfg.SignedURLTTL {
		return nil, fmt.Errorf("uploads: RetainFor (%s) must be >= SignedURLTTL (%s)", cfg.RetainFor, cfg.SignedURLTTL)
	}
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = MaxUploadBytes
	}
	if len(cfg.AcceptedMimePrefixes) == 0 {
		cfg.AcceptedMimePrefixes = defaultAcceptedMimePrefixes
	}
	if err := os.MkdirAll(cfg.Dir, 0o700); err != nil {
		return nil, fmt.Errorf("uploads: create dir %s: %w", cfg.Dir, err)
	}
	// Hash the signing key once so HMAC operations don't pay the
	// string-decode cost per request, and so we never log or expose
	// the raw key by accident.
	sum := sha256.Sum256([]byte(cfg.SigningKey))
	return &Service{cfg: cfg, keyHash: sum[:]}, nil
}

// Save persists the upload bytes to disk and returns the metadata
// (with a freshly-generated id). The reader is consumed up to
// cfg.MaxBytes; anything beyond returns ErrTooLarge.
func (s *Service) Save(ownerUserID, originalName, mime string, r io.Reader) (*Upload, error) {
	if ownerUserID == "" {
		return nil, errors.New("uploads: ownerUserID is required")
	}
	mime = strings.TrimSpace(strings.SplitN(mime, ";", 2)[0])
	if mime == "" {
		return nil, errors.New("uploads: mime is required")
	}
	if !s.acceptsMime(mime) {
		return nil, fmt.Errorf("uploads: mime %q not accepted (need one of %v)", mime, s.cfg.AcceptedMimePrefixes)
	}

	id, err := newID()
	if err != nil {
		return nil, err
	}
	blobPath := s.blobPath(id)
	metaPath := s.metaPath(id)

	f, err := os.OpenFile(blobPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("uploads: create blob: %w", err)
	}
	// LimitReader caps cleanly; we then peek a one-byte read after to
	// detect overflow so the operator gets a clear "too large" error
	// instead of a silent truncation.
	limited := io.LimitReader(r, s.cfg.MaxBytes)
	written, copyErr := io.Copy(f, limited)
	if copyErr != nil {
		_ = f.Close()
		_ = os.Remove(blobPath)
		return nil, fmt.Errorf("uploads: write blob: %w", copyErr)
	}
	// Detect overflow: if we hit cfg.MaxBytes and there are still
	// bytes available on r, the source was bigger than the cap.
	if written == s.cfg.MaxBytes {
		probe := make([]byte, 1)
		n, _ := r.Read(probe)
		if n > 0 {
			_ = f.Close()
			_ = os.Remove(blobPath)
			return nil, ErrTooLarge
		}
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(blobPath)
		return nil, fmt.Errorf("uploads: close blob: %w", err)
	}

	up := &Upload{
		ID:           id,
		OwnerUserID:  ownerUserID,
		OriginalName: trimName(originalName),
		MimeType:     mime,
		Size:         written,
		CreatedAt:    time.Now().UTC(),
	}
	metaBytes, err := json.Marshal(up)
	if err != nil {
		_ = os.Remove(blobPath)
		return nil, fmt.Errorf("uploads: marshal meta: %w", err)
	}
	if err := os.WriteFile(metaPath, metaBytes, 0o600); err != nil {
		_ = os.Remove(blobPath)
		return nil, fmt.Errorf("uploads: write meta: %w", err)
	}
	return up, nil
}

// SignedURL builds the externally-shareable URL for `id`, valid for
// cfg.SignedURLTTL from `now`.
func (s *Service) SignedURL(id string, now time.Time) string {
	exp := now.Add(s.cfg.SignedURLTTL).Unix()
	sig := s.sign(id, exp)
	q := url.Values{}
	q.Set("exp", strconv.FormatInt(exp, 10))
	q.Set("sig", sig)
	return strings.TrimRight(s.cfg.BaseURL, "/") + "/api/uploads/" + url.PathEscape(id) + "?" + q.Encode()
}

// VerifyAndOpen validates the signed URL parameters and returns an
// open file handle + metadata. Caller must Close the returned file.
//
// Time comparisons use the wall clock; signed URLs that have expired
// return ErrExpired. Bad/missing signatures return ErrBadSignature
// without distinguishing them from each other (no oracle).
func (s *Service) VerifyAndOpen(id, expRaw, sig string, now time.Time) (*os.File, *Upload, error) {
	if id == "" {
		return nil, nil, ErrNotFound
	}
	exp, err := strconv.ParseInt(expRaw, 10, 64)
	if err != nil {
		return nil, nil, ErrBadSignature
	}
	want := s.sign(id, exp)
	if !hmac.Equal([]byte(want), []byte(sig)) {
		return nil, nil, ErrBadSignature
	}
	if now.Unix() > exp {
		return nil, nil, ErrExpired
	}

	meta, err := s.readMeta(id)
	if err != nil {
		return nil, nil, err
	}
	f, err := os.Open(s.blobPath(id))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, fmt.Errorf("uploads: open blob: %w", err)
	}
	return f, meta, nil
}

// StartJanitor runs the cleanup goroutine until ctx is cancelled or
// stopCh closed. Pass a stopCh from the server's shutdown handler so
// the process can exit cleanly. Safe to call once.
func (s *Service) StartJanitor(stopCh <-chan struct{}) {
	go func() {
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		s.sweepOnce()
		for {
			select {
			case <-stopCh:
				return
			case <-t.C:
				s.sweepOnce()
			}
		}
	}()
}

// sweepOnce deletes any blob+meta pair older than cfg.RetainFor.
// Errors are ignored intentionally — the next sweep retries, and we
// don't want a stuck file to wedge the goroutine.
func (s *Service) sweepOnce() {
	cutoff := time.Now().Add(-s.cfg.RetainFor)
	entries, err := os.ReadDir(s.cfg.Dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(s.cfg.Dir, e.Name()))
		}
	}
}

func (s *Service) sign(id string, exp int64) string {
	mac := hmac.New(sha256.New, s.keyHash)
	mac.Write([]byte(id))
	mac.Write([]byte{':'})
	mac.Write([]byte(strconv.FormatInt(exp, 10)))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Service) acceptsMime(mime string) bool {
	for _, prefix := range s.cfg.AcceptedMimePrefixes {
		if strings.HasPrefix(mime, prefix) {
			return true
		}
	}
	return false
}

func (s *Service) readMeta(id string) (*Upload, error) {
	data, err := os.ReadFile(s.metaPath(id))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("uploads: read meta: %w", err)
	}
	var up Upload
	if err := json.Unmarshal(data, &up); err != nil {
		return nil, fmt.Errorf("uploads: parse meta: %w", err)
	}
	return &up, nil
}

func (s *Service) blobPath(id string) string { return filepath.Join(s.cfg.Dir, id) }
func (s *Service) metaPath(id string) string { return filepath.Join(s.cfg.Dir, id+".json") }

// Sentinel errors callers can use with errors.Is.
var (
	ErrNotFound     = errors.New("uploads: not found")
	ErrBadSignature = errors.New("uploads: bad signature")
	ErrExpired      = errors.New("uploads: expired")
	ErrTooLarge     = errors.New("uploads: too large")
)

// newID generates an unguessable 16-byte hex string. We don't use
// UUIDs because the colliding namespace + version digit doesn't add
// anything for a one-hour-TTL signed-URL store.
func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("uploads: generate id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// trimName drops directory components and over-long basenames so we
// don't write `Original-Name: ../../etc/passwd` into the metadata
// file (the metadata isn't passed to a shell, but it does end up in
// the chat-rendered JSON, and a sanitized name is friendlier).
func trimName(s string) string {
	s = filepath.Base(strings.TrimSpace(s))
	if s == "" || s == "." || s == "/" {
		return ""
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}
