# Canectors Runtime

Portable runtime CLI for executing connector pipelines. Canectors is a cross-platform tool that reads pipeline configurations (JSON/YAML) and executes Input, Filter, and Output modules to transfer data between systems.

## Features

- **Pipeline Execution**: Execute data pipelines defined in JSON/YAML configuration files
- **Modular Architecture**: Input, Filter, and Output modules for flexible data processing
- **CRON Scheduling**: Built-in scheduler for periodic pipeline execution
- **Cross-Platform**: Runs on Windows, macOS (Intel & Apple Silicon), and Linux
- **Configuration Validation**: Validate pipeline configurations before execution

## Project Structure

```
canectors-runtime/
├── cmd/
│   └── canectors/          # CLI entry point
│       └── main.go
├── internal/               # Private packages
│   ├── config/             # Configuration parsing and validation
│   ├── logger/             # Structured logging
│   ├── modules/
│   │   ├── input/          # Input modules (HTTP polling, webhook, etc.)
│   │   ├── filter/         # Filter modules (mapping, conditions, etc.)
│   │   └── output/         # Output modules (HTTP request, etc.)
│   ├── runtime/            # Pipeline execution engine
│   └── scheduler/          # CRON scheduling
├── pkg/
│   └── connector/          # Public types and interfaces
├── configs/                # Example configuration files
├── .github/
│   └── workflows/
│       └── ci.yml          # GitHub Actions CI/CD
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

## Requirements

- **Go**: 1.21 or later
- **Make**: For build automation (optional, but recommended)

## Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/canectors/runtime.git
cd canectors-runtime

# Build the binary
make build

# Or install to GOPATH/bin
make install
```

### Pre-built Binaries

Download the latest release for your platform from the [Releases](https://github.com/canectors/runtime/releases) page.

## Usage

### Basic Commands

```bash
# Display help
canectors --help

# Display version
canectors version

# Validate a pipeline configuration
canectors validate ./configs/my-pipeline.json

# Execute a pipeline
canectors run ./configs/my-pipeline.json

# Execute with dry-run (validate only)
canectors run ./configs/my-pipeline.json --dry-run

# List available modules
canectors list
```

### Example Pipeline Configuration

See [configs/example-connector.json](configs/example-connector.json) for a complete example.

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
      "method": "GET"
    }
  },
  "filters": [
    {
      "type": "mapping",
      "config": {
        "mappings": {
          "sourceField": "targetField"
        }
      }
    }
  ],
  "output": {
    "type": "http-request",
    "config": {
      "endpoint": "https://api.destination.com/import",
      "method": "POST"
    }
  },
  "schedule": "*/5 * * * *"
}
```

## Development

### Building

```bash
# Build for current platform
make build

# Build for all platforms (Linux, macOS, Windows)
make build-all

# Clean build artifacts
make clean
```

### Testing

```bash
# Run all tests
make test

# Run tests with coverage report
make test-coverage

# Run tests with race detector
make test-race
```

### Code Quality

```bash
# Format code
make fmt

# Check code formatting
make fmt-check

# Run go vet
make vet

# Run all quality checks
make check

# Run linter (requires golangci-lint)
make lint
```

### Dependencies

```bash
# Download dependencies
make deps

# Tidy dependencies
make deps-tidy

# Update all dependencies
make deps-update
```

## Cross-Platform Compilation

The CLI is designed to be portable and can be compiled for multiple platforms:

| Platform         | Architecture | Binary Name                    |
|------------------|--------------|--------------------------------|
| Linux            | amd64        | `canectors-linux-amd64`        |
| macOS            | amd64        | `canectors-darwin-amd64`       |
| macOS            | arm64        | `canectors-darwin-arm64`       |
| Windows          | amd64        | `canectors-windows-amd64.exe`  |

Build all platforms with:

```bash
make build-all
```

Binaries are output to the `dist/` directory.

## CI/CD

The project includes a GitHub Actions workflow (`.github/workflows/ci.yml`) that:

1. **Lint**: Runs `golangci-lint` for code quality
2. **Test**: Runs all tests with race detection and coverage
3. **Build**: Creates cross-platform binaries for all supported platforms
4. **Verify**: Tests the built binary

## Architecture

Canectors is designed with a modular architecture:

- **Input Modules**: Fetch data from source systems (HTTP polling, webhooks)
- **Filter Modules**: Transform and filter data (mapping, conditions)
- **Output Modules**: Send data to destination systems (HTTP requests)
- **Scheduler**: CRON-based scheduling for periodic execution
- **Runtime**: Pipeline orchestration and execution engine

For detailed architecture documentation, see the [Architecture Document](../_bmad-output/planning-artifacts/architecture.md).

## Related Projects

- **Canectors Web App**: Next.js application for managing connectors (separate project)
- **Pipeline Schema**: JSON Schema for pipeline configurations

## Roadmap

- [x] Project structure initialization (Story 2.1)
- [ ] Configuration parser (Story 2.2)
- [ ] Pipeline orchestration (Story 2.3)
- [ ] HTTP polling input module (Story 3.1)
- [ ] Webhook input module (Story 3.2)
- [ ] Mapping filter module (Story 3.3)
- [ ] Condition filter module (Story 3.4)
- [ ] HTTP request output module (Story 3.5)
- [ ] CRON scheduler (Story 4.1)
- [ ] Dry-run mode (Story 4.2)
- [ ] Execution logging (Story 4.3)
- [ ] Error handling and retry (Story 4.4)

## License

[MIT License](LICENSE)

## Contributing

Contributions are welcome! Please read the [Contributing Guide](CONTRIBUTING.md) before submitting a pull request.
