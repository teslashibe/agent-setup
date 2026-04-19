-- +goose Up
-- +goose StatementBegin
-- agent_messages stores every turn (user, assistant, tool_use, tool_result).
-- It's a Timescale hypertable partitioned on created_at so heavy chat history
-- stays fast. session_id is included in the primary key because hypertables
-- require the partitioning column in any unique constraint.
CREATE TABLE IF NOT EXISTS agent_messages (
    id          UUID NOT NULL DEFAULT gen_random_uuid(),
    session_id  UUID NOT NULL REFERENCES agent_sessions(id) ON DELETE CASCADE,
    role        TEXT NOT NULL CHECK (role IN ('user','assistant')),
    content     JSONB NOT NULL,
    stop_reason TEXT,
    input_tokens  INTEGER,
    output_tokens INTEGER,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, created_at)
);

SELECT create_hypertable('agent_messages', 'created_at', if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_agent_messages_session
    ON agent_messages(session_id, created_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS agent_messages;
-- +goose StatementEnd
