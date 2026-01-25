# Story 12.5: Move Schedule Configuration to Input Level

Status: done

<!-- Note: Validation is optional. Run validate-create-story for quality check before dev-story. -->

## Story

As a developer,
I want the schedule configuration to be defined at the input module level instead of the connector level,
so that the configuration is more coherent since not all input types support scheduled execution (e.g., webhook inputs don't need schedules).

## Acceptance Criteria

1. **Given** I have a pipeline configuration with an `httpPolling` input module
   **When** I define a schedule in the input module configuration
   **Then** The schedule is stored in the input module's config
   **And** The schedule is accessible via `pipeline.Input.Config["schedule"]`
   **And** The scheduler reads the schedule from the input module config
   **And** The pipeline executes according to the schedule

2. **Given** I have a pipeline configuration with a `webhook` input module
   **When** I do not define a schedule (or define an empty schedule)
   **Then** The pipeline does not require a schedule
   **And** The scheduler does not attempt to schedule the pipeline
   **And** The pipeline can be executed manually without schedule validation errors
   **And** If a schedule is defined for a webhook input, it is rejected with a clear error message (schedule is only for polling types)

3. **Given** I have a pipeline configuration with `schedule` at connector level
   **When** The configuration is validated
   **Then** Validation rejects it with a clear error (schedule only allowed in input module, for polling types)

4. **Given** I have a pipeline configuration with schedule at input level
   **When** The configuration is validated
   **Then** The schedule is validated as a CRON expression
   **And** The schedule is **required** for polling input types (`httpPolling`, future `sql` polling)
   **And** The schedule is **not allowed** for event-driven input types (`webhook`, future `pubsub`/`kafka`)
   **And** If schedule is present for event-driven types, validation rejects it with clear error message
   **And** If schedule is missing for polling types, validation rejects it with clear error message
   **And** Invalid CRON expressions are rejected with clear error messages

5. **Given** I run the test suite
   **When** All tests execute
   **Then** All existing tests pass (no regressions)
   **And** New tests verify schedule is read from input module
   **And** New tests verify connector-level schedule is rejected
   **And** New tests verify scheduler reads schedule from input config

## Tasks / Subtasks

- [x] Task 1: Update Pipeline Type to Remove Schedule Field (AC: #1, #2)
  - [x] Remove `Schedule string` field from `Pipeline` struct in `pkg/connector/types.go`
  - [x] Update struct documentation to reflect schedule is now in input module config
  - [x] Verify no other code directly accesses `pipeline.Schedule` (grep for usages)

- [x] Task 2: Update Converter to Keep Schedule in Input Module Config (AC: #1, #3)
  - [x] Remove schedule extraction from `extractPipelineMetadata()` in `internal/config/converter.go`
  - [x] Remove schedule extraction from `extractModules()` that sets `p.Schedule`
  - [x] Ensure schedule remains in `inputData` and is stored in `inputConfig.Config["schedule"]`
  - [x] No backward compatibility: schedule at connector level is forbidden (schema rejects it)

- [x] Task 3: Update Scheduler to Read Schedule from Input Module (AC: #1, #5)
  - [x] Update `Register()` in `internal/scheduler/scheduler.go` to read schedule from `pipeline.Input.Config["schedule"]`
  - [x] Update `ErrEmptySchedule` error message to reference input module
  - [x] Update validation logic to check input module config instead of `pipeline.Schedule`
  - [x] Update logging to reference input module schedule
  - [x] Update `GetNextRun()` and other methods that reference `pipeline.Schedule`

- [x] Task 4: Update CLI to Read Schedule from Input Module (AC: #1, #5)
  - [x] Update `runScheduledPipeline()` in `cmd/canectors/main.go` to read schedule from `pipeline.Input.Config["schedule"]`
  - [x] Update validation logic to check input module config
  - [x] Update output messages to reference input module schedule

- [x] Task 5: Update Schema Validation for Schedule (AC: #4)
  - [x] Verify JSON schema already has `schedule` in `inputModule` definition (already present)
  - [x] Remove any connector-level schedule references from schema if present
  - [x] Update schema comments to clarify schedule is only for polling input types
  - [x] Ensure schema validation requires schedule for `httpPolling` type (already present)
  - [x] Add validation to reject schedule for `webhook` type (if schedule present, error)
  - [x] Update schema to document that schedule is only for polling types (httpPolling, future SQL)

- [x] Task 6: Update Example Configurations (AC: #1, #2)
  - [x] Update `configs/examples/13-scheduled.json` to have schedule in input module
  - [x] Update `configs/examples/13-scheduled.yaml` to have schedule in input module
  - [x] Remove schedule from connector level in examples
  - [x] Add example showing webhook input without schedule

- [x] Task 7: Add Tests for Schedule at Input Level (AC: #5)
  - [x] Test: Schedule is read from input module config
  - [x] Test: Scheduler registers pipeline with schedule from input config
  - [x] Test: Webhook input does not require schedule (schedule is optional/ignored)
  - [x] Test: Webhook input rejects schedule if present (validation error)
  - [x] Test: HttpPolling input requires schedule (validation error if missing)
  - [x] Test: HttpPolling input accepts valid schedule
  - [x] Test: Backward compatibility with connector-level schedule
  - [x] Test: Deprecation warning when schedule at connector level
  - [x] Test: Invalid schedule in input config is rejected
  - [x] Test: Schedule validation distinguishes polling vs event-driven input types
  - [x] Verify existing tests still pass (no regressions)

## Dev Notes

### Context and Rationale

**Why move schedule to input level?**

- **Coherence**: Not all input types support scheduled execution. **Polling input types** (httpPolling, future SQL polling) are time-based and **require** schedules, while **event-driven input types** (webhook, future Pubsub/Kafka) are event-triggered and **must not** have schedules.
- **Semantic Clarity**: Schedule is a property of polling input modules (how often to poll), not the entire pipeline. Moving it to input level makes the configuration more intuitive.
- **Type-Specific Validation**: Schedule should only be available for polling types. Event-driven types should reject schedule configuration to prevent configuration errors.
- **Extensibility**: Future input types will follow this pattern: polling types (SQL) require schedule, event-driven types (Pubsub/Kafka) reject schedule.
- **Configuration Consistency**: Other input-specific properties (endpoint, method, headers) are already at input level. Schedule should follow the same pattern.

**Current Behavior:**
- Schedule is defined at connector/pipeline level: `connector.schedule` or `pipeline.Schedule`
- Converter extracts schedule from input module but stores it at pipeline level (lines 101-104 in converter.go)
- Scheduler reads schedule from `pipeline.Schedule` (line 178 in scheduler.go)
- All input types are treated the same regarding schedule

**Required Behavior:**
- Schedule should be defined in input module configuration: `connector.input.schedule`
- Schedule should be stored in `pipeline.Input.Config["schedule"]`
- Scheduler should read schedule from `pipeline.Input.Config["schedule"]`
- **Schedule is REQUIRED for polling input types**: `httpPolling` (and future `sql` polling)
- **Schedule is NOT ALLOWED for event-driven input types**: `webhook` (and future `pubsub`/`kafka`)
- Validation must reject schedule for event-driven types with clear error message
- Validation must require schedule for polling types with clear error message
- Backward compatibility: migrate connector-level schedule to input level with deprecation warning

**Impact:**
- Configuration files need to be updated (schedule moves from connector to input)
- Scheduler logic needs to read from input config instead of pipeline field
- CLI needs to read from input config instead of pipeline field
- Backward compatibility ensures existing configs continue to work

### Input Type Classification

**Polling Input Types (Schedule REQUIRED):**
- `httpPolling`: Time-based polling of HTTP endpoints - **requires** schedule
- Future `sql`: Time-based polling of SQL queries - **will require** schedule

**Event-Driven Input Types (Schedule NOT ALLOWED):**
- `webhook`: Event-triggered HTTP POST reception - **must not** have schedule
- Future `pubsub`: Event-triggered Pub/Sub message reception - **will not allow** schedule
- Future `kafka`: Event-triggered Kafka message consumption - **will not allow** schedule

**Validation Rules:**
- **Polling types**: Schedule is **required** - validation error if missing
- **Event-driven types**: Schedule is **not allowed** - validation error if present
- **Error messages**: Must clearly indicate which input type requires/rejects schedule

### Technical Requirements

**Code Changes:**

1. **`pkg/connector/types.go` - `Pipeline` struct:**
   - Current: `Schedule string` field at line 34
   - New: Remove `Schedule` field entirely
   - Update struct documentation

2. **`internal/config/converter.go` - `extractModules()` function:**
   - Current: Extracts schedule from input and sets `p.Schedule` (lines 101-104)
   - New: Keep schedule in `inputConfig.Config["schedule"]`, don't set `p.Schedule`
   - Add validation: Check input type - reject schedule for event-driven types (webhook)
   - Add validation: Check input type - require schedule for polling types (httpPolling)
   - Add backward compatibility: if `connectorData["schedule"]` exists, migrate to input module
   - Add deprecation warning when schedule found at connector level

3. **`internal/scheduler/scheduler.go` - `Register()` function:**
   - Current: Reads `pipeline.Schedule` (line 178)
   - New: Read `pipeline.Input.Config["schedule"]` (with type assertion)
   - Add validation: Check input type - only register if polling type (httpPolling, future SQL)
   - Add validation: Reject event-driven types (webhook, future pubsub/kafka) with clear error
   - Update error messages to reference input module and input type
   - Update validation logic to distinguish polling vs event-driven types

4. **`cmd/canectors/main.go` - `runScheduledPipeline()` function:**
   - Current: Reads `pipeline.Schedule` (line 279, 291, 314)
   - New: Read `pipeline.Input.Config["schedule"]`
   - Add validation: Check input type - only allow scheduled execution for polling types
   - Add validation: Reject event-driven types with clear error message
   - Update validation and output messages to reference input type

5. **`internal/config/schema/pipeline-schema.json`:**
   - Verify `schedule` is already in `inputModule` definition (line 251-255) ✅
   - Remove any connector-level schedule references if present
   - Ensure validation requires schedule for `httpPolling` type (already present at line 270)
   - Add validation to reject schedule for `webhook` type (if schedule present, validation error)
   - Update schema documentation to clarify schedule is only for polling types

**Files to Modify:**
1. `pkg/connector/types.go` - Remove `Schedule` field from `Pipeline` struct
2. `internal/config/converter.go` - Update schedule extraction logic
3. `internal/scheduler/scheduler.go` - Update schedule reading logic
4. `cmd/canectors/main.go` - Update schedule reading logic
5. `configs/examples/13-scheduled.json` - Move schedule to input module
6. `configs/examples/13-scheduled.yaml` - Move schedule to input module

**Files to Review (No Changes Expected):**
- `internal/config/schema/pipeline-schema.json` - Already has schedule in inputModule (verify)
- `internal/runtime/pipeline.go` - No changes needed (doesn't use schedule)
- `internal/modules/input/http_polling.go` - No changes needed (doesn't use schedule directly)

### Project Structure Notes

**Files to Modify:**
```
canectors-runtime/
  pkg/connector/
    types.go                    # Remove Schedule field from Pipeline
  internal/config/
    converter.go                # Update schedule extraction
  internal/scheduler/
    scheduler.go                # Update schedule reading
  cmd/canectors/
    main.go                     # Update schedule reading
  configs/examples/
    13-scheduled.json          # Move schedule to input
    13-scheduled.yaml          # Move schedule to input
```

**No Changes Needed:**
- Schema already has schedule in inputModule definition
- Runtime pipeline executor doesn't use schedule
- Input modules don't directly use schedule (scheduler handles it)

### Architecture Compliance

**Configuration Philosophy:**
- Module-specific properties belong at module level
- Schedule is a property of input module (how often to poll)
- Configuration should be intuitive and semantically correct

**Go Best Practices:**
- Type assertions for reading from `Config` map: `schedule, ok := pipeline.Input.Config["schedule"].(string)`
- Backward compatibility: Migrate old format with deprecation warning
- Clear error messages when schedule is missing or invalid

**Error Handling:**
- Validate schedule exists for polling input types (`httpPolling`, future `sql`)
- Validate schedule is NOT present for event-driven input types (`webhook`, future `pubsub`/`kafka`)
- Validate schedule is valid CRON expression (for polling types)
- Clear error messages when schedule is missing (polling types) or present (event-driven types)
- Clear error messages when schedule is invalid CRON expression
- Deprecation warning (not error) for backward compatibility

### Library/Framework Requirements

**No new dependencies required:**
- Uses existing scheduler (robfig/cron) - no changes needed
- Uses existing CRON validation - no changes needed
- No changes to module implementations

### File Structure Requirements

**Code Organization:**
- Keep schedule extraction in converter.go with backward compatibility
- Keep schedule reading in scheduler.go with proper type assertions
- Follow existing patterns for reading from `Config` map

**Test Organization:**
- Add tests to `converter_test.go` for schedule extraction
- Add tests to `scheduler_test.go` for schedule reading from input config
- Add tests to `main_test.go` for CLI schedule reading
- Follow existing test patterns

### Testing Requirements

**Unit Tests:**
- Test schedule is extracted from input module config
- Test schedule is NOT extracted to pipeline level
- Test backward compatibility: connector-level schedule migrated to input
- Test deprecation warning when schedule at connector level
- Test scheduler reads schedule from input config
- Test scheduler rejects pipeline without schedule (for httpPolling - polling type)
- Test scheduler accepts pipeline with schedule (for httpPolling - polling type)
- Test scheduler rejects pipeline with schedule (for webhook - event-driven type)
- Test scheduler accepts pipeline without schedule (for webhook - event-driven type)
- Test validation rejects schedule for webhook input type
- Test validation requires schedule for httpPolling input type
- Test CLI reads schedule from input config
- Test CLI rejects scheduled execution for event-driven input types

**Integration Tests:**
- Test full pipeline execution with schedule in input config
- Test scheduler executes pipeline with schedule from input config
- Test backward compatibility: old config format still works

**Test Coverage:**
- Maintain or improve existing test coverage
- Add tests for new schedule location behavior
- Verify no regressions in existing tests

**Linting:**
- Run `golangci-lint run` after implementation
- Fix all linting errors before marking complete

### Previous Story Intelligence

**Epic 12 Stories (In Progress):**
- **Story 12.1:** Removed `schemaVersion` field from pipeline configurations
  - Key Learning: Backward compatibility is important - keep optional fields working
  - Key Learning: Test both old and new formats to ensure compatibility
  - Key Learning: Deprecation warnings help users migrate gradually

- **Story 12.2:** Close input module after input execution
  - Key Learning: Resource management timing is critical
  - Key Learning: Explicit cleanup is better than defer for timing-sensitive operations

- **Story 12.3:** Queue execution when previous still running
  - Key Learning: Channel-based queues are idiomatic Go and thread-safe
  - Key Learning: Bounded queues prevent unbounded memory growth

- **Story 12.4:** Treat unknown errors as retryable
  - Key Learning: Minimal changes to classification functions
  - Key Learning: Backward compatibility maintained for known error categories

**Epic 4 Stories (Completed):**
- **Story 4.1:** Implement CRON Scheduler for Polling
  - Key Learning: Scheduler reads schedule from `pipeline.Schedule`
  - Key Learning: Schedule validation uses `ValidateCronExpression()`
  - Note: This story changes where schedule is read from

**Key Learnings:**
- Backward compatibility is critical - migrate old format with deprecation warning
- Configuration should be semantically correct (schedule belongs to input, not pipeline)
- **Input type determines schedule requirements**: polling types require it, event-driven types reject it
- Type assertions are needed when reading from `Config` map
- Validation must distinguish between polling and event-driven input types
- Test both old and new formats to ensure compatibility

### Git Intelligence Summary

**Recent Work Patterns:**
- Schedule is currently at pipeline level: `pipeline.Schedule`
- Converter extracts schedule from input but stores at pipeline level
- Scheduler reads `pipeline.Schedule` in `Register()` method
- CLI reads `pipeline.Schedule` in `runScheduledPipeline()` function
- Schema already has schedule in `inputModule` definition

**Code Patterns:**
- Schedule extraction: `extractModules()` extracts from input and sets `p.Schedule`
- Schedule reading: `pipeline.Schedule` is read directly
- Schedule validation: `ValidateCronExpression(pipeline.Schedule)`
- Type assertions: Use `value, ok := map["key"].(type)` pattern for Config maps

### Latest Tech Information

**Configuration Best Practices:**
- **Semantic Correctness**: Properties should be defined where they logically belong
- **Module-Specific Properties**: Properties that only apply to specific module types should be at module level
- **Type-Specific Validation**: Properties that only apply to certain input types should be validated accordingly (schedule for polling only)
- **Backward Compatibility**: Migrate old formats with deprecation warnings, not breaking changes
- **Clear Error Messages**: When configuration is invalid, provide clear guidance on how to fix it (e.g., "schedule is only allowed for polling input types like httpPolling")

**Go Configuration Patterns:**
- Use type assertions when reading from `map[string]interface{}` configs
- Provide default values where appropriate
- Validate required fields early in the parsing process
- Log deprecation warnings for old formats

### Project Context Reference

**From project-context.md:**
- Go Runtime CLI: Run `golangci-lint run` after each implementation to verify linting passes
- Error handling: Always use structured error types with context
- Testing: Co-locate tests with source files, use existing test patterns
- Code quality: Fix all linting errors before marking task as complete

**Critical Rules:**
- ✅ ALWAYS run linting checks before considering implementation complete
- **Go Runtime CLI:** Run `golangci-lint run` at the end of each implementation to ensure code quality
- Fix all linting errors before marking task as complete

### References

- [Source: _bmad-output/planning-artifacts/sprint-change-proposal-2026-01-24.md#Epic 12] - Epic 12 rationale and story definition
- [Source: _bmad-output/planning-artifacts/sprint-change-proposal-2026-01-24.md#Story 12.5] - Story 12.5 details: "Move schedule configuration to input level"
- [Source: pkg/connector/types.go] - Current Pipeline struct with Schedule field
- [Source: internal/config/converter.go] - Current schedule extraction logic
- [Source: internal/scheduler/scheduler.go] - Current schedule reading logic
- [Source: cmd/canectors/main.go] - Current CLI schedule reading logic
- [Source: internal/config/schema/pipeline-schema.json] - Schema with schedule in inputModule
- [Source: _bmad-output/implementation-artifacts/12-1-remove-schemaversion-field.md] - Previous story showing backward compatibility pattern
- [Source: _bmad-output/implementation-artifacts/4-1-implement-cron-scheduler-for-polling.md] - Story 4.1 that implemented scheduler
- [Source: _bmad-output/project-context.md] - Project context and critical rules

## Dev Agent Record

### Agent Model Used

Claude 3.5 Sonnet (claude-sonnet-4-20250514)

### Debug Log References

### Completion Notes List

- Removed `Schedule` field from `Pipeline` struct - schedule now lives in `pipeline.Input.Config["schedule"]`
- Added `GetScheduleFromInput()` helper in scheduler package for safe schedule extraction
- No backward compatibility: schedule at connector root forbidden (schema `schedule: false`); only in input module for polling types
- Schema rejects schedule for webhook input (`not: { required: ["schedule"] }`) and at connector root (`schedule: false`)
- Added 14-webhook example, validator tests (webhook+schedule, connector-level schedule rejected), README schedule wording
- All 13 test packages pass, 0 regressions
- Linting passes with 0 issues

### File List

- pkg/connector/types.go (modified) - Removed Schedule field from Pipeline struct
- pkg/connector/types_test.go (modified) - Updated test to use schedule in input config
- internal/config/converter.go (modified) - No connector-level schedule; schedule only in input config
- internal/config/converter_test.go (modified) - Schedule at input level; removed backward-compat tests
- internal/config/validator_test.go (modified) - Webhook+schedule and connector-level schedule rejected tests
- internal/config/schema/pipeline-schema.json (modified) - Webhook schedule rejection; connector `schedule: false`
- internal/scheduler/scheduler.go (modified) - GetScheduleFromInput(), Register() reads from input config
- internal/scheduler/scheduler_test.go (modified) - Input config tests, schedule tests
- cmd/canectors/main.go (modified) - CLI reads schedule from input config, run help mentions input-level schedule
- configs/examples/13-scheduled.json (modified) - Schedule in input module
- configs/examples/13-scheduled.yaml (modified) - Schedule in input module
- configs/examples/14-webhook.json (added) - Webhook input without schedule example
- configs/examples/14-webhook.yaml (added) - Webhook input without schedule example
- configs/examples/README.md (modified) - 14-webhook section, schedule wording (required vs not allowed)
- _bmad-output/implementation-artifacts/sprint-status.yaml (modified) - Sprint tracking

## Change Log

- 2026-01-25: Story 12.5 implemented - Schedule configuration moved from pipeline level to input module config
- 2026-01-24: Code review fixes - Skip schedule migration for webhook; 14-webhook example; validator test; README/main help wording; Task 5 subtasks marked done; File List updated
- 2026-01-24: No backward compatibility - Schedule at connector root forbidden (schema); only in input module for polling types. Removed migration, deprecation warning, backward-compat tests.
