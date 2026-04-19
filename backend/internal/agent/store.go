package agent

import (
	"context"
	"encoding/json"
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

func (s *Store) CreateSession(ctx context.Context, userID, title string, systemPrompt, model *string) (Session, error) {
	const query = `
		INSERT INTO agent_sessions (user_id, title, system_prompt, model)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text, user_id::text, title, system_prompt, model, metadata, created_at, updated_at
	`
	var sess Session
	err := s.pool.QueryRow(ctx, query, userID, strings.TrimSpace(title), systemPrompt, model).Scan(
		&sess.ID,
		&sess.UserID,
		&sess.Title,
		&sess.SystemPrompt,
		&sess.Model,
		&sess.Metadata,
		&sess.CreatedAt,
		&sess.UpdatedAt,
	)
	if err != nil {
		return Session{}, err
	}
	return sess, nil
}

func (s *Store) GetSession(ctx context.Context, userID, sessionID string) (Session, error) {
	const query = `
		SELECT id::text, user_id::text, title, system_prompt, model, metadata, created_at, updated_at
		FROM agent_sessions
		WHERE id = $1 AND user_id = $2
	`
	var sess Session
	err := s.pool.QueryRow(ctx, query, sessionID, userID).Scan(
		&sess.ID,
		&sess.UserID,
		&sess.Title,
		&sess.SystemPrompt,
		&sess.Model,
		&sess.Metadata,
		&sess.CreatedAt,
		&sess.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Session{}, apperrors.ErrNotFound
		}
		return Session{}, err
	}
	return sess, nil
}

func (s *Store) ListSessions(ctx context.Context, userID string, limit int) ([]Session, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	const query = `
		SELECT id::text, user_id::text, title, system_prompt, model, metadata, created_at, updated_at
		FROM agent_sessions
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`
	rows, err := s.pool.Query(ctx, query, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Session
	for rows.Next() {
		var sess Session
		if err := rows.Scan(
			&sess.ID,
			&sess.UserID,
			&sess.Title,
			&sess.SystemPrompt,
			&sess.Model,
			&sess.Metadata,
			&sess.CreatedAt,
			&sess.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

func (s *Store) TouchSession(ctx context.Context, sessionID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE agent_sessions SET updated_at = NOW() WHERE id = $1`, sessionID)
	return err
}

// AppendMessage stores a single conversation turn.
// content must be a JSON-encoded array of Anthropic content blocks.
func (s *Store) AppendMessage(ctx context.Context, sessionID, role string, content json.RawMessage, stopReason *string, usage *Usage) (Message, error) {
	const query = `
		INSERT INTO agent_messages (session_id, role, content, stop_reason, input_tokens, output_tokens)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id::text, session_id::text, role, content, stop_reason, input_tokens, output_tokens, created_at
	`
	var (
		inTok  *int
		outTok *int
	)
	if usage != nil {
		i, o := usage.InputTokens, usage.OutputTokens
		inTok = &i
		outTok = &o
	}

	var m Message
	err := s.pool.QueryRow(ctx, query, sessionID, role, content, stopReason, inTok, outTok).Scan(
		&m.ID,
		&m.SessionID,
		&m.Role,
		&m.Content,
		&m.StopReason,
		&m.InputTokens,
		&m.OutputTokens,
		&m.CreatedAt,
	)
	if err != nil {
		return Message{}, err
	}
	return m, nil
}

func (s *Store) ListMessages(ctx context.Context, sessionID string) ([]Message, error) {
	const query = `
		SELECT id::text, session_id::text, role, content, stop_reason, input_tokens, output_tokens, created_at
		FROM agent_messages
		WHERE session_id = $1
		ORDER BY created_at ASC, id ASC
	`
	rows, err := s.pool.Query(ctx, query, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(
			&m.ID,
			&m.SessionID,
			&m.Role,
			&m.Content,
			&m.StopReason,
			&m.InputTokens,
			&m.OutputTokens,
			&m.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
