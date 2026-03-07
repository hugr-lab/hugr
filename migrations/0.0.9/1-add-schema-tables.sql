-- Migration 0.0.9: Add _schema_* tables for DB-backed schema provider.
-- FK constraints are intentionally omitted for DuckDB compatibility;
-- referential integrity is maintained at the application level.

{{ if isPostgres }}CREATE EXTENSION IF NOT EXISTS vector;{{ end }}

CREATE TABLE IF NOT EXISTS {{ if isAttachedDuckdb }}core.{{ end }}_schema_catalogs (
    name VARCHAR NOT NULL PRIMARY KEY,
    version VARCHAR NOT NULL DEFAULT '',
    description VARCHAR NOT NULL DEFAULT '',
    long_description VARCHAR NOT NULL DEFAULT '',
    is_summarized BOOLEAN NOT NULL DEFAULT FALSE,
    disabled BOOLEAN NOT NULL DEFAULT FALSE,
    suspended BOOLEAN NOT NULL DEFAULT FALSE,
    vec {{if isPostgres }} vector({{ .VectorSize }}) {{ else }} FLOAT[{{ .VectorSize }}] {{ end }}
);

CREATE TABLE IF NOT EXISTS {{ if isAttachedDuckdb }}core.{{ end }}_schema_catalog_dependencies (
    catalog_name VARCHAR NOT NULL,
    depends_on VARCHAR NOT NULL,
    PRIMARY KEY (catalog_name, depends_on)
);

CREATE TABLE IF NOT EXISTS {{ if isAttachedDuckdb }}core.{{ end }}_schema_types (
    name VARCHAR NOT NULL PRIMARY KEY,
    kind VARCHAR NOT NULL,
    description VARCHAR NOT NULL DEFAULT '',
    long_description VARCHAR NOT NULL DEFAULT '',
    hugr_type VARCHAR NOT NULL DEFAULT '',
    module VARCHAR NOT NULL DEFAULT '',
    catalog VARCHAR,
    directives {{if isPostgres }} JSONB {{ else }} JSON {{ end }} NOT NULL DEFAULT '[]',
    interfaces VARCHAR NOT NULL DEFAULT '',
    union_types VARCHAR NOT NULL DEFAULT '',
    is_summarized BOOLEAN NOT NULL DEFAULT FALSE,
    vec {{if isPostgres }} vector({{ .VectorSize }}) {{ else }} FLOAT[{{ .VectorSize }}] {{ end }}
);

CREATE TABLE IF NOT EXISTS {{ if isAttachedDuckdb }}core.{{ end }}_schema_fields (
    type_name VARCHAR NOT NULL,
    name VARCHAR NOT NULL,
    field_type VARCHAR NOT NULL,
    description VARCHAR NOT NULL DEFAULT '',
    long_description VARCHAR NOT NULL DEFAULT '',
    hugr_type VARCHAR NOT NULL DEFAULT '',
    catalog VARCHAR,
    dependency_catalog VARCHAR,
    directives {{if isPostgres }} JSONB {{ else }} JSON {{ end }} NOT NULL DEFAULT '[]',
    is_summarized BOOLEAN NOT NULL DEFAULT FALSE,
    vec {{if isPostgres }} vector({{ .VectorSize }}) {{ else }} FLOAT[{{ .VectorSize }}] {{ end }},
    ordinal INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (type_name, name)
);

CREATE TABLE IF NOT EXISTS {{ if isAttachedDuckdb }}core.{{ end }}_schema_arguments (
    type_name VARCHAR NOT NULL,
    field_name VARCHAR NOT NULL,
    name VARCHAR NOT NULL,
    arg_type VARCHAR NOT NULL,
    default_value VARCHAR,
    description VARCHAR NOT NULL DEFAULT '',
    directives {{if isPostgres }} JSONB {{ else }} JSON {{ end }} NOT NULL DEFAULT '[]',
    ordinal INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (type_name, field_name, name)
);

CREATE TABLE IF NOT EXISTS {{ if isAttachedDuckdb }}core.{{ end }}_schema_enum_values (
    type_name VARCHAR NOT NULL,
    name VARCHAR NOT NULL,
    description VARCHAR NOT NULL DEFAULT '',
    directives {{if isPostgres }} JSONB {{ else }} JSON {{ end }} NOT NULL DEFAULT '[]',
    ordinal INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (type_name, name)
);

CREATE TABLE IF NOT EXISTS {{ if isAttachedDuckdb }}core.{{ end }}_schema_directives (
    name VARCHAR NOT NULL PRIMARY KEY,
    description VARCHAR NOT NULL DEFAULT '',
    locations VARCHAR NOT NULL DEFAULT '', -- pipe-separated: e.g. "FIELD_DEFINITION|ARGUMENT_DEFINITION"
    is_repeatable BOOLEAN NOT NULL DEFAULT FALSE,
    arguments VARCHAR NOT NULL DEFAULT '[]' -- JSON array of argument definitions
);

CREATE TABLE IF NOT EXISTS {{ if isAttachedDuckdb }}core.{{ end }}_schema_modules (
    name VARCHAR NOT NULL PRIMARY KEY,
    description VARCHAR NOT NULL DEFAULT '',
    long_description VARCHAR NOT NULL DEFAULT '',
    query_root VARCHAR,
    mutation_root VARCHAR,
    function_root VARCHAR,
    mut_function_root VARCHAR,
    is_summarized BOOLEAN NOT NULL DEFAULT FALSE,
    disabled BOOLEAN NOT NULL DEFAULT FALSE,
    vec {{if isPostgres }} vector({{ .VectorSize }}) {{ else }} FLOAT[{{ .VectorSize }}] {{ end }}
);

CREATE TABLE IF NOT EXISTS {{ if isAttachedDuckdb }}core.{{ end }}_schema_module_type_catalogs (
    module_name VARCHAR NOT NULL,
    type_name VARCHAR NOT NULL,
    catalog_name VARCHAR NOT NULL,
    PRIMARY KEY (type_name, catalog_name)
);

CREATE TABLE IF NOT EXISTS {{ if isAttachedDuckdb }}core.{{ end }}_schema_data_objects (
    name VARCHAR NOT NULL PRIMARY KEY,
    filter_type_name VARCHAR,
    args_type_name VARCHAR
);

CREATE TABLE IF NOT EXISTS {{ if isAttachedDuckdb }}core.{{ end }}_schema_data_object_queries (
    name VARCHAR NOT NULL,
    object_name VARCHAR NOT NULL,
    query_root VARCHAR NOT NULL,
    query_type VARCHAR NOT NULL,
    PRIMARY KEY (name, object_name)
);

CREATE TABLE IF NOT EXISTS {{ if isAttachedDuckdb }}core.{{ end }}_schema_settings (
    key VARCHAR NOT NULL PRIMARY KEY,
    value {{if isPostgres }} JSONB {{ else }} JSON {{ end }} NOT NULL
);

{{ if isPostgres }}
CREATE INDEX IF NOT EXISTS _schema_catalogs_vec_idx ON _schema_catalogs USING hnsw (vec vector_cosine_ops);
CREATE INDEX IF NOT EXISTS _schema_types_vec_idx ON _schema_types USING hnsw (vec vector_cosine_ops);
CREATE INDEX IF NOT EXISTS _schema_fields_vec_idx ON _schema_fields USING hnsw (vec vector_cosine_ops);
CREATE INDEX IF NOT EXISTS _schema_modules_vec_idx ON _schema_modules USING hnsw (vec vector_cosine_ops);
{{ end }}
