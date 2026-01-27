# Configuration Examples

This directory contains comprehensive configuration examples covering all use cases for Canectors Runtime pipelines. Each example is available in both JSON and YAML formats.

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
- Output path parameters from record data
- Query parameters from record data
- Headers from record data
- Retry configuration with exponential backoff
- Custom success status codes
- Error handling configuration
- CRON schedule

#### 12-output-single-record.json / 12-output-single-record.yaml
Single record mode output example.

**Features:**
- One request per record (`bodyFrom: record`)
- Path parameter substitution from record data
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
canectors run ./configs/examples/13-scheduled.yaml

# Test with dry-run
canectors run --dry-run ./configs/examples/13-scheduled.yaml
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
canectors validate ./configs/examples/14-webhook.yaml
canectors run --dry-run ./configs/examples/14-webhook.yaml
```

#### 15-retry-configuration.json / 15-retry-configuration.yaml
Advanced retry configuration example (Story 13.3).

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
canectors validate ./configs/examples/15-retry-configuration.yaml
canectors run --dry-run ./configs/examples/15-retry-configuration.yaml
```

### Templating Examples

#### 21-output-templating.json / 21-output-templating.yaml
Dynamic output templating with record data (Story 14.6).

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
- `request.bodyTemplateFile`: External file with template (supports JSON, XML, SOAP, etc.)

**Usage:**
```bash
canectors validate ./configs/examples/21-output-templating.yaml
canectors run --dry-run ./configs/examples/21-output-templating.yaml
```

#### 22-output-templating-batch.json / 22-output-templating-batch.yaml
Templating in batch mode (`bodyFrom: "records"`).

**Features:**
- Batch mode templating: Templates use the FIRST record's data
- Multi-tenant scenarios: All records in batch share tenant context
- External template file for batch payloads

**Use Case:** Multi-tenant API where all records belong to the same tenant, tenant ID from first record routes the entire batch.

**Usage:**
```bash
canectors validate ./configs/examples/22-output-templating-batch.yaml
canectors run --dry-run ./configs/examples/22-output-templating-batch.yaml
```

#### 23-output-templating-soap.json / 23-output-templating-soap.yaml
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
canectors validate ./configs/examples/23-output-templating-soap.yaml
canectors run --dry-run ./configs/examples/23-output-templating-soap.yaml
```

#### 24-enrichment-templating.json / 24-enrichment-templating.yaml
Templating in enrichment filters for dynamic API lookups.

**Features:**
- Templated endpoints in enrichment filter
- Templated headers for authentication/correlation
- Templated request bodies for POST/PUT enrichment calls
- Multiple enrichment strategies with different template patterns

**Use Case:** Enrich order records with customer data by calling CRM API with customer ID from each record.

**Usage:**
```bash
canectors validate ./configs/examples/24-enrichment-templating.yaml
canectors run --dry-run ./configs/examples/24-enrichment-templating.yaml
```

## Using the Examples

### Validate an Example

```bash
# JSON format
canectors validate ./configs/examples/01-simple.json

# YAML format
canectors validate ./configs/examples/01-simple.yaml
```

### Run an Example (Dry-Run)

```bash
# Test without executing output module
canectors run --dry-run ./configs/examples/10-complete.yaml
```

### Use as Templates

Copy any example file and modify it for your use case:

```bash
# Copy example
cp ./configs/examples/05-pagination.json my-pipeline.json

# Edit with your endpoints and credentials
# Then validate and run
canectors validate my-pipeline.json
canectors run my-pipeline.json
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

### Output Modes

| Mode | Description |
|------|-------------|
| `records` (default) | Send all records in single request as JSON array |
| `record` | Send one request per record with path/query parameters |

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

### Output Templating Examples (Story 14.6)

#### 21-output-templating.yaml
Output templating with record data for dynamic endpoints, headers, and bodies.

**Features:**
- Templated endpoint URLs (`/customers/{{record.customer_id}}/orders`)
- Templated HTTP headers (`X-Correlation-ID: {{record.correlation_id}}`)
- Custom body templates with field mapping
- Default values for missing fields (`{{record.field | default: "value"}}`)
- Single record mode (`bodyFrom: "record"`)

**Template Syntax:**
- `{{record.field}}` - Access field from record
- `{{record.nested.field}}` - Access nested fields using dot notation
- `{{record.array[0].field}}` - Access array elements by index
- `{{record.field | default: "value"}}` - Use default value if field is missing/null

**Usage:**
```bash
canectors validate ./configs/examples/21-output-templating.yaml
canectors run --dry-run ./configs/examples/21-output-templating.yaml
```

#### 22-output-templating-batch.yaml
Output templating in batch mode for multi-tenant or grouped data scenarios.

**Features:**
- Batch mode with templated endpoint (uses first record for template evaluation)
- Tenant-based routing (`/tenants/{{record.tenant_id}}/events/bulk`)
- Body template applied to each record in the batch
- Suitable for multi-tenant APIs

**Batch Mode Behavior:**
- In batch mode (`bodyFrom: "records"`), endpoint and header templates use the **first record**
- All records should share common metadata (tenant ID, batch ID) for batch mode templating
- Each record in the batch array is individually templated using `bodyTemplate`

**Usage:**
```bash
canectors validate ./configs/examples/22-output-templating-batch.yaml
canectors run --dry-run ./configs/examples/22-output-templating-batch.yaml
```

## Notes

- All examples use placeholder endpoints (`https://api.example.com`, etc.)
- Replace with your actual API endpoints
- Credentials are shown as environment variables - in production, use secure credential management
- Timeouts are specified in seconds (Input) or milliseconds (Output)
- Schedules use CRON expression format: **required** for polling input types (e.g. httpPolling), **not allowed** for event-driven types (e.g. webhook)
