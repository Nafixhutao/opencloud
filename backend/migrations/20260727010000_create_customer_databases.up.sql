-- Phase 2 customer database lifecycle. This migration is additive: every
-- previously shipped migration remains immutable and production is
-- forward-only.
CREATE TABLE databases (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id             UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    name                   TEXT NOT NULL
                           CHECK (name ~ '^[a-z][a-z0-9_-]{0,62}$'),
    engine                 TEXT NOT NULL
                           CHECK (engine IN ('postgres', 'mariadb')),
    physical_database_name TEXT NOT NULL UNIQUE
                           CHECK (physical_database_name ~ '^ocdb_[0-9a-f]{32}$'),
    physical_username      TEXT NOT NULL UNIQUE
                           CHECK (physical_username ~ '^ocu_[0-9a-f]{32}$'),
    status                 TEXT NOT NULL DEFAULT 'provisioning'
                           CHECK (status IN (
                               'provisioning', 'active', 'deleting',
                               'deleted', 'failed'
                           )),
    idempotency_key        TEXT NOT NULL
                           CHECK (length(idempotency_key) BETWEEN 1 AND 128),
    last_error             TEXT,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at             TIMESTAMPTZ,
    CHECK ((status = 'deleted') = (deleted_at IS NOT NULL))
);

CREATE INDEX idx_databases_account_created
    ON databases (account_id, created_at DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_databases_status
    ON databases (status, updated_at);
CREATE UNIQUE INDEX idx_databases_account_name
    ON databases (account_id, lower(name))
    WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_databases_account_idempotency
    ON databases (account_id, idempotency_key);

-- The encrypted blob contains a versioned AES-256-GCM envelope. Plaintext
-- credentials never enter PostgreSQL, jobs, audit metadata, or logs. Deleting
-- this row is the durable one-time-reveal marker.
CREATE TABLE database_credentials (
    database_id UUID PRIMARY KEY REFERENCES databases(id) ON DELETE CASCADE,
    ciphertext  BYTEA NOT NULL CHECK (octet_length(ciphertext) >= 33),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check
    CHECK (kind IN (
        'provision_site', 'delete_site', 'suspend_site',
        'resume_site', 'cleanup_site', 'reconcile_site',
        'provision_database', 'delete_database', 'cleanup_database'
    ));

-- Only one pending/running operation of a given kind may exist for a managed
-- database. Payloads contain a server-generated UUID and never credentials.
CREATE UNIQUE INDEX idx_jobs_active_database_kind
    ON jobs (kind, ((payload ->> 'database_id')))
    WHERE status IN ('queued', 'running')
      AND payload ? 'database_id';
