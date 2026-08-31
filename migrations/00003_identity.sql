-- +goose Up
-- Adds RBAC and hash-only opaque session persistence.
ALTER TABLE users
    ADD COLUMN role TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin'));

CREATE TABLE user_sessions (
    token_hash BYTEA PRIMARY KEY CHECK (octet_length(token_hash) = 32),
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    CHECK (expires_at > created_at),
    CHECK (revoked_at IS NULL OR revoked_at >= created_at)
);

CREATE INDEX user_sessions_user_active_idx
    ON user_sessions (user_id, expires_at DESC)
    WHERE revoked_at IS NULL;

CREATE INDEX audit_logs_occurred_at_idx ON audit_logs (occurred_at DESC, id DESC);

-- +goose Down
DROP INDEX audit_logs_occurred_at_idx;
DROP TABLE user_sessions;
ALTER TABLE users DROP COLUMN role;
