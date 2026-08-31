-- +goose Up
-- Establishes users, instruments, candles and append-only audit foundations.
CREATE TABLE users (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    email TEXT NOT NULL CHECK (char_length(email) BETWEEN 3 AND 320),
    password_hash TEXT NOT NULL CHECK (char_length(password_hash) BETWEEN 8 AND 512),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX users_email_unique_ci ON users (lower(email));

CREATE TABLE instruments (
    symbol TEXT PRIMARY KEY CHECK (symbol ~ '^[A-Z0-9]{4,24}-PERP$'),
    base_asset TEXT NOT NULL CHECK (base_asset ~ '^[A-Z0-9]{2,12}$'),
    quote_asset TEXT NOT NULL CHECK (quote_asset ~ '^[A-Z0-9]{2,12}$'),
    base_scale SMALLINT NOT NULL CHECK (base_scale BETWEEN 0 AND 18),
    quote_scale SMALLINT NOT NULL CHECK (quote_scale BETWEEN 0 AND 18),
    price_scale SMALLINT NOT NULL CHECK (price_scale BETWEEN 0 AND 18),
    quantity_scale SMALLINT NOT NULL CHECK (quantity_scale BETWEEN 0 AND 18),
    price_step_atoms BIGINT NOT NULL CHECK (price_step_atoms > 0),
    quantity_step_atoms BIGINT NOT NULL CHECK (quantity_step_atoms > 0),
    min_notional_atoms BIGINT NOT NULL CHECK (min_notional_atoms > 0),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (base_asset <> quote_asset),
    CHECK (symbol = base_asset || quote_asset || '-PERP'),
    CHECK (price_scale + quantity_scale <= quote_scale)
);

INSERT INTO instruments (
    symbol,
    base_asset,
    quote_asset,
    base_scale,
    quote_scale,
    price_scale,
    quantity_scale,
    price_step_atoms,
    quantity_step_atoms,
    min_notional_atoms
) VALUES (
    'BTCUSDT-PERP',
    'BTC',
    'USDT',
    8,
    8,
    2,
    3,
    10,
    1,
    500000000
);

CREATE TABLE candles (
    symbol TEXT NOT NULL REFERENCES instruments(symbol),
    interval_seconds INTEGER NOT NULL CHECK (interval_seconds > 0),
    open_time TIMESTAMPTZ NOT NULL,
    price_scale SMALLINT NOT NULL CHECK (price_scale BETWEEN 0 AND 18),
    quantity_scale SMALLINT NOT NULL CHECK (quantity_scale BETWEEN 0 AND 18),
    open_atoms BIGINT NOT NULL CHECK (open_atoms > 0),
    high_atoms BIGINT NOT NULL CHECK (high_atoms > 0),
    low_atoms BIGINT NOT NULL CHECK (low_atoms > 0),
    close_atoms BIGINT NOT NULL CHECK (close_atoms > 0),
    volume_atoms BIGINT NOT NULL CHECK (volume_atoms >= 0),
    trade_count BIGINT NOT NULL DEFAULT 0 CHECK (trade_count >= 0),
    is_final BOOLEAN NOT NULL DEFAULT false,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (symbol, interval_seconds, open_time),
    CHECK (high_atoms >= open_atoms AND high_atoms >= close_atoms AND high_atoms >= low_atoms),
    CHECK (low_atoms <= open_atoms AND low_atoms <= close_atoms AND low_atoms <= high_atoms)
);

CREATE TABLE audit_logs (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_user_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    action TEXT NOT NULL CHECK (char_length(action) BETWEEN 1 AND 128),
    resource_type TEXT NOT NULL CHECK (char_length(resource_type) BETWEEN 1 AND 64),
    resource_id TEXT NOT NULL CHECK (char_length(resource_id) BETWEEN 1 AND 128),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX audit_logs_resource_time_idx
    ON audit_logs (resource_type, resource_id, occurred_at DESC);

-- +goose Down
DROP TABLE audit_logs;
DROP TABLE candles;
DROP TABLE instruments;
DROP TABLE users;
