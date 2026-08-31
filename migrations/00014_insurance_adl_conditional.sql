-- +goose Up
-- 添加只减仓订单、保险事实、ADL 证据和价格条件。
ALTER TABLE orders ADD COLUMN reduce_only BOOLEAN NOT NULL DEFAULT false;
CREATE TABLE insurance_fund_entries (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    liquidation_job_id TEXT REFERENCES liquidation_jobs(id) ON DELETE RESTRICT,
    amount_atoms BIGINT NOT NULL CHECK(amount_atoms <> 0),
    balance_after_atoms BIGINT NOT NULL CHECK(balance_after_atoms >= 0),
    reason TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE adl_candidates (
    liquidation_job_id TEXT NOT NULL REFERENCES liquidation_jobs(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    symbol TEXT NOT NULL REFERENCES contracts(symbol) ON DELETE RESTRICT,
    profit_atoms BIGINT NOT NULL CHECK(profit_atoms >= 0),
    leverage SMALLINT NOT NULL CHECK(leverage > 0),
    rank INTEGER NOT NULL CHECK(rank > 0),
    evidence JSONB NOT NULL CHECK(jsonb_typeof(evidence)='object'),
    PRIMARY KEY(liquidation_job_id,user_id,symbol)
);
CREATE TABLE adl_events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    liquidation_job_id TEXT NOT NULL REFERENCES liquidation_jobs(id) ON DELETE RESTRICT,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    quantity_atoms BIGINT NOT NULL CHECK(quantity_atoms > 0),
    loss_covered_atoms BIGINT NOT NULL CHECK(loss_covered_atoms > 0),
    occurred_at TIMESTAMPTZ NOT NULL,
    UNIQUE(liquidation_job_id,user_id)
);
CREATE TABLE conditional_orders (
    id TEXT PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    symbol TEXT NOT NULL REFERENCES contracts(symbol) ON DELETE RESTRICT,
    condition_type TEXT NOT NULL CHECK(condition_type IN ('take_profit','stop_loss')),
    trigger_source TEXT NOT NULL CHECK(trigger_source IN ('mark','last')),
    trigger_direction TEXT NOT NULL CHECK(trigger_direction IN ('lte','gte')),
    trigger_price_atoms BIGINT NOT NULL CHECK(trigger_price_atoms > 0),
    quantity_atoms BIGINT NOT NULL CHECK(quantity_atoms > 0),
    reduce_only BOOLEAN NOT NULL DEFAULT true CHECK(reduce_only),
    status TEXT NOT NULL CHECK(status IN ('pending','triggered','canceled','expired')),
    idempotency_key TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE(user_id,idempotency_key)
);
INSERT INTO insurance_fund_entries(liquidation_job_id,amount_atoms,balance_after_atoms,reason,occurred_at)
VALUES(NULL,100000000000000,100000000000000,'initial_simulated_fund',now());
-- +goose Down
DROP TABLE conditional_orders;
DROP TABLE adl_events;
DROP TABLE adl_candidates;
DROP TABLE insurance_fund_entries;
ALTER TABLE orders DROP COLUMN reduce_only;
