ALTER TABLE data_sources
    ADD COLUMN as_module BOOLEAN DEFAULT false;

ALTER TABLE data_sources
    ADD COLUMN disabled BOOLEAN DEFAULT false;