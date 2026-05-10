# Operations Guide

## Validating Examples

Maintained examples live in `examples/` and should validate before release.

```bash
make validate-examples
go test ./internal/config -run TestExampleConfigsAreSchemaCompliant -count=1
```

## Dry Runs

Use dry-run mode before pointing a pipeline at production destinations.

```bash
./cannectors run --dry-run ./examples/20-http-output-retry-auth-api-key.yaml
```

Dry-run validates the pipeline and prepares output previews without executing
output side effects.

## Scheduling

When an input module defines `schedule`, `cannectors run` keeps the process
alive until interrupted.

```yaml
input:
  type: httpPolling
  schedule: "*/15 * * * *"
```

Use a process manager or container runtime to keep scheduled pipelines running
in production.

## State Files

State persistence stores resume information for incremental polling. Keep state
storage durable if the process or container can be replaced.

Examples:

- [offset pagination with state](../examples/03-http-polling-offset-pagination-state.yaml)
- [database cursor incremental](../examples/09-database-input-cursor-incremental.yaml)

## Secrets

Use environment variable references for credentials:

```yaml
authentication:
  type: bearer
  credentials:
    token: ${SOURCE_BEARER_TOKEN}
```

Avoid committing concrete tokens, passwords, or database URLs.

## Local Test Lab

The local test lab starts PostgreSQL and WireMock for deterministic E2E checks.

```bash
make test-lab-up
make test-lab-run
make test-lab-down
```

See [test-lab/README.md](../test-lab/README.md) for scenario details.

## Cross-Platform Builds

```bash
make build-linux
make build-darwin
make build-windows
make build-all
```
