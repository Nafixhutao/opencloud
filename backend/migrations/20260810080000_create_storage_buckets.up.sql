-- storage_buckets: tenant-owned desired-state bucket metadata
CREATE TABLE storage_buckets (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id              UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    project_id              UUID NOT NULL,
    name                    TEXT NOT NULL CONSTRAINT buckets_name_valid CHECK (
        btrim(name) = name AND
        length(btrim(name)) BETWEEN 1 AND 63 AND
        name ~ '^[a-z][a-z0-9-]{0,62}$'
    ),
    physical_name           TEXT NOT NULL UNIQUE,  -- server-generated: ocb-<uuidhex>, implicit btree index
    visibility              TEXT NOT NULL DEFAULT 'private'
        CHECK (visibility IN ('public', 'private')),
    status                  TEXT NOT NULL DEFAULT 'creating'
        CHECK (status IN ('creating', 'active', 'deleting', 'deleted', 'failed')),
    storage_limit_bytes     BIGINT NOT NULL DEFAULT 1073741824
        CHECK (storage_limit_bytes > 0),
    bytes_used              BIGINT NOT NULL DEFAULT 0
        CHECK (bytes_used >= 0),
    object_count            BIGINT NOT NULL DEFAULT 0
        CHECK (object_count >= 0),
    max_object_size_bytes   BIGINT NOT NULL DEFAULT 104857600
        CHECK (max_object_size_bytes > 0 AND max_object_size_bytes <= storage_limit_bytes),
    allowed_mime_types      JSONB NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(allowed_mime_types) = 'array'),
    last_error              TEXT,
    last_reconciled_at      TIMESTAMPTZ,
    idempotency_key         TEXT
        CHECK (idempotency_key IS NULL OR length(idempotency_key) BETWEEN 1 AND 128),
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at              TIMESTAMPTZ,
    CONSTRAINT buckets_deleted_status_check
        CHECK ((status = 'deleted') = (deleted_at IS NOT NULL)),
    -- Tenant-safe composite FK ensuring delete/restrict at both account and project level
    CONSTRAINT buckets_project_account_fk
        FOREIGN KEY (project_id, account_id)
        REFERENCES projects (id, account_id) ON DELETE RESTRICT,
    CONSTRAINT buckets_id_project_account_unique
        UNIQUE (id, project_id, account_id)
);

-- Index for project-scoped idempotency lookup (partial index where key is present)
CREATE UNIQUE INDEX idx_storage_buckets_account_project_idempotency
    ON storage_buckets (account_id, project_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

-- Indexes for tenant-scoped queries
CREATE UNIQUE INDEX idx_storage_buckets_account_project_name_live
    ON storage_buckets (account_id, project_id, lower(name))
    WHERE deleted_at IS NULL;

CREATE INDEX idx_storage_buckets_account_project_created
    ON storage_buckets (account_id, project_id, created_at DESC)
    WHERE deleted_at IS NULL;

-- Extend the jobs kind whitelist with storage bucket operations (see
-- 20260727010000 and 20260730010000 for the same pattern).
ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check
    CHECK (kind IN (
        'provision_site', 'delete_site', 'suspend_site',
        'resume_site', 'cleanup_site', 'reconcile_site',
        'provision_database', 'delete_database', 'cleanup_database',
        'verify_domain', 'provision_domain', 'deprovision_domain',
        'reconcile_domain', 'observe_domain_certificate',
        'provision_storage_bucket', 'delete_storage_bucket',
        'reconcile_storage_bucket'
    ));

-- IMPORTANT OPERATIONAL WARNING:
-- ON DELETE RESTRICT on both account_id and project_id prevents deletion of accounts/projects
-- that have existing storage buckets. This protects against orphaning physical RustFS/S3 buckets.
-- Bucket cleanup must be explicit: customers must delete all buckets before deleting account/project.
-- Future slices will implement:
-- 1. Pre-deletion validation for accounts/projects with existing storage buckets
-- 2. Async cleanup jobs that invoke provider.DeleteBucket() safely during account/project deletion flow
-- 3. Operator dashboards showing orphaned resources if physical cleanup fails
