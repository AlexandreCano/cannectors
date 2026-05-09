#!/usr/bin/env bash
# Verify database input/output scenarios for story 22.2.
#
# Prerequisites:
#   - `make test-lab-up` (PostgreSQL + WireMock running)
#   - `make test-lab-db-reset` to start from a clean seed
#   - `make test-lab-requests-reset`
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PIPELINES_DIR="$REPO_ROOT/test-lab/pipelines"
ASSERTIONS_DIR="$REPO_ROOT/test-lab/postgres/assertions"
COMPOSE="docker compose -f $REPO_ROOT/test-lab/docker-compose.yml"
WIREMOCK_BASE="${WIREMOCK_BASE:-http://localhost:18080}"

PASS=0
FAIL=0

reset_journal() {
  curl -fsS -X DELETE "$WIREMOCK_BASE/__admin/requests" >/dev/null
}

run_pipeline() {
  "$SCRIPT_DIR/run-pipeline-once.sh" "$1" >/dev/null 2>&1 || true
}

psql_run() {
  $COMPOSE exec -T postgres psql -U cannectors_test -d cannectors_test -At -F '|' -X "$@"
}

assert_eq() {
  local label="$1" expected="$2" actual="$3"
  if [[ "$expected" == "$actual" ]]; then
    echo "  ok  $label"
    PASS=$((PASS+1))
  else
    echo "  FAIL $label"
    echo "    expected: $expected"
    echo "    actual:   $actual"
    FAIL=$((FAIL+1))
  fi
}

reset_dest() {
  psql_run -c "DELETE FROM dest_customers WHERE external_id LIKE 'DBOUT-%'; DELETE FROM dest_orders WHERE external_order_id LIKE 'DBOUT-%';" >/dev/null
}

count_destination_records() {
  local path="$1"
  curl -fsS "$WIREMOCK_BASE/__admin/requests" \
    | jq --arg p "$path" '
        [.requests[] | select(.request.method=="POST" and .request.url==$p)
           | (.request.body | fromjson | .[]?)] | length'
}

# --- Database input scenarios -------------------------------------------------

echo "[scenario] db-input-basic"
reset_journal
run_pipeline "$PIPELINES_DIR/db-input-basic.yaml"
assert_eq "destination received seeded customers" 6 \
  "$(count_destination_records /destination/db-input/customers)"

echo "[scenario] db-input-parameters"
reset_journal
run_pipeline "$PIPELINES_DIR/db-input-parameters.yaml"
assert_eq "destination received only active customers" 4 \
  "$(count_destination_records /destination/db-input/active-customers)"

echo "[scenario] db-input-query-file"
reset_journal
run_pipeline "$PIPELINES_DIR/db-input-query-file.yaml"
assert_eq "destination received only paid orders" 4 \
  "$(count_destination_records /destination/db-input/paid-orders)"

# --- Database output scenarios ------------------------------------------------

echo "[scenario] db-output-insert"
reset_dest
run_pipeline "$PIPELINES_DIR/db-output-insert.yaml"
assert_eq "dest_customers count for DBOUT-001..003" 3 \
  "$(psql_run -c "SELECT COUNT(*) FROM dest_customers WHERE external_id IN ('DBOUT-001','DBOUT-002','DBOUT-003')")"

echo "[scenario] db-output-upsert-query-file"
reset_dest
# First run inserts, second run with different segment must update.
run_pipeline "$PIPELINES_DIR/db-output-upsert-query-file.yaml"
psql_run -c "UPDATE dest_customers SET segment='legacy' WHERE external_id IN ('DBOUT-001','DBOUT-002','DBOUT-003')" >/dev/null
run_pipeline "$PIPELINES_DIR/db-output-upsert-query-file.yaml"
assert_eq "queryFile upsert restored segment 'premium' for DBOUT-001" "premium" \
  "$(psql_run -c "SELECT segment FROM dest_customers WHERE external_id='DBOUT-001'")"

echo "[scenario] db-output-transaction-rollback"
reset_dest
run_pipeline "$PIPELINES_DIR/db-output-transaction-rollback.yaml"
assert_eq "transaction rolled back: no DBOUT-10x in dest_customers" 0 \
  "$(psql_run -c "SELECT COUNT(*) FROM dest_customers WHERE external_id IN ('DBOUT-100','DBOUT-101','DBOUT-102')")"

echo "[scenario] db-output-on-error-skip"
reset_dest
run_pipeline "$PIPELINES_DIR/db-output-on-error-skip.yaml"
assert_eq "skip: DBOUT-100 inserted" 1 \
  "$(psql_run -c "SELECT COUNT(*) FROM dest_customers WHERE external_id='DBOUT-100'")"
assert_eq "skip: DBOUT-101 absent (email conflict skipped)" 0 \
  "$(psql_run -c "SELECT COUNT(*) FROM dest_customers WHERE external_id='DBOUT-101'")"
assert_eq "skip: DBOUT-102 inserted" 1 \
  "$(psql_run -c "SELECT COUNT(*) FROM dest_customers WHERE external_id='DBOUT-102'")"

echo "[scenario] db-output-on-error-log"
reset_dest
run_pipeline "$PIPELINES_DIR/db-output-on-error-log.yaml"
assert_eq "log: DBOUT-200 inserted" 1 \
  "$(psql_run -c "SELECT COUNT(*) FROM dest_customers WHERE external_id='DBOUT-200'")"
assert_eq "log: DBOUT-201 absent (missing email logged, not inserted)" 0 \
  "$(psql_run -c "SELECT COUNT(*) FROM dest_customers WHERE external_id='DBOUT-201'")"
assert_eq "log: DBOUT-202 inserted" 1 \
  "$(psql_run -c "SELECT COUNT(*) FROM dest_customers WHERE external_id='DBOUT-202'")"

reset_dest

echo
echo "database scenarios: $PASS passed, $FAIL failed"
exit $((FAIL > 0))
