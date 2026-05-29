CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS transactions_external_ref_credit_unique_idx
    ON transactions (external_ref)
    WHERE type = 'credit' AND external_ref IS NOT NULL AND external_ref <> '';
