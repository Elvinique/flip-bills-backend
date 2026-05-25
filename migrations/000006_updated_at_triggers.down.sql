DROP TRIGGER IF EXISTS users_set_updated_at           ON users;
DROP TRIGGER IF EXISTS wallets_set_updated_at         ON wallets;
DROP TRIGGER IF EXISTS transactions_set_updated_at    ON transactions;
DROP TRIGGER IF EXISTS travel_bookings_set_updated_at ON travel_bookings;
DROP FUNCTION IF EXISTS set_updated_at();
