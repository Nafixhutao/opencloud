-- Phase 3 customer domains. This migration is additive and keeps every shipped
-- Phase 0-2 migration immutable.
BEGIN;

-- Hold the source table stable until its existing live hostnames are backfilled
-- and the claim trigger is installed. The explicit transaction also makes
-- direct psql application match Bun's all-or-nothing migration semantics.
-- Fail instead of waiting indefinitely when the required maintenance lock
-- cannot be acquired; operators retry only inside the declared window.
SET LOCAL lock_timeout = '5s';
LOCK TABLE sites IN ACCESS EXCLUSIVE MODE;

ALTER TABLE sites
    ADD CONSTRAINT sites_id_account_id_unique UNIQUE (id, account_id);
ALTER TABLE sites
    ADD COLUMN last_reconciled_at TIMESTAMPTZ;

CREATE TABLE domains (
    id                         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id                 UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    site_id                    UUID NOT NULL,
    hostname                   TEXT NOT NULL,
    status                     TEXT NOT NULL DEFAULT 'pending'
                               CHECK (status IN (
                                   'pending', 'verifying', 'dns_pending',
                                   'provisioning', 'active', 'failed',
                                   'deleting', 'deleted'
                               )),
    verification_token_digest BYTEA NOT NULL
                               CHECK (octet_length(verification_token_digest) = 32),
    verification_expires_at    TIMESTAMPTZ NOT NULL,
    verification_consumed_at   TIMESTAMPTZ,
    verified_at                TIMESTAMPTZ,
    dns_provider               TEXT NOT NULL DEFAULT 'manual'
                               CHECK (dns_provider IN ('manual', 'cloudflare')),
    dns_zone_id                TEXT,
    dns_record_ids             JSONB NOT NULL DEFAULT '[]'::jsonb
                               CHECK (jsonb_typeof(dns_record_ids) = 'array'),
    cert_status                TEXT NOT NULL DEFAULT 'none'
                               CHECK (cert_status IN (
                                   'none', 'issuing', 'active',
                                   'expiring', 'error'
                               )),
    cert_expires_at            TIMESTAMPTZ,
    cert_observed_at           TIMESTAMPTZ,
    cert_auto_renew            BOOLEAN NOT NULL DEFAULT true,
    last_reconciled_at         TIMESTAMPTZ,
    idempotency_key            TEXT CHECK (
                                   idempotency_key IS NULL OR
                                   length(idempotency_key) BETWEEN 1 AND 128
                               ),
    last_error                 TEXT,
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                 TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at                 TIMESTAMPTZ,
    CONSTRAINT domains_site_account_fk
        FOREIGN KEY (site_id, account_id)
        REFERENCES sites(id, account_id) ON DELETE CASCADE,
    CONSTRAINT domains_hostname_canonical_check CHECK (
        hostname = lower(hostname) AND
        hostname !~ '[[:space:]]' AND
        hostname !~ '\\.$' AND
        length(hostname) BETWEEN 3 AND 253 AND
        position('.' IN hostname) > 1
    ),
    CONSTRAINT domains_verification_consumption_check CHECK (
        (verified_at IS NULL AND verification_consumed_at IS NULL) OR
        (
            verified_at IS NOT NULL AND
            verification_consumed_at IS NOT NULL AND
            verified_at = verification_consumed_at AND
            verification_consumed_at <= verification_expires_at
        )
    ),
    CONSTRAINT domains_verification_expiry_check CHECK (
        verification_expires_at > created_at
    ),
    CONSTRAINT domains_verified_status_check CHECK (
        status NOT IN ('dns_pending', 'provisioning', 'active') OR
        verification_consumed_at IS NOT NULL
    ),
    CONSTRAINT domains_certificate_state_check CHECK (
        (cert_status IN ('active', 'expiring')) = (cert_expires_at IS NOT NULL)
    ),
    CONSTRAINT domains_deleted_status_check CHECK (
        (status = 'deleted') = (deleted_at IS NOT NULL)
    )
);

-- One registry makes primary-site and verified custom-domain hostname claims
-- contend on the same unique key. Pending challenges deliberately do not claim
-- a global hostname: multiple tenants may prove the same public DNS name, but
-- exactly one verification transaction can consume its challenge and win.
-- Both ownership links are created with the table so no trigger event can
-- block a later ALTER during this transaction.
CREATE TABLE hostname_claims (
    hostname   TEXT PRIMARY KEY,
    site_id    UUID UNIQUE REFERENCES sites(id) ON DELETE CASCADE
               DEFERRABLE INITIALLY DEFERRED,
    domain_id  UUID UNIQUE REFERENCES domains(id) ON DELETE CASCADE
               DEFERRABLE INITIALLY DEFERRED,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (num_nonnulls(site_id, domain_id) = 1)
);

INSERT INTO hostname_claims (hostname, site_id)
SELECT domain, id
FROM sites
WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX idx_domains_account_hostname_live
    ON domains (account_id, hostname)
    WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX idx_domains_account_idempotency
    ON domains (account_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;
CREATE INDEX idx_domains_account_site_created
    ON domains (account_id, site_id, created_at DESC)
    WHERE deleted_at IS NULL;
CREATE INDEX idx_domains_site_account_fk
    ON domains (site_id, account_id);
CREATE INDEX idx_domains_caddy_permission
    ON domains (hostname)
    WHERE status = 'active'
      AND verified_at IS NOT NULL
      AND deleted_at IS NULL;
CREATE INDEX idx_domains_reconcile
    ON domains ((COALESCE(last_reconciled_at, created_at)))
    WHERE deleted_at IS NULL
      AND status IN (
          'verifying', 'dns_pending', 'provisioning',
          'active', 'deleting'
      );
CREATE INDEX idx_sites_reconcile
    ON sites ((COALESCE(last_reconciled_at, created_at)), id)
    WHERE deleted_at IS NULL
      AND status IN ('active', 'suspended', 'deleting');

CREATE FUNCTION sync_opencloud_site_hostname_claim() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    old_hostname TEXT;
    new_hostname TEXT;
BEGIN
    IF TG_OP <> 'INSERT' THEN
        old_hostname := OLD.domain;
        IF OLD.deleted_at IS NULL AND
           (
               TG_OP = 'DELETE' OR
               NEW.deleted_at IS NOT NULL OR
               old_hostname IS DISTINCT FROM NEW.domain
           ) THEN
            DELETE FROM hostname_claims
            WHERE hostname = old_hostname
              AND site_id = OLD.id;
        END IF;
    END IF;

    IF TG_OP <> 'DELETE' AND NEW.deleted_at IS NULL THEN
        new_hostname := NEW.domain;
        IF TG_OP = 'INSERT' OR
           OLD.deleted_at IS NOT NULL OR
           new_hostname IS DISTINCT FROM old_hostname THEN
            INSERT INTO hostname_claims (hostname, site_id)
            VALUES (new_hostname, NEW.id);
        END IF;
    END IF;

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;

CREATE FUNCTION sync_opencloud_domain_hostname_claim() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    old_claimed BOOLEAN := false;
    new_claimed BOOLEAN := false;
    claim_owner UUID;
BEGIN
    IF TG_OP <> 'INSERT' THEN
        old_claimed := OLD.deleted_at IS NULL
            AND OLD.verified_at IS NOT NULL
            AND OLD.verification_consumed_at IS NOT NULL;
    END IF;
    IF TG_OP <> 'DELETE' THEN
        new_claimed := NEW.deleted_at IS NULL
            AND NEW.verified_at IS NOT NULL
            AND NEW.verification_consumed_at IS NOT NULL;
    END IF;

    IF old_claimed AND (
        NOT new_claimed OR
        TG_OP = 'DELETE' OR
        NEW.hostname IS DISTINCT FROM OLD.hostname
    ) THEN
        DELETE FROM hostname_claims
        WHERE hostname = OLD.hostname AND domain_id = OLD.id;
    END IF;

    IF new_claimed AND (
        NOT old_claimed OR
        TG_OP = 'INSERT' OR
        NEW.hostname IS DISTINCT FROM OLD.hostname
    ) THEN
        INSERT INTO hostname_claims (hostname, domain_id)
        VALUES (NEW.hostname, NEW.id)
        ON CONFLICT (hostname) DO NOTHING;

        SELECT domain_id INTO claim_owner
        FROM hostname_claims
        WHERE hostname = NEW.hostname;
        IF claim_owner IS DISTINCT FROM NEW.id THEN
            RAISE EXCEPTION 'hostname claim is unavailable'
                USING ERRCODE = 'unique_violation',
                      CONSTRAINT = 'hostname_claims_pkey';
        END IF;
    END IF;

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER sites_hostname_claim
BEFORE INSERT OR UPDATE OF domain, deleted_at ON sites
FOR EACH ROW
EXECUTE FUNCTION sync_opencloud_site_hostname_claim();

CREATE TRIGGER domains_hostname_claim
BEFORE INSERT OR UPDATE OF
    hostname, deleted_at, verified_at, verification_consumed_at
ON domains
FOR EACH ROW
EXECUTE FUNCTION sync_opencloud_domain_hostname_claim();

CREATE TRIGGER sites_hostname_claim_delete
BEFORE DELETE ON sites
FOR EACH ROW
EXECUTE FUNCTION sync_opencloud_site_hostname_claim();

CREATE TRIGGER domains_hostname_claim_delete
BEFORE DELETE ON domains
FOR EACH ROW
EXECUTE FUNCTION sync_opencloud_domain_hostname_claim();

ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check
    CHECK (kind IN (
        'provision_site', 'delete_site', 'suspend_site',
        'resume_site', 'cleanup_site', 'reconcile_site',
        'provision_database', 'delete_database', 'cleanup_database',
        'verify_domain', 'provision_domain', 'deprovision_domain',
        'reconcile_domain', 'observe_domain_certificate'
    ));

CREATE UNIQUE INDEX idx_jobs_active_domain_kind
    ON jobs (kind, ((payload ->> 'domain_id')))
    WHERE status IN ('queued', 'running')
      AND payload ? 'domain_id';

COMMIT;
