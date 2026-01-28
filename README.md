# Canectors Runtime

Portable runtime CLI for executing connector pipelines. Canectors is a cross-platform tool that reads pipeline configurations (JSON/YAML) and executes Input, Filter, and Output modules to transfer data between systems.

## Features

- **Pipeline Execution**: Execute data pipelines defined in JSON/YAML configuration files
- **Configuration Validation**: Validate pipeline configurations against JSON Schema before execution
- **Modular Architecture**: Input, Filter, and Output modules for flexible data processing (Epic 3)
- **Cross-Platform**: Runs on Windows, macOS (Intel & Apple Silicon), and Linux
- **Dry-Run Mode**: Validate and test pipelines without executing output modules
- **Execution Logging**: Comprehensive structured logging with JSON and human-readable formats
  - Per-stage timing (input, filter, output durations)
  - Performance metrics (records/sec, throughput)
  - Configurable log levels and output formats
  - File logging support (`--log-file`)
- **Resource Cleanup**: Automatic cleanup of module resources (connections, file handles)
- **Error Handling & Retry**: Classified errors (network, auth, validation, server), configurable retry with exponential backoff, timeout handling, and structured error logging (Story 4.4)
- **State Persistence**: Reliable resumption of polling after restarts by persisting execution timestamp and/or last processed ID (Story 14.5)
- **Record Metadata Storage**: Store internal state in records (e.g., `_metadata.processed_at`) that persists through the pipeline but is excluded from output request body (Story 14.7)

## Project Status

**Epic 2: CLI Runtime Foundation** ✅ **COMPLETE**

- ✅ **Story 2.1**: Project structure initialized
- ✅ **Story 2.2**: Configuration parser with JSON/YAML support
- ✅ **Story 2.3**: Pipeline orchestration engine (Input → Filter → Output)

**Epic 3: Module Execution** ✅ **COMPLETE**

- ✅ **Story 3.1**: HTTP polling input module with pagination support
- ✅ **Story 3.2**: Webhook input module
- ✅ **Story 3.3**: Mapping filter module with transformations
- ✅ **Story 3.4**: Condition filter module with expression evaluation
- ✅ **Story 3.5**: HTTP request output module with retry logic
- ✅ **Story 3.6**: Authentication handling (API key, Bearer, Basic, OAuth2)

**Epic 4: Advanced Runtime Features** 🚧 **IN PROGRESS**

- ✅ **Story 4.1**: CRON scheduler for periodic pipeline execution
- ✅ **Story 4.2**: Dry-run mode with request preview
- ✅ **Story 4.3**: Execution logging with structured output and metrics
- ✅ **Story 4.4**: Error handling and retry logic (classification, retry config, onError, timeout, ExecutionResult retry/error details)

## Project Structure

```
canectors-runtime/
├── cmd/
│   └── canectors/          # CLI entry point
│       ├── main.go         # CLI commands (validate, run, version)
│       └── main_test.go    # CLI integration tests (15 tests)
├── internal/               # Private packages
│   ├── config/             # Configuration parsing and validation
│   │   ├── parser.go       # JSON/YAML parser with auto-detection
│   │   ├── validator.go    # JSON Schema validation
│   │   ├── converter.go    # Config to Pipeline type conversion
│   │   └── types.go        # ConfigResult, ParseError, ValidationError
│   ├── errhandling/        # Error classification and retry (Story 4.4)
│   │   ├── errors.go       # Error categories, ClassifyError, ClassifyHTTPStatus
│   │   ├── retry.go        # RetryConfig, RetryExecutor, exponential backoff
│   │   └── *_test.go       # Unit tests
│   ├── logger/             # Structured JSON logging (slog)
│   ├── modules/            # Module implementations
│   │   ├── input/          # Input modules (HTTP Polling, Webhook)
│   │   ├── filter/         # Filter modules (Mapping, Condition)
│   │   └── output/         # Output modules (HTTP Request)
│   ├── runtime/            # Pipeline execution engine
│   │   ├── pipeline.go     # Executor with Input → Filter → Output orchestration
│   │   ├── errors.go       # Re-exports from errhandling
│   │   ├── retry.go        # Re-exports from errhandling
│   │   └── pipeline_test.go # Executor tests
│   └── scheduler/          # CRON scheduling (Epic 4)
├── pkg/
│   └── connector/          # Public types (Pipeline, ExecutionResult, ModuleConfig)
├── configs/                # Example configuration files
├── .github/
│   └── workflows/
│       └── ci.yml          # GitHub Actions CI/CD (lint, test, build)
├── go.mod                  # Go 1.23.5
├── .golangci.yml           # golangci-lint v2.7.1 configuration
└── README.md
```

## Requirements

- **Go**: 1.23.5 or later
- **golangci-lint**: v2.7.1+ (for linting, optional)

## Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/canectors/runtime.git
cd canectors-runtime

# Build the binary
go build -o canectors ./cmd/canectors

# Or install to GOPATH/bin
go install ./cmd/canectors
```

### Pre-built Binaries

Download the latest release for your platform from the [Releases](https://github.com/canectors/runtime/releases) page.

## Usage

### Basic Commands

```bash
# Display help
canectors --help

# Display version information
canectors version

# Validate a pipeline configuration
canectors validate ./configs/example-connector.json

# Validate with verbose output
canectors validate --verbose ./configs/example-connector.json

# Execute a pipeline
canectors run ./configs/example-connector.json

# Execute with dry-run (validate only, skip output module)
canectors run --dry-run ./configs/example-connector.json

# Quiet mode (suppress non-error output)
canectors validate --quiet ./configs/example-connector.json

# Run a pipeline on a schedule (auto-detected from 'schedule' field)
canectors run ./configs/examples/13-scheduled.yaml
```

### Dry-Run Mode

Dry-run mode validates and tests your pipeline configuration without actually sending data to the target system. This is useful for:
- **Testing configurations** before production use
- **Validating transformations** to ensure data is processed correctly
- **Debugging pipelines** without side effects

```bash
# Execute pipeline in dry-run mode
canectors run --dry-run ./configs/example-connector.json

# With verbose output (shows full request details)
canectors run --dry-run --verbose ./configs/example-connector.json
```

**What happens in dry-run mode:**
1. ✅ **Input module executes normally** - Data is fetched from the source
2. ✅ **Filter modules execute normally** - Data transformations are applied
3. ❌ **Output module is skipped** - No HTTP requests are sent to the target
4. 📋 **Preview is displayed (when supported by the configured output module)** - Shows what would have been sent when preview is available

> Note: Some output modules do not support preview. In those cases, dry-run will still execute inputs and filters and skip sending data, but no "Dry-Run Preview" section will be shown in the output.
**Example output:**
```
✓ Pipeline executed successfully
  Status: success
  Records processed: 15

📋 Dry-Run Preview (what would have been sent):

  Endpoint: POST https://api.destination.com/import
  Records: 15
  Headers:
    Authorization: Bearer [MASKED-TOKEN]
    Content-Type: application/json
    User-Agent: Canectors-Runtime/1.0
    X-Custom-Header: custom-value
  Body:
    [
      {
        "externalId": "123",
        "name": "Example Record",
        ...
      }
    ]

ℹ️  No data was sent to the target system (dry-run mode)
```

**Verbose mode** (`--verbose`) shows additional details:
- Full request body (without truncation for large payloads)
- Execution duration

**Show credentials for debugging** (`dryRunOptions.showCredentials`):

By default, authentication credentials are masked in dry-run preview for security. If you need to debug authentication issues, you can enable credential display in your configuration:

```yaml
# In your pipeline configuration
dryRunOptions:
  showCredentials: true  # WARNING: Only use in secure environments

# Or in JSON
"dryRunOptions": {
  "showCredentials": true
}
```

⚠️ **Security Warning**: Only enable `showCredentials` in secure, local debugging environments. Never commit configurations with this option enabled to version control or use in production.

### Scheduled Mode (Auto-Detected)

If your configuration includes a `schedule` field with a CRON expression, the `run` command automatically switches to scheduler mode. The scheduler keeps running until interrupted (Ctrl+C / SIGINT / SIGTERM).

```bash
# Run with schedule (auto-detected from config)
canectors run ./configs/examples/13-scheduled.yaml

# Output example:
# Loading pipeline configuration: ./configs/examples/13-scheduled.yaml
# ✓ Configuration loaded successfully (format: yaml)
# 🕐 Scheduler started
#   Pipeline: hourly-sync
#   Schedule: 0 * * * *
#   Next run: 2026-01-22T20:00:00Z
#   Press Ctrl+C to stop...
```

**CRON Expression Format**:

| Expression | Description |
|------------|-------------|
| `* * * * *` | Every minute |
| `*/5 * * * *` | Every 5 minutes |
| `0 * * * *` | Every hour at minute 0 |
| `0 0 * * *` | Every day at midnight |
| `0 9 * * 1-5` | Weekdays at 9 AM |
| `* * * * * *` | Every second (6-field extended format) |

**Scheduler Behavior**:
- **Overlap handling**: If a pipeline execution is still running when the next trigger occurs, the new execution is skipped
- **Graceful shutdown**: On SIGINT/SIGTERM, waits for in-flight executions to complete (30s timeout)
- **Structured logging**: All scheduled executions are logged with timestamps and durations

### State Persistence for Polling Inputs

State persistence enables reliable resumption of data polling after restarts, avoiding duplicates or data loss. The runtime persists the last execution timestamp and/or last processed ID, then uses this state to filter API requests on subsequent executions.

**How it works:**
1. **First execution**: No state exists, fetches all records
2. **Runtime persists state**: After successful execution (Input → Filter → Output), saves execution start timestamp and/or ID from the last processed record (in reception order)
3. **Subsequent executions**: Loads persisted state and adds query parameters to API requests (e.g., `?since=2026-01-26T10:30:00Z` or `?after_id=12345`)
4. **API filtering**: Only processes records newer than the last execution timestamp or with ID greater than the last processed ID

**Configuration Example (Timestamp Persistence):**

```yaml
input:
  type: httpPolling
  endpoint: https://api.example.com/v1/orders
  schedule: "*/5 * * * *"
  
  statePersistence:
    timestamp:
      enabled: true
      queryParam: updated_after  # API adds ?updated_after=<timestamp>
```

**Configuration Example (ID Persistence):**

```yaml
input:
  type: httpPolling
  endpoint: https://api.example.com/v1/orders
  schedule: "*/5 * * * *"
  
  statePersistence:
    id:
      enabled: true
      field: id  # Field path to extract ID from records
      queryParam: after_id  # API adds ?after_id=<lastId>
```

**Configuration Example (Both Timestamp and ID):**

```yaml
input:
  type: httpPolling
  endpoint: https://api.example.com/v1/orders
  schedule: "*/5 * * * *"
  
  statePersistence:
    timestamp:
      enabled: true
      queryParam: updated_after
    id:
      enabled: true
      field: order_id
      queryParam: after_id
    # Optional: custom storage path (defaults to ./canectors-data/state)
    # storagePath: /custom/path/to/state
```

**State Storage:**
- **Location**: `./canectors-data/state/{pipeline-id}.json` (default)
- **Format**: JSON with `pipelineId`, `lastTimestamp`, `lastId`, `updatedAt`
- **Isolation**: Each pipeline maintains its own separate state file
- **Thread-safe**: Safe for concurrent executions (mutex protection)

**Features:**
- ✅ **Timestamp persistence**: Uses pipeline execution start timestamp (not extracted from records)
- ✅ **ID persistence**: Extracts and tracks last ID from processed records (supports nested field paths with dot notation)
- ✅ **API query filtering**: Automatically adds query parameters to API requests when state exists
- ✅ **Graceful error handling**: State loading/saving errors don't fail pipeline execution (logs warning, continues without persistence)
- ✅ **Works with scheduled executions**: State persistence works correctly with CRON scheduler
- ✅ **Works with manual executions**: State persists between manual runs

**Troubleshooting:**
- **State not persisting**: Check that `SetStateStore` is called (should be automatic in CLI)
- **State file not found**: First execution creates the state file after successful execution
- **State loading errors**: Check file permissions on `./canectors-data/state/` directory
- **ID extraction fails**: Verify `field` path matches your record structure (supports dot notation like `data.id`)

See example configurations in `configs/examples/18-timestamp-persistence.yaml`, `19-id-persistence.yaml`, and `20-timestamp-and-id-persistence.yaml`.

### Record Metadata Storage (Story 14.7)

The runtime supports storing metadata in records using the `_metadata` field. This metadata persists through the entire pipeline (Input → Filter → Output) but is automatically excluded from the output request body.

#### Key Features

- **Fixed field name**: Always uses `_metadata` (not configurable)
- **Persistent**: Metadata survives filter transformations and record cloning
- **Excluded from body**: Automatically stripped from request body before sending
- **Template access**: Can be used in endpoint URLs, headers, and query parameters via templating
- **Nested support**: Supports nested objects for organizing related values

#### Use Cases

- **Execution tracking**: Store timestamps (`_metadata.processed_at`, `_metadata.received_at`)
- **Processing flags**: Track validation status (`_metadata.validated`, `_metadata.enriched`)
- **Error tracking**: Store error information (`_metadata.errors`, `_metadata.warnings`)
- **Custom state**: Application-specific values (`_metadata.source_system`, `_metadata.batch_id`)
- **Nested organization**: Group related values (`_metadata.timing.start`, `_metadata.timing.end`)

#### Accessing Metadata in Filters

Filter modules can read, write, and modify metadata values:

**Script Filter:**
```javascript
function transform(record) {
  // Initialize metadata
  record._metadata = record._metadata || {};
  
  // Set metadata values
  record._metadata.processed_at = new Date().toISOString();
  record._metadata.source_system = "external-api";
  record._metadata.validated = record.email && record.email.includes("@");
  
  return record;
}
```

**Mapping Filter:**
The mapping filter automatically preserves `_metadata` when transforming records. Metadata is available for use in mapping source paths.

**Condition Filter:**
Metadata can be used in condition expressions:
```yaml
filters:
  - type: condition
    config:
      expression: _metadata.validated == true
      onTrue: continue
      onFalse: skip
```

#### Using Metadata in Templates

Metadata values can be accessed in template expressions for dynamic content:

**Endpoint URLs:**
```yaml
output:
  type: http-request
  config:
    endpoint: /api/{{_metadata.region}}/orders
```

**HTTP Headers:**
```yaml
output:
  type: http-request
  config:
    headers:
      X-Processed-At: "{{_metadata.processed_at}}"
      X-Source-System: "{{_metadata.source_system}}"
      X-Batch-ID: "{{_metadata.batch_id}}"
```

**Query Parameters:**
```yaml
output:
  type: http-request
  config:
    request:
      queryFromRecord:
        priority: _metadata.priority
```

#### Important Notes

- **Always excluded**: The `_metadata` field is automatically stripped from request bodies before sending
- **Not configurable**: The field name is fixed as `_metadata` and cannot be changed
- **Template syntax**: Use `{{_metadata.field}}` or `{{_metadata.nested.field}}` in templates
- **Preservation**: Metadata is preserved when records are cloned, duplicated, or transformed by filters

#### Example Configuration

See `configs/examples/25-record-metadata-storage.yaml` and `configs/examples/26-metadata-in-templates.yaml` for complete examples.

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Validation errors (schema violations) |
| 2 | Parse errors (invalid JSON/YAML syntax) |
| 3 | Runtime errors (execution failures) |

### Logging

The CLI provides comprehensive execution logging with configurable output formats and levels.

#### Log Formats

| Format | Description | Use Case |
|--------|-------------|----------|
| **JSON** (default) | Machine-readable structured logs | Production, log aggregation, automated analysis |
| **Human** (`--verbose`) | Human-readable console output with colors and prefixes | Development, debugging, manual inspection |

#### Log Levels

| Level | Description | Flag |
|-------|-------------|------|
| **Debug** | Detailed execution information (requests, transformations) | `--verbose` |
| **Info** | Important execution events (start, end, summary) | default |
| **Warn** | Warnings (non-fatal issues, retries) | default |
| **Error** | Errors (execution failures) | `--quiet` (only errors) |

#### CLI Flags for Logging

```bash
# Verbose mode: Human-readable output, debug level
canectors run --verbose config.yaml

# Quiet mode: Only show errors
canectors run --quiet config.yaml

# Log to file (always JSON, in addition to console)
canectors run --log-file execution.log config.yaml

# Combine flags
canectors run --verbose --log-file debug.log config.yaml
```

#### Log Output Examples

**JSON format (default):**
```json
{"time":"2026-01-22T16:00:00Z","level":"INFO","msg":"input fetch started","module_type":"httpPolling","endpoint":"https://api.example.com/data","timeout":"30s"}
{"time":"2026-01-22T16:00:01Z","level":"INFO","msg":"input fetch completed","module_type":"httpPolling","record_count":100,"duration":"1.2s"}
{"time":"2026-01-22T16:00:02Z","level":"INFO","msg":"execution metrics","pipeline_id":"sync-pipeline","records_processed":100,"records_per_second":50.5,"total_duration":"2s"}
```

**Human-readable format (`--verbose`):**
```
16:00:00 ℹ input fetch started module_type=httpPolling endpoint=https://api.example.com/data
16:00:01 ℹ input fetch completed module_type=httpPolling record_count=100 duration=1.2s
16:00:01 ℹ filter processing completed module_type=mapping input_records=100 output_records=95
16:00:02 ℹ output send completed records_sent=95 duration=800ms
16:00:02 ℹ execution metrics records_processed=95 records_per_second=47.5 total_duration=2s
```

#### Structured Log Fields

All logs include consistent, structured fields for easy parsing:

| Field | Description |
|-------|-------------|
| `pipeline_id` | Pipeline identifier |
| `pipeline_name` | Human-readable pipeline name |
| `stage` | Execution stage (input, filter, output) |
| `module_type` | Module type (httpPolling, mapping, httpRequest) |
| `duration` | Execution duration |
| `record_count` | Number of records processed |
| `records_processed` | Total records successfully processed |
| `records_failed` | Number of failed records |
| `error` | Error message (for error logs) |
| `error_code` | Error code (HTTP_ERROR, INPUT_FAILED, etc.) |
| `error_category` | Classified category (network, authentication, validation, server, etc.) |
| `error_type` | `retryable` or `fatal` |

#### Analyzing Logs

**Parse JSON logs with `jq`:**
```bash
# Show all errors
cat execution.log | jq 'select(.level == "ERROR")'

# Show execution metrics
cat execution.log | jq 'select(.msg == "execution metrics")'

# Filter by pipeline
cat execution.log | jq 'select(.pipeline_id == "my-pipeline")'

# Calculate average throughput
cat execution.log | jq 'select(.msg == "execution metrics") | .records_per_second' | awk '{sum+=$1; count++} END {print sum/count}'
```

**Monitor logs in real-time:**
```bash
# Follow log file with formatted output
tail -f execution.log | jq .

# Filter for errors only
tail -f execution.log | jq 'select(.level == "ERROR")'
```

#### Troubleshooting with Logs

**Common log patterns:**

| Log Message | Meaning | Action |
|-------------|---------|--------|
| `http error response` with `status_code=401` | Authentication failed | Check credentials configuration |
| `http error response` with `status_code=429` | Rate limited | Reduce request frequency or add delays |
| `retrying request` with `attempt=3` | Transient failures | Check network/target availability |
| `filter processing failed` | Transformation error | Check filter configuration and data format |
| `all retry attempts exhausted` | Persistent failure | Investigate target system issues |

**Debug authentication issues:**
```bash
# Run with verbose logging
canectors run --verbose --dry-run config.yaml 2>&1 | grep -i auth

# Check for 401 errors in logs
cat execution.log | jq 'select(.http_status == 401)'
```

**Debug data transformation issues:**
```bash
# See filter processing details
canectors run --verbose config.yaml 2>&1 | grep -i filter

# Check for transformation errors
cat execution.log | jq 'select(.msg | contains("mapping error"))'
```

### Error Handling and Retry (Story 4.4)

The runtime classifies errors and applies configurable retry logic. **Precedence:** `module` > `defaults` > `errorHandling`.

#### Error categories

| Category | Examples | Retryable |
|----------|----------|-----------|
| `network` | Timeout, connection refused, DNS | Yes |
| `authentication` | 401, 403 | No |
| `validation` | 400, 422 | No |
| `not_found` | 404 | No |
| `rate_limit` | 429 | Yes |
| `server` | 500, 502, 503, 504 | Yes |

#### Retry configuration

| Field | Description | Default |
|-------|-------------|---------|
| `maxAttempts` | Max retries (0 = no retry) | 3 |
| `delayMs` | Initial delay (ms) | 1000 |
| `backoffMultiplier` | Exponential backoff factor | 2 |
| `maxDelayMs` | Cap per delay (ms) | 30000 |
| `retryableStatusCodes` | HTTP codes that trigger retry | [429, 500, 502, 503, 504] |

#### Error handling strategies (`onError`)

| Value | Behavior |
|-------|----------|
| `fail` | Stop execution, return error (default) |
| `skip` | Skip failed record/module, continue |
| `log` | Log error, continue |

#### Timeout (`timeoutMs`)

Request timeout in milliseconds. Applied to HTTP Polling and HTTP Request modules. Default: 30000.

#### Example configuration

```yaml
connector:
  name: my-connector
  version: 1.0.0
  defaults:
    onError: fail
    timeoutMs: 30000
    retry:
      maxAttempts: 5
      delayMs: 500
      backoffMultiplier: 2
      maxDelayMs: 15000
  input:
    type: httpPolling
    endpoint: https://api.example.com/data
    retry:
      maxAttempts: 3
  output:
    type: httpRequest
    endpoint: https://api.example.com/ingest
    method: POST
    onError: skip
```

#### Troubleshooting common errors

| Scenario | Cause | Action |
|----------|--------|--------|
| `connection refused` | Target unreachable | Check URL, firewall, network |
| `401` / `403` | Bad or missing credentials | Verify `authentication` in config |
| `429` | Rate limiting | Increase `delayMs`, reduce frequency, or use backoff |
| `500` / `502` / `503` | Upstream failures | Retries apply; check target health |
| `timeout` | Slow or stuck request | Increase `timeoutMs` or fix upstream |

### Pipeline Configuration Format

The configuration file uses a simple structure with Input, Filters (optional), and Output modules:

```json
{
  "id": "example-pipeline",
  "name": "Example Pipeline",
  "version": "1.0.0",
  "enabled": true,
  "input": {
    "type": "http-polling",
    "config": {
      "endpoint": "https://api.example.com/data",
      "timeout": 30
    },
    "authentication": {
      "type": "bearer",
      "credentials": {
        "token": "${API_TOKEN}"
      }
    }
  },
  "filters": [
    {
      "type": "mapping",
      "config": {
        "mappings": [
          {
            "source": "id",
            "target": "externalId"
          }
        ]
      }
    }
  ],
  "output": {
    "type": "http-request",
    "config": {
      "endpoint": "https://api.destination.com/import",
      "method": "POST"
    },
    "authentication": {
      "type": "api-key",
      "credentials": {
        "key": "${DEST_API_KEY}",
        "location": "header",
        "headerName": "X-API-Key"
      }
    }
  },
  "schedule": "*/5 * * * *",
  "dryRunOptions": {
    "showCredentials": false
  },
  "errorHandling": {
    "retryCount": 3,
    "retryDelay": 5000,
    "onError": "stop"
  }
}
```

#### Supported Authentication Types

- **API Key**: Header or query parameter location
- **Bearer Token**: Authorization header with Bearer token
- **Basic Auth**: HTTP Basic Authentication (username:password)
- **OAuth2**: Client credentials flow with automatic token caching and refresh

#### Supported Pagination Types

- **Page-based**: `pageParam` with `totalPagesField`
- **Offset-based**: `offsetParam` / `limitParam` with `totalField`
- **Cursor-based**: `cursorParam` with `nextCursorField`

#### Supported Filter Modules

- **Mapping**: Field-to-field mapping with transformations (formatDate, toFloat, etc.)
- **Condition**: Filter records based on expressions (`status == 'active' && price > 0`)
- **Script**: JavaScript transformation using Goja for complex business logic
- **Enrichment**: Dynamic enrichment with HTTP requests and caching

##### Script Filter Module

The script filter module allows you to write JavaScript code to transform records. It uses the [Goja](https://github.com/dop251/goja) JavaScript engine (ECMAScript 5.1+, pure Go, no cgo).

**Configuration (inline script):**
```yaml
filters:
  - type: script
    script: |
      function transform(record) {
        // Calculate total with discount
        var discount = record.quantity >= 10 ? 0.15 : 0.05;
        record.subtotal = record.price * record.quantity;
        record.discount = record.subtotal * discount;
        record.total = record.subtotal - record.discount;
        return record;
      }
    onError: skip  # optional: fail (default), skip, log
```

**Configuration (from file):**
```yaml
filters:
  - type: script
    scriptFile: ./scripts/transform.js  # Path to JavaScript file
    onError: fail
```

**Requirements:**
- Use either `script` (inline) or `scriptFile` (file path), not both
- Script must define a `transform(record)` function
- Function receives the record as a JavaScript object
- Function must return the transformed record (or a new object)
- Function can throw exceptions for error handling

**Security:**
- Maximum script length: 100KB (prevents DoS)
- Path traversal protection for `scriptFile` paths (prevents `../` attacks)
- Goja is sandboxed (no file system, network access)
- Scripts are compiled once at initialization for performance
- Script file paths are validated before reading

##### Enrichment Filter Module

The enrichment filter module enriches records with additional data fetched from external APIs. It supports configurable caching to reduce redundant API calls and supports all authentication types.

**Configuration:**
```yaml
filters:
  - type: enrichment
    endpoint: https://api.example.com/customers/{id}
    key:
      field: customerId        # Dot-notation path to extract key from record
      paramType: path          # How to include key: query, path, or header
      paramName: id            # Parameter name (matches {id} in endpoint)
    authentication:            # Optional: same auth types as input modules
      type: api-key
      credentials:
        key: ${API_KEY}
        headerName: X-API-Key
    cache:                     # Optional: cache configuration
      maxSize: 1000            # Maximum cache entries (default: 1000)
      defaultTTL: 300          # Cache TTL in seconds (default: 300 = 5 minutes)
    mergeStrategy: merge       # Optional: merge (default), replace, append
    dataField: data            # Optional: extract data from nested field
    onError: fail              # Optional: fail (default), skip, log
    timeoutMs: 5000            # Optional: request timeout (default: 30000)
    headers:                  # Optional: custom headers
      Accept: application/json
```

**Key Configuration:**
- `field`: Dot-notation path to extract the key value from the record (e.g., `customer.id`, `order.customerId`)
- `paramType`: How to include the key in the HTTP request:
  - `query`: Adds as query parameter (e.g., `?id=value`)
  - `path`: Replaces `{paramName}` placeholder in endpoint URL
  - `header`: Adds as HTTP header
- `paramName`: The parameter name to use (must match placeholder in endpoint for `path` type)

**Cache Configuration:**
- Cache is scoped per filter module instance (not shared across instances)
- Uses LRU (Least Recently Used) eviction when size limit is reached
- Cache entries expire according to TTL configuration
- Only successful responses are cached (errors are not cached)
- **Cache Key**: Configurable via `cache.key` (optional):
  - If not specified: uses `endpoint + "::" + keyValue` (default behavior)
  - Static string: `"my-cache-key"` (all records share same cache entry)
  - JSON path: `"$.customerId"` or `"customerId"` (extracts value from record)
  - Dot notation: `"user.profile.id"` (extracts nested value from record)
  - If path not found, falls back to default behavior

**Merge Strategies:**
- `merge` (default): Deep merge - adds/updates fields recursively, preserves nested structures
- `replace`: Overwrites matching fields, preserves non-matching original fields
- `append`: Adds enrichment data under `_enrichment` key, preserves all original fields

**Security Considerations:**
- **Rate Limiting**: The enrichment module does not implement built-in rate limiting. For high-volume scenarios, consider:
  - Implementing rate limiting at the API gateway level
  - Using appropriate cache TTL to reduce request frequency
  - Configuring reasonable `timeoutMs` values to prevent hanging requests
- **Cache Size Limits**: The `maxSize` configuration prevents memory exhaustion (DoS protection). Default is 1000 entries per filter instance.
- **Authentication**: All authentication types are supported (API key, Bearer, Basic, OAuth2). Credentials are never logged.
- **Error Handling**: Failed requests are not cached. Use `onError: skip` or `onError: log` for graceful degradation.

**Example Use Cases:**
- Enrich orders with customer details from CRM API
- Add product information to order items
- Fetch geolocation data for addresses
- Lookup user preferences or settings

**See also:** `configs/examples/17-filters-enrichment.yaml` for a complete example.

##### SQL Call Filter Module

The SQL call filter module enriches records by executing SQL queries against databases. It supports PostgreSQL, MySQL, and SQLite with parameterized queries for security.

**Configuration:**
```yaml
filters:
  - type: sql_call
    connectionString: postgres://user:pass@localhost:5432/mydb
    # Or use environment variable reference:
    # connectionStringRef: ${DATABASE_URL}
    query: |
      SELECT department, manager 
      FROM user_details 
      WHERE user_id = {{record.user_id}}
    # Or load from file:
    # queryFile: ./queries/get-user-details.sql
    mergeStrategy: merge       # Optional: merge (default), replace, append
    dataField: data            # Optional: extract from nested field
    resultKey: _db_data        # Optional: key for append mode
    onError: skip              # Optional: fail (default), skip, log
    cache:                     # Optional: cache configuration
      enabled: true
      maxSize: 1000
      defaultTTL: 300
      key: "{{record.user_id}}"  # Optional: custom cache key
```

**Query Templating:**
- Use `{{record.field}}` syntax to inject record values into queries
- Nested fields supported: `{{record.customer.id}}`
- Values are automatically converted to parameterized queries (SQL injection safe)

**Multiple Queries:**
```yaml
filters:
  - type: sql_call
    connectionString: postgres://localhost/db
    queries:
      - SELECT name FROM users WHERE id = {{record.user_id}}
      - SELECT address FROM addresses WHERE user_id = {{record.user_id}}
```

**See also:** `configs/examples/29-sql-call-enrichment.yaml` for a complete example.

#### Supported Input Modules

- **HTTP Polling**: Fetch data from REST APIs with pagination support
- **Webhook**: Receive data via HTTP POST endpoints
- **Database**: Query data from PostgreSQL, MySQL, or SQLite databases

##### Database Input Module

The database input module fetches records by executing SQL queries against databases.

**Configuration:**
```yaml
input:
  type: database
  config:
    connectionString: postgres://user:pass@localhost:5432/mydb
    # Or use environment variable:
    # connectionStringRef: ${DATABASE_URL}
    query: SELECT id, name, email FROM users WHERE status = 'active'
    # Or load from file:
    # queryFile: ./queries/get-active-users.sql
    driver: postgres           # Optional: auto-detected from connection string
    timeoutMs: 30000           # Optional: query timeout (default: 30000)
    maxOpenConns: 10           # Optional: connection pool settings
    maxIdleConns: 5
```

**Incremental Queries with `{{lastRunTimestamp}}`:**

Use `{{lastRunTimestamp}}` placeholder for incremental data sync:
```yaml
input:
  type: database
  config:
    connectionString: postgres://localhost/db
    query: |
      SELECT * FROM orders 
      WHERE updated_at > {{lastRunTimestamp}}
      ORDER BY updated_at ASC
      LIMIT 1000
    incremental:
      enabled: true
      timestampField: updated_at
```

- **First run**: `{{lastRunTimestamp}}` is replaced with epoch time (1970-01-01) to fetch all records
- **Subsequent runs**: Uses the timestamp from the last successful execution
- Requires `incremental.enabled: true` to persist state between runs

**Pagination:**
```yaml
input:
  type: database
  config:
    connectionString: postgres://localhost/db
    query: SELECT * FROM large_table
    pagination:
      type: limit-offset
      limit: 1000
      offsetParam: offset
```

**See also:** `configs/examples/27-database-input.yaml` and `configs/examples/28-database-incremental.yaml`

#### Supported Output Modules

- **HTTP Request**: Send data to REST APIs
- **Database**: Write data to PostgreSQL, MySQL, or SQLite databases

##### Database Output Module

The database output module writes records to databases using SQL queries.

**Configuration:**
```yaml
output:
  type: database
  config:
    connectionString: postgres://user:pass@localhost:5432/mydb
    # Or use environment variable:
    # connectionStringRef: ${DATABASE_URL}
    query: |
      INSERT INTO orders (order_id, customer_id, total)
      VALUES ({{record.order_id}}, {{record.customer_id}}, {{record.total}})
    # Or load from file:
    # queryFile: ./queries/insert-order.sql
    transaction: true          # Optional: wrap all inserts in transaction
    onError: skip              # Optional: fail (default), skip, log
    timeoutMs: 30000
```

**Query Templating:**
- Use `{{record.field}}` to inject record values
- Nested fields: `{{record.customer.name}}`
- All values are parameterized (SQL injection safe)

**Upsert Example (PostgreSQL):**
```yaml
output:
  type: database
  config:
    connectionString: postgres://localhost/db
    query: |
      INSERT INTO products (sku, name, price)
      VALUES ({{record.sku}}, {{record.name}}, {{record.price}})
      ON CONFLICT (sku) DO UPDATE SET
        name = EXCLUDED.name,
        price = EXCLUDED.price
    transaction: true
```

**Transaction Mode:**
- When `transaction: true`, all records are processed within a single transaction
- **onError: fail** (default): On any error, entire transaction is rolled back (no partial writes)
- **onError: skip**: Individual record failures are skipped, successful records are committed together
- **onError: log**: Individual record failures are logged, successful records are committed together
- Note: With `skip` or `log`, partial commits are allowed within the transaction

**See also:** `configs/examples/30-database-output-insert.yaml`, `configs/examples/31-database-output-upsert.yaml`

#### Supported Output Modes

- **Batch mode** (default): Sends all records in a single request as JSON array
- **Single record mode**: Sends one request per record with path/query parameters from record data

**Note**: Both JSON and YAML formats are supported. The format is auto-detected based on file extension (`.json`, `.yaml`, `.yml`) or content analysis.

### Configuration Examples

The `configs/examples/` directory contains comprehensive examples covering all use cases:

#### Basic Examples
- **01-simple.json / 01-simple.yaml** - Simple pipeline without authentication
- **02-all-authentication-types.json / 02-all-authentication-types.yaml** - All authentication types (API key, Bearer, Basic, OAuth2)

#### Authentication Examples
- **03-basic-auth.json / 03-basic-auth.yaml** - Basic authentication (username/password)
- **04-oauth2.json / 04-oauth2.yaml** - OAuth2 authentication (client credentials flow)
- **11-api-key-query.json / 11-api-key-query.yaml** - API key in query parameter

#### Pagination Examples
- **05-pagination.json / 05-pagination.yaml** - Page-based pagination
- **06-pagination-offset.json / 06-pagination-offset.yaml** - Offset-based pagination
- **07-pagination-cursor.json / 07-pagination-cursor.yaml** - Cursor-based pagination

#### Filter Examples
- **08-filters-mapping.json / 08-filters-mapping.yaml** - Field mapping filter with transformations
- **09-filters-condition.json / 09-filters-condition.yaml** - Conditional filter to filter data
- **16-filters-script.yaml** - Script filter with JavaScript transformations

#### Advanced Examples
- **10-complete.json / 10-complete.yaml** - Complete example with OAuth2 authentication, pagination, multiple filters, retry, and advanced configurations
- **12-output-single-record.json / 12-output-single-record.yaml** - Single record mode with path parameters
- **13-scheduled.json / 13-scheduled.yaml** - CRON scheduled pipeline for periodic data polling

#### Database Examples
- **27-database-input.yaml** - Database input module with SQL query
- **28-database-incremental.yaml** - Incremental queries with `{{lastRunTimestamp}}`
- **29-sql-call-enrichment.yaml** - SQL call filter for record enrichment
- **30-database-output-insert.yaml** - Database output with INSERT queries
- **31-database-output-upsert.yaml** - Database output with UPSERT (PostgreSQL/MySQL/SQLite)
- **32-database-custom-query.yaml** - Database output with queryFile

#### Using Examples

```bash
# Validate an example configuration
canectors validate ./configs/examples/01-simple.json

# Run an example pipeline (dry-run)
canectors run --dry-run ./configs/examples/10-complete.yaml

# Use YAML examples (same functionality)
canectors validate ./configs/examples/05-pagination.yaml
```

See [configs/example-connector.json](configs/example-connector.json) for a basic example, or browse [configs/examples/](configs/examples/) for comprehensive use case examples.

## Error Handling and Retry Logic

Canectors provides robust error handling with automatic retry logic for transient errors.

### Error Categories

Errors are automatically classified into categories:

| Category | Description | Retryable | HTTP Status Codes |
|----------|-------------|-----------|-------------------|
| `network` | Network-related errors (timeout, connection refused, DNS) | Yes | N/A |
| `authentication` | Authentication/authorization failures | No | 401, 403 |
| `validation` | Request validation errors | No | 400, 422 |
| `rate_limit` | Rate limiting (too many requests) | Yes | 429 |
| `server` | Server-side errors | Yes | 500, 502, 503, 504 |
| `not_found` | Resource not found | No | 404 |

### Retry Configuration

Configure retry behavior per module:

```yaml
input:
  type: http-polling
  config:
    endpoint: "https://api.example.com/data"
    retry:
      maxAttempts: 3        # Maximum retry attempts (0 = no retry)
      delayMs: 1000         # Initial delay between retries (ms)
      backoffMultiplier: 2  # Exponential backoff multiplier
      maxDelayMs: 30000     # Maximum delay cap (ms)
      retryableStatusCodes: [429, 500, 502, 503, 504]  # Custom retry codes
```

**Default values:**
- `maxAttempts`: 3
- `delayMs`: 1000 (1 second)
- `backoffMultiplier`: 2.0
- `maxDelayMs`: 30000 (30 seconds)
- `retryableStatusCodes`: [429, 500, 502, 503, 504]

### Exponential Backoff

Retry delay is calculated using exponential backoff:

```
delay = min(delayMs × (backoffMultiplier ^ attempt), maxDelayMs)
```

Example with default values:
- Attempt 1: 1000ms (1s)
- Attempt 2: 2000ms (2s)
- Attempt 3: 4000ms (4s)
- Attempt 4: 8000ms (8s) → capped at maxDelayMs if exceeded

### Error Handling Strategies

Configure how errors are handled with `onError`:

```yaml
output:
  type: http-request
  config:
    endpoint: "https://api.example.com/data"
    method: POST
    onError: "skip"  # "fail" | "skip" | "log"
```

| Strategy | Behavior |
|----------|----------|
| `fail` | Stop execution immediately, return error (default) |
| `skip` | Skip failed record, continue with next records |
| `log` | Log error, continue execution |

### Timeout Configuration

Configure request timeouts:

```yaml
input:
  type: http-polling
  config:
    endpoint: "https://api.example.com/data"
    timeout: 30  # Timeout in seconds (default: 30)
```

Timeout errors are classified as `network` errors and are retryable.

### Example: Complete Error Handling Configuration

```yaml
name: "robust-data-sync"
description: "Pipeline with comprehensive error handling"

input:
  type: http-polling
  config:
    endpoint: "https://api.source.com/data"
    timeout: 60
    retry:
      maxAttempts: 5
      delayMs: 2000
      backoffMultiplier: 2
      maxDelayMs: 60000

output:
  type: http-request
  config:
    endpoint: "https://api.destination.com/data"
    method: POST
    timeout: 30
    onError: "skip"
    retry:
      maxAttempts: 3
      delayMs: 1000
      backoffMultiplier: 2
      maxDelayMs: 30000
```

### Troubleshooting Common Errors

| Error | Cause | Solution |
|-------|-------|----------|
| `authentication error (status 401)` | Invalid or expired credentials | Check API key, token, or OAuth2 configuration |
| `validation error (status 400)` | Invalid request format | Verify endpoint URL and request body format |
| `server error (status 503)` | Service unavailable | Will auto-retry; check target API status |
| `network error: timeout` | Request timeout | Increase timeout value or check network |
| `rate_limit error (status 429)` | Too many requests | Will auto-retry with backoff; reduce request frequency |

## Development

### Building

```bash
# Build for current platform
go build -o canectors ./cmd/canectors

# Build with version information
go build -ldflags "-X main.version=1.0.0 -X main.commit=$(git rev-parse --short HEAD) -X main.buildDate=$(date -u +"%Y-%m-%dT%H:%M:%SZ")" -o canectors ./cmd/canectors

# Build for specific platform
GOOS=linux GOARCH=amd64 go build -o canectors-linux-amd64 ./cmd/canectors
GOOS=darwin GOARCH=arm64 go build -o canectors-darwin-arm64 ./cmd/canectors
GOOS=windows GOARCH=amd64 go build -o canectors-windows-amd64.exe ./cmd/canectors
```

### Testing

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run tests with race detector
go test -race ./...

# Run tests with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run specific package tests
go test ./cmd/canectors/...
go test ./internal/runtime/...
```

**Test Coverage**: 100 tests across all packages (15 CLI tests, 12 runtime tests, 11 converter tests, + parser/validator tests)

### Code Quality

```bash
# Format code
go fmt ./...

# Run go vet
go vet ./...

# Run linter (requires golangci-lint v2.7.1+)
golangci-lint run ./...

# Verify configuration
golangci-lint config verify
```

### Dependencies

```bash
# Download dependencies
go mod download

# Tidy dependencies
go mod tidy

# Update specific dependency
go get -u github.com/spf13/cobra

# Update all dependencies
go get -u ./...
```

## Cross-Platform Compilation

The CLI is designed to be portable and can be compiled for multiple platforms:

| Platform | Architecture | Binary Name                    |
|----------|--------------|--------------------------------|
| Linux    | amd64        | `canectors-linux-amd64`        |
| macOS    | amd64        | `canectors-darwin-amd64`       |
| macOS    | arm64        | `canectors-darwin-arm64`       |
| Windows  | amd64        | `canectors-windows-amd64.exe`  |

Build all platforms:

```bash
# Linux
GOOS=linux GOARCH=amd64 go build -o dist/canectors-linux-amd64 ./cmd/canectors

# macOS Intel
GOOS=darwin GOARCH=amd64 go build -o dist/canectors-darwin-amd64 ./cmd/canectors

# macOS Apple Silicon
GOOS=darwin GOARCH=arm64 go build -o dist/canectors-darwin-arm64 ./cmd/canectors

# Windows
GOOS=windows GOARCH=amd64 go build -o dist/canectors-windows-amd64.exe ./cmd/canectors
```

Binaries are output to the `dist/` directory (automatically created).

## CI/CD

The project includes a GitHub Actions workflow (`.github/workflows/ci.yml`) that:

1. **Lint**: Runs `golangci-lint v2.7.1` for code quality checks
2. **Test**: Runs all 100+ tests with race detection and coverage reporting
3. **Build**: Creates cross-platform binaries for all supported platforms (Linux, macOS Intel/ARM, Windows)
4. **Verify**: Tests the built binary with `version` and `--help` commands

The workflow runs on:
- Push to `main` and `develop` branches
- Pull requests targeting `main` and `develop`

## Architecture

Canectors Runtime follows a modular architecture:

### Pipeline Execution Flow

1. **Input Module**: Fetches data from source systems
   - `Input.Fetch()` → `[]map[string]interface{}`
   - Handles errors gracefully, stops execution on failure

2. **Filter Modules** (optional, executed in sequence):
   - Each filter processes records from previous stage
   - `Filter.Process([]map[string]interface{})` → transformed records
   - Stops execution on any filter error

3. **Output Module**: Sends data to destination systems
   - `Output.Send([]map[string]interface{})` → number of records sent
   - Skipped in dry-run mode
   - Handles partial failures

### Execution Result

Each pipeline execution returns an `ExecutionResult`:
- `Status`: `success`, `error`, or `partial`
- `StartedAt` / `CompletedAt`: Timestamps
- `RecordsProcessed` / `RecordsFailed`: Counts
- `Error`: Detailed error information (module, code, message)

### Resource Management

- Modules are automatically closed after execution (success or failure)
- Input and Output modules implement `Close()` for cleanup
- Deferred cleanup ensures no resource leaks

### Deterministic Execution

- Same pipeline configuration + same input data = same output
- Fixed execution order: Input → Filters (in order) → Output
- No random behavior or time-dependent logic (except timestamps)

For detailed architecture documentation, see the Architecture Document in the `canectors` planning repository (`_bmad-output/planning-artifacts/architecture.md`).

## Extending Canectors

Canectors is designed to be extensible - you can add your own custom modules without modifying the core codebase.

### Module Architecture

The runtime uses a **module registry pattern** that enables clean extensibility:

1. **Implement the interface** for your module type (`input.Module`, `filter.Module`, or `output.Module`)
2. **Register your constructor** via `registry.RegisterInput()`, `RegisterFilter()`, or `RegisterOutput()`
3. **Use your module type** in pipeline configurations - the runtime instantiates it automatically

**No core modifications needed** - no edits to factory code, no changes to main.go, complete independence.

### Module Boundaries

Understanding module boundaries is critical for correct implementation:

- **Input modules**: Fetch data from sources (APIs, databases, files, etc.)
  - ✅ Do: Fetch data, manage resources, handle authentication
  - ❌ Don't: Transform data, send data, perform business logic

- **Filter modules**: Transform and process data
  - ✅ Do: Map fields, validate, filter, aggregate
  - ❌ Don't: Fetch from external sources, send to destinations, store state between executions

- **Output modules**: Send data to destinations
  - ✅ Do: Send data, format for destination, manage connections
  - ❌ Don't: Fetch data, transform data, perform business logic

### Documentation

- **[Module Extensibility Guide](docs/MODULE_EXTENSIBILITY.md)** - Complete guide to adding custom modules
  - Registry API reference
  - Step-by-step implementation examples
  - Built-in module reference
  - Nested module support

- **[Module Boundaries Documentation](docs/MODULE_BOUNDARIES.md)** - Detailed boundary documentation
  - Module responsibilities and anti-patterns
  - Runtime boundary enforcement
  - Common pitfalls and solutions
  - Interface stability guarantees

### Interface Stability

The core module interfaces are **designed to remain stable** across versions:

- `input.Module`: `Fetch()`, `Close()` - stable, no new methods will be added
- `filter.Module`: `Process()` - stable, no new methods will be added
- `output.Module`: `Send()`, `Close()` - stable, no new methods will be added

Optional capabilities (like `output.PreviewableModule`) use **interface composition** to extend functionality without breaking existing implementations.

### Quick Example

```go
package kafka

import (
    "context"
    "github.com/canectors/runtime/internal/modules/input"
    "github.com/canectors/runtime/internal/registry"
)

// Register module at init time
func init() {
    registry.RegisterInput("kafka", NewKafkaModule)
}

// Implement the interface
type KafkaModule struct {
    brokers []string
    topic   string
}

func NewKafkaModule(cfg *connector.ModuleConfig) (input.Module, error) {
    return &KafkaModule{...}, nil
}

func (m *KafkaModule) Fetch(ctx context.Context) ([]map[string]interface{}, error) {
    // Fetch from Kafka
    return records, nil
}

func (m *KafkaModule) Close() error {
    return nil
}

// Compile-time interface check
var _ input.Module = (*KafkaModule)(nil)
```

See the [extensibility documentation](docs/MODULE_EXTENSIBILITY.md) for complete implementation guides.

## Module Status

| Module Type | Status | Story |
|-------------|--------|-------|
| **Input Modules** | ✅ **COMPLETE** | Story 3.1 (HTTP Polling), 3.2 (Webhook) |
| **Filter Modules** | ✅ **COMPLETE** | Story 3.3 (Mapping), 3.4 (Conditions) |
| **Output Modules** | ✅ **COMPLETE** | Story 3.5 (HTTP Request) |
| **Authentication** | ✅ **COMPLETE** | Story 3.6 (Authentication Handling) |
| **Scheduler** | ✅ **COMPLETE** | Story 4.1 (CRON) |

**Current Implementation**: 
- ✅ Pipeline orchestration engine (Epic 2)
- ✅ HTTP Polling Input module with pagination support (Story 3.1)
- ✅ Webhook Input module (Story 3.2)
- ✅ Mapping Filter module with transformations (Story 3.3)
- ✅ Condition Filter module with expression evaluation (Story 3.4)
- ✅ HTTP Request Output module with retry logic (Story 3.5)
- ✅ Authentication handling: API key, Bearer, Basic, OAuth2 (Story 3.6)

## Roadmap

### Epic 2: CLI Runtime Foundation ✅ **COMPLETE**

- [x] Project structure initialization (Story 2.1)
- [x] Configuration parser with JSON/YAML support (Story 2.2)
- [x] Pipeline orchestration engine (Story 2.3)

### Epic 3: Module Execution ✅ **COMPLETE**

- [x] HTTP polling input module with pagination (Story 3.1)
- [x] Webhook input module (Story 3.2)
- [x] Mapping filter module with transformations (Story 3.3)
- [x] Condition filter module with expression evaluation (Story 3.4)
- [x] HTTP request output module with retry logic (Story 3.5)
- [x] Authentication handling: API key, Bearer, Basic, OAuth2 (Story 3.6)

### Epic 4: Advanced Runtime Features 🚧 **IN PROGRESS**

- [x] CRON scheduler (Story 4.1)
- [x] Enhanced dry-run mode with request preview (Story 4.2)
- [x] Execution logging with structured output and metrics (Story 4.3)
- [ ] Error handling and retry logic (Story 4.4)
- [ ] CLI commands interface enhancements (Story 4.5)
- [ ] Cross-platform CLI support verification (Story 4.6)

## Testing

The project includes comprehensive test coverage:

- **100 tests** across all packages
- **15 CLI integration tests** (help, validate, run, version, dry-run)
- **12 runtime/executor tests** (success, errors, filters, resource cleanup)
- **11 converter tests** (config to pipeline conversion)
- **Parser/Validator tests** (JSON/YAML parsing, schema validation)

All tests pass with race detection enabled.

## License

[MIT License](LICENSE)

## Contributing

Contributions are welcome! Please read the [Contributing Guide](CONTRIBUTING.md) before submitting a pull request.

## Related Projects

- **Canectors Web App**: Next.js application for managing connectors (separate project)
- **Pipeline Schema**: JSON Schema for pipeline configurations (`internal/config/schema/pipeline-schema.json`)
- **BMAD Planning**: Project planning artifacts in `canectors-BMAD/_bmad-output/`
