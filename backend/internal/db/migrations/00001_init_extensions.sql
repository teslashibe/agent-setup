-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS timescaledb;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- timescaledb extension is intentionally not dropped on down (data-bearing)
DROP EXTENSION IF EXISTS pgcrypto;
-- +goose StatementEnd
