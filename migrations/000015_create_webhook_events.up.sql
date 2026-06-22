-- 0010_create_webhook_events.up.sql
-- Audit log for all incoming provider webhooks.
-- Processed=false rows are picked up by the reconciliation worker.

CREATE TABLE IF NOT EXISTS webhook_events (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider      VARCHAR(50) NOT NULL,       -- flutterwave | monnify | paystack | opay
    event_type    VARCHAR(100) NOT NULL,      -- charge.success | transfer.success, etc.
    reference     VARCHAR(120),               -- the payment reference in the event
    payload       JSONB NOT NULL,             -- full raw webhook payload
    signature     VARCHAR(256),               -- raw signature header for re-verification
    processed     BOOLEAN NOT NULL DEFAULT FALSE,
    retries       INT NOT NULL DEFAULT 0,
    error_log     TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at  TIMESTAMPTZ,

    CONSTRAINT uq_webhook_provider_reference UNIQUE (provider, reference, event_type)
);

CREATE INDEX idx_webhook_reference  ON webhook_events(reference);
CREATE INDEX idx_webhook_processed  ON webhook_events(processed);
CREATE INDEX idx_webhook_created    ON webhook_events(created_at DESC);
CREATE INDEX idx_webhook_provider   ON webhook_events(provider);
