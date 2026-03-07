# hugr

[Documentation](https://hugr-lab.github.io/) | [GitHub](https://github.com/hugr-lab/hugr)

The DataMesh service that provides access to various data sources through common GraphQL API.

The hugr is built on the top of [DuckDB](https://duckdb.org) and uses it as an calculation engine. The hugr can work with following data sources:

- PostgreSQL (incl. with extensions: PostGIS, TimescaleDB)
- DuckDB files
- HTTP REST API (support OpenAPI v3)
- All file formats and data sources that supported by DuckDB (CSV, Parquet, JSON, ESRI Shape, etc.)

Files can be stored in the local file system or in the cloud storage (currently support only s3 cloud storage, in plan: Azure, GCS, AWS, R2).

MySQL, SQLLite, Clickhouse will be supported in the future.

## Executable

There are 2 executables provided in the repository:

- `cmd/server/main.go` - main executable for the Hugr server (handles standalone, management, and worker roles),
- `cmd/migrate/main.go` - executable for the core-db migrations.

A single `server` binary serves all roles. The role is determined by `CLUSTER_ROLE` environment variable.

The executable is built with Go and can be run on any platform that supports Go:

```bash
CGO_ENABLED=1 go build -tags='duckdb_arrow' -o server cmd/server/*.go
```

## Dependencies

The Hugr uses the [Hugr query engine package](https://github.com/hugr-lab/query-engine).

## Deployment

The common way to deploy the Hugr is to use Docker. The Docker image provided by repository [docker](https://hub.docker.com/r/hugr-lab/docker). There are two images provided:

- ghcr.io/hugr-lab/server - hugr server (for standalone, management, and worker modes),
- ghcr.io/hugr-lab/automigrate - hugr server with automigration for the core-db schema.

## Modes of Operation

### Standalone Mode (default)

Start the server without any `CLUSTER_*` env vars. Uses DuckDB or PostgreSQL as CoreDB.

```bash
./server
```

### Cluster Mode — Management Node

```bash
CLUSTER_ENABLED=true \
CLUSTER_ROLE=management \
CLUSTER_NODE_NAME=mgmt-1 \
CLUSTER_NODE_URL=http://mgmt-1:15000/ipc \
CLUSTER_SECRET=my-secret \
CORE_DB_PATH="postgres://user:pass@db:5432/hugr" \
./server
```

### Cluster Mode — Worker Node

```bash
CLUSTER_ENABLED=true \
CLUSTER_ROLE=worker \
CLUSTER_NODE_NAME=worker-1 \
CLUSTER_NODE_URL=http://worker-1:15000/ipc \
CLUSTER_SECRET=my-secret \
CORE_DB_PATH="postgres://user:pass@db:5432/hugr" \
./server
```

**Important**: Cluster mode requires PostgreSQL as CoreDB (`CORE_DB_PATH` must be a `postgres://` DSN). The server will refuse to start in cluster mode with DuckDB CoreDB.

## Environment variables for the server

### General

- BIND - string, that defines network interface and port, default: :15000
- SERVICE_BIND - string, that defines network interface and port for the metrics and health check, if it is not set up than http server for the service will not start, default: ""
- ADMIN_UI - flag to enable AdminUI, for path /admin ([GraphiQL](https://github.com/graphql/graphiql)), default: true
- ADMIN_UI_FETCH_PATH - path to fetch AdminUI, default: "/admin"
- DEBUG - flag to run in debug mode (SQL queries will output to the stdout), default: false
- ALLOW_PARALLEL - flag to allow run queries in parallel, default: true
- MAX_PARALLEL_QUERIES - limit to numbers of parallels queries executed, default: 0 (unlimited)
- MAX_DEPTH - maximal depth of GraphQL types hierarchy, default: 7

### DuckDB engine settings

- DB_HOME_DIRECTORY - path to the DuckDB home directory, default: "", example: "/data/.hugr". This very important to make persistence secrets (like S3 credentials) in container environments.
- DB_PATH - path to management db file, if empty in memory storage is used, default: ""
- DB_MAX_OPEN_CONNS - maximal number of open connections to the database, default: 0 (unlimited)
- DB_MAX_IDLE_CONNS - maximal number of idle connections to the database, default: 0 (unlimited)
- DB_ALLOWED_DIRECTORIES - comma separated list of allowed directories for the database, default: "", example: "/data,/tmp"
- DB_ALLOWED_PATHS - comma separated list of allowed paths for the database, default: "", example: "/data/.local,/tmp/.local"
- DB_ENABLE_LOGGING - flag to enable logging, default: false
- DB_MAX_MEMORY - maximal memory limit for the database, default: 80% of the system memory
- DB_MAX_TEMP_DIRECTORY_SIZE - maximal size of the temporary directory, default: 80% of the system memory
- DB_TEMP_DIRECTORY - path to the temporary directory, default: ".tmp"
- DB_WORKER_THREADS - number of worker threads, default: 0 (number of CPU cores)
- DB_PG_CONNECTION_LIMIT - maximal number of connections to the database, default: 64
- DB_PG_PAGES_PER_TASK - number of pages per task, default: 1000

### Cluster settings

- CLUSTER_ENABLED - flag to enable cluster mode, default: false
- CLUSTER_ROLE - role of the node in the cluster: "management" or "worker", required when CLUSTER_ENABLED=true
- CLUSTER_NODE_NAME - name of the node, default: "", example: "mgmt-1"
- CLUSTER_NODE_URL - URL of the node for IPC communication, default: "", example: "http://mgmt-1:15000/ipc"
- CLUSTER_SECRET - secret key for the cluster communication, default: ""
- CLUSTER_HEARTBEAT - interval for node heartbeat, default: 30s
- CLUSTER_GHOST_TTL - time-to-live for unresponsive nodes before removal, default: 2m
- CLUSTER_POLL_INTERVAL - interval for workers to poll schema changes, default: 30s

**Note**: Cluster mode requires PostgreSQL as CoreDB. The `CORE_DB_PATH` must be a `postgres://` DSN shared between management and worker nodes.

### CoreDB settings

The core-db is used to store the metadata for the data sources and to manage the access to the data sources.
The core-db  stores data sources, catalog sources, roles and role permissions, and other metadata.
It can be a DuckDB file, in-memory storage or PostgreSQL. Core-db based on the PostgreSQL is required for the cluster mode (to make all replicas on the same page).
The DuckDB file can placed in the local file system or in the cloud storage (currently support only s3 cloud storage, in plan: Azure, GCS, AWS, R2), in that case path should be s3://bucket/path/to/file.duckdb.

- CORE_DB_PATH - path to core-db file or PostgreSQL DSN, if empty in memory storage is used, default: ""
- CORE_DB_READONLY - flag to open core-db in read-only mode, default: false
- CORE_DB_S3_ENDPOINT - s3 endpoint, default: ""
- CORE_DB_S3_REGION - s3 region, default: ""
- CORE_DB_S3_KEY - s3 access key, default: ""
- CORE_DB_S3_SECRET - s3 secret key, default: ""
- CORE_DB_S3_USE_SSL - flag to use SSL for s3, default: false

### CORS

- CORS_ALLOWED_ORIGINS - comma separated list of allowed origins for CORS, default: "", example: ```"http://localhost:3000,http://localhost:3001"```
- CORS_ALLOWED_METHODS - comma separated list of allowed methods for CORS, default: "GET,POST,PUT,DELETE,OPTIONS"
- CORS_ALLOWED_HEADERS - comma separated list of allowed headers for CORS, default: ""Content-Type,Authorization,x-api-key,Accept,Content-Length,Accept-Encoding,X-CSRF-Token"

### Authentication and authorization

Each node configures auth independently from its own env vars — there is no push of auth config from management to workers.

- ALLOWED_ANONYMOUS - flag to allow anonymous access, default: true
- ALLOWED_MANAGED_API_KEYS - flag to allow managed API keys (though GraphQL API), default: false
- ANONYMOUS_ROLE - role for anonymous user, default: "anonymous"
- SECRET_KEY - api key that used for authentication, default: ""
- AUTH_CONFIG_FILE - path to the file with authentication config, default: ""

The format of the config file is described in the [auth.md](auth.md) file. The config file can be in JSON or YAML format. The config file is used to configure authentication and authorization for the server.

### OIDC integration settings

- OIDC_ISSUER - OIDC issuer URL, default: "", example: ```"https://example.com"```
- OIDC_CLIENT_ID - OIDC client ID, default: "", example: "client_id"
- OIDC_CLIENT_TIMEOUT - OIDC client timeout, default: 5s, example: "5s". It is used to set the timeout for the OIDC client.
- OIDC_TLS_INSECURE - flag to disable TLS verification, default: false
- OIDC_COOKIE_NAME - OIDC cookie name, default: "", example: "hugr_oidc"
- OIDC_SCOPE_ROLE_PREFIX - OIDC scope role prefix, default: "", example: "hugr:"
- OIDC_USERNAME_CLAIM - OIDC username claim, default: "name", example: "name"
- OIDC_USERID_CLAIM - OIDC user ID claim, default: "sub", example: "sub"
- OIDC_ROLE_CLAIM - OIDC role claim, default: "x-hugr-role", example: "x-hugr-role"

### Cache

There are two types of cache: L1 and L2. L1 cache is in-memory cache (using [bigcache](https://github.com/allegro/bigcache)), L2 cache is external cache (Redis, Memcached). The L1 cache is used for the most frequently used data, while the L2 cache is used for less frequently used data.

- CACHE_TTL - time to live for cache, default: 0
- CACHE_L1_ENABLED - enabled L1 cache, default: false
- CACHE_L1_MAX_SIZE - memory limit for L1 cache in MB
- CACHE_L1_CLEAN_TIME - memory cleaning interval
- CACHE_L1_EVICTION_TIME - eviction from the L1 cache interval
- CACHE_L1_MAX_ITEM_SIZE - max size of the item in L1 cache
- CACHE_L1_SHARDS - number of shards in L1 cache
- CACHE_L2_ENABLED - enabled L2 cache, default: false
- CACHE_L2_BACKEND - L2 cache backend, can be "redis" or "memcached"
- CACHE_L2_ADDRESSES - addresses of L2 cache servers, comma separated list, default: ""
- CACHE_L2_DATABASE - database name for L2 cache (use for redis only)
- CACHE_L2_USERNAME - username for L2 cache (use for redis only)
- CACHE_L2_PASSWORD - password for L2 cache (use for redis only)

## Migration from Old Cluster Setup

If you are migrating from the old cluster setup with separate management binary:

| Old Setup | New Setup |
|-----------|-----------|
| Separate `management` binary | Same `server` binary with `CLUSTER_ROLE=management` |
| `CLUSTER_MANAGEMENT_URL=http://mgmt:14000` | `CLUSTER_ENABLED=true CLUSTER_ROLE=worker` |
| `CLUSTER_TIMEOUT=5s` | *(removed — engine handles internally)* |
| Workers receive config from management | Workers read from shared CoreDB + own env vars |

**Removed env vars**: `CLUSTER_MANAGEMENT_URL`, `CLUSTER_TIMEOUT`
**New env vars**: `CLUSTER_ENABLED`, `CLUSTER_ROLE`, `CLUSTER_HEARTBEAT`, `CLUSTER_GHOST_TTL`, `CLUSTER_POLL_INTERVAL`
**Unchanged env vars**: `CLUSTER_SECRET`, `CLUSTER_NODE_NAME`, `CLUSTER_NODE_URL`

## CoreDB migrations

For some reason it can be needed to run migrations for the core db. It makes manually by the special tool - migrate that provided in this repository (cmd/migrate). The following command run migrations:

```bash
CGO_ENABLED=1 go build -tags='duckdb_arrow' -o migrate cmd/migrate/main.go

./migrate -path /migrations -core-db core-db.duckdb
```
