INSERT INTO _schema_module_type_catalogs 
    (module_name, type_name, catalog_name) 
VALUES (
    'core.catalog', '_module_core_catalog_query', 'core'
) ON 
CONFLICT (type_name, catalog_name) DO NOTHING;

UPDATE "version" SET "version" = '0.0.16';