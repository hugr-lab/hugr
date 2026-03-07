-- Migration 0.0.10: Add indexes on _schema_* tables for read/reconcile query performance.
--
-- Covers the main non-PK columns used in WHERE/JOIN clauses by the DB provider:
--   read.go    — type loading (catalog, hugr_type, kind), field/arg JOINs on type_name
--   reconcile.go — module/data-object collection, dependency detection
--   drop.go    — cascade suspension, cleanup DELETEs

-- _schema_types: frequent filters in type_info CTE and reconcile queries
CREATE INDEX IF NOT EXISTS idx_schema_types_catalog   ON {{ if isAttachedDuckdb }}core.{{ end }}_schema_types (catalog);
CREATE INDEX IF NOT EXISTS idx_schema_types_hugr_type ON {{ if isAttachedDuckdb }}core.{{ end }}_schema_types (hugr_type);
CREATE INDEX IF NOT EXISTS idx_schema_types_kind      ON {{ if isAttachedDuckdb }}core.{{ end }}_schema_types (kind);

-- _schema_fields: FK-like lookups on type_name (JOINs in type loading CTE),
-- field filtering, dependency detection, reconcile scans
CREATE INDEX IF NOT EXISTS idx_schema_fields_type_name          ON {{ if isAttachedDuckdb }}core.{{ end }}_schema_fields (type_name);
CREATE INDEX IF NOT EXISTS idx_schema_fields_catalog            ON {{ if isAttachedDuckdb }}core.{{ end }}_schema_fields (catalog);
CREATE INDEX IF NOT EXISTS idx_schema_fields_hugr_type          ON {{ if isAttachedDuckdb }}core.{{ end }}_schema_fields (hugr_type);
CREATE INDEX IF NOT EXISTS idx_schema_fields_dependency_catalog ON {{ if isAttachedDuckdb }}core.{{ end }}_schema_fields (dependency_catalog);

-- _schema_arguments: FK-like lookups on type_name and (type_name, field_name)
-- used in field_args CTE and reconcile queries
CREATE INDEX IF NOT EXISTS idx_schema_args_type_name            ON {{ if isAttachedDuckdb }}core.{{ end }}_schema_arguments (type_name);
CREATE INDEX IF NOT EXISTS idx_schema_args_type_field           ON {{ if isAttachedDuckdb }}core.{{ end }}_schema_arguments (type_name, field_name);

-- _schema_enum_values: FK-like lookups on type_name in enum_vals CTE
CREATE INDEX IF NOT EXISTS idx_schema_enumvals_type_name ON {{ if isAttachedDuckdb }}core.{{ end }}_schema_enum_values (type_name);

-- _schema_module_type_catalogs: PK is (type_name, catalog_name); catalog_name-only lookups
-- used by reconcile cleanup DELETE and cascade module resolution need a separate index.
CREATE INDEX IF NOT EXISTS idx_schema_mtc_catalog_name ON {{ if isAttachedDuckdb }}core.{{ end }}_schema_module_type_catalogs (catalog_name);

-- _schema_data_object_queries: PK is (name, object_name); cleanup DELETEs filter by object_name.
CREATE INDEX IF NOT EXISTS idx_schema_doq_object_name ON {{ if isAttachedDuckdb }}core.{{ end }}_schema_data_object_queries (object_name);

-- _schema_catalog_dependencies: PK is (catalog_name, depends_on); cascade suspension
-- queries filter by depends_on (reverse lookup).
CREATE INDEX IF NOT EXISTS idx_schema_catdeps_depends_on ON {{ if isAttachedDuckdb }}core.{{ end }}_schema_catalog_dependencies (depends_on);
