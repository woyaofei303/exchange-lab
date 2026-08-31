-- +goose Up
-- 添加幂等清算任务、订单和状态转换证据。
CREATE TABLE liquidation_jobs (
    id TEXT PRIMARY KEY,
    idempotency_key TEXT NOT NULL UNIQUE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    symbol TEXT NOT NULL REFERENCES contracts(symbol) ON DELETE RESTRICT,
    margin_mode TEXT NOT NULL CHECK (margin_mode IN ('isolated','cross')),
    risk_version BIGINT NOT NULL CHECK (risk_version > 0),
    mark_price_atoms BIGINT NOT NULL CHECK (mark_price_atoms > 0),
    bankruptcy_price_atoms BIGINT NOT NULL CHECK (bankruptcy_price_atoms > 0),
    status TEXT NOT NULL CHECK (status IN ('pending','executing','settling','completed','parked')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK(attempts >= 0),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX liquidation_jobs_pending_idx ON liquidation_jobs(status,created_at,id) WHERE status IN ('pending','executing','settling');
CREATE TABLE liquidation_orders (
    id TEXT PRIMARY KEY,
    job_id TEXT NOT NULL REFERENCES liquidation_jobs(id) ON DELETE RESTRICT,
    order_id TEXT,
    quantity_atoms BIGINT NOT NULL CHECK(quantity_atoms > 0),
    reduce_only BOOLEAN NOT NULL DEFAULT true CHECK(reduce_only),
    status TEXT NOT NULL CHECK(status IN ('pending','filled','failed')),
    created_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE liquidation_events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    job_id TEXT NOT NULL REFERENCES liquidation_jobs(id) ON DELETE RESTRICT,
    event_type TEXT NOT NULL,
    evidence JSONB NOT NULL CHECK(jsonb_typeof(evidence)='object'),
    occurred_at TIMESTAMPTZ NOT NULL
);
-- +goose Down
DROP TABLE liquidation_events;
DROP TABLE liquidation_orders;
DROP INDEX liquidation_jobs_pending_idx;
DROP TABLE liquidation_jobs;
