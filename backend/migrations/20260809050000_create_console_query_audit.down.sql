-- Migration: 20260809050000_create_console_query_audit
-- Description: Rollback console_query_audit table

DROP TABLE IF EXISTS console_query_audit CASCADE;
