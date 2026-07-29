-- Migration 0.0.19: catalog entity namespace (design 034 — base-entity schema
-- storage + annotations overlay). FROZEN COPY of the query-engine template
-- pkg/data-sources/sources/runtime/core-db/hugr_catalog.sql — keep
-- column-for-column in sync. Additive; _schema_* untouched. Ships together
-- with the coredb.Version bump to 0.0.19.

-- catalog namespace (schema "catalog"): base-entity schema storage (design 034).
--
-- Delivery model (no runtime DDL):
--   * NEW databases  — this file is part of coredb.InitSchema, applied once by
--     the engine (applySchema on ErrDBIsNotInitialized) or by the hugr migrate
--     tool (initDB). Render contexts: attached DuckDB (tables core.catalog.*),
--     attached Postgres (via postgres_execute, schema catalog), direct DuckDB
--     and direct Postgres (hugr migrate).
--   * EXISTING databases — the hugr repo migration 0.0.19 (a frozen copy of
--     this file; keep column-for-column in sync) applied once on upgrade.
--
-- Runtime DML is IDENTICAL on both backends: INSERT ... ON CONFLICT executed
-- from the DuckDB context with property bags as plain JSON text. On DuckDB the
-- text casts implicitly into the native STRUCT columns; on Postgres the INOUT
-- assignment casts below make the same text assignable to JSONB and vector.
--
-- NO column DEFAULTs anywhere: the attached-Postgres write path (postgres
-- extension COPY) sends explicit NULLs for omitted columns, so PG defaults
-- never fire while DuckDB defaults would — the same statement would diverge.
-- Contract: the writer lists every NOT NULL column explicitly; nullable
-- columns may be omitted (NULL on both backends).
--
-- Shape notes (design 034 research):
--   * data_source_meta carries the STATE FLAGS loaded/disabled/suspended
--     (D26/G1: unload is a flag, rows stay; deletion only on unregister).
--     A pseudo-row ('_embedder', version='<hash>') fingerprints the embedder
--     config (G5).
--   * relations holds fk | m2m edges only (D17: @join and friends are
--     declared FIELDS — their bags live in fields.properties).
--   * NO visibility macro/function (D25: access control is ordinary
--     data-object permissions on the entity views).

CREATE SCHEMA IF NOT EXISTS {{ if isAttachedDuckdb }}core.{{ end }}catalog;

-- Data-source load state: version = writer-format prefix + the
-- catalogVersionWithOptions content hash (G4).
CREATE TABLE IF NOT EXISTS {{ if isAttachedDuckdb }}core.{{ end }}catalog.data_source_meta (
    data_source  VARCHAR NOT NULL PRIMARY KEY,
    version      VARCHAR NOT NULL,
    capabilities {{ if isPostgres }}JSONB{{ else }}JSON{{ end }},
    -- engine is the source's engine type string: @catalog(name, engine) — name
    -- IS the data source — is re-attached from here to every generated
    -- type/field at read time. read_only is a per-source OPTION (not an engine
    -- capability): it gates all mutation generation and the X_mut_* types.
    engine       VARCHAR NOT NULL,
    read_only    BOOLEAN NOT NULL,
    -- prefix is recorded for PROVENANCE only -- entity names are stored
    -- already prefixed (with @original_name preserved); nothing reads it back.
    -- as_module IS read at generation time: it makes the source a module and
    -- drives module query/mutation field naming.
    prefix       VARCHAR,
    as_module    BOOLEAN NOT NULL,
    -- is_extension marks an extension source: generated members of its
    -- objects (root fields, shared members, _join/_spatial) carry
    -- @dependency(name: <source>) so dependency tracking can drop them.
    is_extension BOOLEAN NOT NULL DEFAULT false,
    loaded       BOOLEAN NOT NULL,
    disabled     BOOLEAN NOT NULL,
    suspended    BOOLEAN NOT NULL,
    loaded_at    TIMESTAMP
);

-- parent is DERIVED from the dotted name by the writer at insert time
-- (a.b.c → a.b; NULL for top-level) — child selection is a plain equality.
CREATE TABLE IF NOT EXISTS {{ if isAttachedDuckdb }}core.{{ end }}catalog.modules (
    name        VARCHAR NOT NULL PRIMARY KEY,
    parent      VARCHAR,
    description VARCHAR
);

-- Module → contributing data sources, as a CLOSURE: a row lists a source that
-- backs data objects / functions / subscriptions in the module OR any of its
-- submodules (the chain includes the ROOT module '' — a WHERE module = ''
-- aggregate covers the whole tree). Maintained by the writer at insert time —
-- module visibility is then a plain semi-join against data_source_meta (a
-- module with no ACTIVE source is off) with no module-tree walk at read time.
--
-- The has_* flags record WHICH ROOT KINDS the source's members contribute to
-- the module subtree, so the logical model resolves a module's root kinds in
-- ONE bool_or aggregate. The mutation kinds are decided at READ time by
-- combining the flags with data_source_meta.read_only (a module whose sources
-- are all read-only has no mutations and no mutation functions):
--   query    = bool_or(has_data_objects)
--   mutation = bool_or(has_tables AND NOT read_only)
--   function / mut_function / subscription = bool_or(has_*_functions ...)
CREATE TABLE IF NOT EXISTS {{ if isAttachedDuckdb }}core.{{ end }}catalog.module_data_sources (
    module             VARCHAR NOT NULL,
    data_source        VARCHAR NOT NULL,
    has_data_objects   BOOLEAN NOT NULL,
    has_tables         BOOLEAN NOT NULL,
    has_functions      BOOLEAN NOT NULL,
    has_mut_functions  BOOLEAN NOT NULL,
    has_subscriptions  BOOLEAN NOT NULL,
    PRIMARY KEY (module, data_source)
);

CREATE TABLE IF NOT EXISTS {{ if isAttachedDuckdb }}core.{{ end }}catalog.data_objects (
    -- name is the COMPILED (prefixed) GraphQL type name; original_name is the
    -- source's unprefixed name, re-attached as @original_name on read.
    name          VARCHAR NOT NULL PRIMARY KEY,
    original_name VARCHAR NOT NULL,
    data_source VARCHAR NOT NULL,
    module      VARCHAR NOT NULL,
    kind        VARCHAR NOT NULL,
    -- Typed property bag: native STRUCT on DuckDB (the planner reads members
    -- natively), JSONB on PostgreSQL (read through the cast path). Written as
    -- the SAME JSON text on both backends (implicit VARCHAR→STRUCT cast).
    properties  {{ if isPostgres }}JSONB{{ else }}STRUCT(
        name VARCHAR,
        sql VARCHAR,
        soft_delete BOOLEAN,
        soft_delete_cond VARCHAR,
        soft_delete_set VARCHAR,
        is_cube BOOLEAN,
        is_m2m BOOLEAN,
        is_hypertable BOOLEAN,
        embeddings STRUCT(model VARCHAR, vector VARCHAR, distance VARCHAR),
        args_type_name VARCHAR,
        required_args BOOLEAN,
        "unique" STRUCT(fields VARCHAR[], query_suffix VARCHAR, skip_query BOOLEAN)[],
        dependencies VARCHAR[],
        cache STRUCT(ttl VARCHAR, "key" VARCHAR, tags VARCHAR[]),
        "at" STRUCT("version" BIGINT, "timestamp" VARCHAR)
    ){{ end }},
    description VARCHAR
);

-- Data-object fields only (struct/input types live in catalog.types as SDL).
-- Declared @join / @function_call / @table_function_call_join fields are
-- ordinary rows here with their typed property bags (D17).
CREATE TABLE IF NOT EXISTS {{ if isAttachedDuckdb }}core.{{ end }}catalog.fields (
    type_name              VARCHAR NOT NULL,
    name                   VARCHAR NOT NULL,
    field_type             VARCHAR NOT NULL,
    properties             {{ if isPostgres }}JSONB{{ else }}STRUCT(
        source VARCHAR,
        computed BOOLEAN,
        sql VARCHAR,
        "default" STRUCT(value JSON, sequence VARCHAR, insert_exp VARCHAR, update_exp VARCHAR),
        geometry STRUCT("type" VARCHAR, srid BIGINT),
        dim BIGINT,
        measurement BOOLEAN,
        timescale_key BOOLEAN,
        unique_rule VARCHAR,
        exclude_filter VARCHAR[],
        filter_required BOOLEAN,
        exclude_mcp BOOLEAN,
        cache STRUCT(ttl VARCHAR, "key" VARCHAR, tags VARCHAR[]),
        "join" STRUCT(references_name VARCHAR, source_fields VARCHAR[], references_fields VARCHAR[], sql VARCHAR),
        function_call STRUCT("function" STRUCT(module VARCHAR, name VARCHAR), args JSON, source_fields VARCHAR[], references_fields VARCHAR[], sql VARCHAR),
        table_function_call_join STRUCT("function" STRUCT(module VARCHAR, name VARCHAR), args JSON, source_fields VARCHAR[], references_fields VARCHAR[], sql VARCHAR)
    ){{ end }},
    -- DECLARED field arguments (SDL), same shape as functions.args — carried
    -- by @table_function_call_join fields (user-facing call parameters);
    -- NULL for plain columns.
    args                   {{ if isPostgres }}JSONB{{ else }}JSON{{ end }},
    data_source            VARCHAR NOT NULL,
    dependency_data_source VARCHAR,
    is_pk                  BOOLEAN NOT NULL,
    ordinal                INTEGER NOT NULL,
    -- @deprecated(reason:) — NULL means active (deprecation applies to fields
    -- and arguments everywhere, so it is a plain column, not a bag member).
    deprecation_reason     VARCHAR,
    description            VARCHAR,
    PRIMARY KEY (type_name, name)
);

-- Relations: ONE row per logical @references edge (fk | m2m), readable from
-- both sides. Identity is (source, name) — the relation name is unique among
-- the declaring object's generated artifacts; data_source is definition
-- provenance / reconcile attribution only, never identity. The generated nav
-- fields are source_field (on source) and destination_field (on destination);
-- their curated descriptions are ordinary field annotations keyed
-- source.source_field / destination.destination_field — the *_description
-- columns here hold only the SDL defaults.
CREATE TABLE IF NOT EXISTS {{ if isAttachedDuckdb }}core.{{ end }}catalog.relations (
    source                        VARCHAR NOT NULL,
    name                          VARCHAR NOT NULL,
    kind                          VARCHAR NOT NULL,
    destination                   VARCHAR NOT NULL,
    m2m_object                    VARCHAR,
    source_keys                   {{ if isPostgres }}JSONB{{ else }}JSON{{ end }},
    destination_keys              {{ if isPostgres }}JSONB{{ else }}JSON{{ end }},
    source_field                  VARCHAR,
    source_field_description      VARCHAR,
    destination_field             VARCHAR,
    destination_field_description VARCHAR,
    -- declared as @field_references on the source field (vs object-level
    -- @references) — the compiler mirrors field-declared legs onto the
    -- matching filter field as @field_references
    field_declared                BOOLEAN NOT NULL,
    data_source                   VARCHAR NOT NULL,
    PRIMARY KEY (source, name)
);

CREATE TABLE IF NOT EXISTS {{ if isAttachedDuckdb }}core.{{ end }}catalog.functions (
    module      VARCHAR NOT NULL,
    name        VARCHAR NOT NULL,
    kind        VARCHAR NOT NULL,
    data_source VARCHAR NOT NULL,
    returns     VARCHAR NOT NULL,
    is_table    BOOLEAN NOT NULL,
    args        {{ if isPostgres }}JSONB{{ else }}STRUCT(name VARCHAR, "type" VARCHAR, description VARCHAR, "default" VARCHAR, arg_default VARCHAR, deprecation_reason VARCHAR)[]{{ end }},
    properties  {{ if isPostgres }}JSONB{{ else }}STRUCT(
        "function" STRUCT(name VARCHAR, sql VARCHAR, skip_null_arg BOOLEAN, is_table BOOLEAN, json_cast BOOLEAN),
        cache STRUCT(ttl VARCHAR, "key" VARCHAR, tags VARCHAR[]),
        geometry STRUCT("type" VARCHAR, srid BIGINT)
    ){{ end }},
    deprecation_reason VARCHAR,
    description VARCHAR,
    -- kind is part of the IDENTITY: query functions, mutation functions and
    -- subscriptions are three independent GraphQL root namespaces, and one
    -- name legally appears in more than one of them (an HTTP source exposes
    -- every operation as both a function and a mutation function; core.models
    -- exposes `completion` as a function AND as a streaming subscription).
    PRIMARY KEY (module, name, kind)
);

-- Residual source-defined base types, raw SDL (parsed on demand later).
CREATE TABLE IF NOT EXISTS {{ if isAttachedDuckdb }}core.{{ end }}catalog.types (
    name        VARCHAR NOT NULL PRIMARY KEY,
    kind        VARCHAR NOT NULL,
    data_source VARCHAR NOT NULL,
    module      VARCHAR NOT NULL,
    definition  VARCHAR NOT NULL,
    description VARCHAR
);

-- Curation overlay: NEVER touched by load/unload/reload (orphans legal).
-- Identity is (entity_kind, entity_key) alone — names are unique in the
-- logical model; source/module attribution lives in the entity tables and is
-- joined on demand, never denormalized here (D15). A function's entity_kind IS
-- its kind (function / mutation / subscription — the value functions.kind
-- stores), so one name declared in several root namespaces curates
-- independently and the overlay joins without a mapping. Rows with NULL texts
-- and a vector are load-time SEEDS (D24) — swept only with an unregistered
-- source.
CREATE TABLE IF NOT EXISTS {{ if isAttachedDuckdb }}core.{{ end }}catalog.annotations (
    entity_kind      VARCHAR NOT NULL,
    entity_key       VARCHAR NOT NULL,
    parent           VARCHAR,
    description      VARCHAR,
    long_description VARCHAR,
    vec              {{ if isPostgres }}vector({{ .VectorSize }}){{ else }}FLOAT[{{ .VectorSize }}]{{ end }},
    updated_at       TIMESTAMP,
    updated_by       VARCHAR,
    PRIMARY KEY (entity_kind, entity_key)
);

CREATE TABLE IF NOT EXISTS {{ if isAttachedDuckdb }}core.{{ end }}catalog.data_source_dependencies (
    data_source VARCHAR NOT NULL,
    depends_on  VARCHAR NOT NULL,
    PRIMARY KEY (data_source, depends_on)
);

{{ if isPostgres }}
CREATE EXTENSION IF NOT EXISTS vector;

-- Non-PK indexes are PostgreSQL-only (legacy schema.sql convention: the
-- catalog tables are small and DuckDB ART indexes only get in the way).
CREATE INDEX IF NOT EXISTS idx_hc_data_objects_source ON catalog.data_objects (data_source);
CREATE INDEX IF NOT EXISTS idx_hc_module_sources_src  ON catalog.module_data_sources (data_source);
CREATE INDEX IF NOT EXISTS idx_hc_data_objects_module ON catalog.data_objects (module);
CREATE INDEX IF NOT EXISTS idx_hc_fields_source       ON catalog.fields (data_source);
CREATE INDEX IF NOT EXISTS idx_hc_fields_dependency   ON catalog.fields (dependency_data_source);
CREATE INDEX IF NOT EXISTS idx_hc_relations_dest      ON catalog.relations (destination);
CREATE INDEX IF NOT EXISTS idx_hc_relations_data_source ON catalog.relations (data_source);
CREATE INDEX IF NOT EXISTS idx_hc_functions_source    ON catalog.functions (data_source);
CREATE INDEX IF NOT EXISTS idx_hc_functions_module    ON catalog.functions (module);
CREATE INDEX IF NOT EXISTS idx_hc_types_source        ON catalog.types (data_source);
CREATE INDEX IF NOT EXISTS idx_hc_annotations_parent  ON catalog.annotations (entity_kind, parent);
CREATE INDEX IF NOT EXISTS idx_hc_annotations_vec     ON catalog.annotations USING hnsw (vec vector_cosine_ops);

-- INOUT assignment casts: the unified DML executed from the DuckDB context
-- arrives as typed varchar — these make it assignable to the typed catalog
-- columns. Only the types the catalog schema actually uses.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_cast
        WHERE castsource = 'varchar'::regtype AND casttarget = 'jsonb'::regtype) THEN
        CREATE CAST (varchar AS jsonb) WITH INOUT AS ASSIGNMENT;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_cast
        WHERE castsource = 'varchar'::regtype AND casttarget = 'vector'::regtype) THEN
        CREATE CAST (varchar AS vector) WITH INOUT AS ASSIGNMENT;
    END IF;
END
$$;
{{ end }}

UPDATE "version" SET "version" = '0.0.19';
