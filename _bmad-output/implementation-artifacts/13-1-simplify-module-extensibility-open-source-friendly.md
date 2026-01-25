# Story 13.1: Simplify Module Extensibility (Open-Source Friendly)

Status: done

<!-- Note: Validation is optional. Run validate-create-story for quality check before dev-story. -->

## Story

As a developer,
I want to add new input, filter, or output module types without modifying the runtime core,
so that I can contribute or use custom modules (e.g. community plugins) without forking the codebase.

## Acceptance Criteria

1. **Given** I want to add a new input module type (e.g. `sql`, `kafka`)
   **When** I implement the `input.Module` interface and register it
   **Then** The runtime can instantiate my module from config using its type string
   **And** I do **not** need to edit `internal/factory/modules.go` or `cmd/canectors/main.go`
   **And** Unknown types still resolve to stubs (or a configurable fallback) with clear behaviour

2. **Given** I want to add a new filter module type (e.g. `transform`, `enrich`)
   **When** I implement the `filter.Module` interface and register it
   **Then** The runtime can instantiate my module from config using its type string
   **And** I do **not** need to edit the factory or main
   **And** Nested filter config (e.g. condition `then`/`else`) uses the same registry for nested types

3. **Given** I want to add a new output module type (e.g. `webhook`, `kafka`)
   **When** I implement the `output.Module` interface and register it
   **Then** The runtime can instantiate my module from config using its type string
   **And** I do **not** need to edit the factory or main

4. **Given** the runtime starts (validate or run)
   **When** module creation runs
   **Then** Built-in modules (`httpPolling`, `webhook`, `mapping`, `condition`, `httpRequest`) are available by default
   **And** All existing tests and example configs continue to pass unchanged
   **And** No regressions in pipeline execution, dry-run, or scheduling

5. **Given** a contributor reads the codebase
   **When** they look for “how to add a module”
   **Then** There is a clear, minimal path: implement interface → register type → use in config
   **And** The registry/registration API is documented (README and/or godoc)

## Tasks / Subtasks

- [x] Task 1: Define Module Registry Abstractions (AC: #1–#3, #5)
  - [x] Add `internal/registry` (or equivalent) with registries for input, filter, output
  - [x] Define `RegisterInput(type string, constructor func(*connector.ModuleConfig) input.Module)`
  - [x] Define `RegisterFilter(type string, constructor func(connector.ModuleConfig, int) (filter.Module, error))`
  - [x] Define `RegisterOutput(type string, constructor func(*connector.ModuleConfig) (output.Module, error))`
  - [x] Document registration contract (config shape, nil handling, stubs) in godoc

- [x] Task 2: Wire Built-Ins via Registry (AC: #4)
  - [x] Register `httpPolling`, `webhook` (and stubs for unknown) in input registry
  - [x] Register `mapping`, `condition` (and stubs for unknown) in filter registry
  - [x] Register `httpRequest` (and stubs for unknown) in output registry
  - [x] Ensure registration happens at init (e.g. `internal/modules` or `internal/factory` init) so CLI always has built-ins

- [x] Task 3: Refactor Factory to Use Registry (AC: #1–#4)
  - [x] Replace `CreateInputModule` switch with registry lookup; call registered constructor or stub
  - [x] Replace `CreateFilterModules` / `createSingleFilterModule` switch with registry lookup
  - [x] Replace `CreateOutputModule` switch with registry lookup
  - [x] Keep parsing helpers (e.g. `ParseConditionConfig`, `ParseFieldMappings`) where they are; factory or modules own config parsing
  - [x] Preserve nested filter handling (condition `then`/`else`) using the same filter registry

- [x] Task 4: Preserve Stub Behaviour for Unknown Types (AC: #1, #4)
  - [x] Unknown input types → `input.NewStub(type, endpoint)` as today
  - [x] Unknown filter types → `filter.NewStub(type, index)` as today
  - [x] Unknown output types → `output.NewStub(type, endpoint, method)` as today
  - [x] Ensure stub behaviour is documented and tested

- [x] Task 5: Update Tests and Examples (AC: #4, #5)
  - [x] All existing unit tests (factory, runtime, integration) pass
  - [x] Add tests: registry registration, lookup, unknown type → stub
  - [x] Add test: custom module registered in test, used in minimal pipeline
  - [x] Example configs under `configs/examples/` unchanged and valid

- [x] Task 6: Document Extension Point (AC: #5)
  - [x] Add "Adding a new module" (or "Module extensibility") section to README or `docs/`
  - [x] Document: implement interface → register → use type in config; link to registry godoc
  - [x] Run `golangci-lint run` and fix any issues

## Dev Notes

### Context and Rationale

**Why a registry?**

- **Open-source friendly**: Contributors can add modules (e.g. `sql` input, `kafka` output) without touching `factory` or `main`. The core stays stable; extensions are additive.
- **Plugin-like**: Registration is explicit (e.g. `registry.RegisterInput("sql", sql.NewFromConfig)`). No plugin loading from disk or RPC in this story; focus is on **in-process** extensibility.
- **Single place to look**: “How do I add a module?” → implement interface + register. No scattered `switch` edits.

**Current behaviour:**

- `internal/factory/modules.go`: `CreateInputModule`, `CreateFilterModules`, `CreateOutputModule` use `switch cfg.Type` to pick implementations. Adding a type = editing the factory.
- `cmd/canectors/main.go`: Calls `factory.CreateInputModule`, `factory.CreateFilterModules`, `factory.CreateOutputModule` (e.g. around lines 246, 252, 363, 364, 377). No registry.
- Modules live in `internal/modules/input`, `internal/modules/filter`, `internal/modules/output`. Interfaces: `input.Module`, `filter.Module`, `output.Module`.

**Target behaviour:**

- **Registry**: Central place to register constructors by type string. Built-ins register at init.
- **Factory**: Uses registry only. No `switch` on type; lookup → call constructor or stub.
- **Stubs**: Unregistered types still get stubs (current behaviour preserved).
- **Nested filters**: Condition `then`/`else` (and any future nested filters) resolve types via the same filter registry.

### Architecture Compliance

- **Go runtime**: This is the `canectors-runtime` CLI. No Next.js/tRPC; all work is in Go.
- **Layout**: Keep `cmd/`, `internal/`, `pkg/` structure. Prefer `internal/registry` or `internal/factory` for registry; avoid new top-level packages unless justified.
- **Interfaces**: Keep `input.Module`, `filter.Module`, `output.Module` unchanged. Registry returns these types.
- **Config**: Continue using `connector.ModuleConfig` (`Type`, `Config`, `Authentication`). No schema changes required for this story.

### Technical Requirements

- **Go version**: Use project’s current Go version (see `go.mod`).
- **Concurrency**: Registration typically happens at init, single-threaded. Lookups can be read-only; no need for complex locking in this story.
- **Errors**: Filter constructors already return `error`; keep that. Input/output constructors that can fail should follow the same pattern where appropriate.
- **Linting**: Run `golangci-lint run` before marking done.

### Project Structure Notes

- **Reuse**: Keep `filter.ParseFieldMappings`, `ParseConditionConfig`, and nested config parsing. Registry handles “who builds the module”; parsing stays in factory or modules.
- **Dependencies**: Factory currently imports `input`, `filter`, `output`, `connector`. Registry will need to reference these. Avoid circular imports (e.g. modules importing registry is fine; registry importing modules is fine; modules ─×→ factory if factory drives init).
- **Nested modules**: `createSingleFilterModule` and `parseNestedConfig` handle nested filters. Use the same filter registry when instantiating nested modules so that custom filter types work in `then`/`else` as well.

### File Structure Requirements

- **New**: `internal/registry/` (or `internal/factory/registry.go`) with registration + lookup.
- **Modify**: `internal/factory/modules.go` – replace switch-based creation with registry lookups.
- **Modify**: Initialisation of built-ins (e.g. in `factory` or dedicated `internal/modules` init) to register default types.
- **Touch**: `cmd/canectors/main.go` only if you need to pass a registry or options; otherwise keep using `factory.Create*` as the public API.
- **Tests**: `internal/registry/*_test.go` and/or `internal/factory/*_test.go` for registry and creation behaviour.

### Testing Requirements

- **Unit**: Registry register + lookup; unknown type → stub; custom module registered in test, pipeline runs.
- **Integration**: Existing pipeline tests (e.g. `configs/examples`) still pass. Dry-run, scheduler, validate unchanged.
- **No regressions**: Full test suite green, including `internal/errhandling`, `internal/runtime`, etc.

### References

- [Source: _bmad-output/planning-artifacts/sprint-change-proposal-2026-01-24.md] – Epic 13, story 13.1 (simplify module extensibility, plugin-like registry).
- [Source: internal/factory/modules.go] – Current `CreateInputModule`, `CreateFilterModules`, `CreateOutputModule` and switch logic.
- [Source: internal/modules/input/input.go, filter/filter.go, output/output.go] – Module interfaces.
- [Source: pkg/connector/types.go] – `ModuleConfig`, `Pipeline`.
- [Source: _bmad-output/planning-artifacts/architecture.md] – CLI runtime layout (`internal/modules`, `internal/config`, etc.).

### Project Context Reference

- **Runtime CLI**: Go, latest stable. Lint with `golangci-lint run`. Use `internal/` for non-public code, `pkg/connector` for public types.
- **Library usage**: Prefer standard library + existing deps. No new external deps required for a simple in-memory registry.

## Dev Agent Record

### Agent Model Used

Claude Sonnet 4 (claude-sonnet-4-20250514)

### Debug Log References

N/A

### Completion Notes List

- Created `internal/registry/registry.go` with thread-safe module registries for input, filter, and output modules
- Created `internal/registry/builtins.go` to register built-in modules (httpPolling, webhook, mapping, condition, httpRequest) at init time
- Refactored `internal/factory/modules.go` to use registry lookups instead of switch statements (replaced switch-based creation with registry pattern)
- Modified `internal/modules/filter/condition.go` to support registry-based nested module creation via `NestedModuleCreator` function variable (enables custom filter types in condition then/else blocks)
- Unknown module types continue to resolve to stub implementations
- Added comprehensive tests in `internal/registry/registry_test.go` and `internal/factory/modules_test.go`
- Created `docs/MODULE_EXTENSIBILITY.md` with complete examples for adding custom modules (input, filter, output)
- Validated all 28 example configs in `configs/examples/` remain valid and unchanged
- All 13 test packages pass (including runtime, scheduler, factory, registry, errhandling, etc.)
- golangci-lint reports 0 issues after fixing gofmt and staticcheck warnings

### File List

- internal/registry/registry.go (new)
- internal/registry/registry_test.go (new)
- internal/registry/builtins.go (new)
- internal/factory/modules.go (modified)
- internal/factory/modules_test.go (new)
- internal/modules/filter/condition.go (modified - added NestedModuleCreator support)
- docs/MODULE_EXTENSIBILITY.md (new)
- _bmad-output/implementation-artifacts/sprint-status.yaml (modified - story status updated)
