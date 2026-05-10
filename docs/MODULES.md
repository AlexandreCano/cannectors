# Module Reference

Pipelines are composed as `input -> filters -> output`. Filters run in the
order they appear.

## Input Modules

### `httpPolling`

Polls an HTTP endpoint and extracts records from the response.

```yaml
input:
  type: httpPolling
  schedule: "0 * * * *"
  endpoint: https://source.example.com/api/customers
  method: GET
  dataField: customers
  pagination:
    type: page
    pageParam: page
    limitParam: limit
    limit: 100
```

Examples:

- [basic polling](../examples/01-http-polling-basic-to-http-batch.yaml)
- [page pagination](../examples/02-http-polling-page-pagination.yaml)
- [offset pagination](../examples/03-http-polling-offset-pagination-state.yaml)
- [cursor pagination and OAuth2](../examples/04-http-polling-cursor-oauth2.yaml)

### `webhook`

Starts an HTTP endpoint and processes POST payloads.

```yaml
input:
  type: webhook
  path: /webhooks/orders
```

Examples:

- [HMAC webhook to HTTP output](../examples/05-webhook-hmac-to-http-single.yaml)
- [queued webhook to database](../examples/06-webhook-queue-rate-limit-to-database.yaml)

### `database`

Reads records from PostgreSQL, MySQL, or SQLite.

```yaml
input:
  type: database
  connectionStringRef: ${SOURCE_DATABASE_URL}
  query: SELECT id, name, email FROM customers
```

Examples:

- [database input to HTTP](../examples/07-database-input-basic-to-http.yaml)
- [limit/offset database input](../examples/08-database-input-limit-offset-to-database.yaml)
- [incremental database input](../examples/09-database-input-cursor-incremental.yaml)

## Filter Modules

### `mapping`

Maps fields and applies transforms.

```yaml
filters:
  - type: mapping
    mappings:
      - source: email
        target: contact.email
        transforms:
          - op: trim
          - op: lowercase
```

See [examples/10-mapping-transforms-all.yaml](../examples/10-mapping-transforms-all.yaml).

### `condition`

Keeps, drops, or routes records based on expressions.

```yaml
filters:
  - type: condition
    expression: "status == 'active'"
    onTrue: continue
    onFalse: skip
```

See [examples/11-condition-nested-routing.yaml](../examples/11-condition-nested-routing.yaml).

### `script`

Runs JavaScript transformations with Goja.

```yaml
filters:
  - type: script
    script: |
      function transform(record) {
        record.processed = true;
        return record;
      }
```

Examples:

- [inline script](../examples/12-script-inline-transform.yaml)
- [script file](../examples/13-script-file-transform.yaml)

### `http_call`

Enriches records with HTTP calls.

```yaml
filters:
  - type: http_call
    endpoint: https://profiles.example.com/api/customers/{customerId}
    method: GET
    keys:
      - field: customerId
        paramType: path
        paramName: customerId
    mergeStrategy: merge
```

Examples:

- [GET and merge with cache](../examples/14-http-call-get-merge-cache.yaml)
- [query/header keys and append](../examples/15-http-call-query-header-append.yaml)
- [POST template replace](../examples/16-http-call-post-template-replace.yaml)

### `sql_call`

Enriches records with SQL queries.

```yaml
filters:
  - type: sql_call
    connectionStringRef: ${REFERENCE_DATABASE_URL}
    query: SELECT tier FROM customers WHERE id = {{record.customerId}}
    mergeStrategy: merge
```

Examples:

- [merge cache](../examples/17-sql-call-merge-cache.yaml)
- [append query file](../examples/18-sql-call-append-query-file.yaml)

### `set` And `remove`

Adds, updates, or removes record fields.

```yaml
filters:
  - type: set
    target: metadata.source
    value: cannectors
  - type: remove
    target:
      - internalId
      - metadata.secret
```

See [examples/24-empty-filter-pass-through.yaml](../examples/24-empty-filter-pass-through.yaml)
for an empty filter chain and [examples/10-mapping-transforms-all.yaml](../examples/10-mapping-transforms-all.yaml)
for field-shaping patterns.

## Output Modules

### `httpRequest`

Sends records to an HTTP endpoint in `batch` or `single` mode.

```yaml
output:
  type: httpRequest
  endpoint: https://destination.example.com/api/tasks/{{record.id}}
  method: POST
  requestMode: single
```

Examples:

- [single output with template](../examples/19-http-output-single-template.yaml)
- [retry and API key](../examples/20-http-output-retry-auth-api-key.yaml)

### `database`

Writes records to a database.

```yaml
output:
  type: database
  connectionStringRef: ${WAREHOUSE_DATABASE_URL}
  queryFile: examples/assets/sql/upsert_product.sql
  transaction: true
```

See [examples/21-database-output-transaction-query-file.yaml](../examples/21-database-output-transaction-query-file.yaml).
