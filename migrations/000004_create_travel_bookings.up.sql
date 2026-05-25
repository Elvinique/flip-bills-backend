CREATE TYPE travel_mode AS ENUM ('bus','flight');
CREATE TYPE booking_status AS ENUM ('pending','confirmed','rescheduled','cancelled','boarded');

CREATE TABLE travel_bookings (
    id                  UUID           PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id             UUID           NOT NULL REFERENCES users(id),
    transaction_id      UUID           NOT NULL REFERENCES transactions(id),
    mode                travel_mode    NOT NULL,
    operator_code       VARCHAR(50)    NOT NULL,
    operator_name       VARCHAR(100)   NOT NULL,
    origin              VARCHAR(100)   NOT NULL,
    destination         VARCHAR(100)   NOT NULL,
    departure_time      TIMESTAMPTZ    NOT NULL,
    seat_number         VARCHAR(10),
    vehicle_ref         VARCHAR(100),
    passenger_name      VARCHAR(200)   NOT NULL,
    passenger_phone     VARCHAR(20)    NOT NULL,
    ticket_code         TEXT           NOT NULL,
    offline_cache_hash  VARCHAR(64),
    price_paid          BIGINT         NOT NULL,
    status              booking_status NOT NULL DEFAULT 'pending',
    created_at          TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ    NOT NULL DEFAULT NOW(),
    CONSTRAINT travel_price_positive CHECK (price_paid > 0)
);

CREATE INDEX travel_bookings_user_id_idx   ON travel_bookings (user_id);
CREATE INDEX travel_bookings_status_idx    ON travel_bookings (status);
CREATE INDEX travel_bookings_departure_idx ON travel_bookings (departure_time);
CREATE INDEX travel_bookings_ticket_idx    ON travel_bookings (ticket_code);
CREATE INDEX travel_bookings_cache_hash_idx ON travel_bookings (offline_cache_hash) WHERE offline_cache_hash IS NOT NULL;
