-- Add can_impersonate column to roles table for perm-based impersonation authorization.
-- Defaults to FALSE (secure by default). Admin role is granted impersonation capability.
ALTER TABLE roles ADD COLUMN IF NOT EXISTS can_impersonate BOOLEAN NOT NULL DEFAULT FALSE;
UPDATE roles SET can_impersonate = TRUE WHERE name = 'admin';


UPDATE "version" SET "version" = '0.0.18';