-- 0008_create_transfers.up.sql
-- Records all outbound transfer attempts (bank and wallet-to-wallet).

CREATE TABLE IF NOT EXISTS transfers (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    reference       VARCHAR(120) NOT NULL,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    wallet_id       UUID NOT NULL REFERENCES wallets(id) ON DELETE RESTRICT,
    amount          BIGINT NOT NULL CHECK (amount > 0),     -- in kobo
    fee             BIGINT NOT NULL DEFAULT 0,              -- in kobo
    status          VARCHAR(20) NOT NULL DEFAULT 'pending', -- pending | success | failed | reversed
    transfer_type   VARCHAR(20) NOT NULL,                   -- BANK | WALLET
    -- Bank transfer fields
    bank_code       VARCHAR(10),
    account_number  VARCHAR(20),
    account_name    VARCHAR(200),
    -- Wallet transfer fields
    recipient_phone VARCHAR(20),
    recipient_user_id UUID REFERENCES users(id),
    -- Common fields
    narration       TEXT,
    provider        VARCHAR(50),                            -- paystack | flutterwave
    provider_ref    VARCHAR(120),
    failure_reason  TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_transfer_reference UNIQUE (reference)
);

CREATE INDEX idx_transfers_user_id   ON transfers(user_id);
CREATE INDEX idx_transfers_status    ON transfers(status);
CREATE INDEX idx_transfers_created   ON transfers(created_at DESC);
