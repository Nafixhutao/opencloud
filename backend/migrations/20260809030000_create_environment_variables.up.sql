-- Phase 4H environment variables and secrets. Variables are tenant-scoped,
-- service-scoped, and environment-scoped (production/preview/development).
-- Secrets are encrypted at rest and never logged.
CREATE TABLE environment_variables (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id      UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    project_id      UUID NOT NULL,
    service_id      UUID NOT NULL,
    key             TEXT NOT NULL
                    CHECK (key ~ '^[A-Z][A-Z0-9_]{0,127}$'),
    value           TEXT,
    is_secret       BOOLEAN NOT NULL DEFAULT false,
    encrypted_value BYTEA,
    environment     TEXT NOT NULL DEFAULT 'production'
                    CHECK (environment IN ('production', 'preview', 'development')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_by      TEXT NOT NULL,
    CONSTRAINT environment_variables_service_project_account_fk
        FOREIGN KEY (service_id, project_id, account_id)
        REFERENCES services (id, project_id, account_id) ON DELETE CASCADE,
    CONSTRAINT environment_variables_value_check
        CHECK (
            (is_secret = false AND value IS NOT NULL AND encrypted_value IS NULL) OR
            (is_secret = true AND value IS NULL AND encrypted_value IS NOT NULL)
        ),
    CONSTRAINT environment_variables_service_env_key_unique
        UNIQUE (service_id, environment, key)
);

CREATE INDEX idx_environment_variables_service_env
    ON environment_variables (account_id, project_id, service_id, environment, created_at DESC);

-- Environment variable audit trail for rotation and access tracking.
CREATE TABLE environment_variable_audit (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id   UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    project_id   UUID NOT NULL,
    service_id   UUID NOT NULL,
    variable_id  UUID REFERENCES environment_variables(id) ON DELETE SET NULL,
    action       TEXT NOT NULL
                 CHECK (action IN ('created', 'updated', 'deleted', 'revealed', 'rotated')),
    key          TEXT NOT NULL,
    is_secret    BOOLEAN NOT NULL,
    environment  TEXT NOT NULL,
    actor_id     TEXT NOT NULL,
    metadata     JSONB NOT NULL DEFAULT '{}'::jsonb
                 CHECK (jsonb_typeof(metadata) = 'object'),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT environment_variable_audit_service_project_account_fk
        FOREIGN KEY (service_id, project_id, account_id)
        REFERENCES services (id, project_id, account_id) ON DELETE CASCADE
);

CREATE INDEX idx_environment_variable_audit_service
    ON environment_variable_audit (account_id, project_id, service_id, created_at DESC);
CREATE INDEX idx_environment_variable_audit_variable
    ON environment_variable_audit (variable_id, created_at DESC)
    WHERE variable_id IS NOT NULL;

-- Audit records are append-only to preserve the complete access trail. The
-- single allowed mutation is the variable_id SET NULL fired by the FK cascade
-- when an environment variable is deleted; every other column is immutable.
CREATE FUNCTION prevent_opencloud_env_audit_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'UPDATE'
       AND NEW.variable_id IS NULL
       AND OLD.variable_id IS NOT NULL
       AND NEW.account_id = OLD.account_id
       AND NEW.project_id = OLD.project_id
       AND NEW.service_id = OLD.service_id
       AND NEW.action = OLD.action
       AND NEW.key = OLD.key
       AND NEW.is_secret = OLD.is_secret
       AND NEW.environment = OLD.environment
       AND NEW.actor_id = OLD.actor_id
       AND NEW.metadata = OLD.metadata
       AND NEW.created_at = OLD.created_at THEN
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'environment variable audit records are append-only';
END;
$$;

CREATE TRIGGER environment_variable_audit_append_only_update
BEFORE UPDATE ON environment_variable_audit
FOR EACH ROW
EXECUTE FUNCTION prevent_opencloud_env_audit_mutation();

CREATE TRIGGER environment_variable_audit_append_only_delete
BEFORE DELETE ON environment_variable_audit
FOR EACH ROW
EXECUTE FUNCTION prevent_opencloud_env_audit_mutation();
