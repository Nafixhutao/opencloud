-- audit_logs: append-only trail of sensitive actions (SECURITY §12).
-- actor_id is better-auth's auth.user.id (text); no cross-schema FK.
CREATE TABLE audit_logs (
    id          BIGSERIAL PRIMARY KEY,
    account_id  UUID REFERENCES accounts(id) ON DELETE SET NULL,
    actor_id    TEXT,
    action      TEXT NOT NULL,
    target      TEXT,
    metadata    JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_audit_account_created
    ON audit_logs (account_id, created_at DESC);

CREATE INDEX idx_audit_actor_created
    ON audit_logs (actor_id, created_at DESC);

CREATE INDEX idx_audit_action_created
    ON audit_logs (action, created_at DESC);
