# CLI Reference

## Build

```bash
go build -o cannectors ./cmd/cannectors
```

The Makefile builds into `bin/cannectors`:

```bash
make build
```

## Commands

### `validate`

Parses and validates a pipeline file.

```bash
./cannectors validate ./examples/01-http-polling-basic-to-http-batch.yaml
./cannectors validate --verbose ./examples/10-mapping-transforms-all.yaml
./cannectors validate --quiet ./examples/22-defaults-inheritance.yaml
```

### `run`

Validates and runs a pipeline.

```bash
./cannectors run ./examples/01-http-polling-basic-to-http-batch.yaml
./cannectors run --dry-run ./examples/19-http-output-single-template.yaml
./cannectors run --verbose ./examples/20-http-output-retry-auth-api-key.yaml
```

When the input has a `schedule`, `run` starts the scheduler. Without a
schedule, the pipeline runs once.

### `version`

```bash
./cannectors version
```

## Flags

| Flag | Command | Purpose |
| --- | --- | --- |
| `--verbose` | all | Human-readable debug output. |
| `--quiet` | all | Suppress success output. |
| `--log-file <path>` | all | Write logs to a file. |
| `--dry-run` | `run` | Validate and preview without executing output side effects. |

## Exit Codes

| Code | Meaning |
| --- | --- |
| `0` | Success |
| `1` | Validation errors |
| `2` | Parse errors |
| `3` | Runtime errors |

## Maintainer Commands

```bash
make validate-examples
make test
make test-coverage
make test-race
make lint
```
