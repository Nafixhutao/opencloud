-- Migration: 20260809050000_create_console_query_audit
-- Description: Add audit table for SQL query logging (hashed, not plaintext)

CREATE TABLE console_query_audit (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id      TEXT NOT NULL,
    database_id     TEXT NOT NULL,
    session_id      TEXT NOT NULL,
    actor_id        TEXT NOT NULL,
    query_hash      TEXT NOT NULL,
    statement_type  TEXT NOT NULL CHECK (statement_type IN ('SELECT', 'INSERT', 'UPDATE', 'DELETE', 'CREATE', 'ALTER', 'DROP', 'TRUNCATE')),
    duration_ms     INT NOT NULL DEFAULT 0,
    affected_rows   BIGINT DEFAULT 0,
    status          TEXT NOT NULL DEFAULT 'success'
                    CHECK (status IN ('success', 'error', 'timeout')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_query_audit_account ON console_query_audit(account_id);
CREATE INDEX idx_query_audit_database ON console_query_audit(database_id);
CREATE INDEX idx_query_audit_session ON console_query_audit(session_id);
CREATE INDEX idx_query_audit_created ON console_query_audit(created_at DESC);
