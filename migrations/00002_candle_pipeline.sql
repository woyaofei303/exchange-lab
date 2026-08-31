-- +goose Up
-- 添加基于事件时间的 K 线排序证据和持久化市场成交去重。
ALTER TABLE candles
    ADD COLUMN first_trade_time TIMESTAMPTZ,
    ADD COLUMN last_trade_time TIMESTAMPTZ,
    ADD COLUMN first_sequence BIGINT NOT NULL DEFAULT 0 CHECK (first_sequence >= 0),
    ADD COLUMN last_sequence BIGINT NOT NULL DEFAULT 0 CHECK (last_sequence >= 0);

UPDATE candles
SET first_trade_time = open_time,
    last_trade_time = open_time
WHERE first_trade_time IS NULL OR last_trade_time IS NULL;

ALTER TABLE candles
    ALTER COLUMN first_trade_time SET NOT NULL,
    ALTER COLUMN last_trade_time SET NOT NULL;

CREATE TABLE market_trades (
    stream_id TEXT NOT NULL CHECK (char_length(stream_id) BETWEEN 1 AND 64),
    trade_id TEXT NOT NULL CHECK (char_length(trade_id) BETWEEN 1 AND 64),
    symbol TEXT NOT NULL REFERENCES instruments(symbol),
    sequence BIGINT NOT NULL CHECK (sequence > 0),
    event_time TIMESTAMPTZ NOT NULL,
    price_scale SMALLINT NOT NULL CHECK (price_scale BETWEEN 0 AND 18),
    quantity_scale SMALLINT NOT NULL CHECK (quantity_scale BETWEEN 0 AND 18),
    price_atoms BIGINT NOT NULL CHECK (price_atoms > 0),
    quantity_atoms BIGINT NOT NULL CHECK (quantity_atoms > 0),
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (stream_id, trade_id),
    UNIQUE (stream_id, sequence)
);

CREATE INDEX market_trades_symbol_event_time_idx
    ON market_trades (symbol, event_time);

CREATE INDEX candles_history_idx
    ON candles (symbol, interval_seconds, open_time DESC);

-- +goose Down
DROP INDEX candles_history_idx;
DROP TABLE market_trades;
ALTER TABLE candles
    DROP COLUMN last_sequence,
    DROP COLUMN first_sequence,
    DROP COLUMN last_trade_time,
    DROP COLUMN first_trade_time;
