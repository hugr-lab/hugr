UPDATE "version" SET "version" = '0.0.15';

UPDATE _schema_fields 
    SET dependency_catalog = NULL 
WHERE hugr_type = 'submodule' OR catalog = '_system';

UPDATE _schema_fields 
    SET dependency_catalog = NULL,
    catalog = '_system' 
WHERE type_name in ('Query', 'Mutation') and name in ('function');