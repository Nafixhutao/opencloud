-- Phase 4A project control-plane state. This migration is additive: existing
-- sites remain independent legacy workloads until an explicit import path lands.
CREATE TABLE projects (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id      UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    name            TEXT NOT NULL
                    CHECK (length(btrim(name)) BETWEEN 1 AND 100),
    status          TEXT NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active', 'archived', 'deleted')),
    idempotency_key TEXT
                    CHECK (
                        idempotency_key IS NULL OR
                        length(idempotency_key) BETWEEN 1 AND 128
                    ),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ,
    CONSTRAINT projects_deleted_status_check
        CHECK ((status = 'deleted') = (deleted_at IS NOT NULL)),
    CONSTRAINT projects_id_account_id_unique UNIQUE (id, account_id)
);

CREATE INDEX idx_projects_account_created
    ON projects (account_id, created_at DESC)
    WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_projects_account_name_live
    ON projects (account_id, lower(name))
    WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_projects_account_idempotency
    ON projects (account_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE TABLE services (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id      UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    project_id      UUID NOT NULL,
    name            TEXT NOT NULL
                    CHECK (name ~ '^[a-z][a-z0-9-]{0,62}$'),
    service_type    TEXT NOT NULL
                    CHECK (service_type IN ('web', 'worker', 'cron', 'static')),
    source_root     TEXT NOT NULL DEFAULT '.'
                    CHECK (
                        source_root <> '' AND
                        source_root !~ '^/' AND
                        source_root !~ '(^|/)\\.\\.(/|$)'
                    ),
    status          TEXT NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active', 'disabled', 'deleted')),
    idempotency_key TEXT
                    CHECK (
                        idempotency_key IS NULL OR
                        length(idempotency_key) BETWEEN 1 AND 128
                    ),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ,
    CONSTRAINT services_project_account_fk
        FOREIGN KEY (project_id, account_id)
        REFERENCES projects (id, account_id) ON DELETE CASCADE,
    CONSTRAINT services_deleted_status_check
        CHECK ((status = 'deleted') = (deleted_at IS NOT NULL)),
    CONSTRAINT services_id_project_account_unique UNIQUE (id, project_id, account_id)
);

CREATE INDEX idx_services_project_created
    ON services (account_id, project_id, created_at DESC)
    WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_services_project_name_live
    ON services (account_id, project_id, lower(name))
    WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_services_project_idempotency
    ON services (account_id, project_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE TABLE deployments (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id       UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    project_id       UUID NOT NULL,
    service_id       UUID NOT NULL,
    revision         INTEGER NOT NULL CHECK (revision > 0),
    image_reference  TEXT NOT NULL CHECK (length(image_reference) BETWEEN 1 AND 512),
    image_digest     TEXT NOT NULL CHECK (image_digest ~ '^sha256:[0-9a-f]{64}$'),
    image_size_bytes BIGINT CHECK (image_size_bytes >= 0),
    build_provider   TEXT NOT NULL CHECK (length(build_provider) BETWEEN 1 AND 100),
    source_revision  TEXT,
    status           TEXT NOT NULL
                     CHECK (status IN (
                         'queued', 'cloning', 'detecting', 'planning', 'building',
                         'pushing', 'scanning', 'deploying', 'ready', 'failed', 'cancelled'
                     )),
    is_active        BOOLEAN NOT NULL DEFAULT false,
    last_error       TEXT,
    started_at       TIMESTAMPTZ,
    ready_at         TIMESTAMPTZ,
    completed_at     TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT deployments_service_project_account_fk
        FOREIGN KEY (service_id, project_id, account_id)
        REFERENCES services (id, project_id, account_id) ON DELETE CASCADE,
    CONSTRAINT deployments_terminal_timestamp_check
        CHECK (
            (status IN ('ready', 'failed', 'cancelled')) = (completed_at IS NOT NULL)
        ),
    CONSTRAINT deployments_ready_timestamp_check
        CHECK ((status = 'ready') = (ready_at IS NOT NULL)),
    CONSTRAINT deployments_active_ready_check
        CHECK (NOT is_active OR status = 'ready'),
    CONSTRAINT deployments_service_revision_unique UNIQUE (service_id, revision)
);

CREATE INDEX idx_deployments_service_created
    ON deployments (account_id, project_id, service_id, created_at DESC);
CREATE UNIQUE INDEX idx_deployments_active_service
    ON deployments (service_id)
    WHERE is_active;

CREATE TABLE deployment_events (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id    UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    project_id    UUID NOT NULL,
    service_id    UUID NOT NULL,
    deployment_id UUID NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    event_type    TEXT NOT NULL CHECK (length(event_type) BETWEEN 1 AND 100),
    message       TEXT NOT NULL CHECK (length(message) BETWEEN 1 AND 1000),
    metadata      JSONB NOT NULL DEFAULT '{}'::jsonb
                  CHECK (jsonb_typeof(metadata) = 'object'),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT deployment_events_service_project_account_fk
        FOREIGN KEY (service_id, project_id, account_id)
        REFERENCES services (id, project_id, account_id) ON DELETE CASCADE
);

CREATE INDEX idx_deployment_events_deployment_created
    ON deployment_events (account_id, project_id, service_id, deployment_id, created_at DESC);

-- Deployment artifacts identify immutable OCI revisions. Status may advance, but
-- a revision can never be repointed to a different image or source identity.
CREATE FUNCTION prevent_opencloud_deployment_identity_update() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.revision IS DISTINCT FROM OLD.revision OR
       NEW.image_reference IS DISTINCT FROM OLD.image_reference OR
       NEW.image_digest IS DISTINCT FROM OLD.image_digest OR
       NEW.image_size_bytes IS DISTINCT FROM OLD.image_size_bytes OR
       NEW.build_provider IS DISTINCT FROM OLD.build_provider OR
       NEW.source_revision IS DISTINCT FROM OLD.source_revision THEN
        RAISE EXCEPTION 'deployment identity is immutable';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER deployments_identity_immutable
BEFORE UPDATE ON deployments
FOR EACH ROW
EXECUTE FUNCTION prevent_opencloud_deployment_identity_update();

-- Deployment events are an append-only activity stream. Detailed errors remain
-- in internal logs; event metadata must contain only safe customer-facing facts.
CREATE FUNCTION prevent_opencloud_deployment_event_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'deployment events are append-only';
END;
$$;

CREATE TRIGGER deployment_events_append_only_update
BEFORE UPDATE ON deployment_events
FOR EACH ROW
EXECUTE FUNCTION prevent_opencloud_deployment_event_mutation();

CREATE TRIGGER deployment_events_append_only_delete
BEFORE DELETE ON deployment_events
FOR EACH ROW
EXECUTE FUNCTION prevent_opencloud_deployment_event_mutation();
