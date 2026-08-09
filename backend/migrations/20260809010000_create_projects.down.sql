-- Development-only rollback for the latest additive Phase 4A migration.
-- Production remains forward-only and corrects schema defects with a new migration.
DROP TRIGGER deployment_events_append_only_delete ON deployment_events;
DROP TRIGGER deployment_events_append_only_update ON deployment_events;
DROP FUNCTION prevent_opencloud_deployment_event_mutation();
DROP TRIGGER deployments_identity_immutable ON deployments;
DROP FUNCTION prevent_opencloud_deployment_identity_update();
DROP TABLE deployment_events;
DROP TABLE deployments;
DROP TABLE services;
DROP TABLE projects;
