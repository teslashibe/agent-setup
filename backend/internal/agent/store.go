package agent

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/teslashibe/agent-setup/backend/internal/apperrors"
)

const sessionFields = `id::text, user_id::text, title, anthropic_session_id, created_at, updated_at`

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

type scanner interface {
	Scan(dest ...any) error
}

func scanSession(row scanner) (Session, error) {
	var s Session
	err := row.Scan(&s.ID, &s.UserID, &s.Title, &s.AnthropicSessionID, &s.CreatedAt, &s.UpdatedAt)
	return s, err
}

func (s *Store) CreateSession(ctx context.Context, userID, title, anthropicSessionID string) (Session, error) {
	return scanSession(s.pool.QueryRow(ctx, `
		INSERT INTO agent_sessions (user_id, title, anthropic_session_id)
		VALUES ($1, $2, $3)
		RETURNING `+sessionFields,
		userID, strings.TrimSpace(title), anthropicSessionID,
	))
}

func (s *Store) GetSession(ctx context.Context, userID, sessionID string) (Session, error) {
	sess, err := scanSession(s.pool.QueryRow(ctx,
		`SELECT `+sessionFields+` FROM agent_sessions WHERE id = $1 AND user_id = $2`,
		sessionID, userID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, apperrors.ErrNotFound
	}
	return sess, err
}

func (s *Store) ListSessions(ctx context.Context, userID string) ([]Session, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+sessionFields+` FROM agent_sessions WHERE user_id = $1 ORDER BY updated_at DESC LIMIT 100`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Session{}
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

func (s *Store) UpdateTitle(ctx context.Context, sessionID, title string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE agent_sessions SET title = $1, updated_at = NOW() WHERE id = $2`,
		title, sessionID,
	)
	return err
}

func (s *Store) DeleteSession(ctx context.Context, userID, sessionID string) error {
	cmd, err := s.pool.Exec(ctx,
		`DELETE FROM agent_sessions WHERE id = $1 AND user_id = $2`,
		sessionID, userID,
	)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}
