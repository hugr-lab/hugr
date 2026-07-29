-- Migration 0.0.19 (2): one-time annotations backfill — legacy CURATED
-- descriptions/vectors (_schema_* rows with is_summarized) → the catalog
-- annotations overlay, entity-keyed. Idempotent (ON CONFLICT DO NOTHING —
-- never overwrites overlay edits). This migration is the ONLY home of the
-- backfill; the engine never runs it.

-- CoreDB catalog-schema annotations backfill (design 034 Phase 2, US5 / research D8).
-- Copies existing CURATED text (is_summarized = true is set by BOTH the
-- summarizer and manual description edits) from the legacy _schema_* rows into
-- the annotations overlay. Idempotent: ON CONFLICT DO NOTHING — never
-- overwrites newer overlay edits; safe to run on every initialization.
--
-- KNOWN LIMITATION (analysis U1): legacy FUNCTION descriptions live as field
-- rows of module ROOT types (_module_<m>_function / Function / ...); the
-- dotted module name is not recoverable from the flattened type name in SQL,
-- so those rows are EXCLUDED here instead of being backfilled with the wrong
-- entity_kind='field'. Curated function texts are re-set via
-- core.catalog.set_function_description (or re-summarized).

INSERT INTO {{ if isAttachedDuckdb }}core.{{ end }}catalog.annotations
  (entity_kind, entity_key, parent, description, long_description, vec)
SELECT 'data_source', c.name, NULL,
       NULLIF(c.description, ''), NULLIF(c.long_description, ''), c.vec
FROM {{ if isAttachedDuckdb }}core.{{ end }}_schema_catalogs c
WHERE c.is_summarized = true AND (c.description <> '' OR c.long_description <> '')
ON CONFLICT (entity_kind, entity_key) DO NOTHING;

INSERT INTO {{ if isAttachedDuckdb }}core.{{ end }}catalog.annotations
  (entity_kind, entity_key, parent, description, long_description, vec)
SELECT 'module', m.name, NULL,
       NULLIF(m.description, ''), NULLIF(m.long_description, ''), m.vec
FROM {{ if isAttachedDuckdb }}core.{{ end }}_schema_modules m
WHERE m.is_summarized = true AND (m.description <> '' OR m.long_description <> '')
ON CONFLICT (entity_kind, entity_key) DO NOTHING;

INSERT INTO {{ if isAttachedDuckdb }}core.{{ end }}catalog.annotations
  (entity_kind, entity_key, parent, description, long_description, vec)
SELECT CASE WHEN t.hugr_type IN ('table', 'view') THEN 'data_object' ELSE 'type' END,
       t.name, NULL,
       NULLIF(t.description, ''), NULLIF(t.long_description, ''), t.vec
FROM {{ if isAttachedDuckdb }}core.{{ end }}_schema_types t
WHERE t.is_summarized = true AND (t.description <> '' OR t.long_description <> '')
  AND t.catalog <> '_system'
ON CONFLICT (entity_kind, entity_key) DO NOTHING;

INSERT INTO {{ if isAttachedDuckdb }}core.{{ end }}catalog.annotations
  (entity_kind, entity_key, parent, description, long_description, vec)
SELECT 'field', f.type_name || '.' || f.name, f.type_name,
       NULLIF(f.description, ''), NULLIF(f.long_description, ''), f.vec
FROM {{ if isAttachedDuckdb }}core.{{ end }}_schema_fields f
WHERE f.is_summarized = true AND (f.description <> '' OR f.long_description <> '')
  AND f.type_name NOT LIKE '\_module\_%' ESCAPE '\'
  AND f.type_name NOT IN ('Query', 'Mutation', 'Subscription', 'Function', 'MutationFunction')
ON CONFLICT (entity_kind, entity_key) DO NOTHING;
