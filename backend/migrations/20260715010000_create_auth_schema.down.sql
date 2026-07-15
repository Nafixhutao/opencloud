-- Keep the namespace on rollback: Better Auth owns any identity data inside it,
-- and Bun must never drop that data. Re-applying the up migration is idempotent.
SELECT 1;
