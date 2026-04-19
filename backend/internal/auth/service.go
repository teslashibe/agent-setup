package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/teslashibe/agent-setup/backend/internal/apperrors"
)

type User struct {
	ID          string    `json:"id"`
	IdentityKey string    `json:"identity_key"`
	Email       string    `json:"email"`
	Name        string    `json:"name"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func (s *Service) GetUser(ctx context.Context, userID string) (User, error) {
	var u User
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, identity_key, email, name, created_at, updated_at
		FROM users WHERE id = $1`, userID,
	).Scan(&u.ID, &u.IdentityKey, &u.Email, &u.Name, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, apperrors.ErrNotFound
		}
		return User{}, err
	}
	return u, nil
}

func (s *Service) GetByEmail(ctx context.Context, email string) (User, error) {
	var u User
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, identity_key, email, name, created_at, updated_at
		FROM users WHERE email = $1`, strings.ToLower(strings.TrimSpace(email)),
	).Scan(&u.ID, &u.IdentityKey, &u.Email, &u.Name, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, apperrors.ErrNotFound
		}
		return User{}, err
	}
	return u, nil
}

func (s *Service) UpsertIdentity(ctx context.Context, identityKey, email, name string) (User, error) {
	cleanEmail := strings.ToLower(strings.TrimSpace(email))
	cleanName := normalizeDisplayName(name, cleanEmail)
	var u User
	err := s.pool.QueryRow(ctx, `
		INSERT INTO users (identity_key, email, name) VALUES ($1, $2, $3)
		ON CONFLICT (identity_key) DO UPDATE
			SET email = EXCLUDED.email, name = EXCLUDED.name, updated_at = NOW()
		RETURNING id::text, identity_key, email, name, created_at, updated_at`,
		strings.TrimSpace(identityKey), cleanEmail, cleanName,
	).Scan(&u.ID, &u.IdentityKey, &u.Email, &u.Name, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}

func normalizeDisplayName(name, email string) string {
	if name = strings.TrimSpace(name); name != "" {
		return name
	}
	if parts := strings.Split(strings.TrimSpace(email), "@"); parts[0] != "" {
		return parts[0]
	}
	return "User"
}
