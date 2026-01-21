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

## Notes

- All examples use placeholder endpoints (`https://api.example.com`, etc.)
- Replace with your actual API endpoints
- Credentials are shown as environment variables - in production, use secure credential management
- Timeouts are specified in seconds (Input) or milliseconds (Output)
- Schedules use CRON expression format (optional)
