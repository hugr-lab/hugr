# HUGR

The DataMesh service that provides access to various data sources through common GraphQL API.

The HUGR is built on the top of [DuckDB](https://duckdb.org) and uses it as an calculation engine. The Hugr can work with following data sources:

- PostgreSQL (incl. with extensions: PostGIS, TimescaleDB)
- DuckDB files
- HTTP REST API (support OpenAPI v3)
- All file formats and data sources that supported by DuckDB (CSV, Parquet, JSON, ESRI Shape, etc.)

Files can be stored in the local file system or in the cloud storage (currently support only s3 cloud storage, in plan: Azure, GCS, AWS, R2).

MySQL, SQLLite, Clickhouse will be supported in the future.

## Executable

The executable is built with Go and can be run on any platform that supports Go. The executable is built with the following command:

```bash
CG_ENABLED=1 go build -o hugr cmd/server/main.go
```

It used the following packages:

- [github.com/marcboeker/go-duckdb/v2](https://github.com/marcboeker/go-duckdb)
- [github.com/apache/arrow-go/v18](https://github.com/apache/arrow-go)
- [github.com/paulmach/orb](https://github.com/paulmach/orb)
- [github.com/vektah/gqlparser/v2](https://github.com/vektah/gqlparser)
- [github.com/eko/gocache/v4](https://github.com/eko/gocache)
- [github.com/vmihailenco/msgpack/v5](https://github.com/vmihailenco/msgpack)
- [github.com/itchyny/gojq](https://github.com/itchyny/gojq)
- [github.com/golang-jwt/jwt/v5](https://github.com/golang-jwt/jwt)
- [github.com/getkin/kin-openapi](https://github.com/getkin/kin-openapi)

## Environment variables

### General

- BIND - string, that defines network interface and port, default: :15000
- ADMIN_UI - flag to enable AdminUI, for path /admin ([GraphiQL](https://github.com/graphql/graphiql)), default: true
- ADMIN_UI_FETCH_PATH - path to fetch AdminUI, default: "/admin"
- DEBUG - flag to run in debug mode (SQL queries will output to the stdout), default: false
- ALLOW_PARALLEL - flag to allow run queries in parallel, default: true
- MAX_PARALLEL_QUERIES - limit to numbers of parallels queries executed, default: 0 (unlimited)
- MAX_DEPTH - maximal depth of GraphQL types hierarchy, default: 7

### DuckDB engine settings

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
- CORE_DB_PATH - path to core-db file, if empty in memory storage is used, default: ""
- CORE_DB_READONLY - flag to open core-db in read-only mode, default: false

### CORS

- CORS_ALLOWED_ORIGINS - comma separated list of allowed origins for CORS, default: "", example: ```"http://localhost:3000,http://localhost:3001"```
- CORS_ALLOWED_METHODS - comma separated list of allowed methods for CORS, default: "GET,POST,PUT,DELETE,OPTIONS"
- CORS_ALLOWED_HEADERS - comma separated list of allowed headers for CORS, default: ""Content-Type,Authorization,x-api-key,Accept,Content-Length,Accept-Encoding,X-CSRF-Token"

### Authentication and authorization

- ALLOWED_ANONYMOUS - flag to allow anonymous access, default: true
- ANONYMOUS_ROLE - role for anonymous user, default: "anonymous"
- SECRET_KEY - api key that used for authentication, default: ""
- AUTH_CONFIG_FILE - path to the file with authentication config, default: ""

The format of the config file is described in the [auth.md](auth.md) file. The config file can be in JSON or YAML format. The config file is used to configure authentication and authorization for the server.

### Cache

There are two types of cache: L1 and L2. L1 cache is in-memory cache (using [bigcache](https://github.com/allegro/bigcache)), L2 cache is external cache (Redis, Memcached, Pegasus). The L1 cache is used for the most frequently used data, while the L2 cache is used for less frequently used data.

- CACHE_TTL - time to live for cache, default: 0
- CACHE_L1_ENABLED - enabled L1 cache, default: false
- CACHE_L1_MAX_SIZE - memory limit for L1 cache in MB
- CACHE_L1_CLEAN_TIME - memory cleaning interval
- CACHE_L1_EVICTION_TIME - eviction from the L1 cache interval
- CACHE_L1_MAX_ITEM_SIZE - max size of the item in L1 cache
- CACHE_L1_SHARDS - number of shards in L1 cache
- CACHE_L2_ENABLED - enabled L2 cache, default: false
- CACHE_L2_BACKEND - L2 cache backend, can be "redis", "memcached" or "pegasus"
- CACHE_L2_ADDRESSES - addresses of L2 cache servers, comma separated list, default: ""
- CACHE_L2_DATABASE - database name for L2 cache (use for redis only)
- CACHE_L2_USERNAME - username for L2 cache (use for redis only)
- CACHE_L2_PASSWORD - password for L2 cache (use for redis only)


## CoreDB migrations

For some reason it can be needed to run migrations for the core db. It makes manually by the special tool - migrate that provided in this repository (cmd/migrate). The following command run migrations:

```bash
CG_ENABLED=1 go build -o migrate cmd/migrate/main.go

./migrate -path /migrations -core-db core-db.duckdb
```
