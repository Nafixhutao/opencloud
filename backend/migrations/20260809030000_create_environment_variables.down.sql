DROP TRIGGER IF EXISTS environment_variable_audit_append_only_delete ON environment_variable_audit;
DROP TRIGGER IF EXISTS environment_variable_audit_append_only_update ON environment_variable_audit;
DROP FUNCTION IF EXISTS prevent_opencloud_env_audit_mutation();
DROP TABLE IF EXISTS environment_variable_audit;
DROP TABLE IF EXISTS environment_variables;
