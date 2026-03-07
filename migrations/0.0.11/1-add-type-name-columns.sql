-- Add field_type_name to _schema_fields: clean type name without [] and ! modifiers
-- Used for @field_references joins to _schema_types
ALTER TABLE _schema_fields ADD COLUMN IF NOT EXISTS field_type_name VARCHAR DEFAULT '';

-- Add arg_type_name to _schema_arguments: clean type name without [] and ! modifiers
ALTER TABLE _schema_arguments ADD COLUMN IF NOT EXISTS arg_type_name VARCHAR DEFAULT '';

-- Backfill field_type_name from field_type (strip list/non-null wrappers)
UPDATE _schema_fields
SET field_type_name = REGEXP_REPLACE(REGEXP_REPLACE(field_type, '[\[\]!]', '', 'g'), '^\s+|\s+$', '')
WHERE field_type_name = '' OR field_type_name IS NULL;

-- Backfill arg_type_name from arg_type
UPDATE _schema_arguments
SET arg_type_name = REGEXP_REPLACE(REGEXP_REPLACE(arg_type, '[\[\]!]', '', 'g'), '^\s+|\s+$', '')
WHERE arg_type_name = '' OR arg_type_name IS NULL;

-- Update version
UPDATE "version" SET "version" = '0.0.11';
