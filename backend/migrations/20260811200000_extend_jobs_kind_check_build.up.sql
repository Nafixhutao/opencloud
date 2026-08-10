ALTER TABLE jobs DROP CONSTRAINT IF EXISTS jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check
    CHECK (kind IN (
        'provision_site', 'delete_site', 'suspend_site',
        'resume_site', 'cleanup_site', 'reconcile_site',
        'provision_database', 'delete_database', 'cleanup_database',
        'verify_domain', 'provision_domain', 'deprovision_domain',
        'reconcile_domain', 'observe_domain_certificate',
        'provision_storage_bucket', 'delete_storage_bucket',
        'reconcile_storage_bucket',
        'clone_git_source', 'build_source',
        'deploy_preview', 'destroy_preview'
    ));
