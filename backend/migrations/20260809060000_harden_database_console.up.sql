-- Harden database console sessions per the master prompt: session lifecycle
-- state, actor identity, engine, revocation, and safe audit metadata.
ALTER TABLE database_console_sessions
    ADD COLUMN actor_id TEXT,
    ADD COLUMN engine VARCHAR(20) NOT NULL DEFAULT 'postgres',
    ADD COLUMN status VARCHAR(20) NOT NULL DEFAULT 'active',
    ADD COLUMN revoked_at TIMESTAMPTZ;

ALTER TABLE database_console_sessions
    ADD CONSTRAINT database_console_sessions_status_check
    CHECK (status IN ('active', 'revoked', 'expired'));

ALTER TABLE database_console_sessions
    ADD CONSTRAINT database_console_sessions_engine_check
    CHECK (engine IN ('postgres', 'mariadb'));

-- Audit records carry the safe statement type and engine, never the query body.
ALTER TABLE console_query_audit
    ADD COLUMN actor_id TEXT,
    ADD COLUMN engine VARCHAR(20) NOT NULL DEFAULT 'postgres',
    ADD COLUMN statement_type VARCHAR(20) NOT NULL DEFAULT 'unknown';

ALTER TABLE console_query_audit
    ADD CONSTRAINT console_query_audit_statement_type_check
    CHECK (statement_type IN ('select', 'explain', 'show', 'describe', 'unknown'));
