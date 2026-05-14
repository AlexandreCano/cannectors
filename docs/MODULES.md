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

### `soapPolling`

Polls a SOAP operation and extracts records from the XML response.

```yaml
input:
  type: soapPolling
  endpoint: https://soap.example.com/orders
  operation: ListOrders
  body: |
    <m:ListOrders xmlns:m="urn:orders"/>
  dataField: Envelope.Body.ListOrdersResponse.Orders.Order
```

Examples:

- [SOAP 1.1 polling](../examples/40-soap-polling-basic-v11.yaml)
- [SOAP 1.2 polling](../examples/40b-soap-polling-basic-v12.yaml)
- [cursor pagination](../examples/41-soap-polling-cursor.yaml)
- [MTOM response attachments](../examples/44b-soap-input-mtom-reception.yaml)

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

Routes records by evaluating an `expr` boolean expression. Matching records
go through the `then` branch; the rest go through `else`. An absent branch
keeps the record unchanged. To remove records, place an explicit `drop`
filter inside the relevant branch.

```yaml
filters:
  - type: condition
    expression: "status == 'active'"
    then:
      - type: set
        target: routing.bucket
        value: active
    else:
      - type: drop
```

See [examples/11-condition-nested-routing.yaml](../examples/11-condition-nested-routing.yaml).

### `loop`

Iterates over an array field on each record and runs a nested filter chain
for every item. The current item is exposed under the `itemName` alias; the
root record stays available as `record`. Loop metadata is exposed read-only
at `_metadata.loop.<itemName>.index`.

```yaml
filters:
  - type: loop
    field: cells
    itemName: cell
    filters:
      - type: condition
        expression: "cell.columnId == 1"
        then:
          - type: mapping
            mappings:
              - source: cell.displayValue
                target: record.eventId
```

Rules to keep in mind:

- `field`, `itemName`, and `filters` are required. `itemName` cannot be
  `record`, `_metadata`, or `loop`, and cannot duplicate an active parent
  loop alias when nesting.
- Nested filters can read `_metadata.loop.<alias>.index` but **must not
  write** under `_metadata.loop`; runtime-owned state is read-only.
- Nested filters returning zero records remove the item from the array.
  Returning more than one record per item is rejected in v1 (item expansion
  is out of scope).
- Items that are not objects pass through unchanged; sub-path writes such
  as `it.foo` on a scalar item are rejected to prevent silent map
  auto-creation.

See [examples/25-loop-cells-extraction.yaml](../examples/25-loop-cells-extraction.yaml).

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

### `soap_call`

Enriches records with SOAP calls.

```yaml
filters:
  - type: soap_call
    endpoint: https://soap.example.com/customer
    operation: GetCustomer
    body: |
      <GetCustomerRequest>
        <id>{{record.customerId}}</id>
      </GetCustomerRequest>
    mergeStrategy: append
    resultKey: soap
```

See [examples/42-soap-call-enrichment.yaml](../examples/42-soap-call-enrichment.yaml).

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

### `soapRequest`

Sends records to a SOAP operation in `batch` or `single` mode.

```yaml
output:
  type: soapRequest
  endpoint: https://soap.example.com/import
  operation: ImportOrders
  requestMode: batch
  body: |
    <m:ImportOrders xmlns:m="urn:orders">
      <m:RecordCount>{{record.recordCount}}</m:RecordCount>
    </m:ImportOrders>
```

Examples:

- [batch SOAP output](../examples/43-soap-output-batch.yaml)
- [MTOM attachment emission](../examples/44-soap-output-mtom-emission.yaml)
- [WS-Security PasswordText](../examples/45-soap-output-wssecurity-passwordtext.yaml)
- [WS-Security PasswordDigest](../examples/45b-soap-output-wssecurity-passworddigest.yaml)
