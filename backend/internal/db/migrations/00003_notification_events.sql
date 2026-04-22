-- +goose Up
-- +goose StatementBegin

-- notification_events stores per-user notifications captured by the mobile
-- app's NotificationListenerService. The Claude agent reads these via the
-- notifications_* MCP tools to produce daily communication rollups across
-- texts, WhatsApp, email, Zillow, etc. without needing per-platform API
-- integrations.
--
-- Schema is opt-in: the table is created on every fork, but stays empty
-- (and adds zero overhead) when NOTIFICATIONS_ENABLED=false. Same posture as
-- the teams tables which exist regardless of TEAMS_ENABLED.
--
-- Stored as a TimescaleDB hypertable (chunked on captured_at) because every
-- access pattern is a time range scan ("today's notifications", "the last
-- 4 hours"). Hypertable chunking gives efficient range scans and lets us
-- attach a retention policy (drop chunks older than N days) without manual
-- partition management.
CREATE TABLE notification_events (
    id           BIGINT GENERATED ALWAYS AS IDENTITY,
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    app_package  TEXT NOT NULL,
    app_label    TEXT NOT NULL DEFAULT '',
    title        TEXT NOT NULL DEFAULT '',
    content      TEXT NOT NULL DEFAULT '',
    category     TEXT NOT NULL DEFAULT '',
    captured_at  TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- Composite PK includes captured_at because TimescaleDB requires the
    -- partition column to be part of any unique constraint on a hypertable.
    PRIMARY KEY (id, captured_at)
);

-- Convert to hypertable. chunk_time_interval defaults to 7 days for
-- notification volume scale (1 user × ~1k notifications/day = 7k rows/chunk).
SELECT create_hypertable(
    'notification_events',
    'captured_at',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE
);

-- Dedupe index: a single (user, app, captured_at, title) tuple identifies a
-- notification. Used by InsertBatch's ON CONFLICT DO NOTHING. Must include
-- captured_at because the hypertable requires it on every unique constraint.
CREATE UNIQUE INDEX uq_notif_event_dedup
    ON notification_events (user_id, app_package, captured_at, title);

-- Hot path: list / paginate notifications by time for a user.
CREATE INDEX idx_notif_user_time
    ON notification_events (user_id, captured_at DESC);

-- Filter-by-app pattern (notifications_list with app_package, ListApps).
CREATE INDEX idx_notif_user_app
    ON notification_events (user_id, app_package, captured_at DESC);

-- Full-text search across title + content for the notifications_search tool.
-- Postgres' built-in to_tsvector + GIN is sufficient for V1; no external
-- search engine required.
CREATE INDEX idx_notif_content_fts
    ON notification_events
 USING gin (to_tsvector('english', coalesce(title, '') || ' ' || coalesce(content, '')));

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_notif_content_fts;
DROP INDEX IF EXISTS idx_notif_user_app;
DROP INDEX IF EXISTS idx_notif_user_time;
DROP INDEX IF EXISTS uq_notif_event_dedup;
DROP TABLE IF EXISTS notification_events;
-- +goose StatementEnd
