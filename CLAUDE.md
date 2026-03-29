# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Hugr is a DataMesh service that provides access to various data sources (PostgreSQL, DuckDB files, HTTP REST APIs, file formats) through a common GraphQL API. It is built on top of DuckDB as a calculation engine. The core query engine logic lives in a separate dependency: `github.com/hugr-lab/query-engine`.

## Build Commands

All builds require CGO and the `duckdb_arrow` build tag:

```bash
# Build all executables
make all

# Build individual executables
make server      # -> ./server
make migrate     # -> ./migrate

# Clean build artifacts
make clean
```

Manual build (if needed):
```bash
CGO_ENABLED=1 go build -tags='duckdb_arrow' -o server cmd/server/*.go
```

## Testing

```bash
# Unit/integration tests (standalone only, no external deps)
CGO_ENABLED=1 go test -tags=duckdb_arrow ./integration-test/...

# E2E tests with Docker Compose (PostgreSQL + cluster)
cd integration-test/e2e && ./run.sh

# E2E with containers kept running after tests
cd integration-test/e2e && ./run.sh --keep

# Update expected test outputs
cd integration-test/e2e && UPDATE_EXPECTED=1 ./run.sh
```

## Running

Configuration is via environment variables (see README.md for full list). A `.env` file is auto-loaded via godotenv.

```bash
# Run server (defaults to :15000)
./server

# Install DuckDB extensions
./server -install

# Run migrations
./migrate -core-db core-db.duckdb -path ./migrations
```

## Architecture

### Executables (`cmd/`)

- **`cmd/server`** — Main Hugr server. Handles standalone, management, and worker roles via `CLUSTER_ROLE` env var. Exposes GraphQL API, optional admin UI (GraphiQL). Config loaded from env vars via viper.
- **`cmd/migrate`** — CoreDB schema migration tool. Supports both DuckDB and PostgreSQL backends. Migrations are version-ordered SQL files in `migrations/`.

### Packages (`pkg/`)

- **`pkg/auth`** — Authentication configuration and providers (API keys, JWT, OIDC, anonymous). Wraps `query-engine/pkg/auth`. Auth config can come from env vars or a JSON/YAML config file.
- **`pkg/cors`** — CORS middleware configuration.
- **`pkg/info`** — Version/build info exposed as a runtime data source. NodeInfo struct includes `node_role`, `node_name`, `cluster_mode`.
- **`pkg/service`** — Sidecar HTTP server for `/health` and `/metrics` (Prometheus).

### Key Dependency

The heavy lifting (GraphQL schema generation, query execution, data source management, caching, cluster coordination) is in `github.com/hugr-lab/query-engine`. This repo is the service layer that configures and runs the engine.

### Modes of Operation

1. **Standalone** — Single server with local DuckDB or in-memory storage for CoreDB. Default mode.
2. **Cluster Management** — `CLUSTER_ENABLED=true CLUSTER_ROLE=management`. CoreDB must be PostgreSQL. Node self-registers, compiles schemas, broadcasts to workers.
3. **Cluster Worker** — `CLUSTER_ENABLED=true CLUSTER_ROLE=worker`. CoreDB must be PostgreSQL (same as management). Reads compiled schema, syncs secrets, serves queries.

Cluster mode always requires PostgreSQL as CoreDB. The server validates this at startup.

## Active Technologies
- Go 1.26+ + stdlib `net/http`, `crypto/tls` (no new external deps) (002-tls-support)
- N/A (reads certificate files from filesystem) (002-tls-support)

## Recent Changes
- 002-tls-support: Added Go 1.26+ + stdlib `net/http`, `crypto/tls` (no new external deps)
