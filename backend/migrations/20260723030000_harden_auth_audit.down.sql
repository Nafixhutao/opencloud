DROP TRIGGER IF EXISTS audit_logs_no_delete ON audit_logs;
DROP TRIGGER IF EXISTS audit_logs_no_update ON audit_logs;
DROP FUNCTION IF EXISTS prevent_audit_log_mutation();

DROP TABLE IF EXISTS auth_token_consumptions;
