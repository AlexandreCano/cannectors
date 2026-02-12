# Configuration Examples

This directory contains comprehensive configuration examples covering all use cases for Cannectors Runtime pipelines. Each example is available in both JSON and YAML formats.

## Examples Index

### Basic Examples

#### 01-simple.json / 01-simple.yaml
Simple pipeline without authentication. Minimal configuration to get started.

**Features:**
- HTTP Polling Input module
- HTTP Request Output module
- No authentication
- No filters

#### 02-all-authentication-types.json / 02-all-authentication-types.yaml
Example demonstrating different authentication types (API key in header, Bearer token).

**Features:**
- API key authentication in Input (header location)
- Bearer token authentication in Output
- Shows different auth types can be used in different modules

### Authentication Examples

#### 03-basic-auth.json / 03-basic-auth.yaml
HTTP Basic Authentication example (username/password).

**Features:**
- Basic authentication in both Input and Output modules
- Uses environment variables for credentials (`${API_USERNAME}`, `${API_PASSWORD}`)

#### 04-oauth2.json / 04-oauth2.yaml
OAuth2 Client Credentials flow with automatic token caching and refresh.

**Features:**
- OAuth2 authentication with token URL, client ID, and client secret
- Scopes configuration
- Automatic token refresh on 401 Unauthorized responses

#### 11-api-key-query.json / 11-api-key-query.yaml
API key authentication using query parameter instead of header.

**Features:**
- API key in query parameter location
- Custom parameter name (`apikey`)

### Pagination Examples

#### 05-pagination.json / 05-pagination.yaml
Page-based pagination example.

**Features:**
- Page parameter with total pages field
- Automatic pagination handling
- Data field extraction from nested objects

#### 06-pagination-offset.json / 06-pagination-offset.yaml
Offset-based pagination example.

**Features:**
- Offset and limit parameters
- Total field for pagination control
- Configurable page size (limit: 100)

#### 07-pagination-cursor.json / 07-pagination-cursor.yaml
Cursor-based pagination example.

**Features:**
- Cursor parameter for pagination
- Next cursor field extraction
- Suitable for APIs that use cursor-based pagination

### Filter Examples

#### 08-filters-mapping.json / 08-filters-mapping.yaml
Field mapping filter with transformations.

**Features:**
- Field-to-field mappings
- Default values for missing fields
- Field transformations (formatDate)
- Error handling configuration (`onError: skip`)

#### 09-filters-condition.json / 09-filters-condition.yaml
Conditional filtering example.

**Features:**
- Expression-based filtering (`status == 'active' && price > 0`)
- Simple expression language
- Configurable behavior for true/false conditions

### Advanced Examples

#### 10-complete.json / 10-complete.yaml
Complete pipeline example with all features.

**Features:**
- OAuth2 authentication with scopes
- Page-based pagination
- Multiple filters (condition + mapping)
- Nested field mappings (`customer.id` → `customerId`)
- Keys for path, query, and header extraction from record data
- Retry configuration with exponential backoff
- Custom success status codes
- Error handling configuration
- CRON schedule

#### 12-output-single-record.json / 12-output-single-record.yaml
Single record mode output example.

**Features:**
- One request per record (`requestMode: single`)
- Keys for path, query, and header extraction from record data
- Query parameters configuration
- Retry logic with custom backoff
- PATCH method for updates

#### 13-scheduled.json / 13-scheduled.yaml
CRON scheduled pipeline for periodic data polling.

**Features:**
- CRON schedule expression (`0 * * * *` = every hour)
- HTTP Polling Input with Bearer authentication
- Mapping filter for field transformations
- HTTP Request Output with API key authentication
- Error handling with retry configuration

**Usage:**
```bash
# Run on schedule (auto-detected, keeps running until Ctrl+C)
cannectors run ./configs/examples/13-scheduled.yaml

# Test with dry-run
cannectors run --dry-run ./configs/examples/13-scheduled.yaml
```

**Scheduler Behavior:**
- Executes pipeline at scheduled times
- Skips execution if previous is still running (overlap handling)
- Graceful shutdown on SIGINT/SIGTERM
- Logs all scheduled executions with timestamps

#### 14-webhook.json / 14-webhook.yaml
Webhook input example (event-driven, no schedule).

**Features:**
- Webhook input (`path`) – receives data via HTTP POST
- No CRON schedule (schedule is not allowed for webhook)
- Compare with 13-scheduled (httpPolling requires schedule)

**Usage:**
```bash
cannectors validate ./configs/examples/14-webhook.yaml
cannectors run --dry-run ./configs/examples/14-webhook.yaml
```

#### 15-retry-configuration.json / 15-retry-configuration.yaml
Advanced retry configuration example.

**Features:**
- Custom retryable status codes (`retryableStatusCodes`)
- Retry-After header support (`useRetryAfterHeader: true`)
- Body hint for retry decision (`retryHintFromBody` with expr expression)
- Demonstrates all three new retry configuration options

**Retry Configuration Options:**
- `retryableStatusCodes`: Custom list of HTTP status codes that trigger retry (overrides defaults)
- `useRetryAfterHeader`: Use Retry-After response header to determine delay (supports seconds or HTTP-date format)
- `retryHintFromBody`: expr expression evaluated against JSON response body to determine retryability

**Example Use Cases:**
- Make 408 (Request Timeout) retryable while excluding 500
- Respect server's Retry-After header for rate limiting
- Use API response body hints (e.g., `{"error": {"code": "TEMPORARY"}}`) to determine retryability

**Usage:**
```bash
cannectors validate ./configs/examples/15-retry-configuration.yaml
cannectors run --dry-run ./configs/examples/15-retry-configuration.yaml
```

### Filter Examples (Advanced)

#### 16-filters-script.yaml
JavaScript script filter for complex business logic transformations.

**Features:**
- Goja JavaScript engine for inline scripts
- Complex calculations (discounts, taxes, totals)
- Script can be inline or loaded from file (`scriptFile`)
- Chaining with condition and mapping filters

**Usage:**
```bash
cannectors validate ./configs/examples/16-filters-script.yaml
cannectors run --dry-run ./configs/examples/16-filters-script.yaml
```

#### 17-filters-enrichment.yaml
HTTP call enrichment filter for fetching additional data from external APIs.

**Features:**
- Dynamic HTTP requests with key extraction from records
- Caching to reduce API calls (configurable TTL and max size)
- Parameter types: query, path, header
- Merge strategies: merge, replace, append
- Error handling modes: fail, skip, log

**Usage:**
```bash
cannectors validate ./configs/examples/17-filters-enrichment.yaml
cannectors run --dry-run ./configs/examples/17-filters-enrichment.yaml
```

### State Persistence Examples

#### 18-timestamp-persistence.yaml
Timestamp-based state persistence for incremental polling.

**Features:**
- Automatic timestamp tracking across executions
- Query parameter injection (`?updated_after=<timestamp>`)
- Reliable resumption after restarts

**Usage:**
```bash
cannectors validate ./configs/examples/18-timestamp-persistence.yaml
cannectors run --dry-run ./configs/examples/18-timestamp-persistence.yaml
```

#### 19-id-persistence.yaml
ID-based state persistence for cursor/ID pagination.

**Features:**
- Max ID extraction from processed records
- Query parameter injection (`?since_id=<lastId>`)
- Supports nested field paths for ID extraction

**Usage:**
```bash
cannectors validate ./configs/examples/19-id-persistence.yaml
cannectors run --dry-run ./configs/examples/19-id-persistence.yaml
```

#### 20-timestamp-and-id-persistence.yaml
Combined timestamp and ID persistence for maximum reliability.

**Features:**
- Dual tracking: both timestamp and last ID
- Custom storage path configuration
- Works with cursor-based pagination

**Usage:**
```bash
cannectors validate ./configs/examples/20-timestamp-and-id-persistence.yaml
cannectors run --dry-run ./configs/examples/20-timestamp-and-id-persistence.yaml
```

### Templating Examples

#### 21-output-templating.yaml
Dynamic output templating with record data.

**Features:**
- Template syntax: `{{record.field}}` for accessing record data
- Templated endpoints: Dynamic URL construction based on record fields
- Templated headers: Extract header values from record data
- External body template files: Support for JSON, XML, SOAP, and any text format
- Nested field access: `{{record.user.profile.name}}`
- Array index access: `{{record.items[0].name}}`
- Default values: `{{record.field | default: "value"}}`

**Template Locations:**
- `endpoint`: Inline template in endpoint URL (e.g., `/customers/{{record.id}}/orders`)
- `headers`: Inline templates in header values (e.g., `X-User-ID: {{record.user_id}}`)
- `bodyTemplateFile`: External file with template (supports JSON, XML, SOAP, etc.)

**Usage:**
```bash
cannectors validate ./configs/examples/21-output-templating.yaml
cannectors run --dry-run ./configs/examples/21-output-templating.yaml
```

#### 22-output-templating-batch.yaml
Templating in batch mode (`requestMode: "batch"`).

**Features:**
- Batch mode templating: Templates use the FIRST record's data
- Multi-tenant scenarios: All records in batch share tenant context
- External template file for batch payloads

**Use Case:** Multi-tenant API where all records belong to the same tenant, tenant ID from first record routes the entire batch.

**Usage:**
```bash
cannectors validate ./configs/examples/22-output-templating-batch.yaml
cannectors run --dry-run ./configs/examples/22-output-templating-batch.yaml
```

#### 23-output-templating-soap.yaml
SOAP/XML templating for legacy API integration.

**Features:**
- XML/SOAP body templates with `{{record.field}}` syntax
- Custom Content-Type header for XML (`text/xml`)
- Template file can be any format (JSON, XML, SOAP, plain text)

**Use Case:** Integration with legacy SOAP web services that require XML payloads with dynamic data.

**Template Example:**
```xml
<?xml version="1.0" encoding="UTF-8"?>
<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/">
  <soap:Body>
    <CreateUser>
      <userId>{{record.user_id}}</userId>
      <userName>{{record.name}}</userName>
    </CreateUser>
  </soap:Body>
</soap:Envelope>
```

**Usage:**
```bash
cannectors validate ./configs/examples/23-output-templating-soap.yaml
cannectors run --dry-run ./configs/examples/23-output-templating-soap.yaml
```

#### 24-enrichment-templating.yaml
Templating in enrichment filters for dynamic API lookups.

**Features:**
- Templated endpoints in enrichment filter
- Templated headers for authentication/correlation
- Templated request bodies for POST/PUT enrichment calls
- Multiple enrichment strategies with different template patterns

**Use Case:** Enrich order records with customer data by calling CRM API with customer ID from each record.

**Usage:**
```bash
cannectors validate ./configs/examples/24-enrichment-templating.yaml
cannectors run --dry-run ./configs/examples/24-enrichment-templating.yaml
```

### Metadata Examples

#### 25-record-metadata-storage.yaml
Internal metadata storage for state tracking through the pipeline.

**Features:**
- `_metadata` field for internal state (excluded from output body)
- Processing timestamps, batch IDs, validation flags
- Metadata accessible in condition filters and headers
- Automatic exclusion from request body

**Usage:**
```bash
cannectors validate ./configs/examples/25-record-metadata-storage.yaml
cannectors run --dry-run ./configs/examples/25-record-metadata-storage.yaml
```

#### 26-metadata-in-templates.yaml
Using metadata values in output templates.

**Features:**
- Metadata in endpoint URLs (`{{_metadata.region}}`)
- Metadata in query parameters and headers
- Routing based on metadata (region, priority)
- TTL and timing information in metadata

**Usage:**
```bash
cannectors validate ./configs/examples/26-metadata-in-templates.yaml
cannectors run --dry-run ./configs/examples/26-metadata-in-templates.yaml
```

### Database Examples

#### 27-database-input.yaml
Database input module for fetching data from SQL databases.

**Features:**
- Supports PostgreSQL, MySQL, SQLite
- Connection string via environment variable
- Custom SQL queries with timeout configuration
- Scheduled polling

**Usage:**
```bash
cannectors validate ./configs/examples/27-database-input.yaml
cannectors run --dry-run ./configs/examples/27-database-input.yaml
```

#### 28-database-incremental.yaml
Incremental database queries using `{{lastRunTimestamp}}`.

**Features:**
- `{{lastRunTimestamp}}` placeholder for incremental queries
- First run uses epoch time (fetches all records)
- Subsequent runs fetch only new/updated records
- State persistence for reliable resumption

**Usage:**
```bash
cannectors validate ./configs/examples/28-database-incremental.yaml
cannectors run --dry-run ./configs/examples/28-database-incremental.yaml
```

#### 29-sql-call-enrichment.yaml
SQL call filter for enriching records with database data.

**Features:**
- Query templating with `{{record.field}}` syntax
- Merge strategies: merge, replace, append
- Caching to avoid repeated queries
- Error handling: fail, skip, log

**Usage:**
```bash
cannectors validate ./configs/examples/29-sql-call-enrichment.yaml
cannectors run --dry-run ./configs/examples/29-sql-call-enrichment.yaml
```

#### 30-database-output-insert.yaml
Database output module with INSERT queries.

**Features:**
- SQL queries with `{{record.field}}` placeholders
- Parameterized values for security
- Transaction mode for atomic writes
- Error handling per record

**Usage:**
```bash
cannectors validate ./configs/examples/30-database-output-insert.yaml
cannectors run --dry-run ./configs/examples/30-database-output-insert.yaml
```

#### 31-database-output-upsert.yaml
Database upsert (insert or update on conflict).

**Features:**
- PostgreSQL `ON CONFLICT DO UPDATE` syntax
- MySQL `ON DUPLICATE KEY UPDATE` (commented example)
- SQLite `INSERT OR REPLACE` (commented example)
- Transaction support

**Usage:**
```bash
cannectors validate ./configs/examples/31-database-output-upsert.yaml
cannectors run --dry-run ./configs/examples/31-database-output-upsert.yaml
```

#### 32-database-custom-query.yaml
Database output with external SQL files.

**Features:**
- `queryFile` for complex SQL queries
- Separation of SQL from configuration
- Same `{{record.field}}` templating in files

**Usage:**
```bash
cannectors validate ./configs/examples/32-database-custom-query.yaml
cannectors run --dry-run ./configs/examples/32-database-custom-query.yaml
```

## Using the Examples

### Validate an Example

```bash
# JSON format
cannectors validate ./configs/examples/01-simple.json

# YAML format
cannectors validate ./configs/examples/01-simple.yaml
```

### Run an Example (Dry-Run)

```bash
# Test without executing output module
cannectors run --dry-run ./configs/examples/10-complete.yaml
```

### Use as Templates

Copy any example file and modify it for your use case:

```bash
# Copy example
cp ./configs/examples/05-pagination.json my-pipeline.json

# Edit with your endpoints and credentials
# Then validate and run
cannectors validate my-pipeline.json
cannectors run my-pipeline.json
```

## Configuration Features Reference

### Authentication Types

| Type | Description | Credentials |
|------|-------------|-------------|
| `api-key` | API key in header or query | `key`, `location`, `headerName`/`paramName` |
| `bearer` | Bearer token | `token` |
| `basic` | HTTP Basic Auth | `username`, `password` |
| `oauth2` | OAuth2 Client Credentials | `tokenUrl`, `clientId`, `clientSecret`, `scopes` |

### Pagination Types

| Type | Parameters |
|------|------------|
| `page` | `pageParam`, `totalPagesField` |
| `offset` | `offsetParam`, `limitParam`, `limit`, `totalField` |
| `cursor` | `cursorParam`, `nextCursorField` |

### Filter Modules

| Type | Configuration |
|------|---------------|
| `mapping` | Array of field mappings with optional transforms |
| `condition` | Expression string with optional nested modules |
| `script` | JavaScript transformation (inline or from file) |
| `http_call` | HTTP enrichment with caching and merge strategies |
| `sql_call` | SQL enrichment with caching and merge strategies |

### Output Modes

| Mode | Description |
|------|-------------|
| `batch` (default) | Send all records in single request as JSON array |
| `single` | Send one request per record with path/query parameters |

### Input/Output Types

| Type | Module | Description |
|------|--------|-------------|
| `httpPolling` | Input | Poll HTTP API on schedule |
| `webhook` | Input | Receive HTTP POST events |
| `database` | Input | Query SQL database |
| `httpRequest` | Output | Send HTTP requests |
| `database` | Output | Execute SQL queries |

## Environment Variables

All examples use environment variables for sensitive credentials:

- `${API_TOKEN}` - Bearer token
- `${API_KEY}` - API key
- `${API_USERNAME}` - Basic auth username
- `${API_PASSWORD}` - Basic auth password
- `${OAUTH_CLIENT_ID}` - OAuth2 client ID
- `${OAUTH_CLIENT_SECRET}` - OAuth2 client secret
- `${SOURCE_*}` / `${DEST_*}` - Source and destination credentials

Set these before running:

```bash
export API_TOKEN="your-token-here"
export API_KEY="your-api-key-here"
# ... etc
```

## Notes

- All examples use placeholder endpoints (`https://api.example.com`, etc.)
- Replace with your actual API endpoints
- Credentials are shown as environment variables - in production, use secure credential management
- Timeouts are specified in seconds (Input) or milliseconds (Output)
- Schedules use CRON expression format: **required** for polling input types (e.g. httpPolling), **not allowed** for event-driven types (e.g. webhook)
