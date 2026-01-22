# Canectors Runtime

Portable runtime CLI for executing connector pipelines. Canectors is a cross-platform tool that reads pipeline configurations (JSON/YAML) and executes Input, Filter, and Output modules to transfer data between systems.

## Features

- **Pipeline Execution**: Execute data pipelines defined in JSON/YAML configuration files
- **Configuration Validation**: Validate pipeline configurations against JSON Schema before execution
- **Modular Architecture**: Input, Filter, and Output modules for flexible data processing (Epic 3)
- **Cross-Platform**: Runs on Windows, macOS (Intel & Apple Silicon), and Linux
- **Dry-Run Mode**: Validate and test pipelines without executing output modules
- **Structured Logging**: JSON-formatted logs with configurable verbosity levels
- **Resource Cleanup**: Automatic cleanup of module resources (connections, file handles)

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
│   ├── logger/             # Structured JSON logging (slog)
│   ├── modules/            # Module implementations
│   │   ├── input/          # Input modules (HTTP Polling, Webhook)
│   │   ├── filter/         # Filter modules (Mapping, Condition)
│   │   └── output/         # Output modules (HTTP Request)
│   ├── runtime/            # Pipeline execution engine
│   │   ├── pipeline.go     # Executor with Input → Filter → Output orchestration
│   │   └── pipeline_test.go # Executor tests (12 tests)
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

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Validation errors (schema violations) |
| 2 | Parse errors (invalid JSON/YAML syntax) |
| 3 | Runtime errors (execution failures) |

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

#### Advanced Examples
- **10-complete.json / 10-complete.yaml** - Complete example with OAuth2 authentication, pagination, multiple filters, retry, and advanced configurations
- **12-output-single-record.json / 12-output-single-record.yaml** - Single record mode with path parameters
- **13-scheduled.json / 13-scheduled.yaml** - CRON scheduled pipeline for periodic data polling

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
- [ ] Enhanced dry-run mode (Story 4.2)
- [ ] Execution logging improvements (Story 4.3)
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
