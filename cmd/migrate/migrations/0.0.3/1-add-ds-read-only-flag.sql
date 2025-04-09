
CREATE TABLE catalog_sources (
    name VARCHAR NOT NULL PRIMARY KEY,
    type VARCHAR NOT NULL,
    description VARCHAR,
    path VARCHAR NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE data_sources (
    name VARCHAR NOT NULL PRIMARY KEY,
    type VARCHAR NOT NULL,
    description VARCHAR,
    prefix VARCHAR NOT NULL,
    path VARCHAR NOT NULL,
    self_defined BOOLEAN NOT NULL DEFAULT FALSE,
    read_only BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE data_source_catalogs (
    data_source_name VARCHAR NOT NULL,
    catalog_name VARCHAR NOT NULL,
    PRIMARY KEY (data_source_name, catalog_name),
    FOREIGN KEY (data_source_name) REFERENCES data_sources(name),
    FOREIGN KEY (catalog_name) REFERENCES catalog_sources(name)
);

CREATE TABLE "version" AS SELECT '0.0.2' AS "version";

INSERT INTO catalog_sources (name, type, description, path)
SELECT name, type, description, path
FROM core.catalog_sources;

INSERT INTO data_sources (name, type, description, prefix, path, self_defined)
SELECT name, type, description, prefix, path, self_defined
FROM core.data_sources;

INSERT INTO data_source_catalogs (data_source_name, catalog_name)
SELECT data_source_name, catalog_name
FROM core.data_source_catalogs;

DROP SCHEMA core CASCADE;
