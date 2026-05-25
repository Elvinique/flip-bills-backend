CREATE TYPE dispatcher_event_type AS ENUM (
    'vehicle_breakdown',
    'route_change',
    'double_booking',
    'schedule_change',
    'cancellation'
);

CREATE TYPE dispatcher_event_status AS ENUM (
    'open',
    'notified',
    'resolved'
);

-- dispatcher_events logs every operational disruption broadcast by an operator.
CREATE TABLE dispatcher_events (
    id               UUID                    PRIMARY KEY DEFAULT uuid_generate_v4(),
    operator_code    VARCHAR(50)             NOT NULL,
    event_type       dispatcher_event_type   NOT NULL,
    vehicle_ref      VARCHAR(100)            NOT NULL,
    origin           VARCHAR(100)            NOT NULL,
    destination      VARCHAR(100)            NOT NULL,
    departure_time   TIMESTAMPTZ             NOT NULL,
    message          TEXT                    NOT NULL,  -- human-readable reason
    status           dispatcher_event_status NOT NULL DEFAULT 'open',
    api_key_hash     VARCHAR(64)             NOT NULL,  -- SHA-256 of operator API key
    created_at       TIMESTAMPTZ             NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ             NOT NULL DEFAULT NOW()
);

-- passenger_disruptions tracks the per-passenger action for each event.
CREATE TYPE disruption_resolution AS ENUM (
    'pending',      -- notified, awaiting passenger action
    'rescheduled',  -- passenger picked a new trip
    'refunded'      -- wallet credit issued
);

CREATE TABLE passenger_disruptions (
    id               UUID                  PRIMARY KEY DEFAULT uuid_generate_v4(),
    event_id         UUID                  NOT NULL REFERENCES dispatcher_events(id),
    booking_id       UUID                  NOT NULL REFERENCES travel_bookings(id),
    user_id          UUID                  NOT NULL REFERENCES users(id),
    resolution       disruption_resolution NOT NULL DEFAULT 'pending',
    new_booking_id   UUID                  REFERENCES travel_bookings(id),
    refund_tx_id     UUID                  REFERENCES transactions(id),
    notified_at      TIMESTAMPTZ,
    resolved_at      TIMESTAMPTZ,
    created_at       TIMESTAMPTZ           NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ           NOT NULL DEFAULT NOW()
);

CREATE INDEX dispatcher_events_operator_idx    ON dispatcher_events (operator_code);
CREATE INDEX dispatcher_events_vehicle_idx     ON dispatcher_events (vehicle_ref);
CREATE INDEX dispatcher_events_status_idx      ON dispatcher_events (status);
CREATE INDEX passenger_disruptions_event_idx   ON passenger_disruptions (event_id);
CREATE INDEX passenger_disruptions_booking_idx ON passenger_disruptions (booking_id);
CREATE INDEX passenger_disruptions_user_idx    ON passenger_disruptions (user_id);
CREATE INDEX passenger_disruptions_status_idx  ON passenger_disruptions (resolution);

CREATE TRIGGER dispatcher_events_set_updated_at
    BEFORE UPDATE ON dispatcher_events
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TRIGGER passenger_disruptions_set_updated_at
    BEFORE UPDATE ON passenger_disruptions
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
