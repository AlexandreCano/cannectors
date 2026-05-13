# Configuration Format

Cannectors pipelines are YAML files with top-level `name`, `input`, `filters`,
and `output` fields. JSON is still supported by the parser, but maintained
examples use YAML.

```yaml
name: sync-orders
version: 1.0.0
description: Poll orders from an API and send them as a batch.

input:
  type: httpPolling
  schedule: "*/15 * * * *"
  endpoint: https://source.example.com/api/orders
  dataField: orders

filters: []

output:
  type: httpRequest
  endpoint: https://destination.example.com/api/orders/import
  method: POST
  requestMode: batch
```

## Required Fields

| Field | Purpose |
| --- | --- |
| `name` | Pipeline identifier shown in logs and execution results. |
| `input` | Source module configuration. |
| `filters` | Ordered list of transformations. Use `[]` when there are no filters. |
| `output` | Destination module configuration. |

Optional top-level fields include `version`, `description`, `tags`, `defaults`,
and `dryRunOptions`.

## Defaults

Use `defaults` for behavior shared by modules. Module-level values override
top-level defaults.

```yaml
defaults:
  timeoutMs: 15000
  onError: log
  retry:
    maxAttempts: 2
    delayMs: 1000
    backoffMultiplier: 2
    maxDelayMs: 10000
    retryableStatusCodes: [429, 500, 503]
    useRetryAfterHeader: true
```

See [examples/22-defaults-inheritance.yaml](../examples/22-defaults-inheritance.yaml).

## Authentication

HTTP and SOAP input, output, and enrichment modules support these authentication
types:

| Type | Required credentials |
| --- | --- |
| `api-key` | `key`, `location`, plus `headerName` or `paramName` depending on location. |
| `bearer` | `token` |
| `basic` | `username`, `password` |
| `oauth2` | `tokenUrl`, `clientId`, `clientSecret`, optional `scope` |

Environment variables can be referenced with `${VAR_NAME}`.

```yaml
authentication:
  type: bearer
  credentials:
    token: ${SOURCE_BEARER_TOKEN}
```

SOAP modules also support WS-Security UsernameToken through `wsSecurity`.

See [examples/23-auth-basic-bearer-query-key.yaml](../examples/23-auth-basic-bearer-query-key.yaml).

## Scheduling

HTTP polling, SOAP polling, and database inputs can run on a CRON schedule.

```yaml
input:
  type: httpPolling
  schedule: "*/15 * * * *"
```

When a schedule is present, `cannectors run` keeps the process alive and runs
the pipeline on schedule. Without a schedule, the pipeline runs once.

## State Persistence

HTTP and SOAP polling can persist cursors, timestamps, or IDs so the next run
resumes from the last processed record.

```yaml
input:
  type: httpPolling
  endpoint: https://source.example.com/api/events
  statePersistence:
    id:
      enabled: true
      field: id
      queryParam: after_id
```

See:

- [examples/03-http-polling-offset-pagination-state.yaml](../examples/03-http-polling-offset-pagination-state.yaml)
- [examples/09-database-input-cursor-incremental.yaml](../examples/09-database-input-cursor-incremental.yaml)

## Retry And Error Handling

`onError` accepts `fail`, `skip`, or `log`.

```yaml
output:
  type: httpRequest
  endpoint: https://destination.example.com/api/shipments
  retry:
    maxAttempts: 3
    delayMs: 500
    backoffMultiplier: 2
    maxDelayMs: 5000
    retryableStatusCodes: [429, 500, 502, 503, 504]
  onError: skip
```

See [examples/20-http-output-retry-auth-api-key.yaml](../examples/20-http-output-retry-auth-api-key.yaml).

## External Assets

Some examples reference files under `examples/assets/`:

- SQL query files in `examples/assets/sql/`
- HTTP body templates in `examples/assets/templates/`
- JavaScript transform files in `examples/assets/scripts/`
