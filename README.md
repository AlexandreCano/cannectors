# Cannectors

Cross-platform CLI for executing declarative data pipelines. Cannectors reads YAML/JSON pipeline configurations and orchestrates data transfers between systems using a modular Input → Filter → Output architecture.

## Features

- **Declarative Pipelines**: Define data flows in YAML or JSON configuration files
- **HTTP Polling Input**: Fetch data from REST APIs with pagination support (page, offset, cursor)
- **Webhook Input**: Receive data via HTTP POST endpoints
- **Database Input**: Query data from PostgreSQL, MySQL, or SQLite
- **Transformation Filters**: Map fields, apply conditions, set/remove fields, run JavaScript scripts, enrich with external data
- **HTTP Output**: Send data to REST APIs with templating support
- **Database Output**: Write records to databases with transaction support
- **Authentication**: API Key, Bearer Token, Basic Auth, OAuth2 (client credentials)
- **CRON Scheduling**: Periodic execution with graceful shutdown
- **State Persistence**: Resume polling after restarts using timestamps or record IDs
- **Retry Logic**: Configurable retry with exponential backoff
- **Dry-Run Mode**: Preview output without sending data

## Quick Start

```bash
# Install from source
go install ./cmd/cannectors

# Validate a configuration
cannectors validate ./configs/examples/01-simple.yaml

# Execute a pipeline
cannectors run ./configs/examples/01-simple.yaml

# Execute in dry-run mode (preview only)
cannectors run --dry-run ./configs/examples/10-complete.yaml
```

## Configuration Format

Cannectors uses a declarative YAML/JSON configuration format:

```yaml
connector:
  name: my-pipeline
  version: "1.0.0"
  description: Sync orders from API to warehouse

  input:
    type: httpPolling
    endpoint: https://api.source.com/orders
    schedule: "*/5 * * * *"  # Every 5 minutes
    authentication:
      type: bearer
      credentials:
        token: ${API_TOKEN}

  filters:
    - type: mapping
      mappings:
        - source: order_id
          target: id
        - source: customer.email
          target: email

    - type: condition
      expression: "status == 'active'"
      onFalse: skip

  output:
    type: httpRequest
    endpoint: https://api.warehouse.com/orders
    method: POST
```

## Input Modules

### HTTP Polling

Fetches data from REST APIs with pagination support.

```yaml
input:
  type: httpPolling
  endpoint: https://api.example.com/data
  schedule: "0 * * * *"  # Hourly
  method: GET
  headers:
    Accept: application/json
  dataField: items  # Extract records from response.items
  pagination:
    type: cursor  # page, offset, or cursor
    cursorParam: after
    cursorPath: meta.next_cursor
  authentication:
    type: api-key
    credentials:
      key: ${API_KEY}
      location: header
      headerName: X-API-Key
```

### Webhook

Receives data via HTTP POST (event-driven, no schedule).

```yaml
input:
  type: webhook
  path: /webhooks/orders
```

### Database

Queries data from PostgreSQL, MySQL, or SQLite.

```yaml
input:
  type: database
  connectionStringRef: ${DATABASE_URL}
  query: |
    SELECT id, name, email FROM users
    WHERE updated_at > {{lastRunTimestamp}}
  schedule: "*/5 * * * *"
```

## Filter Modules

### Mapping

Transforms record fields with optional transformations.

```yaml
filters:
  - type: mapping
    mappings:
      - source: full_name
        target: name
      - source: created_at
        target: createdAt
        transforms:
          - op: dateFormat
            format: "2006-01-02"
      - source: amount
        target: total
        transforms:
          - op: toFloat
    onError: skip  # fail, skip, or log
```

### Condition

Filters records based on expressions.

```yaml
filters:
  - type: condition
    expression: "status == 'active' && amount > 0"
    onTrue: continue
    onFalse: skip
```

### Script

Transforms records using JavaScript (Goja engine).

```yaml
filters:
  - type: script
    script: |
      function transform(record) {
        record.total = record.price * record.quantity;
        record.processed_at = new Date().toISOString();
        return record;
      }
    onError: skip
```

### HTTP Call (Enrichment)

Enriches records with data from external APIs.

```yaml
filters:
  - type: http_call
    endpoint: https://api.crm.com/customers/{id}
    key:
      field: customerId
      paramType: path
      paramName: id
    mergeStrategy: merge
    cache:
      maxSize: 1000
      defaultTTL: 300
```

### SQL Call

Enriches records with data from databases.

```yaml
filters:
  - type: sql_call
    connectionStringRef: ${DATABASE_URL}
    query: |
      SELECT name, tier FROM customers
      WHERE customer_id = {{record.customer_id}}
    mergeStrategy: merge
```

### Set

Sets a field to a literal value on each record. Supports nested paths with dot notation.

```yaml
filters:
  - type: set
    target: metadata.source
    value: "api-v2"
```

### Remove

Removes one or more fields from each record. Supports nested paths with dot notation and array indices.

```yaml
filters:
  # Remove a single field
  - type: remove
    target: internalId

  # Remove multiple fields
  - type: remove
    targets:
      - password
      - metadata.internal
      - items[0].secret
```

## Output Modules

### HTTP Request

Sends data to REST APIs.

```yaml
output:
  type: httpRequest
  endpoint: https://api.destination.com/orders
  method: POST
  headers:
    Content-Type: application/json
  request:
    bodyFrom: records  # records (batch) or record (single)
    pathParams:
      orderId: id
  authentication:
    type: bearer
    credentials:
      token: ${API_TOKEN}
```

### Database

Writes records to databases.

```yaml
output:
  type: database
  connectionStringRef: ${DATABASE_URL}
  query: |
    INSERT INTO orders (id, customer_id, total)
    VALUES ({{record.id}}, {{record.customer_id}}, {{record.total}})
  transaction: true
  onError: skip
```

## Authentication

All input and output modules support authentication:

| Type | Configuration |
|------|---------------|
| **API Key** | `type: api-key`, `credentials: {key, location, headerName}` |
| **Bearer** | `type: bearer`, `credentials: {token}` |
| **Basic** | `type: basic`, `credentials: {username, password}` |
| **OAuth2** | `type: oauth2`, `credentials: {tokenUrl, clientId, clientSecret, scope}` |

Environment variables can be referenced using `${VAR_NAME}` syntax.

## Scheduling

Add a CRON schedule to the input module for periodic execution:

```yaml
input:
  type: httpPolling
  schedule: "*/5 * * * *"  # Every 5 minutes
```

Common CRON expressions:

| Expression | Description |
|------------|-------------|
| `*/5 * * * *` | Every 5 minutes |
| `0 * * * *` | Every hour |
| `0 0 * * *` | Daily at midnight |
| `0 9 * * 1-5` | Weekdays at 9 AM |

## State Persistence

Resume polling after restarts by tracking the last processed timestamp or record ID:

```yaml
input:
  type: httpPolling
  endpoint: https://api.example.com/orders
  schedule: "*/5 * * * *"
  statePersistence:
    timestamp:
      enabled: true
      queryParam: updated_after
    id:
      enabled: true
      field: id
      queryParam: after_id
```

## Error Handling & Retry

Configure retry behavior for transient errors:

```yaml
output:
  type: httpRequest
  retry:
    maxAttempts: 3
    delayMs: 1000
    backoffMultiplier: 2
    maxDelayMs: 30000
    retryableStatusCodes: [429, 500, 502, 503, 504]
  onError: skip  # fail, skip, or log
```

Error categories:

| Category | Retryable | HTTP Status |
|----------|-----------|-------------|
| network | Yes | Timeout, connection refused |
| rate_limit | Yes | 429 |
| server | Yes | 500, 502, 503, 504 |
| authentication | No | 401, 403 |
| validation | No | 400, 422 |

## Defaults

Set default error handling and retry for all modules:

```yaml
connector:
  name: my-pipeline
  defaults:
    onError: fail
    timeoutMs: 30000
    retry:
      maxAttempts: 3
      delayMs: 1000
```

## CLI Commands

```bash
# Display help
cannectors --help

# Show version
cannectors version

# Validate configuration
cannectors validate config.yaml
cannectors validate --verbose config.yaml

# Execute pipeline
cannectors run config.yaml
cannectors run --dry-run config.yaml
cannectors run --verbose config.yaml
cannectors run --log-file execution.log config.yaml
```

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Validation errors |
| 2 | Parse errors |
| 3 | Runtime errors |

## Examples

The [`configs/examples/`](configs/examples) directory contains comprehensive examples:

- **01-simple.yaml** - Minimal pipeline
- **02-05** - Authentication types
- **05-07** - Pagination (page, offset, cursor)
- **08-09** - Filters (mapping, condition)
- **10-complete.yaml** - Full-featured pipeline
- **13-scheduled.yaml** - CRON scheduling
- **14-webhook.yaml** - Webhook input
- **15** - Retry configuration
- **16** - Script filter
- **17** - HTTP enrichment
- **18-20** - State persistence
- **21-24** - Output templating
- **25-26** - Record metadata
- **27-32** - Database modules
- **35** - Set filter
- **36** - Remove filter

## Development

```bash
# Build
go build -o cannectors ./cmd/cannectors

# Test
go test ./...

# Lint (requires golangci-lint v2.7.1+)
golangci-lint run ./...
```

## Cross-Platform Builds

```bash
# Linux
GOOS=linux GOARCH=amd64 go build -o dist/cannectors-linux-amd64 ./cmd/cannectors

# macOS Apple Silicon
GOOS=darwin GOARCH=arm64 go build -o dist/cannectors-darwin-arm64 ./cmd/cannectors

# Windows
GOOS=windows GOARCH=amd64 go build -o dist/cannectors-windows-amd64.exe ./cmd/cannectors
```

## Requirements

- Go 1.23.5+
- golangci-lint v2.7.1+ (optional, for linting)

## Extending Cannectors

Cannectors uses a module registry pattern for extensibility. See [docs/MODULE_EXTENSIBILITY.md](docs/MODULE_EXTENSIBILITY.md) for details on adding custom modules.

## License

[Apache License 2.0](LICENSE)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## Commercial Support

See [COMMERCIAL.md](COMMERCIAL.md) for details on community vs commercial usage.

For production support and commercial licensing:

📧 **alexanndre.cano@gmail.com**
