-- 0007_create_ledger_entries.up.sql
-- Double-entry accounting table.
-- RULE: balance = SUM(credit) - SUM(debit). Never store a running balance.

CREATE TABLE IF NOT EXISTS ledger_entries (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    wallet_id    UUID NOT NULL REFERENCES wallets(id) ON DELETE RESTRICT,
    debit        BIGINT NOT NULL DEFAULT 0 CHECK (debit >= 0),  -- kobo leaving wallet
    credit       BIGINT NOT NULL DEFAULT 0 CHECK (credit >= 0), -- kobo entering wallet
    reference    VARCHAR(120) NOT NULL,                         -- globally unique, provider ref
    entry_type   VARCHAR(50) NOT NULL,                          -- FUNDING | TRANSFER_OUT | VAS | REVERSAL | FEE
    description  TEXT,
    metadata     JSONB DEFAULT '{}',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_debit_or_credit CHECK (
        (debit > 0 AND credit = 0) OR (credit > 0 AND debit = 0)
    ),
    CONSTRAINT uq_ledger_reference UNIQUE (reference)
);

CREATE INDEX idx_ledger_wallet_id    ON ledger_entries(wallet_id);
CREATE INDEX idx_ledger_reference    ON ledger_entries(reference);
CREATE INDEX idx_ledger_created_at   ON ledger_entries(created_at DESC);
CREATE INDEX idx_ledger_entry_type   ON ledger_entries(entry_type);
