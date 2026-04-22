package credentials

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned by Store.Get when no credential exists for the
// given (user, platform) pair.
var ErrNotFound = errors.New("credentials: not found")

// Record is a credential row joined with metadata. The Credential field is
// the encrypted blob from the database; callers should pass it through
// Cipher.Open before use, or use Service.Decrypted to combine the two.
type Record struct {
	ID          string
	UserID      string
	Platform    string
	Credential  []byte
	Label       string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	LastUsedAt  *time.Time
}

// Store provides persistence for platform_credentials rows. The encryption
// of the credential blob is the caller's responsibility (see Service for the
// combined transactional API).
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Upsert stores or replaces the credential for (userID, platform). The
// caller must have already encrypted credential. The label is free-form
// (e.g. "Personal LinkedIn") and may be empty.
func (s *Store) Upsert(ctx context.Context, userID, platform, label string, credential []byte) (Record, error) {
	const q = `
		INSERT INTO platform_credentials (user_id, platform, label, credential)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, platform) DO UPDATE
			SET credential = EXCLUDED.credential,
			    label      = EXCLUDED.label,
			    updated_at = NOW()
		RETURNING id, user_id, platform, credential, label, created_at, updated_at, last_used_at`
	row := s.pool.QueryRow(ctx, q, userID, platform, label, credential)
	return scanRecord(row)
}

// Get returns the (encrypted) credential for (userID, platform), or
// ErrNotFound. It also updates last_used_at to NOW().
func (s *Store) Get(ctx context.Context, userID, platform string) (Record, error) {
	const q = `
		UPDATE platform_credentials
		   SET last_used_at = NOW()
		 WHERE user_id = $1 AND platform = $2
		 RETURNING id, user_id, platform, credential, label, created_at, updated_at, last_used_at`
	row := s.pool.QueryRow(ctx, q, userID, platform)
	rec, err := scanRecord(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, fmt.Errorf("%w: user=%s platform=%s", ErrNotFound, userID, platform)
	}
	return rec, err
}

// List returns all credentials for the user (encrypted). Useful for the
// settings screen, which only needs to know which platforms are connected.
func (s *Store) List(ctx context.Context, userID string) ([]Record, error) {
	const q = `
		SELECT id, user_id, platform, credential, label, created_at, updated_at, last_used_at
		  FROM platform_credentials
		 WHERE user_id = $1
		 ORDER BY platform`
	rows, err := s.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Record
	for rows.Next() {
		rec, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// Delete removes the credential for (userID, platform). Returns ErrNotFound
// if no such row existed.
func (s *Store) Delete(ctx context.Context, userID, platform string) error {
	const q = `DELETE FROM platform_credentials WHERE user_id = $1 AND platform = $2`
	tag, err := s.pool.Exec(ctx, q, userID, platform)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: user=%s platform=%s", ErrNotFound, userID, platform)
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanRecord(row rowScanner) (Record, error) {
	var (
		rec  Record
		last *time.Time
	)
	err := row.Scan(&rec.ID, &rec.UserID, &rec.Platform, &rec.Credential, &rec.Label, &rec.CreatedAt, &rec.UpdatedAt, &last)
	if err != nil {
		return Record{}, err
	}
	rec.LastUsedAt = last
	return rec, nil
}
