-- Create console_query_audit table for query execution audit logging
CREATE TABLE IF NOT EXISTS console_query_audit (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    session_id UUID NOT NULL REFERENCES database_console_sessions(id) ON DELETE CASCADE,
    database_id UUID NOT NULL REFERENCES managed_databases(id) ON DELETE CASCADE,
    query_hash VARCHAR(64) NOT NULL, -- SHA-256 hash of the query (not storing actual query)
    query_length INTEGER NOT NULL,
    status VARCHAR(20) NOT NULL CHECK (status IN ('success', 'error', 'blocked')),
    error_msg TEXT,
    rows_affected BIGINT,
    execution_time DOUBLE PRECISION,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Indexes for efficient queries
CREATE UNIQUE INDEX idx_audit_session ON console_query_audit(session_id, created_at DESC);
CREATE INDEX idx_audit_account ON console_query_audit(account_id, created_at DESC);
CREATE INDEX idx_audit_query_hash ON console_query_audit(query_hash);

-- Comment
COMMENT ON TABLE console_query_audit IS 'Audit logs for SQL queries executed in database console';
COMMENT ON COLUMN console_query_audit.query_hash IS 'SHA-256 hash of query (privacy-preserving)';
COMMENT ON COLUMN console_query_audit.execution_time IS 'Query execution time in seconds';
