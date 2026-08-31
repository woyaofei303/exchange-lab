-- +goose Up
-- 添加单向持仓、成交事实和幂等成交结算记录。
CREATE TABLE positions (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    symbol TEXT NOT NULL REFERENCES contracts(symbol) ON DELETE RESTRICT,
    size_atoms BIGINT NOT NULL DEFAULT 0,
    entry_price_atoms BIGINT NOT NULL DEFAULT 0 CHECK (entry_price_atoms >= 0),
    price_scale SMALLINT NOT NULL CHECK (price_scale BETWEEN 0 AND 18),
    quantity_scale SMALLINT NOT NULL CHECK (quantity_scale BETWEEN 0 AND 18),
    realized_pnl_atoms BIGINT NOT NULL DEFAULT 0,
    fee_atoms BIGINT NOT NULL DEFAULT 0 CHECK (fee_atoms >= 0),
    version BIGINT NOT NULL DEFAULT 0 CHECK (version >= 0),
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (user_id, symbol),
    CHECK ((size_atoms = 0 AND entry_price_atoms = 0) OR (size_atoms <> 0 AND entry_price_atoms > 0))
);

CREATE TABLE position_events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    event_id TEXT NOT NULL,
    trade_id TEXT NOT NULL REFERENCES contract_trades(id) ON DELETE RESTRICT,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    symbol TEXT NOT NULL REFERENCES contracts(symbol) ON DELETE RESTRICT,
    side TEXT NOT NULL CHECK (side IN ('buy','sell')),
    fill_price_atoms BIGINT NOT NULL CHECK (fill_price_atoms > 0),
    fill_quantity_atoms BIGINT NOT NULL CHECK (fill_quantity_atoms > 0),
    size_before_atoms BIGINT NOT NULL,
    size_after_atoms BIGINT NOT NULL,
    realized_pnl_atoms BIGINT NOT NULL,
    fee_atoms BIGINT NOT NULL CHECK (fee_atoms >= 0),
    occurred_at TIMESTAMPTZ NOT NULL,
    UNIQUE (event_id, user_id)
);

CREATE TABLE position_snapshots (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    symbol TEXT NOT NULL REFERENCES contracts(symbol) ON DELETE RESTRICT,
    size_atoms BIGINT NOT NULL,
    entry_price_atoms BIGINT NOT NULL CHECK (entry_price_atoms >= 0),
    mark_price_atoms BIGINT NOT NULL CHECK (mark_price_atoms > 0),
    unrealized_pnl_atoms BIGINT NOT NULL,
    position_version BIGINT NOT NULL CHECK (position_version >= 0),
    as_of TIMESTAMPTZ NOT NULL
);
CREATE INDEX position_snapshots_user_time_idx ON position_snapshots (user_id, as_of DESC, id DESC);

CREATE TABLE trade_settlements (
    trade_id TEXT PRIMARY KEY REFERENCES contract_trades(id) ON DELETE RESTRICT,
    source_event_id TEXT NOT NULL UNIQUE,
    settled_at TIMESTAMPTZ NOT NULL
);

INSERT INTO ledger_accounts(owner_user_id,asset,account_type) VALUES
    (NULL,'USDT','fees'),(NULL,'USDT','insurance') ON CONFLICT DO NOTHING;

-- +goose Down
DROP TABLE trade_settlements;
DROP INDEX position_snapshots_user_time_idx;
DROP TABLE position_snapshots;
DROP TABLE position_events;
DROP TABLE positions;
