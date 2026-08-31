-- +goose Up
-- 添加确定性订单簿快照、偏移量、成交和撮合事实。
CREATE TABLE matching_book_orders (
    order_id TEXT PRIMARY KEY REFERENCES orders(id) ON DELETE RESTRICT,
    symbol TEXT NOT NULL REFERENCES instruments(symbol) ON DELETE RESTRICT,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    side TEXT NOT NULL CHECK (side IN ('buy','sell')),
    price_atoms BIGINT NOT NULL CHECK (price_atoms > 0),
    quantity_atoms BIGINT NOT NULL CHECK (quantity_atoms > 0),
    remaining_atoms BIGINT NOT NULL CHECK (remaining_atoms > 0 AND remaining_atoms <= quantity_atoms),
    order_sequence BIGINT NOT NULL UNIQUE CHECK (order_sequence > 0),
    updated_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX matching_book_price_idx ON matching_book_orders (symbol, side, price_atoms, order_sequence);

CREATE TABLE matching_offsets (
    consumer_name TEXT PRIMARY KEY CHECK (char_length(consumer_name) BETWEEN 1 AND 128),
    last_stream_sequence BIGINT NOT NULL DEFAULT 0 CHECK (last_stream_sequence >= 0),
    last_trade_sequence BIGINT NOT NULL DEFAULT 0 CHECK (last_trade_sequence >= 0),
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE contract_trades (
    id TEXT PRIMARY KEY CHECK (char_length(id) BETWEEN 1 AND 128),
    symbol TEXT NOT NULL REFERENCES instruments(symbol) ON DELETE RESTRICT,
    maker_order_id TEXT NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    taker_order_id TEXT NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    maker_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    taker_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    taker_side TEXT NOT NULL CHECK (taker_side IN ('buy','sell')),
    price_atoms BIGINT NOT NULL CHECK (price_atoms > 0),
    price_scale SMALLINT NOT NULL CHECK (price_scale BETWEEN 0 AND 18),
    quantity_atoms BIGINT NOT NULL CHECK (quantity_atoms > 0),
    quantity_scale SMALLINT NOT NULL CHECK (quantity_scale BETWEEN 0 AND 18),
    matching_sequence BIGINT NOT NULL UNIQUE CHECK (matching_sequence > 0),
    occurred_at TIMESTAMPTZ NOT NULL,
    CHECK (maker_order_id <> taker_order_id)
);
CREATE INDEX contract_trades_symbol_time_idx ON contract_trades (symbol, occurred_at DESC, matching_sequence DESC);

CREATE TABLE matching_events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source_event_id TEXT NOT NULL,
    order_id TEXT NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
    event_type TEXT NOT NULL CHECK (event_type IN ('open','matched','partially_filled','filled','canceled')),
    remaining_atoms BIGINT NOT NULL CHECK (remaining_atoms >= 0),
    trade_id TEXT,
    occurred_at TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX matching_events_source_order_type_trade_unique
    ON matching_events (source_event_id, order_id, event_type, COALESCE(trade_id,''));

-- +goose Down
DROP INDEX matching_events_source_order_type_trade_unique;
DROP TABLE matching_events;
DROP INDEX contract_trades_symbol_time_idx;
DROP TABLE contract_trades;
DROP TABLE matching_offsets;
DROP INDEX matching_book_price_idx;
DROP TABLE matching_book_orders;
