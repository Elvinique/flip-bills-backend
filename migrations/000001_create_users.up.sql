CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE users (
    id            UUID          PRIMARY KEY DEFAULT uuid_generate_v4(),
    phone         VARCHAR(20)   NOT NULL,
    email         VARCHAR(255),
    password_hash TEXT          NOT NULL,
    first_name    VARCHAR(100)  NOT NULL,
    last_name     VARCHAR(100)  NOT NULL,
    kyc_tier      SMALLINT      NOT NULL DEFAULT 0,
    bvn           TEXT,
    nin           TEXT,
    is_active     BOOLEAN       NOT NULL DEFAULT TRUE,
    pin_hash      TEXT,
    created_at    TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    deleted_at    TIMESTAMPTZ
);

CREATE UNIQUE INDEX users_phone_unique_idx ON users (phone) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX users_email_unique_idx ON users (email) WHERE deleted_at IS NULL;
CREATE INDEX users_phone_idx ON users (phone);
