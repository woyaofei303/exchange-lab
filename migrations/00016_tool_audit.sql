-- +goose Up
-- 添加只读工具调用的主体、范围和证据审计记录。
CREATE TABLE tool_audit_logs (
    id TEXT PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    tool_name TEXT NOT NULL,
    input_scope JSONB NOT NULL CHECK(jsonb_typeof(input_scope)='object'),
    evidence_ids TEXT[] NOT NULL,
    success BOOLEAN NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX tool_audit_user_time_idx ON tool_audit_logs(user_id,occurred_at DESC,id DESC);
-- +goose Down
DROP INDEX tool_audit_user_time_idx;
DROP TABLE tool_audit_logs;
