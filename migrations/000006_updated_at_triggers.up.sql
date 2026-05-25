CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$;

CREATE TRIGGER users_set_updated_at         BEFORE UPDATE ON users          FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER wallets_set_updated_at       BEFORE UPDATE ON wallets        FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER transactions_set_updated_at  BEFORE UPDATE ON transactions   FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER travel_bookings_set_updated_at BEFORE UPDATE ON travel_bookings FOR EACH ROW EXECUTE FUNCTION set_updated_at();
