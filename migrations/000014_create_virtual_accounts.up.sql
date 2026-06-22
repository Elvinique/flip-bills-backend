-- 0009_create_virtual_accounts.up.sql
-- Stores dedicated virtual bank accounts per user (Monnify reserved accounts).

CREATE TABLE IF NOT EXISTS virtual_accounts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    account_number  VARCHAR(20) NOT NULL,
    account_name    VARCHAR(200) NOT NULL,
    bank_name       VARCHAR(100) NOT NULL DEFAULT 'Moniepoint',
    bank_code       VARCHAR(10),
    provider        VARCHAR(50) NOT NULL DEFAULT 'monnify',  -- monnify | paystack
    provider_ref    VARCHAR(120),
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_virtual_account_user UNIQUE (user_id),    -- one virtual account per user
    CONSTRAINT uq_virtual_account_number UNIQUE (account_number)
);

CREATE INDEX idx_virtual_accounts_user_id ON virtual_accounts(user_id);
