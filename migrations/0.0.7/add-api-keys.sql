CREATE TABLE IF NOT EXISTS api_keys (
    name VARCHAR PRIMARY KEY,
    key VARCHAR NOT NULL UNIQUE,
    description VARCHAR,
    default_role VARCHAR NOT NULL,
    disabled BOOLEAN NOT NULL DEFAULT FALSE,
    is_temporal BOOLEAN NOT NULL DEFAULT FALSE,
    expires_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    headers {{if isPostgres }} JSONB {{ else }} JSON {{ end }},
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);