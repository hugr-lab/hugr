ALTER TABLE api_keys
    ADD COLUMN claims {{if isPostgres }} JSONB {{ else }} JSON {{ end }};
