# Cannectors local test lab

This directory contains a Docker Compose lab for local integration testing. It is for development and tests only; credentials, ports, and volumes are intentionally isolated from production.

## Prerequisites

- Docker Engine with Docker Compose v2
- `make`
- `curl` for manual HTTP checks
- Optional: `psql` if you want to inspect PostgreSQL from the host

## Services and ports

| Service | Host port | Container port | Notes |
| --- | ---: | ---: | --- |
| WireMock | `18080` | `8080` | Source and destination HTTP stubs |
| PostgreSQL | `15432` | `5432` | Local database seeded at startup |

PostgreSQL local credentials:

```text
database: cannectors_test
user: cannectors_test
password: cannectors_test
connection string: postgres://cannectors_test:cannectors_test@localhost:15432/cannectors_test?sslmode=disable
```

## Commands

```bash
make test-lab-up
make test-lab-down
make test-lab-reset
```

`test-lab-up` starts WireMock and PostgreSQL and waits for Compose healthchecks. `test-lab-reset` removes the PostgreSQL volume, recreates both services, reloads WireMock mappings from disk, and resets the WireMock request journal.

Additional helpers:

```bash
make test-lab-db-reset
make test-lab-requests
make test-lab-requests-reset
```

## Readiness checks

```bash
docker compose -f test-lab/docker-compose.yml ps
curl -fsS http://localhost:18080/__admin/mappings
docker compose -f test-lab/docker-compose.yml exec -T postgres pg_isready -U cannectors_test -d cannectors_test
```

## Source API stubs

These endpoints support `httpPolling` scenarios.

| Scenario | Request | Shape |
| --- | --- | --- |
| Root array customers | `GET /source/customers` | JSON array at response root |
| Nested orders | `GET /source/orders` | JSON object with records in `orders` |
| Header-protected inventory | `GET /source/inventory` with `X-Source-Token: local-source-token` | JSON object with records in `inventory` |

Manual checks:

```bash
curl -fsS http://localhost:18080/source/customers
curl -fsS http://localhost:18080/source/orders
curl -fsS -H 'X-Source-Token: local-source-token' http://localhost:18080/source/inventory
curl -i http://localhost:18080/source/inventory
```

Example nested response shape for `dataField: orders`:

```json
{
  "meta": {
    "scenario": "orders-data-field"
  },
  "orders": [
    {
      "id": "ORD-1001",
      "customerId": "CUST-001"
    }
  ]
}
```

## Destination API stubs

These endpoints support `httpRequest` batch and single-record scenarios. WireMock records all matched and unmatched requests in its request journal.

Batch imports:

```bash
curl -fsS -X POST http://localhost:18080/destination/customers/import \
  -H 'Content-Type: application/json' \
  -d '[{"id":"CUST-001","email":"ada.lovelace@example.test"}]'

curl -fsS -X POST http://localhost:18080/destination/orders/import \
  -H 'Content-Type: application/json' \
  -d '[{"id":"ORD-1001","customerId":"CUST-001"}]'

curl -fsS -X POST http://localhost:18080/destination/inventory/import \
  -H 'Content-Type: application/json' \
  -d '[{"sku":"SKU-001","warehouse":"east"}]'
```

Single-record matching examples:

```bash
curl -fsS -X PUT 'http://localhost:18080/destination/customers/CUST-001?source=cannectors' \
  -H 'Content-Type: application/json' \
  -H 'X-Tenant-Id: tenant-local' \
  -d '{"email":"ada.lovelace@example.test"}'

curl -fsS -X PATCH 'http://localhost:18080/destination/orders/ORD-1001/status?notify=true' \
  -H 'Content-Type: application/json' \
  -H 'X-Trace-Id: local-trace-001' \
  -d '{"status":"synced"}'

curl -fsS -X PUT 'http://localhost:18080/destination/inventory/SKU-001?warehouse=east' \
  -H 'Content-Type: application/json' \
  -H 'X-Sync-Mode: incremental' \
  -d '{"quantityAvailable":12}'
```

Inspect and reset captured destination requests:

```bash
curl -fsS http://localhost:18080/__admin/requests
curl -fsS -X DELETE http://localhost:18080/__admin/requests
```

## PostgreSQL seed data

The schema is created by `postgres/init/001_schema.sql`; deterministic rows are inserted by `postgres/init/002_seed.sql`. The seed file is insert-only. For resets, `make test-lab-db-reset` runs `postgres/reset.sql` first and then replays `postgres/init/002_seed.sql`.

Tables:

- Source: `source_customers`, `source_orders`, `source_inventory`
- Destination: `dest_customers`, `dest_orders`, `inventory_snapshot`
- Reference: `customer_reference`, `product_reference`

The seed data includes nominal rows, nullable fields, pagination and incremental timestamps, missing-reference rows, duplicate destination keys, and rows that can trigger controlled destination check constraint failures.

## Scenario pipelines

Pipelines that exercise the lab live under `test-lab/pipelines/`. Each pipeline targets WireMock at `http://localhost:18080` and PostgreSQL at `localhost:15432` and is validated by the canonical schema.

All pipelines use the CRON expression `* * * * * *` (every second) so that the helper script `test-lab/scripts/run-pipeline-once.sh` can start the binary, wait for the first `execution completed` event, and SIGTERM the process.

```bash
# Run a pipeline once and dump its logs:
test-lab/scripts/run-pipeline-once.sh test-lab/pipelines/pagination-page.yaml
```

### HTTP pagination (story 22.1)

Three pipelines validate the three pagination strategies of `httpPolling` against deterministic WireMock stubs.

| Pipeline | Pagination type | Source endpoint | Expected pages |
| --- | --- | --- | --- |
| `pagination-page.yaml` | `page` | `GET /source/paginated/customers` | 3 pages of 2 customers |
| `pagination-offset.yaml` | `offset` | `GET /source/paginated/orders` | 3 pages of 2 orders |
| `pagination-cursor.yaml` | `cursor` | `GET /source/paginated/inventory` | 3 cursor iterations of 2 inventory rows |

Run all three with the helper script:

```bash
make test-lab-up
make test-lab-requests-reset
test-lab/scripts/verify-pagination.sh
```

The script asserts the number of GET requests on each source endpoint, that the destination batch endpoint received the records in deterministic order, and that the polling loop terminates without running forever.

### Database input/output (story 22.2)

Three database input pipelines exercise simple reads, parameter binding, and `queryFile` against the seeded `source_*` tables; five database output pipelines exercise plain INSERT, ON CONFLICT upsert via `queryFile`, transactional rollback, and `onError=skip` / `onError=log` strategies against `dest_customers`.

| Pipeline | Path | Notes |
| --- | --- | --- |
| `db-input-basic.yaml` | source_customers → POST /destination/db-input/customers | inline query |
| `db-input-parameters.yaml` | source_customers WHERE status=:status | named parameter |
| `db-input-query-file.yaml` | source_orders WHERE status='paid' | reads `test-lab/assets/sql/select-paid-orders.sql` |
| `db-output-insert.yaml` | /source/db-feed/customers → INSERT dest_customers | template-driven INSERT |
| `db-output-upsert-query-file.yaml` | same feed → ON CONFLICT upsert | reads `test-lab/assets/sql/upsert-dest-customer.sql` |
| `db-output-transaction-rollback.yaml` | feed with email collision → transaction=true, onError=fail | rollback semantics |
| `db-output-on-error-skip.yaml` | same feed → transaction=false, onError=skip | skip the offending row |
| `db-output-on-error-log.yaml` | feed missing required field → onError=log | log and continue |

Run all of them with:

```bash
make test-lab-up
make test-lab-db-reset
test-lab/scripts/verify-database.sh
```

The script asserts WireMock received the expected batch from the database inputs, and that each database output left `dest_customers` in the expected state. It always restarts from a clean test seed so re-runs are deterministic.

### `sql_call` enrichment (story 22.3)

Four pipelines exercise the `sql_call` filter against the seeded `customer_reference` and `product_reference` tables, covering the three merge strategies and `queryFile` loading.

| Pipeline | Merge strategy | Notes |
| --- | --- | --- |
| `sql-call-merge.yaml` | `merge` | deep-merges customer reference into orders, with cache enabled |
| `sql-call-replace.yaml` | `replace` | shallow overwrite — SQL columns win, other record fields kept |
| `sql-call-append.yaml` | `append` | stores product reference under `record.reference` |
| `sql-call-query-file.yaml` | `merge` | same lookup as `sql-call-merge` but query loaded from `test-lab/assets/sql/customer-lookup.sql` |

Run with `test-lab/scripts/verify-sql-call.sh` (or `make test-lab-verify-sql-call`). The script re-issues each pipeline and asserts the resulting destination payloads carry the expected enriched fields, including the empty-result case where no SQL row matches.

### `http_call` enrichment (story 22.4)

Four pipelines exercise the `http_call` filter against WireMock enrichment stubs.

| Pipeline | Strategy | What it covers |
| --- | --- | --- |
| `http-call-path-merge.yaml` | `merge` | path key (`{customerId}`), `dataField`, cache deduplicates on `customer.id`, `onError=skip` for the 404 case |
| `http-call-query-replace.yaml` | `replace` | query key (`?customerId=...`), shallow overwrite |
| `http-call-header-append.yaml` | `append` | header key (`X-Customer-Id`), response stored under `_response` |
| `http-call-post-template.yaml` | `replace` | POST with `bodyTemplateFile`, per-record cache key |

Run `make test-lab-verify-http-call` (or `bash test-lab/scripts/verify-http-call.sh`). Assertions cover the WireMock journal (request count after caching, headers received) and the enriched payloads forwarded to the destination.

### Transformation filters (story 22.5)

Five pipelines exercise the `mapping`, `set`, `remove`, `condition`, and `script` filters end to end.

| Pipeline | What it covers |
| --- | --- |
| `filters-mapping-transforms.yaml` | every supported transform (trim/upper/lower/replace/dateFormat/toInt/toFloat/toBool/split/join/toString) plus `onMissing` strategies (`setNull`, `skipField`, `useDefault`) and target-only deletion |
| `filters-set-remove.yaml` | `set` on flat and nested paths, `remove` on flat and nested paths |
| `filters-condition.yaml` | `condition` with `then`/`else` blocks containing nested `set`/`remove` filters |
| `filters-script-inline.yaml` | inline JavaScript `transform(record)` |
| `filters-script-file.yaml` | JavaScript transform loaded from `test-lab/assets/scripts/normalize.js` |

Run with `make test-lab-verify-filters` (or `bash test-lab/scripts/verify-filters.sh`). All assertions read the destination payload from the WireMock journal.

### State persistence (story 22.6)

Three pipelines exercise the `statePersistence` config of the `httpPolling` input, with `storagePath: ./test-lab/state`.

| Pipeline | What it covers |
| --- | --- |
| `state-timestamp.yaml` | `timestamp.enabled` only — `updated_after` query param added on subsequent runs |
| `state-id.yaml`        | `id.enabled` only — `field: event.id`, `after_id` query param added on subsequent runs |
| `state-both.yaml`      | both timestamp + id enabled together |

Run with `make test-lab-verify-state-persistence` (or `bash test-lab/scripts/verify-state-persistence.sh`). The script clears the state directory and the WireMock journal, runs each pipeline twice, and asserts that (a) the first request carries no state query params, (b) the second request carries the expected params with the values persisted to disk, and (c) the `state-<pipeline>.json` file contains the expected `lastTimestamp` / `lastId` keys.

### Webhook input (story 22.7)

Three pipelines exercise the `webhook` input. Unlike scheduled inputs the binary stays alive listening on the configured `listenAddress` (loopback ports 18181/18182/18183 in the lab).

| Pipeline | What it covers |
| --- | --- |
| `webhook-simple.yaml` | unauthenticated webhook, payload forwarded to a destination stub |
| `webhook-hmac.yaml`   | HMAC-SHA256 signature validation (`secret: lab-secret`) — accepts valid sig, rejects invalid + missing |
| `webhook-queue.yaml`  | `queueSize: 50`, `maxConcurrent: 2`, `rateLimit: 5 rps / burst 5` — observable 429s during a 10-request burst |

Run with `make test-lab-verify-webhook` (or `bash test-lab/scripts/verify-webhook.sh`). The script starts each pipeline as a long-running process, fires curl requests, then asserts response codes and the destination journal entries before SIGTERMing the process.

### HTTP authentication (story 23.1)

Nine pipelines exercise the four supported auth types end-to-end against
WireMock stubs in `wiremock/mappings/auth/`:

| Pipeline | What it covers |
| --- | --- |
| `auth-input-api-key-header.yaml` | `api-key` in `X-API-Key` header on the source |
| `auth-input-api-key-query.yaml`  | `api-key` in `api_key` query param on the source |
| `auth-input-bearer.yaml`         | bearer token on the source |
| `auth-input-basic.yaml`          | HTTP basic on the source |
| `auth-input-oauth2.yaml`         | OAuth2 client credentials — token endpoint then protected endpoint |
| `auth-output-api-key.yaml`       | api-key on the destination (output auth) |
| `auth-http-call-basic.yaml`      | basic auth on a `http_call` enrichment |
| `auth-negative-bearer-wrong.yaml` | wrong bearer → source 401, pipeline error, destination not called |
| `auth-negative-oauth2-bad-secret.yaml` | wrong OAuth2 client secret → token 401, pipeline error |

Run with `make test-lab-verify-auth` (or `bash test-lab/scripts/verify-auth.sh`).
The script asserts the WireMock journal received each authenticated request
with the expected header/query/body and that negative scenarios fail with an
explicit pipeline status.

### Reliability — retry / timeout / errors (story 23.2)

Six pipelines exercise the runtime's reliability behavior against WireMock
stubs in `wiremock/mappings/reliability/`. Several stubs use
[WireMock scenario state](https://wiremock.org/docs/stateful-behaviour/) to
return `500`/`429`/`503` on the first call and `200` on subsequent calls.

| Pipeline | What it covers |
| --- | --- |
| `retry-output-500.yaml`            | 500 → 200, retry succeeds |
| `retry-output-429-retry-after.yaml` | 429 + `Retry-After: 1`, `useRetryAfterHeader: true` |
| `retry-output-body-hint-true.yaml`  | `retryHintFromBody: body.retryable == true` retries on 503 |
| `retry-output-body-hint-false.yaml` | same expression, body returns `retryable: false` → no retry |
| `retry-output-timeout.yaml`         | destination delays 3s, `timeoutMs: 500` → fast timeout error |
| `retry-input-invalid-json.yaml`     | source returns malformed JSON → input error, destination not called |

Run with `make test-lab-verify-retry` (or `bash test-lab/scripts/verify-retry.sh`).

### Local E2E test runner (story 23.3)

The runner orchestrates the whole lab from a single command. Each scenario is
a small YAML file under `test-lab/scenarios/` describing which pipeline to
run, the expected pipeline status, and a list of declarative assertions
against the WireMock journal, the PostgreSQL database, and the pipeline log.

Run the full suite:

```bash
make test-lab-up
make test-lab-run
```

Run a subset with a substring filter:

```bash
make test-lab-run SCENARIO=auth
make test-lab-run SCENARIO=retry-output-500,retry-output-timeout
# or directly:
python3 test-lab/run.py auth-input-bearer
SCENARIO=db- python3 test-lab/run.py
```

The runner exits non-zero if any scenario fails. On failure it prints the
failing assertions and the last 15 log lines of the pipeline. Webhook
pipelines (story 22.7) are not yet integrated into the runner — keep using
`make test-lab-verify-webhook` for those.

#### Scenario YAML format

```yaml
name: my-scenario              # default = filename stem
description: optional one-liner
pipeline: test-lab/pipelines/<file>.yaml
expect_status: success         # success | error
timeout: 30                    # seconds passed to run-pipeline-once.sh
setup:
  reset_journal: true          # default true
  reset_scenarios: true        # default true (WireMock scenario state)
  reset_state: false           # default false (clears test-lab/state/state-*.json)
  reset_db: false              # default false (re-runs reset.sql + 002_seed.sql)
  commands:                    # optional list of bash commands to run before the pipeline
    - "echo prepared"
assertions:
  - http_count_eq:
      method: GET
      path: /auth/source/bearer
      headers: { Authorization: "Bearer lab-bearer-token" }
      query: { page: "1" }      # optional
      status: 200               # optional WireMock response status filter
      expected: 1
  - http_count_ge:
      method: POST
      path: /auth/destination/public
      expected: 1
  - http_count_eq_zero:
      method: POST
      path: /destination/never-called
  - sql_eq:
      query: "SELECT COUNT(*) FROM dest_customers"
      expected: "4"
  - log_contains: "request timeout"
  - log_not_contains: "panic:"
```

#### Adding a CI-safe scenario

CI runs a curated subset (see below). To make a scenario CI-safe:

- Keep total wall time under a few seconds — avoid pipelines that wait many
  seconds on `Retry-After` or polling backoff.
- Use deterministic stubs, deterministic seed data, and explicit assertions;
  avoid `http_count_ge` with a vague lower bound when an exact count is
  knowable.
- Prefer `reset_db: true` / `reset_state: true` over relying on prior state.
- Add the scenario name to the `SCENARIO` list in
  `.github/workflows/test-lab.yml` once it is stable locally.

### CI workflow (story 23.4)

`.github/workflows/test-lab.yml` runs a curated subset of scenarios on every
push and pull request to `main` / `develop`:

- builds the CLI
- starts WireMock + PostgreSQL via Docker Compose with healthchecks
- runs `python3 test-lab/run.py` filtered by a `SCENARIO=...` allowlist
- on failure, dumps the WireMock journal, the WireMock mappings, the
  container logs and the PostgreSQL row counts as workflow artifacts under
  `test-lab-logs/`

To reproduce the CI run locally:

```bash
make test-lab-reset
SCENARIO=pagination-page,db-input-basic,db-output-insert,auth-input-bearer,auth-input-api-key-header,auth-negative-bearer-wrong,retry-output-500,retry-input-invalid-json \
  python3 test-lab/run.py
```
