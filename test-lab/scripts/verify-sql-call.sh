#!/usr/bin/env bash
# Verify sql_call enrichment scenarios for story 22.3.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PIPELINES_DIR="$REPO_ROOT/test-lab/pipelines"
WIREMOCK_BASE="${WIREMOCK_BASE:-http://localhost:18080}"

PASS=0
FAIL=0

reset_journal() { curl -fsS -X DELETE "$WIREMOCK_BASE/__admin/requests" >/dev/null; }

run_pipeline() { "$SCRIPT_DIR/run-pipeline-once.sh" "$1" >/dev/null 2>&1 || true; }

assert_eq() {
  local label="$1" expected="$2" actual="$3"
  if [[ "$expected" == "$actual" ]]; then
    echo "  ok  $label"; PASS=$((PASS+1))
  else
    echo "  FAIL $label"; echo "    expected: $expected"; echo "    actual:   $actual"; FAIL=$((FAIL+1))
  fi
}

batch_records() {
  curl -fsS "$WIREMOCK_BASE/__admin/requests" \
    | jq -c --arg p "$1" '
        [.requests[] | select(.request.method=="POST" and .request.url==$p)
           | (.request.body | fromjson | .[]?)]'
}

echo "[scenario] sql-call-merge"
reset_journal
run_pipeline "$PIPELINES_DIR/sql-call-merge.yaml"
records=$(batch_records /destination/enrichment/merge)
assert_eq "merge: ORD-A1 segment is 'premium'" "premium" \
  "$(echo "$records" | jq -r '.[] | select(.order_id=="ORD-A1") | .segment')"
assert_eq "merge: ORD-A2 segment is 'enterprise'" "enterprise" \
  "$(echo "$records" | jq -r '.[] | select(.order_id=="ORD-A2") | .segment')"
assert_eq "merge: ORD-A1 retains original amount" "100" \
  "$(echo "$records" | jq -r '.[] | select(.order_id=="ORD-A1") | .amount')"
assert_eq "merge: ORD-A5 (no SQL match) keeps original record only" "null" \
  "$(echo "$records" | jq -r '.[] | select(.order_id=="ORD-A5") | .segment')"

echo "[scenario] sql-call-replace"
reset_journal
run_pipeline "$PIPELINES_DIR/sql-call-replace.yaml"
records=$(batch_records /destination/enrichment/replace)
assert_eq "replace: first record external_id replaced" "CUST-001" \
  "$(echo "$records" | jq -r '.[0].external_id')"
assert_eq "replace: original order_id preserved (shallow overwrite)" "ORD-A1" \
  "$(echo "$records" | jq -r '.[0].order_id')"

echo "[scenario] sql-call-append"
reset_journal
run_pipeline "$PIPELINES_DIR/sql-call-append.yaml"
records=$(batch_records /destination/enrichment/append)
assert_eq "append: L-1 reference.product_name set" "Local Widget" \
  "$(echo "$records" | jq -r '.[] | select(.line_id=="L-1") | .reference.product_name')"
assert_eq "append: L-2 reference.category set" "gadgets" \
  "$(echo "$records" | jq -r '.[] | select(.line_id=="L-2") | .reference.category')"
assert_eq "append: original sku preserved on L-1" "SKU-001" \
  "$(echo "$records" | jq -r '.[] | select(.line_id=="L-1") | .sku')"
assert_eq "append: L-4 (unknown sku) has empty reference object" "{}" \
  "$(echo "$records" | jq -c '.[] | select(.line_id=="L-4") | .reference')"

echo "[scenario] sql-call-query-file"
reset_journal
run_pipeline "$PIPELINES_DIR/sql-call-query-file.yaml"
records=$(batch_records /destination/enrichment/query-file)
assert_eq "query-file: ORD-A1 enriched with segment from file-loaded query" "premium" \
  "$(echo "$records" | jq -r '.[] | select(.order_id=="ORD-A1") | .segment')"
assert_eq "query-file: ORD-A2 enriched with region 'north-america'" "north-america" \
  "$(echo "$records" | jq -r '.[] | select(.order_id=="ORD-A2") | .region')"

echo
echo "sql_call scenarios: $PASS passed, $FAIL failed"
exit $((FAIL > 0))
