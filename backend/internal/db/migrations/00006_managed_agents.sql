-- +goose Up
-- +goose StatementBegin
ALTER TABLE agent_sessions ADD COLUMN anthropic_session_id TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_sessions_anthropic_id
    ON agent_sessions(anthropic_session_id)
    WHERE anthropic_session_id IS NOT NULL;
DROP TABLE IF EXISTS agent_messages;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_agent_sessions_anthropic_id;
ALTER TABLE agent_sessions DROP COLUMN IF EXISTS anthropic_session_id;
-- +goose StatementEnd
