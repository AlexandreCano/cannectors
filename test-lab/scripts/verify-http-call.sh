#!/usr/bin/env bash
# Verify http_call enrichment scenarios for story 22.4.
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
    | jq -c --arg p "$1" '[.requests[] | select(.request.method=="POST" and .request.url==$p) | (.request.body | fromjson | .[]?)]'
}

count_method_path() {
  local method="$1" pattern="$2"
  curl -fsS "$WIREMOCK_BASE/__admin/requests" \
    | jq --arg m "$method" --arg p "$pattern" '[.requests[] | select(.request.method==$m and (.request.url | test($p)))] | length'
}

count_with_header() {
  local header="$1" value="$2"
  curl -fsS "$WIREMOCK_BASE/__admin/requests" \
    | jq --arg h "$header" --arg v "$value" '[.requests[] | select(.request.headers[$h] == $v)] | length'
}

echo "[scenario] http-call-path-merge (cache)"
reset_journal
run_pipeline "$PIPELINES_DIR/http-call-path-merge.yaml"
records=$(batch_records /destination/enrichment/http-merge)
# 5 source records, 3 distinct customer ids (CUST-001 x3, CUST-002, CUST-999) → 3 unique GETs
assert_eq "path: GET /enrichment/customers/.* called 3 times (cache hit on duplicates)" 3 \
  "$(count_method_path GET '^/enrichment/customers/CUST-')"
assert_eq "path: ORD-A1 enriched with tier=gold" "gold" \
  "$(echo "$records" | jq -r '.[] | select(.order_id=="ORD-A1") | .tier')"
assert_eq "path: ORD-A2 enriched with tier=silver" "silver" \
  "$(echo "$records" | jq -r '.[] | select(.order_id=="ORD-A2") | .tier')"
assert_eq "path: ORD-A5 (404) skipped from output" "null" \
  "$(echo "$records" | jq -r 'map(select(.order_id=="ORD-A5")) | (.[0] // null) | (.order_id // "null")')"

echo "[scenario] http-call-query-replace"
reset_journal
run_pipeline "$PIPELINES_DIR/http-call-query-replace.yaml"
records=$(batch_records /destination/enrichment/http-replace)
assert_eq "query: GET /enrichment/segments?customerId=... received" 1 \
  "$(curl -fsS "$WIREMOCK_BASE/__admin/requests" | jq '[.requests[] | select(.request.url | test("^/enrichment/segments\\?customerId=CUST-001"))] | length // 0' | head -1)"
assert_eq "query: ORD-A1 has segment from response" "premium" \
  "$(echo "$records" | jq -r '.[] | select(.order_id=="ORD-A1") | .segment')"
assert_eq "query: ORD-A1 score from response" "92" \
  "$(echo "$records" | jq -r '.[] | select(.order_id=="ORD-A1") | .score')"

echo "[scenario] http-call-header-append"
reset_journal
run_pipeline "$PIPELINES_DIR/http-call-header-append.yaml"
records=$(batch_records /destination/enrichment/http-append)
assert_eq "header: 3 unique X-Customer-Id requests (cache hit on duplicates)" 3 \
  "$(curl -fsS "$WIREMOCK_BASE/__admin/requests" | jq '[.requests[] | select(.request.url=="/enrichment/by-header" and ((.request.headers["X-Customer-Id"] // "") != ""))] | length')"
assert_eq "header: ORD-A1 has _response.permissions[0]=read" "read" \
  "$(echo "$records" | jq -r '.[] | select(.order_id=="ORD-A1") | ._response.permissions[0]')"

echo "[scenario] http-call-post-template"
reset_journal
run_pipeline "$PIPELINES_DIR/http-call-post-template.yaml"
records=$(batch_records /destination/enrichment/http-post)
assert_eq "post: POST /enrichment/geocode called twice (one per address)" 2 \
  "$(count_method_path POST '^/enrichment/geocode$')"
assert_eq "post: body template substituted address.line1 for both addresses" "1 rue Test|2 av Lab" \
  "$(curl -fsS "$WIREMOCK_BASE/__admin/requests" | jq -r '[.requests[] | select(.request.method=="POST" and .request.url=="/enrichment/geocode") | (.request.body | fromjson | .address)] | sort | join("|")')"
assert_eq "post: ADDR-1 record has regionCode after replace" "FR-IDF" \
  "$(echo "$records" | jq -r '.[] | select(.id=="ADDR-1") | .regionCode')"

echo
echo "http_call scenarios: $PASS passed, $FAIL failed"
exit $((FAIL > 0))
