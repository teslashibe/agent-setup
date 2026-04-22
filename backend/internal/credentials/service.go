package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ConnectionSummary is a credential record with the encrypted blob stripped —
// safe to return to the client (settings screen).
type ConnectionSummary struct {
	Platform   string     `json:"platform"`
	Label      string     `json:"label,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// Validator validates a parsed credential payload for a specific platform.
// Implementations live next to the corresponding MCP package registration so
// the agent-setup core stays platform-agnostic.
type Validator interface {
	Platform() string
	Validate(raw json.RawMessage) error
}

// Service combines Store with a Cipher and per-platform Validators to
// provide a transaction-aware credential CRUD API.
type Service struct {
	store      *Store
	cipher     *Cipher
	validators map[string]Validator
}

// NewService constructs a Service. validators may be nil — a missing entry
// for a platform causes Set to skip platform-specific validation but still
// requires that the input parse as valid JSON.
func NewService(store *Store, cipher *Cipher, validators ...Validator) *Service {
	m := map[string]Validator{}
	for _, v := range validators {
		m[v.Platform()] = v
	}
	return &Service{store: store, cipher: cipher, validators: m}
}

// HasCipher reports whether this service is wired with an encryption key.
// Useful at startup to fail fast when a deployment forgets to set the key.
func (s *Service) HasCipher() bool { return s != nil && s.cipher != nil }

// Set encrypts and persists the credential for (userID, platform). raw must
// be valid JSON; if a Validator is registered for the platform, it is also
// invoked.
func (s *Service) Set(ctx context.Context, userID, platform, label string, raw json.RawMessage) (ConnectionSummary, error) {
	platform = strings.TrimSpace(strings.ToLower(platform))
	if platform == "" {
		return ConnectionSummary{}, errors.New("credentials: platform is required")
	}
	if !json.Valid(raw) {
		return ConnectionSummary{}, errors.New("credentials: payload is not valid JSON")
	}
	if v, ok := s.validators[platform]; ok {
		if err := v.Validate(raw); err != nil {
			return ConnectionSummary{}, fmt.Errorf("credentials: validate %s: %w", platform, err)
		}
	}
	if s.cipher == nil {
		return ConnectionSummary{}, errors.New("credentials: cipher not configured (set CREDENTIALS_ENCRYPTION_KEY)")
	}
	ct, err := s.cipher.Seal(raw)
	if err != nil {
		return ConnectionSummary{}, err
	}
	rec, err := s.store.Upsert(ctx, userID, platform, label, ct)
	if err != nil {
		return ConnectionSummary{}, err
	}
	return summary(rec), nil
}

// Decrypted fetches and decrypts the credential for (userID, platform).
// Returns ErrNotFound if no credential exists.
func (s *Service) Decrypted(ctx context.Context, userID, platform string) (json.RawMessage, error) {
	if s.cipher == nil {
		return nil, errors.New("credentials: cipher not configured")
	}
	rec, err := s.store.Get(ctx, userID, platform)
	if err != nil {
		return nil, err
	}
	pt, err := s.cipher.Open(rec.Credential)
	if err != nil {
		return nil, err
	}
	return pt, nil
}

// List returns connection summaries (no encrypted payload).
func (s *Service) List(ctx context.Context, userID string) ([]ConnectionSummary, error) {
	recs, err := s.store.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]ConnectionSummary, len(recs))
	for i, r := range recs {
		out[i] = summary(r)
	}
	return out, nil
}

// Delete removes the credential for (userID, platform).
func (s *Service) Delete(ctx context.Context, userID, platform string) error {
	return s.store.Delete(ctx, userID, platform)
}

// Platforms returns the list of platforms with a registered Validator,
// sorted alphabetically so the settings UI sees a stable order across
// restarts and across instances.
func (s *Service) Platforms() []string {
	out := make([]string, 0, len(s.validators))
	for p := range s.validators {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func summary(r Record) ConnectionSummary {
	return ConnectionSummary{
		Platform:   r.Platform,
		Label:      r.Label,
		CreatedAt:  r.CreatedAt,
		UpdatedAt:  r.UpdatedAt,
		LastUsedAt: r.LastUsedAt,
	}
}
