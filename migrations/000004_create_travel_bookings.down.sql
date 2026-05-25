DROP INDEX IF EXISTS travel_bookings_user_id_idx;
DROP INDEX IF EXISTS travel_bookings_status_idx;
DROP INDEX IF EXISTS travel_bookings_departure_idx;
DROP INDEX IF EXISTS travel_bookings_ticket_idx;
DROP INDEX IF EXISTS travel_bookings_cache_hash_idx;
DROP TABLE IF EXISTS travel_bookings;
DROP TYPE IF EXISTS booking_status;
DROP TYPE IF EXISTS travel_mode;
