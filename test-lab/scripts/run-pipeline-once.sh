#!/usr/bin/env bash
# Run a single pipeline against the local test lab and exit after the first
# execution completes.
#
# Usage: run-pipeline-once.sh <pipeline.yaml> [timeout_seconds=20]
#
# Cannectors httpPolling requires a CRON `schedule`, so the binary stays alive
# after the first poll. We start it, watch the structured log for the first
# `execution completed` event, then SIGTERM. WireMock journal + downstream
# side effects let us verify the run.
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "usage: $0 <pipeline.yaml> [timeout_seconds]" >&2
  exit 64
fi

PIPELINE="$1"
TIMEOUT="${2:-20}"

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
BIN="$REPO_ROOT/cannectors"
if [[ ! -x "$BIN" ]]; then
  BIN="$REPO_ROOT/bin/cannectors"
fi
if [[ ! -x "$BIN" ]]; then
  (cd "$REPO_ROOT" && go build -o cannectors ./cmd/cannectors)
  BIN="$REPO_ROOT/cannectors"
fi

LOG_FILE="$(mktemp)"
"$BIN" run "$PIPELINE" >"$LOG_FILE" 2>&1 &
PID=$!

# Wait for the first execution to finish (success OR error). The pipeline runs
# again on its CRON schedule, so we kill the process as soon as the first run
# is observed in the log.
DEADLINE=$((SECONDS + TIMEOUT))
while (( SECONDS < DEADLINE )); do
  if grep -q '"execution completed"' "$LOG_FILE" 2>/dev/null; then
    break
  fi
  if ! kill -0 "$PID" 2>/dev/null; then
    break
  fi
  sleep 0.2
done

if kill -0 "$PID" 2>/dev/null; then
  kill -TERM "$PID" 2>/dev/null || true
  wait "$PID" 2>/dev/null || true
fi

cat "$LOG_FILE"
rm -f "$LOG_FILE"

