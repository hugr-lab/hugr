#!/bin/bash
# Create additional databases for e2e tests.
# core-postgres auto-creates POSTGRES_DB=coredb_e2e; this script adds coredb_cluster.
set -e
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    CREATE DATABASE coredb_cluster OWNER $POSTGRES_USER;
EOSQL
