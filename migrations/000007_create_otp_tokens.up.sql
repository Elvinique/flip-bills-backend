CREATE TABLE IF NOT EXISTS otp_tokens (
    id          UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    phone       VARCHAR(20) NOT NULL,
    otp_hash    TEXT        NOT NULL,
    purpose     VARCHAR(20) NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    used        BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_otp_phone ON otp_tokens(phone, purpose) WHERE NOT used;
