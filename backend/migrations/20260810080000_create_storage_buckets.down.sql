-- Restore the pre-storage-bucket jobs kind whitelist.
ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check
    CHECK (kind IN (
        'provision_site', 'delete_site', 'suspend_site',
        'resume_site', 'cleanup_site', 'reconcile_site',
        'provision_database', 'delete_database', 'cleanup_database',
        'verify_domain', 'provision_domain', 'deprovision_domain',
        'reconcile_domain', 'observe_domain_certificate'
    ));

DROP INDEX IF EXISTS idx_storage_buckets_account_project_idempotency;
DROP INDEX IF EXISTS idx_storage_buckets_account_project_name_live;
DROP INDEX IF EXISTS idx_storage_buckets_account_project_created;
DROP TABLE IF EXISTS storage_buckets;
