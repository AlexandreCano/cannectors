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

**Next**: Epic 3 - Module Execution (Input, Filter, Output implementations)

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
│   ├── modules/            # Module interfaces (implementations in Epic 3)
│   │   ├── input/          # Input module interface
│   │   ├── filter/         # Filter module interface
│   │   └── output/         # Output module interface
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
```

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Validation errors (schema violations) |
| 2 | Parse errors (invalid JSON/YAML syntax) |
| 3 | Runtime errors (execution failures) |

### Pipeline Configuration Format

The configuration file must follow this JSON Schema structure:

```json
{
  "schemaVersion": "1.1.0",
  "connector": {
    "name": "example-pipeline",
    "version": "1.0.0",
    "description": "An example connector pipeline",
    "input": {
      "type": "httpPolling",
      "endpoint": "https://api.example.com/data",
      "schedule": "*/5 * * * *",
      "method": "GET",
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
        "mappings": [
          {
            "source": "id",
            "target": "externalId"
          }
        ]
      }
    ],
    "output": {
      "type": "httpRequest",
      "endpoint": "https://api.destination.com/import",
      "method": "POST",
      "authentication": {
        "type": "apiKey",
        "credentials": {
          "key": "${DEST_API_KEY}",
          "header": "X-API-Key"
        }
      }
    },
    "errorHandling": {
      "retryCount": 3,
      "retryDelay": 5000,
      "onError": "stop"
    }
  }
}
```

**Note**: Both JSON and YAML formats are supported. The format is auto-detected based on file extension (`.json`, `.yaml`, `.yml`) or content analysis.

See [configs/example-connector.json](configs/example-connector.json) for a complete example.

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
| **Input Modules** | 🔜 Coming in Epic 3 | Story 3.1 (HTTP Polling), 3.2 (Webhook) |
| **Filter Modules** | 🔜 Coming in Epic 3 | Story 3.3 (Mapping), 3.4 (Conditions) |
| **Output Modules** | 🔜 Coming in Epic 3 | Story 3.5 (HTTP Request) |
| **Scheduler** | 🔜 Coming in Epic 4 | Story 4.1 (CRON) |

**Current Implementation**: Pipeline orchestration engine with stub modules for testing. Real module implementations will be added in Epic 3.

## Roadmap

### Epic 2: CLI Runtime Foundation ✅ **COMPLETE**

- [x] Project structure initialization (Story 2.1)
- [x] Configuration parser with JSON/YAML support (Story 2.2)
- [x] Pipeline orchestration engine (Story 2.3)

### Epic 3: Module Execution 🔜 **NEXT**

- [ ] HTTP polling input module (Story 3.1)
- [ ] Webhook input module (Story 3.2)
- [ ] Mapping filter module (Story 3.3)
- [ ] Condition filter module (Story 3.4)
- [ ] HTTP request output module (Story 3.5)
- [ ] Authentication handling (Story 3.6)

### Epic 4: Advanced Runtime Features 📋 **PLANNED**

- [ ] CRON scheduler (Story 4.1)
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
