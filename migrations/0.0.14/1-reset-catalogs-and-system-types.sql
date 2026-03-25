-- Migration 0.0.14: Reset catalog versions and system types for recompilation.
-- Forces full schema recompilation after system type and extension fixes.

-- Reset all catalog versions to force recompilation
UPDATE _schema_catalogs SET version = '';

-- Reset system types version to force regeneration
DELETE FROM _schema_settings WHERE key = 'system_types_version';

-- Update version
UPDATE "version" SET "version" = '0.0.14';
