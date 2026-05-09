#!/usr/bin/env bash
# Verify reliability scenarios for story 23.2 — retry, error, timeout.
#
# Each scenario:
#   - resets the WireMock journal AND scenario state (since several stubs use
#     scenario state to alternate 500→200, 429→200, etc.)
#   - runs the pipeline once
#   - asserts the journal and the pipeline status
#
# Some pipelines run more than once because the first execution is >1s (e.g.
# Retry-After=1s) and the * * * * * * cron triggers again before SIGTERM.
# Assertions are written to count only the deterministic side effects of the
# first run (e.g. exactly one 500 response, one 429 response).
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PIPELINES_DIR="$REPO_ROOT/test-lab/pipelines"
WM="${WIREMOCK_BASE:-http://localhost:18080}"

PASS=0
FAIL=0
LAST_LOG=""

reset_lab() {
  curl -fsS -X POST "$WM/__admin/scenarios/reset" >/dev/null
  curl -fsS -X DELETE "$WM/__admin/requests" >/dev/null
}

run_pipeline() {
  LAST_LOG="$(mktemp)"
  bash "$SCRIPT_DIR/run-pipeline-once.sh" "$1" >"$LAST_LOG" 2>&1 || true
}

assert_eq() {
  local label="$1" expected="$2" actual="$3"
  if [[ "$expected" == "$actual" ]]; then
    echo "  ok  $label"; PASS=$((PASS+1))
  else
    echo "  FAIL $label"; echo "    expected: $expected"; echo "    actual:   $actual"; FAIL=$((FAIL+1))
  fi
}

assert_ge() {
  local label="$1" min="$2" actual="$3"
  if (( actual >= min )); then
    echo "  ok  $label (got $actual >= $min)"; PASS=$((PASS+1))
  else
    echo "  FAIL $label"; echo "    expected: >= $min"; echo "    actual:   $actual"; FAIL=$((FAIL+1))
  fi
}

count_with_status() {
  local method="$1" urlpath="$2" status="$3"
  curl -fsS "$WM/__admin/requests" \
    | jq --arg m "$method" --arg u "$urlpath" --argjson s "$status" \
        '[.requests[] | select(.request.method==$m and (.request.url | split("?")[0])==$u and .responseDefinition.status==$s)] | length'
}

count_method_path() {
  local method="$1" urlpath="$2"
  curl -fsS "$WM/__admin/requests" \
    | jq --arg m "$method" --arg u "$urlpath" \
        '[.requests[] | select(.request.method==$m and (.request.url | split("?")[0])==$u)] | length'
}

last_status_for() {
  local pipeline_id="$1"
  grep '"msg":"execution completed"' "$LAST_LOG" \
    | jq -r --arg id "$pipeline_id" 'select(."pipeline_id"==$id) | .status' \
    | tail -1
}

log_contains() { grep -q "$1" "$LAST_LOG"; }

# --- 1. 500 then 200 ---------------------------------------------------------
echo "[scenario] retry-output-500"
reset_lab
run_pipeline "$PIPELINES_DIR/retry-output-500.yaml"
assert_eq "destination saw exactly one 500 response" 1 \
  "$(count_with_status POST /reliability/destination/500-then-200 500)"
assert_ge "destination then got at least one 200 response" 1 \
  "$(count_with_status POST /reliability/destination/500-then-200 200)"
assert_eq "pipeline status=success" success "$(last_status_for retry-output-500)"

# --- 2. 429 + Retry-After ---------------------------------------------------
echo "[scenario] retry-output-429-retry-after"
reset_lab
run_pipeline "$PIPELINES_DIR/retry-output-429-retry-after.yaml"
assert_eq "destination saw exactly one 429 response" 1 \
  "$(count_with_status POST /reliability/destination/429-retry-after 429)"
assert_ge "destination then got at least one 200 response" 1 \
  "$(count_with_status POST /reliability/destination/429-retry-after 200)"
assert_eq "pipeline status=success" success "$(last_status_for retry-output-429-retry-after)"

# --- 3. body retryable=true (retries) ---------------------------------------
echo "[scenario] retry-output-body-hint-true"
reset_lab
run_pipeline "$PIPELINES_DIR/retry-output-body-hint-true.yaml"
assert_eq "destination saw exactly one 503 response" 1 \
  "$(count_with_status POST /reliability/destination/body-hint-true 503)"
assert_ge "destination then got at least one 200 response" 1 \
  "$(count_with_status POST /reliability/destination/body-hint-true 200)"
assert_eq "pipeline status=success" success "$(last_status_for retry-output-body-hint-true)"

# --- 4. body retryable=false (does NOT retry) -------------------------------
echo "[scenario] retry-output-body-hint-false"
reset_lab
run_pipeline "$PIPELINES_DIR/retry-output-body-hint-false.yaml"
# The endpoint always returns 500. Without the body hint we'd see >=2 attempts.
# With retryHintFromBody:'body.retryable == true' returning false, we expect
# exactly one POST per scheduled run (no retry), and the pipeline must error.
assert_eq "destination got exactly one 500 (no retry attempted)" 1 \
  "$(count_with_status POST /reliability/destination/body-hint-false 500)"
assert_eq "pipeline status=error" error "$(last_status_for retry-output-body-hint-false)"

# --- 5. timeout --------------------------------------------------------------
echo "[scenario] retry-output-timeout"
reset_lab
run_pipeline "$PIPELINES_DIR/retry-output-timeout.yaml"
assert_ge "destination /slow received at least 1 attempt" 1 \
  "$(count_method_path POST /reliability/destination/slow)"
assert_eq "pipeline status=error" error "$(last_status_for retry-output-timeout)"
if log_contains '"error":"executing output module: network error: request timeout"'; then
  echo "  ok  log mentions 'network error: request timeout'"; PASS=$((PASS+1))
else
  echo "  FAIL log does not mention timeout"; FAIL=$((FAIL+1))
fi

# --- 6. invalid JSON in source response -------------------------------------
echo "[scenario] retry-input-invalid-json"
reset_lab
run_pipeline "$PIPELINES_DIR/retry-input-invalid-json.yaml"
assert_ge "source /invalid-json was called at least once" 1 \
  "$(count_method_path GET /reliability/source/invalid-json)"
assert_eq "destination /never-called was NOT called" 0 \
  "$(count_method_path POST /reliability/destination/never-called)"
assert_eq "pipeline status=error" error "$(last_status_for retry-input-invalid-json)"
if log_contains 'failed to parse JSON response'; then
  echo "  ok  log mentions JSON parse failure"; PASS=$((PASS+1))
else
  echo "  FAIL log does not mention JSON parse failure"; FAIL=$((FAIL+1))
fi

echo
echo "reliability scenarios: $PASS passed, $FAIL failed"
[[ -n "$LAST_LOG" ]] && rm -f "$LAST_LOG"
exit $((FAIL > 0))
