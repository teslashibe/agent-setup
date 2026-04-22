-- +goose Up
-- +goose StatementBegin

-- platform_credentials stores per-user, per-platform authentication blobs
-- (typically cookie JSON for scraper-style platforms). The blob is encrypted
-- at rest with AES-GCM using a key derived from CREDENTIALS_ENCRYPTION_KEY.
-- One row per (user, platform); reconnecting the same platform replaces the
-- existing row.
CREATE TABLE platform_credentials (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    platform      TEXT NOT NULL,
    credential    BYTEA NOT NULL,
    label         TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at  TIMESTAMPTZ,
    UNIQUE (user_id, platform)
);
CREATE INDEX idx_platform_credentials_user ON platform_credentials(user_id);

-- Per-user agent provisioning cache. agent_setup originally provisioned a
-- single global Anthropic Agent + Environment via cmd/provision/main.go and
-- pinned the IDs in env. With MCP and per-user credentials, each user gets
-- their own Agent+Environment so their MCP server URL (which encodes their
-- JWT) and tool inventory are isolated. The columns here cache the result of
-- lazy provisioning so we don't recreate on every request.
ALTER TABLE users
    ADD COLUMN anthropic_agent_id        TEXT,
    ADD COLUMN anthropic_environment_id  TEXT,
    ADD COLUMN anthropic_provisioned_at  TIMESTAMPTZ;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE users
    DROP COLUMN IF EXISTS anthropic_provisioned_at,
    DROP COLUMN IF EXISTS anthropic_environment_id,
    DROP COLUMN IF EXISTS anthropic_agent_id;
DROP TABLE IF EXISTS platform_credentials;
-- +goose StatementEnd
