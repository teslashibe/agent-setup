package agent

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/teslashibe/agent-setup/backend/internal/apperrors"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) CreateSession(ctx context.Context, userID, title, anthropicSessionID string) (Session, error) {
	const q = `
		INSERT INTO agent_sessions (user_id, title, anthropic_session_id)
		VALUES ($1, $2, $3)
		RETURNING id::text, user_id::text, title, anthropic_session_id, created_at, updated_at
	`
	var sess Session
	err := s.pool.QueryRow(ctx, q, userID, strings.TrimSpace(title), anthropicSessionID).Scan(
		&sess.ID, &sess.UserID, &sess.Title, &sess.AnthropicSessionID, &sess.CreatedAt, &sess.UpdatedAt,
	)
	return sess, err
}

func (s *Store) GetSession(ctx context.Context, userID, sessionID string) (Session, error) {
	const q = `
		SELECT id::text, user_id::text, title, anthropic_session_id, created_at, updated_at
		FROM agent_sessions
		WHERE id = $1 AND user_id = $2
	`
	var sess Session
	err := s.pool.QueryRow(ctx, q, sessionID, userID).Scan(
		&sess.ID, &sess.UserID, &sess.Title, &sess.AnthropicSessionID, &sess.CreatedAt, &sess.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Session{}, apperrors.ErrNotFound
		}
		return Session{}, err
	}
	return sess, nil
}

func (s *Store) ListSessions(ctx context.Context, userID string) ([]Session, error) {
	const q = `
		SELECT id::text, user_id::text, title, anthropic_session_id, created_at, updated_at
		FROM agent_sessions
		WHERE user_id = $1
		ORDER BY updated_at DESC
		LIMIT 100
	`
	rows, err := s.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Session
	for rows.Next() {
		var sess Session
		if err := rows.Scan(
			&sess.ID, &sess.UserID, &sess.Title, &sess.AnthropicSessionID, &sess.CreatedAt, &sess.UpdatedAt,
		); err != nil {
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
