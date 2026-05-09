#!/usr/bin/env bash
# Verify Story 22.6 — http_polling state persistence.
#
# For each scenario:
#   1. Reset WireMock journal + state directory.
#   2. Run pipeline once → assert first source request has NO state query params.
#   3. Run pipeline again → assert second request includes the expected query
#      params with values from the first run, AND inspect the state file.
#
# WireMock @ http://localhost:18080, state dir test-lab/state.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$REPO_ROOT"

WM=http://localhost:18080
STATE_DIR=test-lab/state
RUN=test-lab/scripts/run-pipeline-once.sh

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

reset_lab() {
  curl -s -X DELETE "$WM/__admin/requests" > /dev/null
  rm -f "$STATE_DIR"/state-*.json
}

source_url() {
  local n="$1"
  curl -s "$WM/__admin/requests" \
    | python3 -c "import json,sys; d=json.load(sys.stdin); reqs=[r['request']['url'] for r in d['requests'] if r['request']['url'].startswith('/source/state/events')]; print(reqs[$n] if len(reqs)>$n else '')"
}

run_scenario() {
  local pipeline="$1"; local label="$2"
  echo "=== $label ($pipeline) ==="
  reset_lab
  bash "$RUN" "$pipeline" 30 > /dev/null 2>&1
  local first
  first="$(source_url 0)"
  check "first run: source request received" "[ -n \"$first\" ]"
  check "first run: no state query params" "! echo '$first' | grep -qE 'updated_after|after_id'"

  bash "$RUN" "$pipeline" 30 > /dev/null 2>&1
  # WireMock returns requests in reverse chronological order, so source_url 0
  # is the most recent request — i.e. the second run we just executed.
  local second
  second="$(source_url 0)"
  check "second run: source request received" "[ -n \"$second\" ]"
  echo "    second URL: $second"
}

# --- Scenario 1: timestamp only -------------------------------------------------
run_scenario test-lab/pipelines/state-timestamp.yaml "Timestamp persistence"
second="$(curl -s $WM/__admin/requests | python3 -c "import json,sys; d=json.load(sys.stdin); reqs=[r['request']['url'] for r in d['requests'] if r['request']['url'].startswith('/source/state/events')]; print(reqs[0] if reqs else '')")"
check "second run: includes updated_after" "echo '$second' | grep -q 'updated_after='"
check "second run: no after_id (id disabled)" "! echo '$second' | grep -q 'after_id='"
check "state file: lastTimestamp is set" "jq -e '.lastTimestamp' $STATE_DIR/state-timestamp.json > /dev/null"
check "state file: lastId is absent"     "! jq -e '.lastId' $STATE_DIR/state-timestamp.json > /dev/null 2>&1"

# --- Scenario 2: id only --------------------------------------------------------
run_scenario test-lab/pipelines/state-id.yaml "ID persistence"
second="$(curl -s $WM/__admin/requests | python3 -c "import json,sys; d=json.load(sys.stdin); reqs=[r['request']['url'] for r in d['requests'] if r['request']['url'].startswith('/source/state/events')]; print(reqs[0] if reqs else '')")"
check "second run: includes after_id"            "echo '$second' | grep -q 'after_id='"
check "second run: after_id picks last record"   "echo '$second' | grep -q 'after_id=EVT-102'"
check "second run: no updated_after"             "! echo '$second' | grep -q 'updated_after='"
check "state file: lastId equals EVT-102"        "[ \"$(jq -r '.lastId' $STATE_DIR/state-id.json)\" = 'EVT-102' ]"
check "state file: lastTimestamp absent"         "! jq -e '.lastTimestamp' $STATE_DIR/state-id.json > /dev/null 2>&1"

# --- Scenario 3: timestamp + id -------------------------------------------------
run_scenario test-lab/pipelines/state-both.yaml "Combined timestamp + ID"
second="$(curl -s $WM/__admin/requests | python3 -c "import json,sys; d=json.load(sys.stdin); reqs=[r['request']['url'] for r in d['requests'] if r['request']['url'].startswith('/source/state/events')]; print(reqs[0] if reqs else '')")"
check "second run: includes updated_after"         "echo '$second' | grep -q 'updated_after='"
check "second run: includes after_id=EVT-102"      "echo '$second' | grep -q 'after_id=EVT-102'"
check "state file: lastTimestamp set"              "jq -e '.lastTimestamp' $STATE_DIR/state-both.json > /dev/null"
check "state file: lastId equals EVT-102"          "[ \"$(jq -r '.lastId' $STATE_DIR/state-both.json)\" = 'EVT-102' ]"

echo
echo "Results: $pass passed, $fail failed"
[ "$fail" -eq 0 ]
