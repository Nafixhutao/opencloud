-- Development-only rollback for the latest additive Phase 2 migration.
-- Production remains forward-only. Database lifecycle jobs belong exclusively
-- to the tables introduced by the matching up migration.
DROP INDEX idx_jobs_active_database_kind;

DELETE FROM jobs
WHERE kind IN ('provision_database', 'delete_database', 'cleanup_database');

ALTER TABLE jobs DROP CONSTRAINT jobs_kind_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_kind_check
    CHECK (kind IN (
        'provision_site', 'delete_site', 'suspend_site',
        'resume_site', 'cleanup_site', 'reconcile_site'
    ));

DROP TABLE database_credentials;
DROP TABLE databases;
