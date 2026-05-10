# Cannectors

Cannectors is a cross-platform CLI for running declarative data pipelines. It
reads YAML pipeline files and moves records through an `input -> filters ->
output` flow.

## What It Does

- Pull records from HTTP APIs, webhooks, PostgreSQL, MySQL, or SQLite.
- Transform records with mapping, condition, script, set, remove, HTTP call, and
  SQL call filters.
- Send records to HTTP APIs or databases.
- Handle authentication, retries, scheduling, state persistence, and dry-run
  previews from configuration.

## Quick Start

```bash
# Build from source
go build -o cannectors ./cmd/cannectors

# Validate a maintained example
./cannectors validate ./examples/01-http-polling-basic-to-http-batch.yaml

# Preview a pipeline without sending output data
./cannectors run --dry-run ./examples/01-http-polling-basic-to-http-batch.yaml

# Run a pipeline
./cannectors run ./examples/01-http-polling-basic-to-http-batch.yaml
```

## Minimal Pipeline

```yaml
name: sync-orders
version: 1.0.0
description: Poll orders from an API and send them as a batch.

input:
  type: httpPolling
  schedule: "*/15 * * * *"
  endpoint: https://source.example.com/api/orders
  dataField: orders

filters:
  - type: mapping
    mappings:
      - source: order_id
        target: id
      - source: customer.email
        target: email

output:
  type: httpRequest
  endpoint: https://destination.example.com/api/orders/import
  method: POST
  requestMode: batch
```

## Documentation

- [Documentation index](docs/README.md)
- [Configuration format](docs/CONFIGURATION.md)
- [Module reference](docs/MODULES.md)
- [CLI reference](docs/CLI.md)
- [Operations guide](docs/OPERATIONS.md)
- [Maintained examples](examples/README.md)
- [Module extensibility](docs/MODULE_EXTENSIBILITY.md)
- [Architecture](docs/ARCHITECTURE.md)

## Examples

The canonical examples live in [examples/](examples). They are validated by the
test suite and should be used as templates for new pipelines.

```bash
./cannectors validate ./examples/10-mapping-transforms-all.yaml
./cannectors validate ./examples/22-defaults-inheritance.yaml
```

## Development

```bash
make build
make test
make validate-examples
make lint
```

Useful test-lab commands:

```bash
make test-lab-up
make test-lab-run
make test-lab-down
```

## Requirements

- Go 1.25.0+
- Docker, for the local test lab
- `golangci-lint` v2.7.1+, for linting

## License

[Apache License 2.0](LICENSE)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## Commercial Support

See [COMMERCIAL.md](COMMERCIAL.md).
