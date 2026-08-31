-- +goose Up
-- Adds isolated collateral, leverage settings, risk limits and margin adjustments.
CREATE TABLE margin_accounts (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE RESTRICT,
    mode TEXT NOT NULL DEFAULT 'isolated' CHECK (mode IN ('isolated','cross')),
    risk_version BIGINT NOT NULL DEFAULT 1 CHECK (risk_version > 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE leverage_settings (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    symbol TEXT NOT NULL REFERENCES contracts(symbol) ON DELETE RESTRICT,
    leverage SMALLINT NOT NULL CHECK (leverage BETWEEN 1 AND 125),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id,symbol)
);
CREATE TABLE position_margins (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    symbol TEXT NOT NULL REFERENCES contracts(symbol) ON DELETE RESTRICT,
    isolated_margin_atoms BIGINT NOT NULL DEFAULT 0 CHECK (isolated_margin_atoms >= 0),
    order_margin_atoms BIGINT NOT NULL DEFAULT 0 CHECK (order_margin_atoms >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id,symbol)
);
CREATE TABLE risk_limits (
    symbol TEXT NOT NULL REFERENCES contracts(symbol) ON DELETE CASCADE,
    tier SMALLINT NOT NULL,
    notional_cap_atoms BIGINT NOT NULL CHECK (notional_cap_atoms > 0),
    max_leverage SMALLINT NOT NULL CHECK (max_leverage BETWEEN 1 AND 125),
    maintenance_margin_bps BIGINT NOT NULL CHECK (maintenance_margin_bps BETWEEN 1 AND 10000),
    PRIMARY KEY(symbol,tier),
    FOREIGN KEY(symbol,tier) REFERENCES leverage_tiers(symbol,tier) ON DELETE CASCADE
);
INSERT INTO risk_limits SELECT symbol,tier,notional_cap_atoms,max_leverage,maintenance_margin_bps FROM leverage_tiers;
CREATE TABLE margin_adjustments (
    id TEXT PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    symbol TEXT NOT NULL REFERENCES contracts(symbol) ON DELETE RESTRICT,
    amount_atoms BIGINT NOT NULL CHECK(amount_atoms <> 0),
    idempotency_key TEXT NOT NULL,
    margin_after_atoms BIGINT NOT NULL CHECK(margin_after_atoms >= 0),
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE(user_id,idempotency_key)
);
-- +goose Down
DROP TABLE margin_adjustments;
DROP TABLE risk_limits;
DROP TABLE position_margins;
DROP TABLE leverage_settings;
DROP TABLE margin_accounts;
