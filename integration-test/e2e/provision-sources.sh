#!/bin/bash
# Registers and loads a PostgreSQL data source via GraphQL mutations.
# Usage: ./provision-sources.sh <engine_url>

set -e

ENGINE_URL="${1:-http://localhost:15000}"

gql() {
  local result
  result=$(curl -sf -X POST "$ENGINE_URL/query" \
    -H "Content-Type: application/json" \
    -d "{\"query\": \"$1\"}")
  echo "$result"
  if echo "$result" | jq -e '.errors | length > 0' > /dev/null 2>&1; then
    echo "  ERROR: $(echo "$result" | jq -r '.errors[0].message')" >&2
    return 1
  fi
  return 0
}

echo "Provisioning data sources on $ENGINE_URL..."

# Clean existing sources
echo "  Cleaning existing data sources..."
for src in pg_store; do
  gql "mutation { core { delete_data_source_catalogs(filter: { data_source_name: { eq: \\\"$src\\\" } }) { data_source_name } } }" > /dev/null 2>&1 || true
  gql "mutation { core { delete_catalog_sources(filter: { name: { eq: \\\"$src\\\" } }) { name } } }" > /dev/null 2>&1 || true
  gql "mutation { core { delete_data_sources(filter: { name: { eq: \\\"$src\\\" } }) { name } } }" > /dev/null 2>&1 || true
done

# Register PostgreSQL data source
echo "  Registering pg_store..."
gql 'mutation { core { insert_data_sources(data: { name: \"pg_store\", prefix: \"pg_store\", type: \"postgres\", path: \"postgres://test:test@postgres:5432/testdb\", as_module: true, catalogs: [{ name: \"pg_store\", type: \"localFS\", path: \"/workspace/schemas/pg_store\" }] }) { name } } }'

# Load source
echo "  Loading pg_store..."
gql "mutation { function { core { load_data_source(name: \\\"pg_store\\\") { success message } } } }"

echo "Data sources provisioned and loaded."
