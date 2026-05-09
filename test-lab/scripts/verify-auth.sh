#!/usr/bin/env bash
# Verify HTTP authentication scenarios for story 23.1.
#
# For each pipeline:
#   - reset the WireMock journal
#   - run the pipeline once
#   - assert the source / token / destination journal matches expectations
#
# WireMock @ http://localhost:18080
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PIPELINES_DIR="$REPO_ROOT/test-lab/pipelines"
WM="${WIREMOCK_BASE:-http://localhost:18080}"

PASS=0
FAIL=0

reset_journal() { curl -fsS -X DELETE "$WM/__admin/requests" >/dev/null; }

# Run a pipeline once. The pipeline log is captured for assertions.
run_pipeline() {
  local pipeline="$1"
  LAST_LOG="$(mktemp)"
  bash "$SCRIPT_DIR/run-pipeline-once.sh" "$pipeline" >"$LAST_LOG" 2>&1 || true
}

assert_eq() {
  local label="$1" expected="$2" actual="$3"
  if [[ "$expected" == "$actual" ]]; then
    echo "  ok  $label"; PASS=$((PASS+1))
  else
    echo "  FAIL $label"; echo "    expected: $expected"; echo "    actual:   $actual"; FAIL=$((FAIL+1))
  fi
}

# Count requests matching method+urlPath where the responseDefinition.status equals the given status.
count_with_status() {
  local method="$1" urlpath="$2" status="$3"
  curl -fsS "$WM/__admin/requests" \
    | jq --arg m "$method" --arg u "$urlpath" --argjson s "$status" \
        '[.requests[] | select(.request.method==$m and (.request.url | split("?")[0])==$u and .responseDefinition.status==$s)] | length'
}

count_with_header_value() {
  local method="$1" urlpath="$2" header="$3" value="$4"
  curl -fsS "$WM/__admin/requests" \
    | jq --arg m "$method" --arg u "$urlpath" --arg h "$header" --arg v "$value" \
        '[.requests[] | select(.request.method==$m and (.request.url | split("?")[0])==$u and (.request.headers[$h] // "")==$v)] | length'
}

count_with_query() {
  local method="$1" urlpath="$2" param="$3" value="$4"
  curl -fsS "$WM/__admin/requests" \
    | jq --arg m "$method" --arg u "$urlpath" --arg p "$param" --arg v "$value" \
        '[.requests[] | select(.request.method==$m and (.request.url | split("?")[0])==$u and ((.request.queryParams[$p].values // [])[0]==$v))] | length'
}

last_status_for() {
  local pipeline_id="$1"
  grep '"msg":"execution completed"' "$LAST_LOG" \
    | jq -r --arg id "$pipeline_id" 'select(."pipeline_id"==$id) | .status' \
    | tail -1
}

# --- 1. api-key in header ----------------------------------------------------
echo "[scenario] auth-input-api-key-header"
reset_journal
run_pipeline "$PIPELINES_DIR/auth-input-api-key-header.yaml"
assert_eq "source got 1 successful api-key-header request" 1 \
  "$(count_with_header_value GET /auth/source/api-key-header X-Api-Key lab-api-key-header)"
assert_eq "destination received 1 batch" 1 \
  "$(count_with_status POST /auth/destination/public 200)"
assert_eq "pipeline status=success" success "$(last_status_for auth-input-api-key-header)"

# --- 2. api-key in query -----------------------------------------------------
echo "[scenario] auth-input-api-key-query"
reset_journal
run_pipeline "$PIPELINES_DIR/auth-input-api-key-query.yaml"
assert_eq "source got 1 successful api-key-query request" 1 \
  "$(count_with_query GET /auth/source/api-key-query api_key lab-api-key-query)"
assert_eq "destination received 1 batch" 1 \
  "$(count_with_status POST /auth/destination/public 200)"
assert_eq "pipeline status=success" success "$(last_status_for auth-input-api-key-query)"

# --- 3. bearer ---------------------------------------------------------------
echo "[scenario] auth-input-bearer"
reset_journal
run_pipeline "$PIPELINES_DIR/auth-input-bearer.yaml"
assert_eq "source got 1 successful bearer request" 1 \
  "$(count_with_header_value GET /auth/source/bearer Authorization 'Bearer lab-bearer-token')"
assert_eq "pipeline status=success" success "$(last_status_for auth-input-bearer)"

# --- 4. basic ----------------------------------------------------------------
echo "[scenario] auth-input-basic"
reset_journal
run_pipeline "$PIPELINES_DIR/auth-input-basic.yaml"
assert_eq "source got 1 successful basic request" 1 \
  "$(count_with_header_value GET /auth/source/basic Authorization 'Basic bGFiLXVzZXI6bGFiLXBhc3M=')"
assert_eq "pipeline status=success" success "$(last_status_for auth-input-basic)"

# --- 5. oauth2 client credentials -------------------------------------------
echo "[scenario] auth-input-oauth2"
reset_journal
run_pipeline "$PIPELINES_DIR/auth-input-oauth2.yaml"
assert_eq "token endpoint called once with 200" 1 \
  "$(count_with_status POST /auth/oauth2/token 200)"
assert_eq "protected endpoint called once with bearer token" 1 \
  "$(count_with_header_value GET /auth/oauth2/protected Authorization 'Bearer lab-oauth2-access-token')"
assert_eq "pipeline status=success" success "$(last_status_for auth-input-oauth2)"

# --- 6. destination api-key (output auth) -----------------------------------
echo "[scenario] auth-output-api-key"
reset_journal
run_pipeline "$PIPELINES_DIR/auth-output-api-key.yaml"
assert_eq "destination received api-key-protected POST with 200" 1 \
  "$(count_with_header_value POST /auth/destination/orders X-Api-Key lab-destination-api-key)"
assert_eq "pipeline status=success" success "$(last_status_for auth-output-api-key)"

# --- 7. http_call basic ------------------------------------------------------
echo "[scenario] auth-http-call-basic"
reset_journal
run_pipeline "$PIPELINES_DIR/auth-http-call-basic.yaml"
assert_eq "enrichment called twice (one per order) with basic auth" 2 \
  "$(curl -fsS "$WM/__admin/requests" | jq '[.requests[] | select(.request.method=="GET" and (.request.url | startswith("/auth/enrichment/orders/")) and (.request.headers.Authorization=="Basic bGFiLWVucmljaC11c2VyOmxhYi1lbnJpY2gtcGFzcw=="))] | length')"
assert_eq "destination received 1 enriched batch" 1 \
  "$(count_with_status POST /auth/destination/public 200)"
assert_eq "pipeline status=success" success "$(last_status_for auth-http-call-basic)"

# --- 8. negative: wrong bearer -> source 401 -> pipeline error ---------------
echo "[scenario] auth-negative-bearer-wrong"
reset_journal
run_pipeline "$PIPELINES_DIR/auth-negative-bearer-wrong.yaml"
assert_eq "source returned 401 for wrong bearer" 1 \
  "$(count_with_status GET /auth/source/bearer 401)"
assert_eq "destination NOT called" 0 \
  "$(count_with_status POST /auth/destination/public 200)"
assert_eq "pipeline status=error" error "$(last_status_for auth-negative-bearer-wrong)"

# --- 9. negative: wrong oauth2 secret -> token 401 -> pipeline error --------
echo "[scenario] auth-negative-oauth2-bad-secret"
reset_journal
run_pipeline "$PIPELINES_DIR/auth-negative-oauth2-bad-secret.yaml"
assert_eq "token endpoint returned 401 for wrong secret" 1 \
  "$(count_with_status POST /auth/oauth2/token 401)"
assert_eq "protected endpoint NOT called" 0 \
  "$(count_with_status GET /auth/oauth2/protected 200)"
assert_eq "pipeline status=error" error "$(last_status_for auth-negative-oauth2-bad-secret)"

echo
echo "auth scenarios: $PASS passed, $FAIL failed"
[[ -n "${LAST_LOG:-}" ]] && rm -f "$LAST_LOG"
exit $((FAIL > 0))
