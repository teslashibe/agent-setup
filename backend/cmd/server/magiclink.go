package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	magiclink "github.com/teslashibe/magiclink-auth-go"
	"github.com/teslashibe/magiclink-auth-go/resend"

	"github.com/teslashibe/agent-setup/backend/internal/auth"
	"github.com/teslashibe/agent-setup/backend/internal/config"
	"github.com/teslashibe/agent-setup/backend/internal/teams"
)

func newMagicLinkService(cfg config.Config, pool *pgxpool.Pool, authSvc *auth.Service, teamsSvc *teams.Service) (*magiclink.Service, *codeStoreAdapter, error) {
	magicCfg := magiclink.Config{
		JWTSecret:   cfg.JWTSecret,
		AppURL:      cfg.AppURL,
		AppName:     "Claude Agent Go",
		FromAddress: cfg.AuthEmailFrom,
		CodeTTL:     10 * time.Minute,
		TokenTTL:    30 * 24 * time.Hour,
		CodeLength:  6,
	}
	if cfg.MobileAppScheme != "" {
		magicCfg.DeepLinkURL = fmt.Sprintf("%s://auth", cfg.MobileAppScheme)
	}

	if err := magicCfg.Validate(); err != nil {
		return nil, nil, fmt.Errorf("invalid magiclink config: %w", err)
	}

	codeStore := newCodeStoreAdapter(pool)
	userStore := &userStoreAdapter{authSvc: authSvc, teamsSvc: teamsSvc}

	var sender magiclink.EmailSender = &devEmailSender{}
	if strings.TrimSpace(cfg.ResendAPIKey) != "" {
		sender = resend.New(cfg.ResendAPIKey, cfg.AuthEmailFrom)
	}

	return magiclink.New(magicCfg, codeStore, userStore, sender, nil), codeStore, nil
}

// codeStoreAdapter persists magic-link codes/tokens and (optionally) the
// invite_token tying this attempt to a pending team invite. The invite token
// is stashed in-memory keyed by lower-cased email and consumed by the next
// Create call — this keeps the magiclink.CodeStore interface untouched while
// still threading our extra payload through.
type codeStoreAdapter struct {
	pool           *pgxpool.Pool
	pendingInvites sync.Map // map[string]string (email → invite_token)
}

func newCodeStoreAdapter(pool *pgxpool.Pool) *codeStoreAdapter {
	return &codeStoreAdapter{pool: pool}
}

// SetPendingInvite records an invite_token to attach to the next Create call
// for the given email. Calling with an empty inviteToken clears any pending
// value, preventing leaks across login attempts.
func (s *codeStoreAdapter) SetPendingInvite(email, inviteToken string) {
	email = normalizeEmail(email)
	if email == "" {
		return
	}
	if strings.TrimSpace(inviteToken) == "" {
		s.pendingInvites.Delete(email)
		return
	}
	s.pendingInvites.Store(email, strings.TrimSpace(inviteToken))
}

// LookupInviteByToken returns the invite_token recorded against the row whose
// magic-link `token` matches. Empty string when no invite was attached.
func (s *codeStoreAdapter) LookupInviteByToken(ctx context.Context, magicToken string) (string, error) {
	var invite *string
	err := s.pool.QueryRow(ctx, `
		SELECT invite_token FROM auth_codes
		WHERE token = $1
		LIMIT 1`,
		strings.TrimSpace(magicToken),
	).Scan(&invite)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	if invite == nil {
		return "", nil
	}
	return *invite, nil
}

// LookupInviteByEmail returns the invite_token from the most recent auth_code
// for the given email, prioritising rows that were just consumed (so /verify
// can inspect them after VerifyCode succeeded).
func (s *codeStoreAdapter) LookupInviteByEmail(ctx context.Context, email string) (string, error) {
	var invite *string
	err := s.pool.QueryRow(ctx, `
		SELECT invite_token FROM auth_codes
		WHERE email = $1
		ORDER BY created_at DESC
		LIMIT 1`,
		normalizeEmail(email),
	).Scan(&invite)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	if invite == nil {
		return "", nil
	}
	return *invite, nil
}

func (s *codeStoreAdapter) Create(ctx context.Context, email, code, token string, expiresAt time.Time) error {
	email = normalizeEmail(email)

	var invitePtr *string
	if v, ok := s.pendingInvites.LoadAndDelete(email); ok {
		if str, _ := v.(string); str != "" {
			invitePtr = &str
		}
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO auth_codes (email, code, token, expires_at, invite_token)
		VALUES ($1, $2, $3, $4, $5)`,
		email, code, token, expiresAt, invitePtr,
	)
	return err
}

func (s *codeStoreAdapter) ConsumeByCode(ctx context.Context, email, code string) error {
	const lookupQuery = `
		SELECT id, used, expires_at
		FROM auth_codes
		WHERE email = $1 AND code = $2
		ORDER BY created_at DESC
		LIMIT 1
	`

	var (
		id        string
		used      bool
		expiresAt time.Time
	)
	err := s.pool.QueryRow(ctx, lookupQuery, normalizeEmail(email), strings.TrimSpace(code)).Scan(&id, &used, &expiresAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return magiclink.ErrInvalidCode
		}
		return err
	}

	if used {
		return magiclink.ErrCodeAlreadyUsed
	}
	if time.Now().After(expiresAt) {
		return magiclink.ErrExpiredCode
	}

	const consumeQuery = `
		UPDATE auth_codes
		SET used = TRUE, used_at = NOW()
		WHERE id = $1 AND used = FALSE
	`
	cmd, err := s.pool.Exec(ctx, consumeQuery, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return magiclink.ErrCodeAlreadyUsed
	}
	return nil
}

func (s *codeStoreAdapter) LookupByToken(ctx context.Context, token string) (string, string, error) {
	const query = `
		SELECT email, code, used, expires_at
		FROM auth_codes
		WHERE token = $1
		LIMIT 1
	`

	var (
		email     string
		code      string
		used      bool
		expiresAt time.Time
	)

	err := s.pool.QueryRow(ctx, query, strings.TrimSpace(token)).Scan(&email, &code, &used, &expiresAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", "", magiclink.ErrInvalidToken
		}
		return "", "", err
	}

	if used {
		return "", "", magiclink.ErrTokenAlreadyUsed
	}
	if time.Now().After(expiresAt) {
		return "", "", magiclink.ErrExpiredToken
	}

	return email, code, nil
}

type userStoreAdapter struct {
	authSvc  *auth.Service
	teamsSvc *teams.Service
}

func (s *userStoreAdapter) UpsertUser(ctx context.Context, identityKey, email, displayName string) (string, error) {
	res, err := s.authSvc.UpsertIdentity(ctx, identityKey, email, displayName)
	if err != nil {
		return "", err
	}
	if _, err := s.teamsSvc.EnsurePersonalTeam(ctx, res.User.ID, res.User.Name, res.User.Email); err != nil {
		// Bootstrap failure is fatal — without a personal team the user has
		// nowhere to land. Surface it so the auth flow short-circuits cleanly.
		return "", fmt.Errorf("bootstrap personal team: %w", err)
	}
	return res.User.ID, nil
}

func (s *userStoreAdapter) GetUserByEmail(ctx context.Context, email string) (string, string, error) {
	user, err := s.authSvc.GetByEmail(ctx, email)
	if err != nil {
		return "", "", err
	}
	return user.ID, user.Name, nil
}

type devEmailSender struct{}

func (s *devEmailSender) Send(_ context.Context, to, subject, htmlBody string) error {
	log.Printf("[magiclink-dev-email] to=%s subject=%q body=%q", to, subject, htmlBody)
	return nil
}

func normalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
