-- Bun bootstraps the namespace only. Better Auth's migration API owns every
-- table and future schema change inside auth (ADR 0006).
CREATE SCHEMA IF NOT EXISTS auth;
