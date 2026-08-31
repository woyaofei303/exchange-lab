-- +goose Up
-- 添加用于只读解释的可审计模型用量和证据引用。
CREATE TABLE ai_runs (
    id TEXT PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    symbol TEXT NOT NULL REFERENCES contracts(symbol) ON DELETE RESTRICT,
    prompt_version TEXT NOT NULL,
    model TEXT NOT NULL,
    input_tokens BIGINT NOT NULL CHECK(input_tokens >= 0),
    output_tokens BIGINT NOT NULL CHECK(output_tokens >= 0),
    latency_ms BIGINT NOT NULL CHECK(latency_ms >= 0),
    evidence_ids TEXT[] NOT NULL,
    status TEXT NOT NULL CHECK(status IN ('completed','failed')),
    created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX ai_runs_user_time_idx ON ai_runs(user_id,created_at DESC,id DESC);
-- +goose Down
DROP INDEX ai_runs_user_time_idx;
DROP TABLE ai_runs;
