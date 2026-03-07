#!/bin/bash
# Provisions data sources on the cluster management node.
# Usage: ./provision-cluster.sh <mgmt_url> [worker1_url]

set -eo pipefail

MGMT_URL="${1:-http://localhost:15010}"
WORKER1_URL="${2:-http://localhost:15011}"

gql() {
  local url="$1"
  local query="$2"
  local result
  result=$(curl -sf -X POST "$url/query" \
    -H "Content-Type: application/json" \
    -d "{\"query\": \"$query\"}")
  echo "$result"
  if echo "$result" | jq -e '.errors | length > 0' > /dev/null 2>&1; then
    echo "  ERROR: $(echo "$result" | jq -r '.errors[0].message')" >&2
    return 1
  fi
  return 0
}

echo "Provisioning cluster data sources on $MGMT_URL..."

# Clean existing sources
echo "  Cleaning existing data sources..."
for src in pg_store; do
  gql "$MGMT_URL" "mutation { core { delete_data_source_catalogs(filter: { data_source_name: { eq: \\\"$src\\\" } }) { data_source_name } } }" > /dev/null 2>&1 || true
  gql "$MGMT_URL" "mutation { core { delete_catalog_sources(filter: { name: { eq: \\\"$src\\\" } }) { name } } }" > /dev/null 2>&1 || true
  gql "$MGMT_URL" "mutation { core { delete_data_sources(filter: { name: { eq: \\\"$src\\\" } }) { name } } }" > /dev/null 2>&1 || true
done

# Register PostgreSQL data source (shared across all cluster nodes)
echo "  Registering pg_store..."
gql "$MGMT_URL" 'mutation { core { insert_data_sources(data: { name: \"pg_store\", prefix: \"pg_store\", type: \"postgres\", path: \"postgres://test:test@postgres:5432/testdb\", as_module: true, catalogs: [{ name: \"pg_store\", type: \"localFS\", path: \"/workspace/schemas/pg_store\" }] }) { name } } }'

# Load source via cluster mutation (management compiles + broadcasts to workers)
echo "  Loading pg_store via cluster..."
gql "$MGMT_URL" "mutation { function { core { cluster { load_source(name: \\\"pg_store\\\") { success message } } } } }"

# Wait for workers to sync
echo "  Waiting for workers to sync schema..."
for i in $(seq 1 30); do
  result=$(curl -sf -X POST "$WORKER1_URL/query" \
    -H "Content-Type: application/json" \
    -d '{"query": "{ pg_store { products(limit: 1) { id } } }"}' 2>/dev/null) && \
  echo "$result" | jq -e '.data.pg_store.products' > /dev/null 2>&1 && {
    echo "  Worker-1 verified: pg_store queryable"
    break
  }
  if [ "$i" -eq 30 ]; then
    echo "  WARNING: Worker verification timed out after 30 attempts"
  fi
  sleep 1
done

echo "Cluster provisioning complete."
