#!/bin/bash
# E2E test runner for hugr.
# Tests standalone mode, PostgreSQL CoreDB mode, and cluster mode.
#
# Usage:
#   ./run.sh              # Full run with teardown
#   ./run.sh --keep       # Keep containers running after tests
#   ./run.sh --standalone # Only standalone tests (no cluster)
#   UPDATE_EXPECTED=1 ./run.sh  # Update expected output files

set -eo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
COMPOSE_FILE="$SCRIPT_DIR/docker-compose.yml"
QUERIES_DIR="$SCRIPT_DIR/testdata/queries"

PASS=0
FAIL=0
SKIP=0
KEEP=false
STANDALONE_ONLY=false

JQ_DEEP_SORT='def sort_arrays: if type == "array" then map(sort_arrays) | sort_by(if type == "object" and has("name") then .name else tostring end) elif type == "object" then to_entries | sort_by(.key) | map(.value = (.value | sort_arrays)) | from_entries else . end; . | sort_arrays'

for arg in "$@"; do
  case $arg in
    --keep) KEEP=true ;;
    --standalone) STANDALONE_ONLY=true ;;
  esac
done

cleanup() {
  if [ "$KEEP" = false ]; then
    echo ""
    echo "Tearing down..."
    docker compose -f "$COMPOSE_FILE" down -v 2>/dev/null || true
  else
    echo ""
    echo "Containers kept running. Tear down with:"
    echo "  docker compose -f $COMPOSE_FILE down -v"
  fi
}

trap cleanup EXIT

# Build and start
echo "Starting E2E environment..."
if [ "$STANDALONE_ONLY" = true ]; then
  docker compose -f "$COMPOSE_FILE" up -d --build --wait hugr-standalone
else
  docker compose -f "$COMPOSE_FILE" up -d --build --wait
fi

run_single_test() {
  local engine_url="$1"
  local test_dir="$2"
  local test_name="$3"
  local variant="${4:-}"
  local query_file="$test_dir/query.graphql"
  local expected_file="$test_dir/expected.json"
  if [ -n "$variant" ] && [ -f "$test_dir/expected_${variant}.json" ]; then
    expected_file="$test_dir/expected_${variant}.json"
  fi

  if [ ! -f "$query_file" ]; then
    return 1
  fi

  local query
  query=$(cat "$query_file")
  local actual
  actual=$(curl -sf -X POST "$engine_url/query" \
    -H "Content-Type: application/json" \
    -d "{\"query\": $(echo "$query" | jq -Rs .)}" 2>/dev/null) || {
    echo "  FAIL: $test_name (request failed)"
    FAIL=$((FAIL + 1))
    return 0
  }

  if [ "${UPDATE_EXPECTED:-0}" = "1" ]; then
    echo "$actual" | jq -S . > "$expected_file"
    echo "  UPDATED: $test_name"
    PASS=$((PASS + 1))
    return 0
  fi

  if [ ! -f "$expected_file" ]; then
    echo "  SKIP: $test_name (no expected.json)"
    SKIP=$((SKIP + 1))
    return 0
  fi

  local actual_norm expected_norm
  actual_norm=$(echo "$actual" | jq -S "$JQ_DEEP_SORT" 2>/dev/null) || actual_norm="$actual"
  expected_norm=$(jq -S "$JQ_DEEP_SORT" "$expected_file" 2>/dev/null) || expected_norm=$(cat "$expected_file")

  if [ "$actual_norm" = "$expected_norm" ]; then
    echo "  PASS: $test_name"
    PASS=$((PASS + 1))
  else
    echo "  FAIL: $test_name"
    diff <(echo "$expected_norm") <(echo "$actual_norm") || true
    FAIL=$((FAIL + 1))
  fi
}

run_tests_against() {
  local engine_url="$1"
  local label="$2"
  local category="$3"
  local variant="${4:-}"

  echo ""
  echo "Running $category tests against $label ($engine_url)..."

  local category_dir="$QUERIES_DIR/$category"
  [ -d "$category_dir" ] || { echo "  No tests found in $category"; return; }

  for test_dir in "$category_dir"/*/; do
    [ -d "$test_dir" ] || continue
    test_name="$category/$(basename "$test_dir")"
    if [ -f "$test_dir/query.graphql" ]; then
      run_single_test "$engine_url" "$test_dir" "$test_name" "$variant"
    fi
  done
}

resolve_cluster_url() {
  local query_file="$1"
  local mgmt_url="$2"
  local w1_url="$3"

  local target
  target=$(head -1 "$query_file" | grep -o '@target: *[a-z0-9_-]*' | sed 's/@target: *//' || true)

  case "$target" in
    mgmt|management) echo "$mgmt_url" ;;
    worker-1)        echo "$w1_url" ;;
    *)               echo "$mgmt_url" ;;
  esac
}

run_cluster_tests() {
  local mgmt_url="$1"
  local w1_url="$2"
  local cluster_dir="$QUERIES_DIR/cluster"

  echo ""
  echo "Running cluster tests (mgmt=$mgmt_url, w1=$w1_url)..."

  for test_dir in "$cluster_dir"/*/; do
    [ -d "$test_dir" ] || continue
    local test_name="cluster/$(basename "$test_dir")"

    local steps
    steps=$(ls "$test_dir"/*.graphql 2>/dev/null | sort)
    [ -z "$steps" ] && continue
    local step_count
    step_count=$(echo "$steps" | wc -l | tr -d ' ')

    local all_pass=true
    local step_num=0

    for query_file in $steps; do
      step_num=$((step_num + 1))
      local base
      base=$(basename "$query_file" .graphql)
      local expected_file="$test_dir/${base}.json"

      local step_url
      step_url=$(resolve_cluster_url "$query_file" "$mgmt_url" "$w1_url")

      local query
      query=$(grep -v '^# *@target:' "$query_file")
      local actual
      actual=$(curl -sf -X POST "$step_url/query" \
        -H "Content-Type: application/json" \
        -d "{\"query\": $(echo "$query" | jq -Rs .)}" 2>/dev/null) || {
        echo "  FAIL: $test_name (step $step_num request failed)"
        FAIL=$((FAIL + 1))
        all_pass=false
        break
      }

      if [ "${UPDATE_EXPECTED:-0}" = "1" ]; then
        echo "$actual" | jq -S . > "$expected_file"
        continue
      fi

      if [ ! -f "$expected_file" ]; then
        echo "  SKIP: $test_name (step $step_num: no expected file)"
        SKIP=$((SKIP + 1))
        all_pass=false
        break
      fi

      local actual_norm expected_norm
      actual_norm=$(echo "$actual" | jq -S "$JQ_DEEP_SORT" 2>/dev/null) || actual_norm="$actual"
      expected_norm=$(jq -S "$JQ_DEEP_SORT" "$expected_file" 2>/dev/null) || expected_norm=$(cat "$expected_file")

      if [ "$actual_norm" != "$expected_norm" ]; then
        echo "  FAIL: $test_name (step $step_num)"
        diff <(echo "$expected_norm") <(echo "$actual_norm") || true
        all_pass=false
        break
      fi
    done

    if [ "${UPDATE_EXPECTED:-0}" = "1" ]; then
      echo "  UPDATED: $test_name ($step_count steps)"
      PASS=$((PASS + 1))
      continue
    fi

    if [ "$all_pass" = true ]; then
      echo "  PASS: $test_name ($step_count steps)"
      PASS=$((PASS + 1))
    else
      FAIL=$((FAIL + 1))
    fi
  done
}

# === Test execution ===

# 1. Standalone tests (DuckDB CoreDB)
STANDALONE_URL="http://localhost:15000"
echo ""
echo "Provisioning data sources (standalone)..."
"$SCRIPT_DIR/provision-sources.sh" "$STANDALONE_URL"
run_tests_against "$STANDALONE_URL" "Standalone (DuckDB CoreDB)" "standalone"

# 2. PostgreSQL CoreDB tests
if [ "$STANDALONE_ONLY" = false ]; then
  PG_URL="http://localhost:15001"
  echo ""
  echo "Provisioning data sources (PostgreSQL CoreDB)..."
  "$SCRIPT_DIR/provision-sources.sh" "$PG_URL"
  run_tests_against "$PG_URL" "PostgreSQL CoreDB" "standalone" "pg"
fi

# 3. Cluster tests
if [ "$STANDALONE_ONLY" = false ]; then
  CLUSTER_MGMT_URL="http://localhost:15010"
  CLUSTER_W1_URL="http://localhost:15011"

  echo ""
  echo "Provisioning cluster data sources..."
  "$SCRIPT_DIR/provision-cluster.sh" "$CLUSTER_MGMT_URL" "$CLUSTER_W1_URL"

  run_cluster_tests "$CLUSTER_MGMT_URL" "$CLUSTER_W1_URL"
fi

# Summary
echo ""
echo "Results: $PASS passed, $FAIL failed, $SKIP skipped"

if [ $FAIL -gt 0 ]; then
  exit 1
fi
exit 0
