//go:build duckdb_arrow

package integration_test

// Cluster mode tests require PostgreSQL as shared CoreDB.
// In-memory DuckDB cannot be used for cluster mode because
// heartbeat, node registration, and schema polling need a
// persistent shared database.
//
// Real cluster tests live in integration-test/e2e/ and use
// Docker Compose with PostgreSQL (see docker-compose.yml).
