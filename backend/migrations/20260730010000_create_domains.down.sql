-- Development-only rollback of the latest additive Phase 3 migration.
BEGIN;

DROP INDEX idx_jobs_active_domain_kind;

DELETE FROM jobs
WHERE kind IN (
    'verify_domain', 'provision_domain', 'deprovision_domain',
    'reconcile_domain', 'observe_domain_certificate'
);

ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check
    CHECK (kind IN (
        'provision_site', 'delete_site', 'suspend_site',
        'resume_site', 'cleanup_site', 'reconcile_site',
        'provision_database', 'delete_database', 'cleanup_database'
    ));

DROP TRIGGER domains_hostname_claim_delete ON domains;
DROP TRIGGER sites_hostname_claim_delete ON sites;
DROP TRIGGER domains_hostname_claim ON domains;
DROP TRIGGER sites_hostname_claim ON sites;
DROP FUNCTION sync_opencloud_domain_hostname_claim();
DROP FUNCTION sync_opencloud_site_hostname_claim();
DROP TABLE hostname_claims;
DROP TABLE domains;
DROP INDEX idx_sites_reconcile;
ALTER TABLE sites DROP COLUMN last_reconciled_at;
ALTER TABLE sites DROP CONSTRAINT sites_id_account_id_unique;

COMMIT;
