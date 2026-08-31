-- +goose Up
-- 添加订单状态、生命周期事实、幂等性和事务性 Outbox 存储。
CREATE TABLE orders (
    id TEXT PRIMARY KEY CHECK (char_length(id) BETWEEN 1 AND 128),
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    client_order_id TEXT NOT NULL CHECK (char_length(client_order_id) BETWEEN 8 AND 128),
    symbol TEXT NOT NULL REFERENCES instruments(symbol) ON DELETE RESTRICT,
    side TEXT NOT NULL CHECK (side IN ('buy', 'sell')),
    order_type TEXT NOT NULL CHECK (order_type IN ('limit', 'market')),
    status TEXT NOT NULL CHECK (status IN ('pending', 'open', 'partially_filled', 'filled', 'canceled', 'rejected')),
    price_atoms BIGINT NOT NULL CHECK (price_atoms > 0),
    price_scale SMALLINT NOT NULL CHECK (price_scale BETWEEN 0 AND 18),
    quantity_atoms BIGINT NOT NULL CHECK (quantity_atoms > 0),
    remaining_atoms BIGINT NOT NULL CHECK (remaining_atoms >= 0 AND remaining_atoms <= quantity_atoms),
    quantity_scale SMALLINT NOT NULL CHECK (quantity_scale BETWEEN 0 AND 18),
    quote_scale SMALLINT NOT NULL CHECK (quote_scale BETWEEN 0 AND 18),
    leverage BIGINT NOT NULL CHECK (leverage BETWEEN 1 AND 125),
    reserved_margin_atoms BIGINT NOT NULL CHECK (reserved_margin_atoms >= 0),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (user_id, client_order_id)
);
CREATE INDEX orders_user_status_time_idx ON orders (user_id, status, created_at DESC, id DESC);

CREATE TABLE order_events (
    id TEXT PRIMARY KEY CHECK (char_length(id) BETWEEN 1 AND 128),
    order_id TEXT NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    event_type TEXT NOT NULL CHECK (event_type IN ('accepted', 'cancel_requested', 'canceled', 'partially_filled', 'filled', 'rejected')),
    version INTEGER NOT NULL DEFAULT 1 CHECK (version = 1),
    payload JSONB NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    occurred_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX order_events_order_time_idx ON order_events (order_id, occurred_at, id);

CREATE TABLE idempotency_keys (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    operation TEXT NOT NULL CHECK (char_length(operation) BETWEEN 1 AND 64),
    idempotency_key TEXT NOT NULL CHECK (char_length(idempotency_key) BETWEEN 8 AND 128),
    request_hash BYTEA NOT NULL CHECK (octet_length(request_hash) = 32),
    resource_id TEXT NOT NULL CHECK (char_length(resource_id) BETWEEN 1 AND 128),
    created_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (user_id, operation, idempotency_key)
);

CREATE TABLE outbox_events (
    id TEXT PRIMARY KEY CHECK (char_length(id) BETWEEN 1 AND 128),
    aggregate_type TEXT NOT NULL CHECK (char_length(aggregate_type) BETWEEN 1 AND 64),
    aggregate_id TEXT NOT NULL CHECK (char_length(aggregate_id) BETWEEN 1 AND 128),
    subject TEXT NOT NULL CHECK (subject ~ '^[a-z][a-z0-9_.-]{2,127}$'),
    event_version INTEGER NOT NULL DEFAULT 1 CHECK (event_version > 0),
    correlation_id TEXT NOT NULL CHECK (char_length(correlation_id) BETWEEN 1 AND 128),
    payload JSONB NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'publishing', 'published', 'parked')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    available_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    published_at TIMESTAMPTZ,
    last_error_code TEXT,
    UNIQUE (aggregate_id, subject, event_version, id)
);
CREATE INDEX outbox_pending_idx ON outbox_events (available_at, created_at, id) WHERE status = 'pending';

-- +goose Down
DROP TABLE outbox_events;
DROP TABLE idempotency_keys;
DROP TABLE order_events;
DROP INDEX orders_user_status_time_idx;
DROP TABLE orders;
