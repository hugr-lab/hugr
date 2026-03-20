-- Migration 0.0.13: Force full schema recompilation.
-- Uses DROP + CREATE to handle potential DuckDB index corruption.
-- Required after fixes to catalog assignment for system/module types.

DROP TABLE IF EXISTS _schema_data_object_queries;
DROP TABLE IF EXISTS _schema_data_objects;
DROP TABLE IF EXISTS _schema_module_type_catalogs;
DROP TABLE IF EXISTS _schema_modules;
DROP TABLE IF EXISTS _schema_catalog_dependencies;
DROP TABLE IF EXISTS _schema_enum_values;
DROP TABLE IF EXISTS _schema_arguments;
DROP TABLE IF EXISTS _schema_fields;
DROP TABLE IF EXISTS _schema_types;
DROP TABLE IF EXISTS _schema_directives;
DROP TABLE IF EXISTS _schema_settings;
DROP TABLE IF EXISTS _schema_catalogs;

-- Recreate tables (from schema.sql)

CREATE TABLE _schema_catalogs (
    name VARCHAR NOT NULL PRIMARY KEY,
    version VARCHAR NOT NULL DEFAULT '',
    description VARCHAR NOT NULL DEFAULT '',
    long_description VARCHAR NOT NULL DEFAULT '',
    source_type VARCHAR NOT NULL DEFAULT '',
    prefix VARCHAR NOT NULL DEFAULT '',
    as_module BOOLEAN NOT NULL DEFAULT FALSE,
    read_only BOOLEAN NOT NULL DEFAULT FALSE,
    is_summarized BOOLEAN NOT NULL DEFAULT FALSE,
    disabled BOOLEAN NOT NULL DEFAULT FALSE,
    suspended BOOLEAN NOT NULL DEFAULT FALSE,
    vec {{if isPostgres }} vector({{ .VectorSize }}) {{ else }} FLOAT[{{ .VectorSize }}] {{ end }}
);

CREATE TABLE _schema_catalog_dependencies (
    catalog_name VARCHAR NOT NULL,
    depends_on VARCHAR NOT NULL,
    PRIMARY KEY (catalog_name, depends_on)
);

CREATE TABLE _schema_types (
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

CREATE TABLE _schema_fields (
    type_name VARCHAR NOT NULL,
    name VARCHAR NOT NULL,
    field_type VARCHAR NOT NULL,
    field_type_name VARCHAR NOT NULL DEFAULT '',
    description VARCHAR NOT NULL DEFAULT '',
    long_description VARCHAR NOT NULL DEFAULT '',
    hugr_type VARCHAR NOT NULL DEFAULT '',
    catalog VARCHAR,
    dependency_catalog VARCHAR,
    directives {{if isPostgres }} JSONB {{ else }} JSON {{ end }} NOT NULL DEFAULT '[]',
    is_pk BOOLEAN NOT NULL DEFAULT FALSE,
    is_summarized BOOLEAN NOT NULL DEFAULT FALSE,
    vec {{if isPostgres }} vector({{ .VectorSize }}) {{ else }} FLOAT[{{ .VectorSize }}] {{ end }},
    ordinal INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (type_name, name)
);

CREATE TABLE _schema_arguments (
    type_name VARCHAR NOT NULL,
    field_name VARCHAR NOT NULL,
    name VARCHAR NOT NULL,
    arg_type VARCHAR NOT NULL,
    arg_type_name VARCHAR NOT NULL DEFAULT '',
    default_value VARCHAR,
    description VARCHAR NOT NULL DEFAULT '',
    directives {{if isPostgres }} JSONB {{ else }} JSON {{ end }} NOT NULL DEFAULT '[]',
    ordinal INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (type_name, field_name, name)
);

CREATE TABLE _schema_enum_values (
    type_name VARCHAR NOT NULL,
    name VARCHAR NOT NULL,
    description VARCHAR NOT NULL DEFAULT '',
    directives {{if isPostgres }} JSONB {{ else }} JSON {{ end }} NOT NULL DEFAULT '[]',
    ordinal INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (type_name, name)
);

CREATE TABLE _schema_directives (
    name VARCHAR NOT NULL PRIMARY KEY,
    description VARCHAR NOT NULL DEFAULT '',
    locations VARCHAR NOT NULL DEFAULT '',
    is_repeatable BOOLEAN NOT NULL DEFAULT FALSE,
    arguments VARCHAR NOT NULL DEFAULT '[]'
);

CREATE TABLE _schema_modules (
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

CREATE TABLE _schema_module_type_catalogs (
    module_name VARCHAR NOT NULL,
    type_name VARCHAR NOT NULL,
    catalog_name VARCHAR NOT NULL,
    PRIMARY KEY (type_name, catalog_name)
);

CREATE TABLE _schema_data_objects (
    name VARCHAR NOT NULL PRIMARY KEY,
    filter_type_name VARCHAR,
    args_type_name VARCHAR
);

CREATE TABLE _schema_data_object_queries (
    name VARCHAR NOT NULL,
    object_name VARCHAR NOT NULL,
    query_root VARCHAR NOT NULL,
    query_type VARCHAR NOT NULL,
    PRIMARY KEY (name, object_name)
);

CREATE TABLE _schema_settings (
    key VARCHAR NOT NULL PRIMARY KEY,
    value {{if isPostgres }} JSONB {{ else }} JSON {{ end }} NOT NULL
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_schema_types_catalog   ON _schema_types (catalog);
CREATE INDEX IF NOT EXISTS idx_schema_types_hugr_type ON _schema_types (hugr_type);
CREATE INDEX IF NOT EXISTS idx_schema_types_kind      ON _schema_types (kind);
CREATE INDEX IF NOT EXISTS idx_schema_fields_type_name          ON _schema_fields (type_name);
CREATE INDEX IF NOT EXISTS idx_schema_fields_catalog            ON _schema_fields (catalog);
CREATE INDEX IF NOT EXISTS idx_schema_fields_hugr_type          ON _schema_fields (hugr_type);
CREATE INDEX IF NOT EXISTS idx_schema_fields_dependency_catalog ON _schema_fields (dependency_catalog);
CREATE INDEX IF NOT EXISTS idx_schema_args_type_name  ON _schema_arguments (type_name);
CREATE INDEX IF NOT EXISTS idx_schema_args_type_field ON _schema_arguments (type_name, field_name);
CREATE INDEX IF NOT EXISTS idx_schema_enumvals_type_name ON _schema_enum_values (type_name);
CREATE INDEX IF NOT EXISTS idx_schema_mtc_catalog_name ON _schema_module_type_catalogs (catalog_name);
CREATE INDEX IF NOT EXISTS idx_schema_doq_object_name ON _schema_data_object_queries (object_name);
CREATE INDEX IF NOT EXISTS idx_schema_catdeps_depends_on ON _schema_catalog_dependencies (depends_on);

{{ if isPostgres }}
CREATE INDEX IF NOT EXISTS _schema_catalogs_vec_idx ON _schema_catalogs USING hnsw (vec vector_cosine_ops);
CREATE INDEX IF NOT EXISTS _schema_types_vec_idx ON _schema_types USING hnsw (vec vector_cosine_ops);
CREATE INDEX IF NOT EXISTS _schema_fields_vec_idx ON _schema_fields USING hnsw (vec vector_cosine_ops);
CREATE INDEX IF NOT EXISTS _schema_modules_vec_idx ON _schema_modules USING hnsw (vec vector_cosine_ops);
{{ end }}

-- Schema version counter
INSERT INTO _schema_settings (key, value) VALUES ('schema_version', '"0"') ON CONFLICT (key) DO NOTHING;

-- Update version
UPDATE "version" SET "version" = '0.0.13';
