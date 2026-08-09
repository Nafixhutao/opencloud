-- Migration: 20260809040000_create_database_console_sessions
-- Description: Create database_console_sessions table for secure console access sessions
-- Author: OpenCloud Implementation
-- Date: 2026-08-09

CREATE TABLE database_console_sessions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id  UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    database_id UUID NOT NULL REFERENCES databases(id) ON DELETE CASCADE,
    actor_id    TEXT NOT NULL,
    engine      TEXT NOT NULL CHECK (engine IN ('postgres', 'mariadb')),
    status      TEXT NOT NULL DEFAULT 'active'
                CHECK (status IN ('active', 'expired', 'revoked')),
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at  TIMESTAMPTZ
);

CREATE INDEX idx_console_sessions_account_id ON database_console_sessions(account_id);
CREATE INDEX idx_console_sessions_database_id ON database_console_sessions(database_id);
CREATE INDEX idx_console_sessions_expires_at ON database_console_sessions(expires_at);
CREATE INDEX idx_console_sessions_active ON database_console_sessions(status) 
    WHERE status = 'active';
