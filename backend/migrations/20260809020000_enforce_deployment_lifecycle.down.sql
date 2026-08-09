-- Development-only rollback for the newest Phase 4D migration.
DROP TRIGGER deployments_lifecycle_guard ON deployments;
DROP FUNCTION enforce_opencloud_deployment_lifecycle();
