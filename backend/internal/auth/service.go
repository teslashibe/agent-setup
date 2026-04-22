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

const userFields = `id::text, identity_key, email, name, created_at, updated_at`

type User struct {
	ID          string    `json:"id"`
	IdentityKey string    `json:"identity_key"`
	Email       string    `json:"email"`
	Name        string    `json:"name"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// UpsertResult adds an "is this row brand new?" signal so callers can wire
// first-login side-effects (e.g. bootstrap a personal team).
type UpsertResult struct {
	User    User
	IsNewly bool
}

type Service struct{ pool *pgxpool.Pool }

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

type scanner interface {
	Scan(dest ...any) error
}

func scanUser(row scanner) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.IdentityKey, &u.Email, &u.Name, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}

func (s *Service) selectUserBy(ctx context.Context, column, value string) (User, error) {
	user, err := scanUser(s.pool.QueryRow(ctx,
		`SELECT `+userFields+` FROM users WHERE `+column+` = $1`, value,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, apperrors.ErrNotFound
	}
	return user, err
}

func (s *Service) GetUser(ctx context.Context, userID string) (User, error) {
	return s.selectUserBy(ctx, "id", userID)
}

func (s *Service) GetByEmail(ctx context.Context, email string) (User, error) {
	return s.selectUserBy(ctx, "email", strings.ToLower(strings.TrimSpace(email)))
}

// UpsertIdentity inserts or updates the user row keyed on identity_key. The
// returned IsNewly flag uses Postgres's `xmax = 0` trick to detect "this row
// was just inserted" — we only get xmax > 0 when ON CONFLICT fired UPDATE.
func (s *Service) UpsertIdentity(ctx context.Context, identityKey, email, name string) (UpsertResult, error) {
	cleanEmail := strings.ToLower(strings.TrimSpace(email))
	row := s.pool.QueryRow(ctx, `
		INSERT INTO users (identity_key, email, name) VALUES ($1, $2, $3)
		ON CONFLICT (identity_key) DO UPDATE
			SET email = EXCLUDED.email, name = EXCLUDED.name, updated_at = NOW()
		RETURNING `+userFields+`, xmax = 0 AS inserted`,
		strings.TrimSpace(identityKey), cleanEmail, displayName(name, cleanEmail),
	)
	var (
		u       User
		isNewly bool
	)
	if err := row.Scan(&u.ID, &u.IdentityKey, &u.Email, &u.Name, &u.CreatedAt, &u.UpdatedAt, &isNewly); err != nil {
		return UpsertResult{}, err
	}
	return UpsertResult{User: u, IsNewly: isNewly}, nil
}

func displayName(name, email string) string {
	if name = strings.TrimSpace(name); name != "" {
		return name
	}
	if local := strings.SplitN(email, "@", 2)[0]; local != "" {
		return local
	}
	return "User"
}
