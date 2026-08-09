-- Create database_console_sessions table for SQL console session management
CREATE TABLE IF NOT EXISTS database_console_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    database_id UUID NOT NULL REFERENCES databases(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    ip_addr VARCHAR(45),
    user_agent TEXT,
    session_token VARCHAR(64) NOT NULL UNIQUE,
    last_activity_at TIMESTAMPTZ,
    
    CONSTRAINT sessions_expires_after_created CHECK (expires_at > created_at)
);

-- Indexes for efficient queries
CREATE INDEX idx_sessions_account_id ON database_console_sessions(account_id);
CREATE INDEX idx_sessions_session_token ON database_console_sessions(session_token);
CREATE INDEX idx_sessions_expires_at ON database_console_sessions(expires_at);

-- Comment
COMMENT ON TABLE database_console_sessions IS 'Database console sessions for secure SQL execution';
COMMENT ON COLUMN database_console_sessions.session_token IS 'Unique token for session identification';
