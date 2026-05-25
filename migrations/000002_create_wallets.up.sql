CREATE TABLE wallets (
    id              UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    balance         BIGINT      NOT NULL DEFAULT 0,
    ledger_balance  BIGINT      NOT NULL DEFAULT 0,
    currency        VARCHAR(3)  NOT NULL DEFAULT 'NGN',
    daily_spend     BIGINT      NOT NULL DEFAULT 0,
    daily_limit     BIGINT      NOT NULL DEFAULT 500000000,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT wallets_balance_non_negative     CHECK (balance >= 0),
    CONSTRAINT wallets_ledger_non_negative      CHECK (ledger_balance >= 0),
    CONSTRAINT wallets_daily_spend_non_negative CHECK (daily_spend >= 0),
    CONSTRAINT wallets_daily_limit_positive     CHECK (daily_limit > 0),
    CONSTRAINT wallets_currency_valid           CHECK (currency IN ('NGN'))
);

CREATE UNIQUE INDEX wallets_user_id_unique_idx ON wallets (user_id);
CREATE INDEX wallets_user_id_idx ON wallets (user_id);
