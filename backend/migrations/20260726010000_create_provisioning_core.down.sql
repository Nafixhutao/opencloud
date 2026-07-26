-- Development-only rollback for the latest additive Phase 2 migration.
-- Production remains forward-only.
DROP TABLE jobs;
DROP TABLE sites;
DROP TABLE nodes;
