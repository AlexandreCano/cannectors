# Story 14.7: Record Metadata Storage

Status: ready-for-dev

<!-- Note: Validation is optional. Run validate-create-story for quality check before dev-story. -->

## Story

As a developer,
I want to store metadata values in records that are not sent in the output request body,
so that I can track internal state, timestamps, processing information, and other metadata without affecting the data sent to target systems.

## Acceptance Criteria

1. **Given** I have a connector with metadata storage configured
   **When** The runtime processes records through the pipeline
   **Then** Records support a `_metadata` field (or configurable field name) for storing metadata
   **And** Metadata values are stored separately from record data
   **And** Metadata values are accessible during filter module processing
   **And** Metadata values are NOT included in the output request body by default
   **And** Metadata values can be accessed in template expressions (for templating feature)
   **And** Metadata values persist through the entire pipeline execution (Input → Filter → Output)

2. **Given** I have a connector with metadata storage enabled
   **When** Records are processed through filter modules
   **Then** Filter modules can read metadata values from records
   **And** Filter modules can write metadata values to records
   **And** Filter modules can modify existing metadata values
   **And** Metadata values are preserved when records are transformed by filter modules
   **And** Metadata values are available for conditional logic in filter modules

3. **Given** I have a connector with metadata storage configured
   **When** The runtime executes the output module
   **Then** Metadata values are excluded from the request body by default
   **And** The output module strips `_metadata` field (or configured field name) before sending
   **And** Metadata values can be optionally included in headers or query parameters via templating
   **And** Metadata values can be optionally included in endpoint URLs via templating
   **And** The exclusion of metadata from body is configurable (can be disabled if needed)

4. **Given** I have a connector with metadata storage
   **When** Metadata values are set during pipeline execution
   **Then** Metadata can store execution timestamps (e.g., `_metadata.processed_at`)
   **And** Metadata can store processing flags (e.g., `_metadata.enriched`, `_metadata.validated`)
   **And** Metadata can store error information (e.g., `_metadata.errors`)
   **And** Metadata can store custom application-specific values
   **And** Metadata supports nested objects for organizing related values

5. **Given** I have a connector with metadata storage configured
   **When** The runtime processes records
   **Then** Metadata field name is configurable (default: `_metadata`)
   **And** Metadata field name can be customized per pipeline
   **And** Metadata field name follows naming conventions (starts with underscore for reserved fields)
   **And** Metadata field name is validated to prevent conflicts with record data fields
   **And** Metadata field name is documented and consistent across the pipeline

6. **Given** I have a connector with metadata storage
   **When** Records are processed through the pipeline
   **Then** Metadata values are preserved across filter module transformations
   **And** Metadata values are preserved when records are cloned or duplicated
   **And** Metadata values are preserved when records are filtered or conditionally processed
   **And** Metadata values are available for logging and debugging
   **And** Metadata values do not interfere with record data transformations

7. **Given** I have a connector with metadata storage and templating enabled
   **When** The runtime evaluates template expressions
   **Then** Template expressions can access metadata values (e.g., `{{_metadata.processed_at}}`)
   **And** Template expressions can use metadata values in endpoint URLs
   **And** Template expressions can use metadata values in HTTP headers
   **And** Template expressions can use metadata values in request body (if explicitly included)
   **And** Metadata access in templates uses the configured metadata field name

## Tasks / Subtasks

- [ ] Task 1: Design metadata storage structure and field naming (AC: #1, #5)
  - [ ] Define metadata field structure (nested object with configurable name)
  - [ ] Design default metadata field name (`_metadata`)
  - [ ] Design metadata field naming conventions (underscore prefix for reserved fields)
  - [ ] Design metadata field name configuration in pipeline schema
  - [ ] Design validation rules for metadata field names
  - [ ] Document metadata field naming conventions

- [ ] Task 2: Implement metadata storage in record structure (AC: #1, #4)
  - [ ] Add metadata field support to record structure (map[string]interface{})
  - [ ] Implement metadata field accessor functions
  - [ ] Implement metadata field setter functions
  - [ ] Support nested metadata objects
  - [ ] Handle metadata field initialization
  - [ ] Add unit tests for metadata storage operations

- [ ] Task 3: Integrate metadata storage with filter modules (AC: #2)
  - [ ] Ensure filter modules preserve metadata when transforming records
  - [ ] Add metadata access helpers for filter modules
  - [ ] Support metadata reading in filter modules
  - [ ] Support metadata writing in filter modules
  - [ ] Support metadata modification in filter modules
  - [ ] Ensure metadata is available for conditional logic
  - [ ] Add integration tests with filter modules and metadata

- [ ] Task 4: Integrate metadata exclusion with output module (AC: #3)
  - [ ] Add metadata field exclusion to output module
  - [ ] Strip metadata field from request body before sending
  - [ ] Support configurable metadata exclusion (can be disabled)
  - [ ] Ensure metadata exclusion works with both `bodyFrom: "record"` and `bodyFrom: "records"` modes
  - [ ] Support metadata inclusion in headers/query params via templating
  - [ ] Support metadata inclusion in endpoint URLs via templating
  - [ ] Add integration tests with output module and metadata exclusion

- [ ] Task 5: Integrate metadata with templating feature (AC: #7)
  - [ ] Add metadata access to template evaluation context
  - [ ] Support metadata access in template expressions (e.g., `{{_metadata.field}}`)
  - [ ] Support nested metadata access in templates
  - [ ] Support metadata in endpoint URL templates
  - [ ] Support metadata in HTTP header templates
  - [ ] Support metadata in request body templates (if explicitly included)
  - [ ] Add integration tests with templating and metadata

- [ ] Task 6: Add metadata configuration to pipeline schema (AC: #5)
  - [ ] Add metadata field name configuration to pipeline schema
  - [ ] Define default metadata field name (`_metadata`)
  - [ ] Add validation rules for metadata field names
  - [ ] Add metadata exclusion configuration to output module schema
  - [ ] Document metadata configuration options
  - [ ] Add configuration examples with metadata

- [ ] Task 7: Add metadata helpers and utilities (AC: #1, #2, #4)
  - [ ] Create metadata accessor utility functions
  - [ ] Create metadata setter utility functions
  - [ ] Create metadata merger utility functions (for combining metadata)
  - [ ] Create metadata copy utility functions (for cloning records)
  - [ ] Add helper functions for common metadata operations (timestamps, flags, errors)
  - [ ] Add unit tests for metadata utilities

- [ ] Task 8: Add comprehensive tests for metadata feature (AC: #1, #2, #3, #4, #5, #6, #7)
  - [ ] Test metadata storage and retrieval
  - [ ] Test metadata preservation through filter modules
  - [ ] Test metadata exclusion from output request body
  - [ ] Test metadata access in template expressions
  - [ ] Test metadata with nested objects
  - [ ] Test metadata field name configuration
  - [ ] Test metadata with record cloning and duplication
  - [ ] Test metadata with conditional filtering
  - [ ] Test metadata with both `bodyFrom: "record"` and `bodyFrom: "records"` modes
  - [ ] Test metadata error handling

- [ ] Task 9: Update documentation (AC: #5, #6)
  - [ ] Document metadata feature in README.md
  - [ ] Create example configurations with metadata storage
  - [ ] Document metadata field naming conventions
  - [ ] Document metadata access in filter modules
  - [ ] Document metadata exclusion from output
  - [ ] Document metadata access in template expressions
  - [ ] Add troubleshooting section for metadata

## Dev Notes

### Relevant Architecture Patterns and Constraints

**Metadata Storage Design:**
- Metadata is stored in a separate field within records (default: `_metadata`)
- Metadata field name is configurable per pipeline
- Metadata field name uses underscore prefix convention for reserved fields
- Metadata values are stored as nested objects (map[string]interface{})
- Metadata values persist through the entire pipeline execution
- Metadata values are excluded from output request body by default

**Metadata Field Structure:**
- Default field name: `_metadata` (configurable)
- Metadata is a nested object: `{"_metadata": {"processed_at": "...", "enriched": true, ...}}`
- Metadata supports arbitrary nested structures
- Metadata values can be of any JSON-serializable type

**Integration with Filter Modules:**
- Filter modules can read metadata values from records
- Filter modules can write metadata values to records
- Filter modules preserve metadata when transforming records
- Metadata is available for conditional logic in filter modules
- Metadata is preserved when records are cloned or duplicated

**Integration with Output Module:**
- Metadata field is excluded from request body by default
- Metadata exclusion is configurable (can be disabled if needed)
- Metadata can be included in headers/query params via templating
- Metadata can be included in endpoint URLs via templating
- Metadata exclusion works with both `bodyFrom: "record"` and `bodyFrom: "records"` modes

**Integration with Templating:**
- Template expressions can access metadata values (e.g., `{{_metadata.processed_at}}`)
- Metadata access in templates uses the configured metadata field name
- Metadata can be used in endpoint URL templates
- Metadata can be used in HTTP header templates
- Metadata can be used in request body templates (if explicitly included)

**Common Metadata Use Cases:**
- Execution timestamps: `_metadata.processed_at`, `_metadata.received_at`
- Processing flags: `_metadata.enriched`, `_metadata.validated`, `_metadata.transformed`
- Error information: `_metadata.errors`, `_metadata.warnings`
- Custom application-specific values: `_metadata.source_system`, `_metadata.batch_id`
- Nested organization: `_metadata.timing.start`, `_metadata.timing.end`

**Error Handling:**
- Missing metadata field is handled gracefully (returns nil or empty map)
- Invalid metadata field names are validated at configuration time
- Metadata field conflicts with record data fields are prevented
- Metadata access errors are logged but don't fail execution

### Project Structure Notes

**Files to Create:**
- `internal/runtime/metadata.go` - Metadata storage and access utilities
- `internal/runtime/metadata_test.go` - Metadata tests
- `configs/examples/24-record-metadata-storage.yaml` - Example with metadata storage
- `configs/examples/25-record-metadata-templating.yaml` - Example with metadata and templating

**Files to Modify:**
- `internal/modules/filter/mapping.go` - Ensure metadata preservation in mapping transformations
- `internal/modules/filter/condition.go` - Add metadata access for conditional logic
- `internal/modules/filter/enrichment.go` - Add metadata access for enrichment operations
- `internal/modules/output/http_request.go` - Add metadata exclusion from request body
- `internal/runtime/pipeline.go` - Ensure metadata preservation through pipeline execution
- `internal/config/schema/pipeline-schema.json` - Add metadata field name configuration
- `README.md` - Document metadata feature

**New Dependencies:**
```go
// No new external dependencies required
// Use standard library: map[string]interface{} for metadata storage
// Reuse existing: internal/logger for logging
```

**Metadata Field Naming:**
- Default: `_metadata` (underscore prefix for reserved fields)
- Configurable per pipeline via `metadata.fieldName` configuration
- Validation: Must start with underscore, must not conflict with record data fields
- Convention: Reserved fields use underscore prefix (e.g., `_metadata`, `_id`, `_timestamp`)

### References

- [Source: internal/modules/output/http_request.go] - HTTP request output module implementation
- [Source: internal/modules/filter/mapping.go] - Mapping filter module with record transformation patterns
- [Source: internal/modules/filter/condition.go] - Condition filter module with record access patterns
- [Source: internal/runtime/pipeline.go] - Pipeline execution engine with record flow
- [Source: _bmad-output/implementation-artifacts/14-6-output-templating-with-record-data.md] - Templating feature (metadata integration)
- [Source: _bmad-output/implementation-artifacts/14-5-last-timestamp-persistence-for-polling-inputs.md] - State persistence patterns (reference for metadata storage)
- [Source: _bmad-output/planning-artifacts/architecture.md] - Architecture patterns and constraints
- [Source: _bmad-output/project-context.md] - Project context and critical rules

### Previous Story Intelligence

**From Story 14.6 (Output Templating):**
- Templating syntax and evaluation patterns
- Template variable access patterns
- Integration with output module
- Template evaluation context and error handling

**From Story 14.5 (Last Timestamp Persistence):**
- State persistence patterns with file-based storage
- Thread-safe operations
- Error handling patterns: log warnings, continue without feature if errors occur
- Configuration validation patterns

**From Story 14.4 (Dynamic Enrichment Inside Filters):**
- Cache implementation patterns
- Filter module integration patterns
- Record transformation patterns

**Git Intelligence:**
- Recent commits show focus on filter modules (enrichment, script, logging)
- Pattern: Create new packages for new features (e.g., `internal/cache/`, `internal/persistence/`)
- Pattern: Comprehensive test coverage (unit tests + integration tests)
- Pattern: Update pipeline schema for new features
- Pattern: Create example configurations for new features

### Latest Technical Information

**Go Map Operations:**
- Use `map[string]interface{}` for metadata storage
- Access nested fields using type assertions and map lookups
- Handle missing keys gracefully with `ok` checks
- Support nested map structures for metadata organization

**Metadata Access Patterns:**
- Use helper functions for metadata access (e.g., `GetMetadata(record, key)`)
- Use helper functions for metadata setting (e.g., `SetMetadata(record, key, value)`)
- Support nested access using dot notation (e.g., `GetMetadataNested(record, "timing.start")`)
- Handle type assertions safely for metadata values

**Record Structure:**
- Records are `map[string]interface{}` type
- Metadata is stored as a nested map within the record
- Metadata field name is configurable (default: `_metadata`)
- Metadata exclusion from output is done by removing the metadata field before serialization

**JSON Serialization:**
- Metadata field is excluded from JSON serialization in output module
- Use `json.Marshal` with custom logic to exclude metadata field
- Or use struct tags or custom serialization logic
- Ensure metadata exclusion works with both single record and batch modes

## Dev Agent Record

### Agent Model Used

Claude Sonnet 4.5 (claude-sonnet-4-5-20250514)

### Debug Log References

### Completion Notes List

### File List
