# Story 14.9: Filter Module – Set Field to Literal Value

Status: review

<!-- Note: Validation is optional. Run validate-create-story for quality check before dev-story. -->

## Story

As a developer,
I want a filter module that sets a field to a literal value on each record (create the field if it does not exist, update it if it exists),
so that I can set fixed values, defaults, or constants without complex mapping configurations.

## Acceptance Criteria

1. **Given** I have a connector with a "set" filter module configured
   **When** The runtime validates the connector configuration
   **Then** The configuration schema supports a filter type `set`
   **And** The configuration requires a target field path (e.g. `id`, `user.id`, `metadata.version`)
   **And** The configuration requires a literal value (string, number, boolean, null)
   **And** The configuration validator rejects missing target field or value

2. **Given** I have a "set" filter with target field `id` and a literal value
   **When** The runtime processes a record that already has an `id` field
   **Then** The runtime overwrites the existing `id` with the configured value
   **And** The record is passed to the next module with the updated `id`
   **And** Other record fields are unchanged

3. **Given** I have a "set" filter with target field `id` and a literal value
   **When** The runtime processes a record that does not have an `id` field
   **Then** The runtime creates the `id` field on the record with the configured value
   **And** The record is passed to the next module with the new `id`
   **And** Other record fields are unchanged

4. **Given** I have a "set" filter with a nested target path (e.g. `metadata.version`)
   **When** The runtime processes a record
   **Then** If intermediate keys (e.g. `metadata`) do not exist, the runtime creates them
   **And** The leaf field is set or updated as in AC#2 and AC#3

5. **Given** I have a "set" filter with a literal value (string, number, boolean, or null)
   **When** The runtime processes records
   **Then** Literal values are applied as-is
   **And** Type of the value is preserved

6. **Given** I have a pipeline with multiple "set" filters or a "set" filter alongside mapping/condition filters
   **When** The runtime executes the pipeline
   **Then** The "set" filter can be placed anywhere in the filter chain
   **And** It processes one record at a time and returns the same record with the field set or updated
   **And** Execution is deterministic and does not alter pipeline semantics of other modules

## Tasks / Subtasks

- [x] Task 1: Define "set" filter module configuration schema (AC: #1)
  - [x] Add filter type identifier `set` in pipeline schema
  - [x] Define `target` (field path) and `value` (literal value)
  - [x] Document configuration in schema and examples

- [x] Task 2: Implement "set" filter module (AC: #2, #3, #6)
  - [x] Create filter module implementation (`internal/modules/filter/set.go`)
  - [x] Implement Process(records) → set one field to literal value per record
  - [x] Support flat target path: if field exists → overwrite; if not → create
  - [x] Register module in filter registry so pipelines can reference it

- [x] Task 3: Support nested target paths (AC: #4)
  - [x] Parse target path (e.g. `metadata.version`) and resolve/create intermediate keys
  - [x] Create missing parent objects automatically
  - [x] Handle array indices in path using existing utilities

- [x] Task 4: Support literal values (AC: #5)
  - [x] Apply literal values (string, number, boolean, null) to target field
  - [x] Preserve value types

- [x] Task 5: Configuration validation and tests (AC: #1, #2, #3, #6)
  - [x] Validate target and value in config validator
  - [x] Unit tests: field exists → overwrite; field missing → create
  - [x] Unit tests: nested path create/update
  - [x] Unit tests: different value types (string, number, bool, null)

## Dev Notes

### Relevant Architecture Patterns and Constraints

- Reuse patterns from existing filter modules (`mapping`, `condition`) for config parsing and registration.
- Record representation: `map[string]interface{}`; nested paths use existing `setNestedValue` utility from mapping.go.
- Preserve compatibility with record metadata (`_metadata`) and existing filter order semantics.

### Design Decisions

- **Scope:** Set filter is ONLY for literal values. To copy from another field, use `mapping` filter.
- **Path creation:** Always create intermediate objects for nested paths (no optional behavior needed).
- **Simplicity:** Minimal config - just `target` and `value`. No source/onMissing/defaultValue/createPath options.

### Out of Scope (this story)

- Copying values from other record fields → use `mapping` filter
- Expression/templating evaluation → can be a separate story
- Bulk "set multiple fields" in one module → chain multiple `set` filters

## Dev Agent Record

### Implementation Plan
- Created `internal/modules/filter/set.go` with `SetModule` implementing `filter.Module` interface
- Registered "set" filter type in `internal/registry/builtins.go`
- Reused existing `setNestedValue` utility from mapping.go for nested path handling
- Simplified design to only support literal values (removed source/onMissing/defaultValue/createPath)

### Completion Notes
✅ All 5 tasks completed with comprehensive unit tests
✅ All 6 acceptance criteria satisfied
✅ Linter passes with no issues
✅ All existing tests continue to pass (no regressions)
✅ Design simplified based on user feedback - minimal config with maximum clarity
✅ Code review fixes applied: pipeline schema supports type `set` with required target/value; test name corrected; in-place mutation and NewSetFromConfig validation documented; File List clarified

## File List

- `internal/modules/filter/set.go` (new)
- `internal/modules/filter/set_test.go` (new)
- `internal/registry/builtins.go` (modified)
- `configs/examples/35-filters-set.json` (new)
- `_bmad-output/implementation-artifacts/sprint-status.yaml` (modified)
- `_bmad-output/implementation-artifacts/14-9-filter-module-set-record-field.md` (story file, updated during implementation)
- `internal/config/schema/pipeline-schema.json` (modified — code review: add set filter schema and required target/value)

## Change Log

- 2026-01-30: Implemented "set" filter module with simplified design - literal values only, automatic path creation
- 2026-01-30: Code review fixes: schema setFilterConfig + required target/value, test name, docs (mutation in place, NewSetFromConfig), File List
