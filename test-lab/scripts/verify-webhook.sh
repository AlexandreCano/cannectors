#!/usr/bin/env bash
# Verify Story 22.7 — webhook input scenarios.
#
# Each scenario starts cannectors with a webhook pipeline (long-running),
# sends curl requests against the listening webhook endpoint, then asserts
# what reached the destination WireMock stub.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$REPO_ROOT"

WM=http://localhost:18080
BIN="$REPO_ROOT/cannectors"
[[ -x "$BIN" ]] || (cd "$REPO_ROOT" && go build -o cannectors ./cmd/cannectors)

pass=0
fail=0
check() {
  local label="$1"; local cond="$2"
  if eval "$cond"; then
    echo "  ✓ $label"
    pass=$((pass+1))
  else
    echo "  ✗ $label"
    echo "    expression: $cond"
    fail=$((fail+1))
  fi
}

start_pipeline() {
  local pipeline="$1"; local port="$2"
  LOG_FILE="$(mktemp)"
  "$BIN" run "$pipeline" >"$LOG_FILE" 2>&1 &
  PID=$!
  for _ in $(seq 1 50); do
    if (echo > /dev/tcp/127.0.0.1/"$port") 2>/dev/null; then
      return 0
    fi
    sleep 0.1
  done
  echo "    ✗ pipeline failed to listen on $port"
  cat "$LOG_FILE" >&2
  return 1
}

stop_pipeline() {
  if [[ -n "${PID:-}" ]] && kill -0 "$PID" 2>/dev/null; then
    kill -TERM "$PID" 2>/dev/null || true
    wait "$PID" 2>/dev/null || true
  fi
  if [[ -n "${LOG_FILE:-}" ]]; then
    rm -f "$LOG_FILE"
  fi
}

journal_count() {
  local path="$1"
  curl -s "$WM/__admin/requests" \
    | python3 -c "import json,sys; d=json.load(sys.stdin); print(sum(1 for r in d['requests'] if r['request']['url']=='$path'))"
}

journal_bodies() {
  local path="$1"
  curl -s "$WM/__admin/requests" \
    | python3 -c "import json,sys; d=json.load(sys.stdin); [print(r['request']['body']) for r in d['requests'] if r['request']['url']=='$path']"
}

reset_journal() { curl -s -X DELETE "$WM/__admin/requests" > /dev/null; }

# ---------- Scenario 1: simple webhook -----------------------------------------
echo "=== webhook-simple ==="
reset_journal
start_pipeline test-lab/pipelines/webhook-simple.yaml 18181
status=$(curl -s -o /tmp/_resp -w "%{http_code}" -X POST http://127.0.0.1:18181/webhooks/simple \
  -H 'Content-Type: application/json' \
  -d '{"orders":[{"id":"O-1","amount":10},{"id":"O-2","amount":20}]}')
sleep 0.5
check "POST returns 2xx" "[ '$status' -ge 200 ] && [ '$status' -lt 300 ]"
count=$(journal_count "/destination/webhook/simple")
check "destination received the forwarded batch" "[ '$count' -eq 1 ]"
bodies=$(journal_bodies "/destination/webhook/simple")
check "destination batch contains record O-1" "echo '$bodies' | grep -q 'O-1'"
check "destination batch contains record O-2" "echo '$bodies' | grep -q 'O-2'"
stop_pipeline

# ---------- Scenario 2: HMAC -- accept valid, reject invalid -------------------
echo "=== webhook-hmac ==="
reset_journal
start_pipeline test-lab/pipelines/webhook-hmac.yaml 18182
PAYLOAD='{"orders":[{"id":"H-1"}]}'
SECRET=lab-secret
SIG=$(printf '%s' "$PAYLOAD" | openssl dgst -sha256 -hmac "$SECRET" -binary | xxd -p -c 256)

# valid
status=$(curl -s -o /dev/null -w "%{http_code}" -X POST http://127.0.0.1:18182/webhooks/hmac \
  -H 'Content-Type: application/json' \
  -H "X-Webhook-Signature: $SIG" \
  -d "$PAYLOAD")
sleep 0.5
check "valid HMAC returns 2xx" "[ '$status' -ge 200 ] && [ '$status' -lt 300 ]"
count=$(journal_count "/destination/webhook/hmac")
check "destination received 1 record (valid sig)" "[ '$count' -eq 1 ]"

# invalid signature
status_bad=$(curl -s -o /dev/null -w "%{http_code}" -X POST http://127.0.0.1:18182/webhooks/hmac \
  -H 'Content-Type: application/json' \
  -H 'X-Webhook-Signature: deadbeef' \
  -d "$PAYLOAD")
check "invalid HMAC rejected (4xx)" "[ '$status_bad' -ge 400 ] && [ '$status_bad' -lt 500 ]"

# missing signature header
status_miss=$(curl -s -o /dev/null -w "%{http_code}" -X POST http://127.0.0.1:18182/webhooks/hmac \
  -H 'Content-Type: application/json' \
  -d "$PAYLOAD")
check "missing signature rejected (4xx)" "[ '$status_miss' -ge 400 ] && [ '$status_miss' -lt 500 ]"

count_after=$(journal_count "/destination/webhook/hmac")
check "destination still 1 record (rejects didn't propagate)" "[ '$count_after' -eq 1 ]"
stop_pipeline

# ---------- Scenario 3: queue + rate limit -------------------------------------
echo "=== webhook-queue ==="
reset_journal
start_pipeline test-lab/pipelines/webhook-queue.yaml 18183
# Burst of 10 requests (5 rps with burst 5 → some accepted immediately)
results_file="$(mktemp)"
curl_pids=()
for i in $(seq 1 10); do
  curl -s -o /dev/null -w "%{http_code}\n" -X POST http://127.0.0.1:18183/webhooks/queue \
    -H 'Content-Type: application/json' \
    -d "{\"orders\":[{\"id\":\"Q-$i\"}]}" >> "$results_file" &
  curl_pids+=("$!")
done
for p in "${curl_pids[@]}"; do wait "$p" || true; done
sleep 4
accepted=$(grep -cE '^2..' "$results_file" || true)
limited=$(grep -cE '^4(29|03)' "$results_file" || true)
total=$(wc -l < "$results_file")
echo "    burst summary: total=$total accepted=$accepted rate-limited=$limited"
check "burst: at least 1 request was accepted" "[ '$accepted' -ge 1 ]"
check "burst: at least 1 request was rate-limited (429)" "[ '$limited' -ge 1 ]"
delivered=$(journal_count "/destination/webhook/queue")
check "destination received exactly the accepted requests" "[ '$delivered' -eq '$accepted' ]"
rm -f "$results_file"
stop_pipeline

echo
echo "Results: $pass passed, $fail failed"
[ "$fail" -eq 0 ]
