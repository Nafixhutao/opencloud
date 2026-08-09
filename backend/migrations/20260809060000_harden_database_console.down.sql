ALTER TABLE console_query_audit
    DROP CONSTRAINT IF EXISTS console_query_audit_statement_type_check;

ALTER TABLE console_query_audit
    DROP COLUMN IF EXISTS actor_id,
    DROP COLUMN IF EXISTS engine,
    DROP COLUMN IF EXISTS statement_type;

ALTER TABLE database_console_sessions
    DROP CONSTRAINT IF EXISTS database_console_sessions_status_check,
    DROP CONSTRAINT IF EXISTS database_console_sessions_engine_check;

ALTER TABLE database_console_sessions
    DROP COLUMN IF EXISTS actor_id,
    DROP COLUMN IF EXISTS engine,
    DROP COLUMN IF EXISTS status,
    DROP COLUMN IF EXISTS revoked_at;
