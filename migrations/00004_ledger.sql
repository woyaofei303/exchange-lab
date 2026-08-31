-- +goose Up
-- 建立精确的复式记账账户、余额和幂等模拟入金。
CREATE TABLE ledger_accounts (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    owner_user_id BIGINT REFERENCES users(id) ON DELETE RESTRICT,
    asset TEXT NOT NULL CHECK (asset ~ '^[A-Z0-9]{2,12}$'),
    account_type TEXT NOT NULL CHECK (account_type IN ('available', 'locked', 'system_faucet', 'fees', 'insurance')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((account_type IN ('available', 'locked') AND owner_user_id IS NOT NULL)
        OR (account_type IN ('system_faucet', 'fees', 'insurance') AND owner_user_id IS NULL)),
    UNIQUE NULLS NOT DISTINCT (owner_user_id, asset, account_type)
);

CREATE TABLE balances (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    asset TEXT NOT NULL CHECK (asset ~ '^[A-Z0-9]{2,12}$'),
    available_atoms BIGINT NOT NULL DEFAULT 0 CHECK (available_atoms >= 0),
    locked_atoms BIGINT NOT NULL DEFAULT 0 CHECK (locked_atoms >= 0),
    version BIGINT NOT NULL DEFAULT 0 CHECK (version >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, asset)
);

CREATE TABLE ledger_transactions (
    id TEXT PRIMARY KEY CHECK (char_length(id) BETWEEN 1 AND 128),
    user_id BIGINT REFERENCES users(id) ON DELETE RESTRICT,
    kind TEXT NOT NULL CHECK (char_length(kind) BETWEEN 1 AND 64),
    idempotency_key TEXT NOT NULL UNIQUE CHECK (char_length(idempotency_key) BETWEEN 1 AND 128),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE ledger_entries (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    transaction_id TEXT NOT NULL REFERENCES ledger_transactions(id) ON DELETE RESTRICT,
    account_id BIGINT NOT NULL REFERENCES ledger_accounts(id) ON DELETE RESTRICT,
    asset TEXT NOT NULL CHECK (asset ~ '^[A-Z0-9]{2,12}$'),
    amount_atoms BIGINT NOT NULL CHECK (amount_atoms <> 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ledger_entries_transaction_idx ON ledger_entries (transaction_id, id);
CREATE INDEX ledger_entries_account_time_idx ON ledger_entries (account_id, created_at, id);

CREATE TABLE faucet_requests (
    idempotency_key TEXT PRIMARY KEY REFERENCES ledger_transactions(idempotency_key) ON DELETE RESTRICT,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    asset TEXT NOT NULL CHECK (asset ~ '^[A-Z0-9]{2,12}$'),
    amount_atoms BIGINT NOT NULL CHECK (amount_atoms > 0),
    transaction_id TEXT NOT NULL UNIQUE REFERENCES ledger_transactions(id) ON DELETE RESTRICT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO ledger_accounts (owner_user_id, asset, account_type)
VALUES (NULL, 'USDT', 'system_faucet');

-- +goose Down
DROP TABLE faucet_requests;
DROP INDEX ledger_entries_account_time_idx;
DROP INDEX ledger_entries_transaction_idx;
DROP TABLE ledger_entries;
DROP TABLE ledger_transactions;
DROP TABLE balances;
DROP TABLE ledger_accounts;
