#!/usr/bin/env bash
# Verify transformation filter scenarios for story 22.5.
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
  if [[ "$expected" == "$actual" ]]; then echo "  ok  $label"; PASS=$((PASS+1));
  else echo "  FAIL $label"; echo "    expected: $expected"; echo "    actual:   $actual"; FAIL=$((FAIL+1)); fi
}

batch_records() {
  curl -fsS "$WIREMOCK_BASE/__admin/requests" \
    | jq -c --arg p "$1" '[.requests[] | select(.request.method=="POST" and .request.url==$p) | (.request.body | fromjson | .[]?)]'
}

echo "[scenario] filters-mapping-transforms"
reset_journal
run_pipeline "$PIPELINES_DIR/filters-mapping-transforms.yaml"
records=$(batch_records /destination/filters/mapping)
rec1='.[] | select(.id=="REC-1")'
assert_eq "mapping: trim+uppercase on name" "ALICE DOE" "$(echo "$records" | jq -r "$rec1 | .name")"
assert_eq "mapping: trim+lowercase on email" "alice@example.com" "$(echo "$records" | jq -r "$rec1 | .email")"
assert_eq "mapping: replace strips non-digits" "+33123456789" "$(echo "$records" | jq -r "$rec1 | .phone")"
assert_eq "mapping: dateFormat YYYY-MM-DD" "2026-03-04" "$(echo "$records" | jq -r "$rec1 | .createdDate")"
assert_eq "mapping: toInt of '42'" "42" "$(echo "$records" | jq -r "$rec1 | .amountInt")"
assert_eq "mapping: toFloat of '98.5'" "98.5" "$(echo "$records" | jq -r "$rec1 | .scoreFloat")"
assert_eq "mapping: toBool of 'true'" "true" "$(echo "$records" | jq -r "$rec1 | .active")"
assert_eq "mapping: split on csv" "vip,beta,early" "$(echo "$records" | jq -r "$rec1 | .tags | join(\",\")")"
assert_eq "mapping: join on tag_list" "red|green|blue" "$(echo "$records" | jq -r "$rec1 | .tagString")"
assert_eq "mapping: toString of 9001" "9001" "$(echo "$records" | jq -r "$rec1 | .externalId")"
assert_eq "mapping: useDefault for missing field" "FALLBACK" "$(echo "$records" | jq -r "$rec1 | .defaultedField")"
assert_eq "mapping: setNull for missing field" "null" "$(echo "$records" | jq -r "$rec1 | .nullField")"
assert_eq "mapping: skipField leaves target absent" "true" "$(echo "$records" | jq -r "$rec1 | (has(\"skippedField\") | not)")"
assert_eq "mapping: target-only mapping removes deprecatedField" "true" "$(echo "$records" | jq -r "$rec1 | (has(\"deprecatedField\") | not)")"

echo "[scenario] filters-set-remove"
reset_journal
run_pipeline "$PIPELINES_DIR/filters-set-remove.yaml"
records=$(batch_records /destination/filters/set-remove)
assert_eq "set: nested routing.bucket" "default" "$(echo "$records" | jq -r '.[0].routing.bucket')"
assert_eq "set: meta.processed=true" "true" "$(echo "$records" | jq -r '.[0].meta.processed')"
assert_eq "remove: card.cvv stripped" "true" "$(echo "$records" | jq -r '.[0] | (.card | has("cvv") | not)')"
assert_eq "remove: card.number stripped" "true" "$(echo "$records" | jq -r '.[0] | (.card | has("number") | not)')"
assert_eq "remove: deprecatedField stripped" "true" "$(echo "$records" | jq -r '.[0] | (has("deprecatedField") | not)')"

echo "[scenario] filters-condition"
reset_journal
run_pipeline "$PIPELINES_DIR/filters-condition.yaml"
records=$(batch_records /destination/filters/condition)
assert_eq "condition: REC-1 (paid, amount_int=120) → bucket=billable" "billable" \
  "$(echo "$records" | jq -r '.[] | select(.id=="REC-1") | .routing.bucket')"
assert_eq "condition: REC-2 (pending) → bucket=review" "review" \
  "$(echo "$records" | jq -r '.[] | select(.id=="REC-2") | .routing.bucket')"
assert_eq "condition: REC-2 had card.cvv removed in else branch" "true" \
  "$(echo "$records" | jq -r '.[] | select(.id=="REC-2") | (.card | has("cvv") | not)')"
assert_eq "condition: REC-1 keeps card.cvv (then branch did not remove)" "123" \
  "$(echo "$records" | jq -r '.[] | select(.id=="REC-1") | .card.cvv')"

echo "[scenario] filters-script-inline"
reset_journal
run_pipeline "$PIPELINES_DIR/filters-script-inline.yaml"
records=$(batch_records /destination/filters/script-inline)
assert_eq "script-inline: REC-1 statusUpper=PAID" "PAID" \
  "$(echo "$records" | jq -r '.[] | select(.id=="REC-1") | .statusUpper')"
assert_eq "script-inline: REC-1 fullLabel" "REC-1:PAID" \
  "$(echo "$records" | jq -r '.[] | select(.id=="REC-1") | .fullLabel')"
assert_eq "script-inline: REC-1 isLargeAmount" "true" \
  "$(echo "$records" | jq -r '.[] | select(.id=="REC-1") | .isLargeAmount')"
assert_eq "script-inline: REC-2 isLargeAmount false" "false" \
  "$(echo "$records" | jq -r '.[] | select(.id=="REC-2") | .isLargeAmount')"

echo "[scenario] filters-script-file"
reset_journal
run_pipeline "$PIPELINES_DIR/filters-script-file.yaml"
records=$(batch_records /destination/filters/script-file)
assert_eq "script-file: REC-1 normalizedEmail" "alice@example.com" \
  "$(echo "$records" | jq -r '.[] | select(.id=="REC-1") | .normalizedEmail')"
assert_eq "script-file: REC-1 tagCount=3" "3" \
  "$(echo "$records" | jq -r '.[] | select(.id=="REC-1") | .tagCount')"
assert_eq "script-file: REC-1 weightDoubled=6" "6" \
  "$(echo "$records" | jq -r '.[] | select(.id=="REC-1") | .weightDoubled')"
assert_eq "script-file: REC-2 tagCount=0" "0" \
  "$(echo "$records" | jq -r '.[] | select(.id=="REC-2") | .tagCount')"

echo
echo "filters scenarios: $PASS passed, $FAIL failed"
exit $((FAIL > 0))
