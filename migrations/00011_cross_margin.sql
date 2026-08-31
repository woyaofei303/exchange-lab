-- +goose Up
-- Adds versioned cross-margin risk snapshots for shared collateral evidence.
CREATE TABLE cross_risk_snapshots (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    risk_version BIGINT NOT NULL CHECK (risk_version > 0),
    wallet_balance_atoms BIGINT NOT NULL,
    unrealized_pnl_atoms BIGINT NOT NULL,
    margin_balance_atoms BIGINT NOT NULL,
    maintenance_margin_atoms BIGINT NOT NULL CHECK (maintenance_margin_atoms >= 0),
    order_margin_atoms BIGINT NOT NULL CHECK (order_margin_atoms >= 0),
    available_balance_atoms BIGINT NOT NULL,
    mark_price_version BIGINT NOT NULL CHECK (mark_price_version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(user_id,risk_version)
);
CREATE INDEX cross_risk_snapshots_user_time_idx ON cross_risk_snapshots(user_id,created_at DESC,id DESC);
-- +goose Down
DROP INDEX cross_risk_snapshots_user_time_idx;
DROP TABLE cross_risk_snapshots;
