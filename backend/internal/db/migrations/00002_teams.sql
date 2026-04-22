-- +goose Up
-- +goose StatementBegin

CREATE TYPE team_role AS ENUM ('owner', 'admin', 'member');

CREATE TABLE teams (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT NOT NULL CHECK (length(trim(name)) > 0),
    slug         TEXT NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9][a-z0-9-]*$'),
    is_personal  BOOLEAN NOT NULL DEFAULT FALSE,
    max_seats    INT NOT NULL DEFAULT 25 CHECK (max_seats >= 1),
    created_by   UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_teams_slug ON teams(slug);
CREATE UNIQUE INDEX idx_teams_personal_per_user
    ON teams(created_by) WHERE is_personal = TRUE;

CREATE TABLE team_members (
    team_id    UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role       team_role NOT NULL,
    joined_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (team_id, user_id)
);
CREATE INDEX idx_team_members_user ON team_members(user_id);
CREATE UNIQUE INDEX idx_team_members_one_owner
    ON team_members(team_id) WHERE role = 'owner';

CREATE TABLE team_invites (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id      UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    email        TEXT NOT NULL CHECK (length(trim(email)) > 0),
    role         team_role NOT NULL CHECK (role IN ('admin', 'member')),
    token        TEXT NOT NULL UNIQUE,
    invited_by   UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at   TIMESTAMPTZ NOT NULL,
    accepted_at  TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_team_invites_token ON team_invites(token);
CREATE INDEX idx_team_invites_team
    ON team_invites(team_id, accepted_at, revoked_at);
CREATE UNIQUE INDEX idx_team_invites_pending_email
    ON team_invites(team_id, lower(email))
    WHERE accepted_at IS NULL AND revoked_at IS NULL;

ALTER TABLE auth_codes
    ADD COLUMN invite_token TEXT;

ALTER TABLE agent_sessions
    ADD COLUMN team_id UUID REFERENCES teams(id) ON DELETE CASCADE;

WITH new_teams AS (
    INSERT INTO teams (name, slug, is_personal, created_by)
    SELECT
        CASE
            WHEN NULLIF(trim(u.name), '') IS NOT NULL THEN trim(u.name) || '''s Workspace'
            ELSE 'Personal Workspace'
        END,
        lower(regexp_replace(
            COALESCE(NULLIF(split_part(u.email, '@', 1), ''), 'user'),
            '[^a-z0-9]+', '-', 'g'
        )) || '-' || substr(u.id::text, 1, 8),
        TRUE,
        u.id
    FROM users u
    RETURNING id, created_by
)
INSERT INTO team_members (team_id, user_id, role)
SELECT id, created_by, 'owner' FROM new_teams;

UPDATE agent_sessions s
SET team_id = t.id
FROM teams t
WHERE t.created_by = s.user_id
  AND t.is_personal = TRUE;

ALTER TABLE agent_sessions
    ALTER COLUMN team_id SET NOT NULL;

DROP INDEX IF EXISTS idx_agent_sessions_user;
CREATE INDEX idx_agent_sessions_team_user
    ON agent_sessions(team_id, user_id, updated_at DESC);
CREATE INDEX idx_agent_sessions_team
    ON agent_sessions(team_id, updated_at DESC);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_agent_sessions_team;
DROP INDEX IF EXISTS idx_agent_sessions_team_user;
CREATE INDEX IF NOT EXISTS idx_agent_sessions_user
    ON agent_sessions(user_id, updated_at DESC);
ALTER TABLE agent_sessions DROP COLUMN IF EXISTS team_id;
ALTER TABLE auth_codes DROP COLUMN IF EXISTS invite_token;
DROP TABLE IF EXISTS team_invites;
DROP TABLE IF EXISTS team_members;
DROP TABLE IF EXISTS teams;
DROP TYPE IF EXISTS team_role;
-- +goose StatementEnd
