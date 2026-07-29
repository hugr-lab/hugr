-- Migration 0.0.20 (1): complete the annotations backfill BEFORE the legacy
-- _schema_* tables are dropped (design 036). This is the last chance to read
-- them, so everything curated that the entity model can address is carried here.
--
-- 0.0.19 carried what it could and documented one gap (functions). Measuring a
-- real database showed the gap is wider, and in a specific way: the legacy model
-- kept every curated text in ONE flat namespace (types + fields), while the
-- entity model has TWO — the LOGICAL one (data sources, modules, data objects,
-- their fields, functions) and the GRAPHQL one (gql_type / gql_field: the
-- generated surface, which has no logical entity to hang a description on).
-- Rows describing generated artifacts therefore landed under logical keys that
-- nothing resolves: of 826 'type' rows only 7 named a type the storage stores,
-- and of 7 220 'field' rows only 2 024 named a stored field.
--
-- This file writes every row under the key its content belongs to, using the
-- legacy hugr_type discriminators; the last two statements remove what 0.0.19
-- misfiled. It is correct whether or not 0.0.19's backfill ever executed — but
-- it MUST run before 2-drop-legacy-schema.sql and cannot be replayed after it,
-- since it reads the tables that file removes. The migrator applies a version
-- once and in filename order, so neither is a live hazard.
--
-- ON CONFLICT ... DO UPDATE, never DO NOTHING. The storage SEEDS annotation
-- rows when it writes a source — empty description plus a vector, so semantic
-- search has a key to rank — and those rows occupy exactly the primary key a
-- backfill targets. On any deployment that started the engine after 0.0.19,
-- DO NOTHING would silently do nothing and the drop would take the only copy of
-- the text.
--
-- "Never overwrite an existing curation" is therefore enforced in the SELECT,
-- by the notCurated() guard below, and the SET takes EXCLUDED only. Doing it in
-- the SET instead — COALESCE(annotations.description, EXCLUDED.description) —
-- reads better and does not work: DuckDB has no CASE/COALESCE over FLOAT[N], so
-- the vec column alone makes that form unportable ("Unimplemented type for case
-- expression: FLOAT[768]").

-- The module-root map: flattened root type name → dotted module. Materialized
-- once because four statements need it.
--
-- min() collapses it to one row per type name: two modules CAN flatten to the
-- same name (`a.b` and `a_b`), and a duplicate would make ON CONFLICT DO UPDATE
-- hit the same row twice, which is an error rather than a silent mess. No
-- collision exists in the measured database; this does not rely on that.
-- rank orders the roots by how representative they are OF THE MODULE, for the
-- statements that must pick one: the query root describes what the module holds,
-- the function roots only describe its callables. Alphabetical order would put
-- _function first, which is the least representative of the five.
DROP TABLE IF EXISTS _mig_module_roots;
CREATE TEMP TABLE _mig_module_roots AS
SELECT type_name, min(module) AS module, min(kind) AS kind, min(rank) AS rank
FROM (
  SELECT '_module_' || replace(m.name, '.', '_') || '_query'         AS type_name, m.name AS module, NULL           AS kind, 1 AS rank FROM {{ if isAttachedDuckdb }}core.{{ end }}_schema_modules m
  UNION ALL
  SELECT '_module_' || replace(m.name, '.', '_') || '_mutation'      AS type_name, m.name AS module, NULL           AS kind, 2 AS rank FROM {{ if isAttachedDuckdb }}core.{{ end }}_schema_modules m
  UNION ALL
  SELECT '_module_' || replace(m.name, '.', '_') || '_function'      AS type_name, m.name AS module, 'function'     AS kind, 3 AS rank FROM {{ if isAttachedDuckdb }}core.{{ end }}_schema_modules m
  UNION ALL
  SELECT '_module_' || replace(m.name, '.', '_') || '_mut_function'  AS type_name, m.name AS module, 'mutation'     AS kind, 4 AS rank FROM {{ if isAttachedDuckdb }}core.{{ end }}_schema_modules m
  UNION ALL
  SELECT '_module_' || replace(m.name, '.', '_') || '_subscription'  AS type_name, m.name AS module, 'subscription' AS kind, 5 AS rank FROM {{ if isAttachedDuckdb }}core.{{ end }}_schema_modules m
  UNION ALL SELECT 'Query',            '', NULL,           1
  UNION ALL SELECT 'Mutation',         '', NULL,           2
  UNION ALL SELECT 'Function',         '', 'function',     3
  UNION ALL SELECT 'MutationFunction', '', 'mutation',     4
  UNION ALL SELECT 'Subscription',     '', 'subscription', 5
) all_roots
GROUP BY type_name;

-- 1. Functions. The legacy model keeps them as FIELDS of module root types, and
--    hugr_type on the field is the discriminator: 'function' is a real function,
--    'submodule' is a nested-module gateway (core { cache { … } }) that the
--    storage synthesizes — its text describes a MODULE and is statement 3.
--    The annotation kind comes from the root suffix; the key is module.name,
--    bare name for the root module, matching functionKey().
INSERT INTO {{ if isAttachedDuckdb }}core.{{ end }}catalog.annotations
  (entity_kind, entity_key, parent, description, long_description, vec)
SELECT r.kind,
       CASE WHEN r.module = '' THEN f.name ELSE r.module || '.' || f.name END,
       r.module,
       NULLIF(f.description, ''), NULLIF(f.long_description, ''), f.vec
FROM {{ if isAttachedDuckdb }}core.{{ end }}_schema_fields f
JOIN _mig_module_roots r ON r.type_name = f.type_name
WHERE f.hugr_type = 'function' AND r.kind IS NOT NULL
  AND f.is_summarized = true AND (f.description <> '' OR f.long_description <> '')
  -- notCurated: never overwrite an existing NON-EMPTY curation. The
  -- guard lives here rather than in the SET because the SET cannot
  -- reference the target row portably (see the header).
  AND NOT EXISTS (
    SELECT 1 FROM {{ if isAttachedDuckdb }}core.{{ end }}catalog.annotations a
    WHERE a.entity_kind = r.kind
      AND a.entity_key = CASE WHEN r.module = '' THEN f.name ELSE r.module || '.' || f.name END
      AND COALESCE(a.description, '') <> ''
  )
ON CONFLICT (entity_kind, entity_key) DO UPDATE SET
  description      = EXCLUDED.description,
  long_description = EXCLUDED.long_description,
  vec              = EXCLUDED.vec;

-- 2. Modules, from their ROOT TYPES. A module has several roots (query,
--    mutation, function, …) and the summarizer described each from that root's
--    angle; the module takes the most representative one — see rank above.
INSERT INTO {{ if isAttachedDuckdb }}core.{{ end }}catalog.annotations
  (entity_kind, entity_key, parent, description, long_description, vec)
SELECT 'module', r.module, NULL,
       NULLIF(t.description, ''), NULLIF(t.long_description, ''), t.vec
FROM {{ if isAttachedDuckdb }}core.{{ end }}_schema_types t
JOIN _mig_module_roots r ON r.type_name = t.name
-- The ROOT module is excluded on purpose: its "roots" are Query / Mutation /
-- Function themselves, which describe the engine's entry points rather than the
-- module. Its own description comes from _schema_modules, carried by 0.0.19.
WHERE t.hugr_type = 'module' AND r.module <> ''
  AND t.is_summarized = true AND (t.description <> '' OR t.long_description <> '')
  AND t.name = (
    SELECT t2.name
    FROM {{ if isAttachedDuckdb }}core.{{ end }}_schema_types t2
    JOIN _mig_module_roots r2 ON r2.type_name = t2.name
    WHERE r2.module = r.module AND t2.hugr_type = 'module'
      AND t2.is_summarized = true AND (t2.description <> '' OR t2.long_description <> '')
    ORDER BY r2.rank, t2.name
    LIMIT 1
  )
  -- notCurated: never overwrite an existing NON-EMPTY curation. The
  -- guard lives here rather than in the SET because the SET cannot
  -- reference the target row portably (see the header).
  AND NOT EXISTS (
    SELECT 1 FROM {{ if isAttachedDuckdb }}core.{{ end }}catalog.annotations a
    WHERE a.entity_kind = 'module'
      AND a.entity_key = r.module
      AND COALESCE(a.description, '') <> ''
  )
ON CONFLICT (entity_kind, entity_key) DO UPDATE SET
  description      = EXCLUDED.description,
  long_description = EXCLUDED.long_description,
  vec              = EXCLUDED.vec;

-- 3. Modules, from the nested-module GATEWAY fields (hugr_type = 'submodule').
--    The gateway `cache` on _module_core_function describes module core.cache:
--    the child's name is parent-module.field-name.
--
--    A child is reachable from SEVERAL of the parent's roots (query, mutation,
--    function …) and the summarizer described each, so the same module arrives
--    up to five times; the first root by name wins.
INSERT INTO {{ if isAttachedDuckdb }}core.{{ end }}catalog.annotations
  (entity_kind, entity_key, parent, description, long_description, vec)
SELECT 'module',
       CASE WHEN r.module = '' THEN f.name ELSE r.module || '.' || f.name END,
       NULL,
       NULLIF(f.description, ''), NULLIF(f.long_description, ''), f.vec
FROM {{ if isAttachedDuckdb }}core.{{ end }}_schema_fields f
JOIN _mig_module_roots r ON r.type_name = f.type_name
WHERE f.hugr_type = 'submodule'
  AND f.is_summarized = true AND (f.description <> '' OR f.long_description <> '')
  AND f.type_name = (
    SELECT f2.type_name
    FROM {{ if isAttachedDuckdb }}core.{{ end }}_schema_fields f2
    JOIN _mig_module_roots r2 ON r2.type_name = f2.type_name
    WHERE r2.module = r.module AND f2.name = f.name AND f2.hugr_type = 'submodule'
      AND f2.is_summarized = true AND (f2.description <> '' OR f2.long_description <> '')
    ORDER BY r2.rank, f2.type_name
    LIMIT 1
  )
  -- notCurated: never overwrite an existing NON-EMPTY curation. The
  -- guard lives here rather than in the SET because the SET cannot
  -- reference the target row portably (see the header).
  AND NOT EXISTS (
    SELECT 1 FROM {{ if isAttachedDuckdb }}core.{{ end }}catalog.annotations a
    WHERE a.entity_kind = 'module'
      AND a.entity_key = CASE WHEN r.module = '' THEN f.name ELSE r.module || '.' || f.name END
      AND COALESCE(a.description, '') <> ''
  )
ON CONFLICT (entity_kind, entity_key) DO UPDATE SET
  description      = EXCLUDED.description,
  long_description = EXCLUDED.long_description,
  vec              = EXCLUDED.vec;

-- 4. Generated TYPES (filters, mutation inputs, list filters, aggregations) →
--    gql_type. They have no logical entity: they are shapes the storage derives
--    from a data object. Aggregations carry no hugr_type in the legacy model
--    (they predate it) and are recognized by the compiler's naming convention.
INSERT INTO {{ if isAttachedDuckdb }}core.{{ end }}catalog.annotations
  (entity_kind, entity_key, parent, description, long_description, vec)
SELECT 'gql_type', t.name, NULL,
       NULLIF(t.description, ''), NULLIF(t.long_description, ''), t.vec
FROM {{ if isAttachedDuckdb }}core.{{ end }}_schema_types t
WHERE (t.hugr_type IN ('data_input', 'filter', 'filter_list')
       OR t.name LIKE '\_%\_aggregation%' ESCAPE '\')
  AND t.is_summarized = true AND (t.description <> '' OR t.long_description <> '')
  AND t.catalog <> '_system'
  -- notCurated: never overwrite an existing NON-EMPTY curation. The
  -- guard lives here rather than in the SET because the SET cannot
  -- reference the target row portably (see the header).
  AND NOT EXISTS (
    SELECT 1 FROM {{ if isAttachedDuckdb }}core.{{ end }}catalog.annotations a
    WHERE a.entity_kind = 'gql_type'
      AND a.entity_key = t.name
      AND COALESCE(a.description, '') <> ''
  )
ON CONFLICT (entity_kind, entity_key) DO UPDATE SET
  description      = EXCLUDED.description,
  long_description = EXCLUDED.long_description,
  vec              = EXCLUDED.vec;

-- 5. Root query/mutation FIELDS on module roots → gql_field. These are the
--    per-query summaries ("Retrieves a list of movement routes. Supports
--    filtering by …") — the richest thing in the legacy corpus and what an
--    agent reads when choosing a query. The storage derives a description for
--    them from the data object; an explicit gql_field row is more specific and
--    wins, which is the intent.
INSERT INTO {{ if isAttachedDuckdb }}core.{{ end }}catalog.annotations
  (entity_kind, entity_key, parent, description, long_description, vec)
SELECT 'gql_field', f.type_name || '.' || f.name, f.type_name,
       NULLIF(f.description, ''), NULLIF(f.long_description, ''), f.vec
FROM {{ if isAttachedDuckdb }}core.{{ end }}_schema_fields f
WHERE f.hugr_type IN ('select', 'select_one', 'aggregate', 'bucket_agg',
                      'mutation_insert', 'mutation_update', 'mutation_delete')
  AND f.is_summarized = true AND (f.description <> '' OR f.long_description <> '')
  -- notCurated: never overwrite an existing NON-EMPTY curation. The
  -- guard lives here rather than in the SET because the SET cannot
  -- reference the target row portably (see the header).
  AND NOT EXISTS (
    SELECT 1 FROM {{ if isAttachedDuckdb }}core.{{ end }}catalog.annotations a
    WHERE a.entity_kind = 'gql_field'
      AND a.entity_key = f.type_name || '.' || f.name
      AND COALESCE(a.description, '') <> ''
  )
ON CONFLICT (entity_kind, entity_key) DO UPDATE SET
  description      = EXCLUDED.description,
  long_description = EXCLUDED.long_description,
  vec              = EXCLUDED.vec;

-- 6. FIELDS of generated types → gql_field, but ONLY where the text says
--    something the logical layer would not already say. A derived type INHERITS
--    its base object's field curation onto same-named fields, so a row identical
--    to the base field is pure duplication — and an explicit gql_field row WINS
--    over inheritance, freezing it against later edits of the base. On the
--    measured database that skips 1 839 of 3 389 rows; the 1 547 kept are
--    written from the derived type's point of view ("Filter by the occurrence
--    date of the payment" vs "The date when the transfer occurred").
INSERT INTO {{ if isAttachedDuckdb }}core.{{ end }}catalog.annotations
  (entity_kind, entity_key, parent, description, long_description, vec)
SELECT 'gql_field', f.type_name || '.' || f.name, f.type_name,
       NULLIF(f.description, ''), NULLIF(f.long_description, ''), f.vec
FROM {{ if isAttachedDuckdb }}core.{{ end }}_schema_fields f
JOIN {{ if isAttachedDuckdb }}core.{{ end }}_schema_types t ON t.name = f.type_name
LEFT JOIN {{ if isAttachedDuckdb }}core.{{ end }}_schema_fields b
  ON b.type_name = regexp_replace(f.type_name, '(_list_filter|_filter|_mut_input_data|_mut_data)$', '')
 AND b.name = f.name
WHERE (t.hugr_type IN ('data_input', 'filter', 'filter_list')
       OR t.name LIKE '\_%\_aggregation%' ESCAPE '\')
  AND t.catalog <> '_system'
  AND f.is_summarized = true AND (f.description <> '' OR f.long_description <> '')
  AND (b.description IS NULL OR b.description = '' OR b.description <> f.description)
  -- notCurated: never overwrite an existing NON-EMPTY curation. The
  -- guard lives here rather than in the SET because the SET cannot
  -- reference the target row portably (see the header).
  AND NOT EXISTS (
    SELECT 1 FROM {{ if isAttachedDuckdb }}core.{{ end }}catalog.annotations a
    WHERE a.entity_kind = 'gql_field'
      AND a.entity_key = f.type_name || '.' || f.name
      AND COALESCE(a.description, '') <> ''
  )
ON CONFLICT (entity_kind, entity_key) DO UPDATE SET
  description      = EXCLUDED.description,
  long_description = EXCLUDED.long_description,
  vec              = EXCLUDED.vec;

-- 7. Remove what 0.0.19 misfiled: logical-kind rows whose key names a GENERATED
--    artifact. They are unreachable — nothing resolves them — and not inert:
--    every annotation row carries a search vector, so they are noise in every
--    semantic ranking. Their text has just been rewritten under the right key.
--    A no-op on a database where 0.0.19's backfill never ran.
DELETE FROM {{ if isAttachedDuckdb }}core.{{ end }}catalog.annotations
WHERE entity_kind = 'type'
  AND entity_key IN (
    SELECT t.name FROM {{ if isAttachedDuckdb }}core.{{ end }}_schema_types t
    WHERE t.hugr_type IN ('data_input', 'filter', 'filter_list', 'module')
       OR t.name LIKE '\_%\_aggregation%' ESCAPE '\'
  );

DELETE FROM {{ if isAttachedDuckdb }}core.{{ end }}catalog.annotations
WHERE entity_kind = 'field'
  AND entity_key IN (
    SELECT f.type_name || '.' || f.name
    FROM {{ if isAttachedDuckdb }}core.{{ end }}_schema_fields f
    JOIN {{ if isAttachedDuckdb }}core.{{ end }}_schema_types t ON t.name = f.type_name
    WHERE t.hugr_type IN ('data_input', 'filter', 'filter_list', 'module')
       OR t.name LIKE '\_%\_aggregation%' ESCAPE '\'
  );

DROP TABLE IF EXISTS _mig_module_roots;
