-- One-time token claims contain only SHA-256 digests, never raw verification
-- tokens or URLs. The Next.js auth boundary uses this table to make Better
-- Auth email-verification links single-use under concurrent requests.
CREATE TABLE auth_token_consumptions (
    id          BIGSERIAL PRIMARY KEY,
    token_hash  BYTEA NOT NULL UNIQUE,
    kind        TEXT NOT NULL
                CHECK (kind IN ('email_verification')),
    expires_at  TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_auth_token_consumptions_expires_at
    ON auth_token_consumptions (expires_at);

-- audit_logs is append-only at the database boundary, not merely by repository
-- convention. Inserts remain transactional with their owning mutation.
CREATE FUNCTION prevent_audit_log_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'audit_logs is append-only';
END;
$$;

CREATE TRIGGER audit_logs_no_update
BEFORE UPDATE ON audit_logs
FOR EACH ROW EXECUTE FUNCTION prevent_audit_log_mutation();

CREATE TRIGGER audit_logs_no_delete
BEFORE DELETE ON audit_logs
FOR EACH ROW EXECUTE FUNCTION prevent_audit_log_mutation();
