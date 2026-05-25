CREATE OR REPLACE FUNCTION reset_daily_spend()
RETURNS void LANGUAGE plpgsql AS $$
BEGIN
    UPDATE wallets SET daily_spend = 0, updated_at = NOW() WHERE daily_spend > 0;
END;
$$;
