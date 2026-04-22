package agent

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/teslashibe/agent-setup/backend/internal/apperrors"
)

const sessionFields = `id::text, team_id::text, user_id::text, title, anthropic_session_id, created_at, updated_at`

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

type scanner interface {
	Scan(dest ...any) error
}

func scanSession(row scanner) (Session, error) {
	var s Session
	err := row.Scan(&s.ID, &s.TeamID, &s.UserID, &s.Title, &s.AnthropicSessionID, &s.CreatedAt, &s.UpdatedAt)
	return s, err
}

func (s *Store) CreateSession(ctx context.Context, teamID, userID, title, anthropicSessionID string) (Session, error) {
	return scanSession(s.pool.QueryRow(ctx, `
		INSERT INTO agent_sessions (team_id, user_id, title, anthropic_session_id)
		VALUES ($1, $2, $3, $4)
		RETURNING `+sessionFields,
		teamID, userID, strings.TrimSpace(title), anthropicSessionID,
	))
}

// GetSessionInTeam fetches a session by id, but only if it belongs to teamID.
// Cross-team access returns ErrNotFound (rather than ErrForbidden) so we don't
// leak the existence of sessions in other teams.
func (s *Store) GetSessionInTeam(ctx context.Context, teamID, sessionID string) (Session, error) {
	sess, err := scanSession(s.pool.QueryRow(ctx,
		`SELECT `+sessionFields+` FROM agent_sessions WHERE id = $1 AND team_id = $2`,
		sessionID, teamID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, apperrors.ErrNotFound
	}
	return sess, err
}

// ListSessionsInTeam returns sessions in teamID. When userIDFilter is non-empty,
// it scopes to a single user; otherwise it returns every session in the team.
func (s *Store) ListSessionsInTeam(ctx context.Context, teamID, userIDFilter string) ([]Session, error) {
	var (
		rows pgx.Rows
		err  error
	)
	if strings.TrimSpace(userIDFilter) == "" {
		rows, err = s.pool.Query(ctx,
			`SELECT `+sessionFields+` FROM agent_sessions
			 WHERE team_id = $1 ORDER BY updated_at DESC LIMIT 100`,
			teamID,
		)
	} else {
		rows, err = s.pool.Query(ctx,
			`SELECT `+sessionFields+` FROM agent_sessions
			 WHERE team_id = $1 AND user_id = $2 ORDER BY updated_at DESC LIMIT 100`,
			teamID, userIDFilter,
		)
	}
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

// DeleteSessionInTeam deletes a session by id within a team. Returns ErrNotFound
// if no row matches both id and team_id.
func (s *Store) DeleteSessionInTeam(ctx context.Context, teamID, sessionID string) error {
	cmd, err := s.pool.Exec(ctx,
		`DELETE FROM agent_sessions WHERE id = $1 AND team_id = $2`,
		sessionID, teamID,
	)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return apperrors.ErrNotFound
	}
	return nil
}
