CREATE TYPE tx_status AS ENUM ('pending','processing','success','failed','reversed');
CREATE TYPE tx_type AS ENUM ('debit','credit','reversal');
CREATE TYPE service_category AS ENUM ('airtime','data','electricity','cable_tv','betting','bus_travel','flight','wallet_funding','transfer');

CREATE TABLE transactions (
    id              UUID             PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID             NOT NULL REFERENCES users(id),
    wallet_id       UUID             NOT NULL REFERENCES wallets(id),
    reference       VARCHAR(100)     NOT NULL,
    external_ref    VARCHAR(200),
    type            tx_type          NOT NULL,
    category        service_category NOT NULL,
    amount          BIGINT           NOT NULL,
    fee             BIGINT           NOT NULL DEFAULT 0,
    balance_before  BIGINT           NOT NULL,
    balance_after   BIGINT           NOT NULL,
    status          tx_status        NOT NULL DEFAULT 'pending',
    provider        VARCHAR(50),
    narration       TEXT,
    meta            JSONB,
    reversed_tx_id  UUID             REFERENCES transactions(id),
    created_at      TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    CONSTRAINT transactions_amount_positive    CHECK (amount > 0),
    CONSTRAINT transactions_fee_non_negative   CHECK (fee >= 0)
);

CREATE UNIQUE INDEX transactions_reference_unique_idx ON transactions (reference);
CREATE INDEX transactions_user_id_idx    ON transactions (user_id);
CREATE INDEX transactions_wallet_id_idx  ON transactions (wallet_id);
CREATE INDEX transactions_status_idx     ON transactions (status);
CREATE INDEX transactions_created_at_idx ON transactions (created_at DESC);
CREATE INDEX transactions_category_idx   ON transactions (category);
CREATE INDEX transactions_recon_idx      ON transactions (status, created_at) WHERE status IN ('pending', 'processing');
CREATE INDEX transactions_meta_gin_idx   ON transactions USING GIN (meta);
