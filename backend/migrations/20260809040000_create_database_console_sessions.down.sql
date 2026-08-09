-- Migration: 20260809040000_create_database_console_sessions
-- Description: Rollback database_console_sessions table creation
-- Note: DROP TABLE CASCADE removes all related data

DROP TABLE IF EXISTS database_console_sessions CASCADE;
