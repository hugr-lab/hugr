-- Migration 0.0.20 (2): drop the compiled-schema storage (design 036).
--
-- The engine no longer has a second catalog storage. A source's logical model
-- lives in catalog.* and the served GraphQL schema is generated from it on
-- read, so these eleven tables have no reader and no writer left. Everything
-- curated in them that the entity model can address was carried over by
-- 1-backfill-annotations.sql — which MUST run first, and does, by filename.
--
-- _schema_settings is NOT dropped. Despite the name it is not compiled-schema
-- data: it carries the schema_version counter that cluster workers poll and the
-- catalog storage reads and writes (store/manager.go). Its config / vec_size
-- key becomes unused; the row is left in place.
--
-- IF EXISTS throughout: safe to re-run, and safe on a database that never had
-- the tables (a CoreDB created after this release).

DROP TABLE IF EXISTS {{ if isAttachedDuckdb }}core.{{ end }}_schema_arguments;
DROP TABLE IF EXISTS {{ if isAttachedDuckdb }}core.{{ end }}_schema_enum_values;
DROP TABLE IF EXISTS {{ if isAttachedDuckdb }}core.{{ end }}_schema_directives;
DROP TABLE IF EXISTS {{ if isAttachedDuckdb }}core.{{ end }}_schema_data_object_queries;
DROP TABLE IF EXISTS {{ if isAttachedDuckdb }}core.{{ end }}_schema_data_objects;
DROP TABLE IF EXISTS {{ if isAttachedDuckdb }}core.{{ end }}_schema_module_type_catalogs;
DROP TABLE IF EXISTS {{ if isAttachedDuckdb }}core.{{ end }}_schema_fields;
DROP TABLE IF EXISTS {{ if isAttachedDuckdb }}core.{{ end }}_schema_types;
DROP TABLE IF EXISTS {{ if isAttachedDuckdb }}core.{{ end }}_schema_modules;
DROP TABLE IF EXISTS {{ if isAttachedDuckdb }}core.{{ end }}_schema_catalog_dependencies;
DROP TABLE IF EXISTS {{ if isAttachedDuckdb }}core.{{ end }}_schema_catalogs;
