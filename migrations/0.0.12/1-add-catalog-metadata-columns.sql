-- Add metadata columns to _schema_catalogs for runtime catalog info
-- (source_type, prefix, as_module, read_only) so that catalogs without
-- a data_sources row (core, storage, cache, etc.) still expose this metadata.
ALTER TABLE _schema_catalogs ADD COLUMN IF NOT EXISTS source_type VARCHAR DEFAULT '';
ALTER TABLE _schema_catalogs ADD COLUMN IF NOT EXISTS prefix VARCHAR DEFAULT '';
ALTER TABLE _schema_catalogs ADD COLUMN IF NOT EXISTS as_module BOOLEAN DEFAULT FALSE;
ALTER TABLE _schema_catalogs ADD COLUMN IF NOT EXISTS read_only BOOLEAN DEFAULT FALSE;

-- Add is_pk flag to _schema_fields for primary key detection.
ALTER TABLE _schema_fields ADD COLUMN IF NOT EXISTS is_pk BOOLEAN DEFAULT FALSE;

-- Cluster node registry
CREATE TABLE IF NOT EXISTS _cluster_nodes (
    name VARCHAR PRIMARY KEY,
    url VARCHAR NOT NULL,
    role VARCHAR NOT NULL,
    version VARCHAR,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_heartbeat TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Schema version counter for cluster change detection
INSERT INTO _schema_settings (key, value)
VALUES ('schema_version', '"0"')
ON CONFLICT (key) DO NOTHING;

-- Update version
UPDATE "version" SET "version" = '0.0.12';
