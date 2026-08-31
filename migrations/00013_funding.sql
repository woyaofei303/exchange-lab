-- +goose Up
-- 添加溢价观测、资金费率和精确用户结算。
CREATE TABLE premium_indexes (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    symbol TEXT NOT NULL REFERENCES contracts(symbol) ON DELETE RESTRICT,
    premium_bps BIGINT NOT NULL CHECK(premium_bps BETWEEN -10000 AND 10000),
    mark_price_atoms BIGINT NOT NULL CHECK(mark_price_atoms > 0),
    index_price_atoms BIGINT NOT NULL CHECK(index_price_atoms > 0),
    observed_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE funding_rates (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    symbol TEXT NOT NULL REFERENCES contracts(symbol) ON DELETE RESTRICT,
    funding_time TIMESTAMPTZ NOT NULL,
    predicted_rate_bps BIGINT NOT NULL CHECK(predicted_rate_bps BETWEEN -10000 AND 10000),
    final_rate_bps BIGINT NOT NULL CHECK(final_rate_bps BETWEEN -10000 AND 10000),
    mark_price_atoms BIGINT NOT NULL CHECK(mark_price_atoms > 0),
    status TEXT NOT NULL CHECK(status IN ('predicted','final','settled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(symbol,funding_time)
);
CREATE TABLE funding_settlements (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    funding_rate_id BIGINT NOT NULL REFERENCES funding_rates(id) ON DELETE RESTRICT,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    symbol TEXT NOT NULL REFERENCES contracts(symbol) ON DELETE RESTRICT,
    position_size_atoms BIGINT NOT NULL,
    payment_atoms BIGINT NOT NULL,
    position_version BIGINT NOT NULL,
    settled_at TIMESTAMPTZ NOT NULL,
    UNIQUE(funding_rate_id,user_id,symbol)
);
-- +goose Down
DROP TABLE funding_settlements;
DROP TABLE funding_rates;
DROP TABLE premium_indexes;
