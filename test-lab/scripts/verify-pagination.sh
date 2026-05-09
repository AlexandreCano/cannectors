#!/usr/bin/env bash
# Verify HTTP pagination scenarios against the local test lab.
#
# Prerequisites:
#   - `make test-lab-up` is running
#   - WireMock journal is empty (`make test-lab-requests-reset`)
#
# For each pagination type, the script runs the pipeline, then asserts that
# WireMock saw the expected pages and that the destination received all
# records in order.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
WIREMOCK_BASE="${WIREMOCK_BASE:-http://localhost:18080}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PIPELINES_DIR="$REPO_ROOT/test-lab/pipelines"

PASS=0
FAIL=0

reset_journal() {
  curl -fsS -X DELETE "$WIREMOCK_BASE/__admin/requests" >/dev/null
}

run_scenario() {
  local pipeline="$1"
  reset_journal
  "$SCRIPT_DIR/run-pipeline-once.sh" "$pipeline" >/dev/null 2>&1 || true
}

count_requests() {
  local method="$1" path="$2"
  curl -fsS "$WIREMOCK_BASE/__admin/requests" \
    | jq --arg m "$method" --arg p "$path" \
        '[.requests[] | select(.request.method==$m and (.request.url|startswith($p)))] | length'
}

destination_records() {
  local path="$1"
  curl -fsS "$WIREMOCK_BASE/__admin/requests" \
    | jq --arg p "$path" '
        [.requests[] | select(.request.method=="POST" and .request.url==$p)
           | (.request.body | fromjson | .[]?.id // .[]?.customer.id // empty)]'
}

assert_eq() {
  local label="$1" expected="$2" actual="$3"
  if [[ "$expected" == "$actual" ]]; then
    echo "  ok  $label"
    PASS=$((PASS+1))
  else
    echo "  FAIL $label: expected $expected got $actual"
    FAIL=$((FAIL+1))
  fi
}

echo "[scenario] pagination-page"
run_scenario "$PIPELINES_DIR/pagination-page.yaml"
assert_eq "page requests count" 3 "$(count_requests GET /source/paginated/customers)"
assert_eq "destination ids order" \
  '["CUST-001","CUST-002","CUST-003","CUST-004","CUST-005","CUST-006"]' \
  "$(destination_records /destination/pagination/customers | jq -c .)"

echo "[scenario] pagination-offset"
run_scenario "$PIPELINES_DIR/pagination-offset.yaml"
assert_eq "offset requests count" 3 "$(count_requests GET /source/paginated/orders)"
assert_eq "destination ids order" \
  '["ORD-1001","ORD-1002","ORD-1003","ORD-1004","ORD-1005","ORD-1006"]' \
  "$(destination_records /destination/pagination/orders | jq -c .)"

echo "[scenario] pagination-cursor"
run_scenario "$PIPELINES_DIR/pagination-cursor.yaml"
assert_eq "cursor requests count" 3 "$(count_requests GET /source/paginated/inventory)"
assert_eq "destination ids order" \
  '["INV-001","INV-002","INV-003","INV-004","INV-005","INV-006"]' \
  "$(destination_records /destination/pagination/inventory | jq -c .)"

echo
echo "pagination scenarios: $PASS passed, $FAIL failed"
exit $((FAIL > 0))
