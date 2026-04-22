package teams

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/teslashibe/agent-setup/backend/internal/apperrors"
)

const (
	teamFields   = `id::text, name, slug, is_personal, max_seats, created_by::text, created_at, updated_at`
	memberFields = `m.team_id::text, m.user_id::text, u.email, u.name, m.role::text, m.joined_at`
	inviteFields = `id::text, team_id::text, email, role::text, token, invited_by::text, expires_at, accepted_at, revoked_at, created_at`
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

type scanner interface{ Scan(dest ...any) error }

func scanTeam(row scanner) (Team, error) {
	var t Team
	err := row.Scan(&t.ID, &t.Name, &t.Slug, &t.IsPersonal, &t.MaxSeats, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}

func scanMember(row scanner) (Member, error) {
	var (
		m       Member
		roleStr string
	)
	err := row.Scan(&m.TeamID, &m.UserID, &m.Email, &m.Name, &roleStr, &m.JoinedAt)
	m.Role = Role(roleStr)
	return m, err
}

func scanInvite(row scanner) (Invite, error) {
	var (
		inv     Invite
		roleStr string
	)
	err := row.Scan(
		&inv.ID, &inv.TeamID, &inv.Email, &roleStr, &inv.Token, &inv.InvitedBy,
		&inv.ExpiresAt, &inv.AcceptedAt, &inv.RevokedAt, &inv.CreatedAt,
	)
	inv.Role = Role(roleStr)
	return inv, err
}

func (s *Store) GetTeam(ctx context.Context, teamID string) (Team, error) {
	t, err := scanTeam(s.pool.QueryRow(ctx,
		`SELECT `+teamFields+` FROM teams WHERE id = $1`, teamID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Team{}, apperrors.ErrTeamNotFound
	}
	return t, err
}

func (s *Store) ListTeamsForUser(ctx context.Context, userID string) ([]Membership, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+teamFields+`, m.role::text
		FROM teams t
		JOIN team_members m ON m.team_id = t.id
		WHERE m.user_id = $1
		ORDER BY t.is_personal DESC, t.created_at ASC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Membership{}
	for rows.Next() {
		var (
			t       Team
			roleStr string
		)
		if err := rows.Scan(
			&t.ID, &t.Name, &t.Slug, &t.IsPersonal, &t.MaxSeats, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt,
			&roleStr,
		); err != nil {
			return nil, err
		}
		out = append(out, Membership{Team: t, Role: Role(roleStr)})
	}
	return out, rows.Err()
}

// CreateTeamWithOwner inserts a team and the owner row in a single transaction.
// Caller must have already validated name + ownerID. slug is derived inside this
// method so callers cannot mint duplicates.
func (s *Store) CreateTeamWithOwner(ctx context.Context, ownerID, name string, isPersonal bool, maxSeats int) (Team, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Team{}, err
	}
	defer tx.Rollback(ctx)

	slug, err := generateUniqueSlug(ctx, tx, name)
	if err != nil {
		return Team{}, err
	}

	team, err := scanTeam(tx.QueryRow(ctx, `
		INSERT INTO teams (name, slug, is_personal, max_seats, created_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+teamFields,
		strings.TrimSpace(name), slug, isPersonal, maxSeats, ownerID,
	))
	if err != nil {
		return Team{}, mapPGError(err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO team_members (team_id, user_id, role)
		VALUES ($1, $2, 'owner')`,
		team.ID, ownerID,
	); err != nil {
		return Team{}, mapPGError(err)
	}

	return team, tx.Commit(ctx)
}

func (s *Store) UpdateTeamName(ctx context.Context, teamID, name string) (Team, error) {
	t, err := scanTeam(s.pool.QueryRow(ctx, `
		UPDATE teams SET name = $1, updated_at = NOW()
		WHERE id = $2
		RETURNING `+teamFields,
		strings.TrimSpace(name), teamID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Team{}, apperrors.ErrTeamNotFound
	}
	return t, mapPGError(err)
}

func (s *Store) DeleteTeam(ctx context.Context, teamID string) error {
	cmd, err := s.pool.Exec(ctx, `DELETE FROM teams WHERE id = $1`, teamID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return apperrors.ErrTeamNotFound
	}
	return nil
}

func (s *Store) GetMembership(ctx context.Context, teamID, userID string) (Membership, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT `+teamFields+`, m.role::text
		FROM teams t
		JOIN team_members m ON m.team_id = t.id
		WHERE t.id = $1 AND m.user_id = $2`,
		teamID, userID,
	)
	var (
		t       Team
		roleStr string
	)
	err := row.Scan(
		&t.ID, &t.Name, &t.Slug, &t.IsPersonal, &t.MaxSeats, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt,
		&roleStr,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Membership{}, apperrors.ErrNotTeamMember
	}
	if err != nil {
		return Membership{}, err
	}
	return Membership{Team: t, Role: Role(roleStr)}, nil
}

// FindPersonalTeam returns the user's personal team. ErrNotTeamMember if none
// exists yet.
func (s *Store) FindPersonalTeam(ctx context.Context, userID string) (Team, error) {
	t, err := scanTeam(s.pool.QueryRow(ctx, `
		SELECT `+teamFields+` FROM teams
		WHERE created_by = $1 AND is_personal = TRUE`,
		userID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Team{}, apperrors.ErrTeamNotFound
	}
	return t, err
}

func (s *Store) ListMembers(ctx context.Context, teamID string) ([]Member, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+memberFields+`
		FROM team_members m
		JOIN users u ON u.id = m.user_id
		WHERE m.team_id = $1
		ORDER BY
			CASE m.role WHEN 'owner' THEN 1 WHEN 'admin' THEN 2 ELSE 3 END,
			u.email ASC`,
		teamID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Member{}
	for rows.Next() {
		m, err := scanMember(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) AddMember(ctx context.Context, teamID, userID string, role Role) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO team_members (team_id, user_id, role)
		VALUES ($1, $2, $3::team_role)`,
		teamID, userID, string(role),
	)
	return mapPGError(err)
}

func (s *Store) UpdateMemberRole(ctx context.Context, teamID, userID string, role Role) error {
	cmd, err := s.pool.Exec(ctx, `
		UPDATE team_members SET role = $3::team_role
		WHERE team_id = $1 AND user_id = $2`,
		teamID, userID, string(role),
	)
	if err != nil {
		return mapPGError(err)
	}
	if cmd.RowsAffected() == 0 {
		return apperrors.ErrNotTeamMember
	}
	return nil
}

func (s *Store) RemoveMember(ctx context.Context, teamID, userID string) error {
	cmd, err := s.pool.Exec(ctx, `
		DELETE FROM team_members WHERE team_id = $1 AND user_id = $2`,
		teamID, userID,
	)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return apperrors.ErrNotTeamMember
	}
	return nil
}

func (s *Store) CountMembers(ctx context.Context, teamID string) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT count(*) FROM team_members WHERE team_id = $1`,
		teamID,
	).Scan(&n)
	return n, err
}

// TransferOwnership atomically demotes fromUserID to admin and promotes
// toUserID to owner. Both users must already be members.
func (s *Store) TransferOwnership(ctx context.Context, teamID, fromUserID, toUserID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Demote first to free up the unique-owner partial index slot.
	cmd, err := tx.Exec(ctx, `
		UPDATE team_members SET role = 'admin'
		WHERE team_id = $1 AND user_id = $2 AND role = 'owner'`,
		teamID, fromUserID,
	)
	if err != nil {
		return mapPGError(err)
	}
	if cmd.RowsAffected() == 0 {
		return apperrors.ErrInsufficientRole
	}

	cmd, err = tx.Exec(ctx, `
		UPDATE team_members SET role = 'owner'
		WHERE team_id = $1 AND user_id = $2`,
		teamID, toUserID,
	)
	if err != nil {
		return mapPGError(err)
	}
	if cmd.RowsAffected() == 0 {
		return apperrors.ErrNotTeamMember
	}

	return tx.Commit(ctx)
}

// CreateInvite inserts an invite and returns the persisted row. The caller is
// responsible for generating the token, expiry, and validating the role.
func (s *Store) CreateInvite(ctx context.Context, teamID, invitedBy, email string, role Role, token string, expiresAt time.Time) (Invite, error) {
	inv, err := scanInvite(s.pool.QueryRow(ctx, `
		INSERT INTO team_invites (team_id, email, role, token, invited_by, expires_at)
		VALUES ($1, $2, $3::team_role, $4, $5, $6)
		RETURNING `+inviteFields,
		teamID, strings.ToLower(strings.TrimSpace(email)), string(role), token, invitedBy, expiresAt,
	))
	return inv, mapPGError(err)
}

func (s *Store) GetInvite(ctx context.Context, inviteID string) (Invite, error) {
	inv, err := scanInvite(s.pool.QueryRow(ctx,
		`SELECT `+inviteFields+` FROM team_invites WHERE id = $1`, inviteID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Invite{}, apperrors.ErrInviteNotFound
	}
	return inv, err
}

func (s *Store) GetInviteByToken(ctx context.Context, token string) (Invite, error) {
	inv, err := scanInvite(s.pool.QueryRow(ctx,
		`SELECT `+inviteFields+` FROM team_invites WHERE token = $1`, token,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Invite{}, apperrors.ErrInviteNotFound
	}
	return inv, err
}

func (s *Store) ListInvitesForTeam(ctx context.Context, teamID string) ([]Invite, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+inviteFields+`
		FROM team_invites
		WHERE team_id = $1 AND accepted_at IS NULL AND revoked_at IS NULL
		ORDER BY created_at DESC`,
		teamID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []Invite{}
	for rows.Next() {
		inv, err := scanInvite(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}

func (s *Store) MarkInviteRevoked(ctx context.Context, inviteID string, at time.Time) error {
	cmd, err := s.pool.Exec(ctx, `
		UPDATE team_invites SET revoked_at = $2
		WHERE id = $1 AND accepted_at IS NULL AND revoked_at IS NULL`,
		inviteID, at,
	)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return apperrors.ErrInviteNotFound
	}
	return nil
}

// ConsumeInvite atomically marks an invite accepted (locking the row to
// prevent double-acceptance) and inserts the team_member row. The caller is
// expected to have already checked role + email match.
func (s *Store) ConsumeInvite(ctx context.Context, token, userID string, at time.Time) (Invite, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Invite{}, err
	}
	defer tx.Rollback(ctx)

	inv, err := scanInvite(tx.QueryRow(ctx, `
		SELECT `+inviteFields+` FROM team_invites
		WHERE token = $1 FOR UPDATE`,
		token,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Invite{}, apperrors.ErrInviteNotFound
	}
	if err != nil {
		return Invite{}, err
	}
	if inv.AcceptedAt != nil {
		return Invite{}, apperrors.ErrInviteAlreadyAccepted
	}
	if inv.RevokedAt != nil {
		return Invite{}, apperrors.ErrInviteRevoked
	}
	if at.After(inv.ExpiresAt) {
		return Invite{}, apperrors.ErrInviteExpired
	}

	if _, err := tx.Exec(ctx,
		`UPDATE team_invites SET accepted_at = $2 WHERE id = $1`,
		inv.ID, at,
	); err != nil {
		return Invite{}, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO team_members (team_id, user_id, role)
		VALUES ($1, $2, $3::team_role)
		ON CONFLICT (team_id, user_id) DO UPDATE SET role = EXCLUDED.role`,
		inv.TeamID, userID, string(inv.Role),
	); err != nil {
		return Invite{}, mapPGError(err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Invite{}, err
	}
	now := at
	inv.AcceptedAt = &now
	return inv, nil
}

// generateUniqueSlug derives a slug from name and appends a short suffix until
// the result is unique. Tries the bare slug first to keep URLs clean.
func generateUniqueSlug(ctx context.Context, q pgx.Tx, name string) (string, error) {
	base := slugify(name)
	if base == "" {
		base = "team"
	}

	candidate := base
	for attempt := 0; attempt < 8; attempt++ {
		var exists bool
		if err := q.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM teams WHERE slug = $1)`, candidate,
		).Scan(&exists); err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
		suffix, err := randomShortToken()
		if err != nil {
			return "", err
		}
		candidate = base + "-" + suffix
	}
	return "", fmt.Errorf("could not allocate unique slug for %q after 8 attempts", name)
}

// mapPGError translates known Postgres errors to apperrors so handlers can
// surface them with the right HTTP status.
func mapPGError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	switch pgErr.Code {
	case "23505":
		switch pgErr.ConstraintName {
		case "idx_team_members_one_owner":
			return apperrors.ErrOwnerExists
		case "team_members_pkey":
			return apperrors.ErrAlreadyMember
		case "idx_team_invites_pending_email":
			return apperrors.ErrInvitePending
		case "idx_teams_personal_per_user":
			return apperrors.ErrPersonalTeamExists
		case "teams_slug_key", "idx_teams_slug":
			return apperrors.New(409, "team slug already taken")
		}
		return apperrors.New(409, "duplicate")
	case "23514":
		return apperrors.New(400, "request violates a database constraint")
	case "23503":
		return apperrors.New(400, "referenced record does not exist")
	}
	return err
}
