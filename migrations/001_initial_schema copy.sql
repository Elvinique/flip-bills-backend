-- ============================================================
-- Flip Bills — Initial Database Schema
-- Migration: 001_initial_schema.sql
-- Run with: golang-migrate or psql
-- ============================================================

-- ── Extensions ───────────────────────────────────────────────
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ── Users ────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS users (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    phone           VARCHAR(20)  NOT NULL UNIQUE,
    email           VARCHAR(255) UNIQUE,
    password_hash   TEXT         NOT NULL,
    first_name      VARCHAR(100) NOT NULL,
    last_name       VARCHAR(100) NOT NULL,
    kyc_tier        SMALLINT     NOT NULL DEFAULT 0,  -- 0=unverified, 1=BVN, 2=NIN
    bvn             TEXT,                             -- encrypted via pgcrypto at app layer
    nin             TEXT,                             -- encrypted via pgcrypto at app layer
    is_active       BOOLEAN      NOT NULL DEFAULT TRUE,
    pin_hash        TEXT,                             -- 4/6-digit transaction PIN (bcrypt)
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);

CREATE INDEX idx_users_phone ON users(phone) WHERE deleted_at IS NULL;

-- ── Wallets ───────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS wallets (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID         NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    balance         BIGINT       NOT NULL DEFAULT 0 CHECK (balance >= 0),   -- kobo
    ledger_balance  BIGINT       NOT NULL DEFAULT 0,                         -- pending/in-flight
    currency        VARCHAR(3)   NOT NULL DEFAULT 'NGN',
    daily_spend     BIGINT       NOT NULL DEFAULT 0,
    daily_limit     BIGINT       NOT NULL DEFAULT 5000000,                  -- ₦50k default (tier 0)
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_wallet_user UNIQUE (user_id)
);

-- ── Transactions (immutable audit ledger) ─────────────────────
CREATE TABLE IF NOT EXISTS transactions (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID             NOT NULL REFERENCES users(id),
    wallet_id       UUID             NOT NULL REFERENCES wallets(id),
    reference       VARCHAR(100)     NOT NULL UNIQUE,
    external_ref    VARCHAR(255),
    type            VARCHAR(20)      NOT NULL CHECK (type IN ('debit','credit','reversal')),
    category        VARCHAR(50)      NOT NULL,
    amount          BIGINT           NOT NULL,           -- kobo
    fee             BIGINT           NOT NULL DEFAULT 0, -- kobo
    balance_before  BIGINT           NOT NULL,
    balance_after   BIGINT           NOT NULL,
    status          VARCHAR(20)      NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending','processing','success','failed','reversed')),
    provider        VARCHAR(50),
    narration       TEXT,
    meta            JSONB,
    reversed_tx_id  UUID REFERENCES transactions(id),
    created_at      TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ      NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_tx_user_id    ON transactions(user_id, created_at DESC);
CREATE INDEX idx_tx_reference  ON transactions(reference);
CREATE INDEX idx_tx_status     ON transactions(status) WHERE status IN ('pending','processing');

-- ── Travel Bookings ───────────────────────────────────────────
CREATE TABLE IF NOT EXISTS travel_bookings (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id             UUID             NOT NULL REFERENCES users(id),
    transaction_id      UUID             NOT NULL REFERENCES transactions(id),
    mode                VARCHAR(10)      NOT NULL CHECK (mode IN ('bus','flight')),
    operator_code       VARCHAR(50)      NOT NULL,
    operator_name       VARCHAR(100)     NOT NULL,
    origin              VARCHAR(100)     NOT NULL,
    destination         VARCHAR(100)     NOT NULL,
    departure_time      TIMESTAMPTZ      NOT NULL,
    seat_number         VARCHAR(10),
    vehicle_ref         VARCHAR(100),
    passenger_name      VARCHAR(200)     NOT NULL,
    passenger_phone     VARCHAR(20)      NOT NULL,
    ticket_code         TEXT             NOT NULL,        -- QR payload
    status              VARCHAR(20)      NOT NULL DEFAULT 'pending'
                            CHECK (status IN ('confirmed','pending','rescheduled','cancelled','boarded')),
    offline_cache_hash  VARCHAR(64),                      -- SHA-256 for offline QR verification
    price_paid          BIGINT           NOT NULL,        -- kobo
    created_at          TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ      NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_bookings_user ON travel_bookings(user_id, departure_time);

-- ── OTP Store ────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS otp_tokens (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    phone       VARCHAR(20) NOT NULL,
    otp_hash    TEXT        NOT NULL,     -- bcrypt hash — never store plain OTP
    purpose     VARCHAR(30) NOT NULL,     -- "phone_verify" | "pin_reset" | "tx_auth"
    expires_at  TIMESTAMPTZ NOT NULL,
    used        BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_otp_phone ON otp_tokens(phone, purpose) WHERE NOT used;

-- ── Daily spend reset (cron / pg_cron suggestion) ─────────────
-- Run nightly: UPDATE wallets SET daily_spend = 0, updated_at = NOW();

-- ── Migration 002 — Phase 2 additions ────────────────────────
-- Operator availability log: used by Terminal Dispatcher (Phase 3)
-- to broadcast real-time fleet changes to affected passengers.
CREATE TABLE IF NOT EXISTS operator_fleet_events (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    operator_code   VARCHAR(50)  NOT NULL,
    vehicle_ref     VARCHAR(100) NOT NULL,
    event_type      VARCHAR(30)  NOT NULL CHECK (event_type IN ('delay','cancelled','rescheduled','platform_change')),
    message         TEXT         NOT NULL,
    new_departure   TIMESTAMPTZ,
    affected_date   DATE         NOT NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_fleet_events_operator ON operator_fleet_events(operator_code, affected_date);

-- Booking passengers notified log: prevents duplicate push/SMS.
CREATE TABLE IF NOT EXISTS booking_notifications (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    booking_id  UUID        NOT NULL REFERENCES travel_bookings(id),
    event_id    UUID        NOT NULL REFERENCES operator_fleet_events(id),
    channel     VARCHAR(10) NOT NULL CHECK (channel IN ('push','sms')),
    sent_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_booking_event_channel UNIQUE (booking_id, event_id, channel)
);
