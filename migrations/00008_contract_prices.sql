-- +goose Up
-- Adds contract risk metadata and source-audited index/mark price history.
CREATE TABLE contracts (
    symbol TEXT PRIMARY KEY REFERENCES instruments(symbol) ON DELETE RESTRICT,
    contract_size_atoms BIGINT NOT NULL CHECK (contract_size_atoms > 0),
    settlement_asset TEXT NOT NULL CHECK (settlement_asset = 'USDT'),
    max_premium_bps BIGINT NOT NULL CHECK (max_premium_bps BETWEEN 0 AND 5000),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','suspended')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE leverage_tiers (
    symbol TEXT NOT NULL REFERENCES contracts(symbol) ON DELETE CASCADE,
    tier SMALLINT NOT NULL CHECK (tier > 0),
    notional_floor_atoms BIGINT NOT NULL CHECK (notional_floor_atoms >= 0),
    notional_cap_atoms BIGINT NOT NULL CHECK (notional_cap_atoms > notional_floor_atoms),
    max_leverage SMALLINT NOT NULL CHECK (max_leverage BETWEEN 1 AND 125),
    maintenance_margin_bps BIGINT NOT NULL CHECK (maintenance_margin_bps BETWEEN 1 AND 10000),
    PRIMARY KEY (symbol, tier),
    UNIQUE (symbol, notional_floor_atoms),
    UNIQUE (symbol, notional_cap_atoms)
);

CREATE TABLE price_sources (
    symbol TEXT NOT NULL REFERENCES contracts(symbol) ON DELETE CASCADE,
    source_name TEXT NOT NULL CHECK (char_length(source_name) BETWEEN 1 AND 64),
    price_atoms BIGINT NOT NULL CHECK (price_atoms > 0),
    price_scale SMALLINT NOT NULL CHECK (price_scale BETWEEN 0 AND 18),
    weight BIGINT NOT NULL CHECK (weight BETWEEN 1 AND 1000000),
    updated_at TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('included','excluded','stale')),
    PRIMARY KEY (symbol, source_name)
);

CREATE TABLE index_prices (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    symbol TEXT NOT NULL REFERENCES contracts(symbol) ON DELETE RESTRICT,
    price_atoms BIGINT NOT NULL CHECK (price_atoms > 0),
    price_scale SMALLINT NOT NULL CHECK (price_scale BETWEEN 0 AND 18),
    included_sources TEXT[] NOT NULL,
    excluded_sources TEXT[] NOT NULL,
    fresh BOOLEAN NOT NULL,
    as_of TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX index_prices_symbol_time_idx ON index_prices (symbol, created_at DESC, id DESC);

CREATE TABLE mark_prices (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    symbol TEXT NOT NULL REFERENCES contracts(symbol) ON DELETE RESTRICT,
    index_price_atoms BIGINT NOT NULL CHECK (index_price_atoms > 0),
    mark_price_atoms BIGINT NOT NULL CHECK (mark_price_atoms > 0),
    price_scale SMALLINT NOT NULL CHECK (price_scale BETWEEN 0 AND 18),
    premium_bps BIGINT NOT NULL CHECK (premium_bps BETWEEN -5000 AND 5000),
    fresh BOOLEAN NOT NULL,
    as_of TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX mark_prices_symbol_time_idx ON mark_prices (symbol, created_at DESC, id DESC);

INSERT INTO contracts (symbol, contract_size_atoms, settlement_asset, max_premium_bps)
VALUES ('BTCUSDT-PERP', 1000, 'USDT', 100);

INSERT INTO leverage_tiers (symbol, tier, notional_floor_atoms, notional_cap_atoms, max_leverage, maintenance_margin_bps)
VALUES
    ('BTCUSDT-PERP', 1, 0, 5000000000000, 50, 50),
    ('BTCUSDT-PERP', 2, 5000000000000, 20000000000000, 25, 100),
    ('BTCUSDT-PERP', 3, 20000000000000, 100000000000000, 10, 250);

-- +goose Down
DROP INDEX mark_prices_symbol_time_idx;
DROP TABLE mark_prices;
DROP INDEX index_prices_symbol_time_idx;
DROP TABLE index_prices;
DROP TABLE price_sources;
DROP TABLE leverage_tiers;
DROP TABLE contracts;
