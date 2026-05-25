-- loyalty_accounts holds the running points balance per user.
CREATE TABLE loyalty_accounts (
    id              UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    points_balance  BIGINT      NOT NULL DEFAULT 0,
    lifetime_points BIGINT      NOT NULL DEFAULT 0,
    tier            VARCHAR(20) NOT NULL DEFAULT 'bronze',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT loyalty_balance_non_negative  CHECK (points_balance >= 0),
    CONSTRAINT loyalty_lifetime_non_negative CHECK (lifetime_points >= 0)
);

CREATE UNIQUE INDEX loyalty_accounts_user_id_idx ON loyalty_accounts (user_id);

CREATE TYPE loyalty_tx_type AS ENUM ('earn', 'redeem', 'expire');

CREATE TABLE loyalty_transactions (
    id             UUID            PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id        UUID            NOT NULL REFERENCES users(id),
    account_id     UUID            NOT NULL REFERENCES loyalty_accounts(id),
    type           loyalty_tx_type NOT NULL,
    points         BIGINT          NOT NULL,
    balance_before BIGINT          NOT NULL,
    balance_after  BIGINT          NOT NULL,
    source_tx_id   UUID            REFERENCES transactions(id),
    category       VARCHAR(50),
    narration      TEXT,
    expires_at     TIMESTAMPTZ,
    created_at     TIMESTAMPTZ     NOT NULL DEFAULT NOW()
);

CREATE INDEX loyalty_tx_user_id_idx    ON loyalty_transactions (user_id);
CREATE INDEX loyalty_tx_account_id_idx ON loyalty_transactions (account_id);
CREATE INDEX loyalty_tx_type_idx       ON loyalty_transactions (type);
CREATE INDEX loyalty_tx_expires_idx    ON loyalty_transactions (expires_at)
    WHERE expires_at IS NOT NULL AND type = 'earn';

CREATE TRIGGER loyalty_accounts_set_updated_at
    BEFORE UPDATE ON loyalty_accounts
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
