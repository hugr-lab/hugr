-- Add can_impersonate column to roles table for perm-based impersonation authorization.
-- Defaults to FALSE (secure by default). Admin role is granted impersonation capability.
-- NOT NULL only on PostgreSQL: DuckDB cannot ADD COLUMN with constraints
-- ("Adding columns with constraints not yet supported") — this file used to
-- hard-fail on DuckDB CoreDBs and block ALL further migrations there.
ALTER TABLE roles ADD COLUMN IF NOT EXISTS can_impersonate BOOLEAN {{ if isPostgres }}NOT NULL {{ end }}DEFAULT FALSE;
UPDATE roles SET can_impersonate = TRUE WHERE name = 'admin';


UPDATE "version" SET "version" = '0.0.18';