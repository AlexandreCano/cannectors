# Story 12.4: Treat Unknown Errors as Retryable

Status: review

<!-- Note: Validation is optional. Run validate-create-story for quality check before dev-story. -->

## Story

As a developer,
I want unknown errors to be treated as retryable by default,
so that transient network, I/O, and timeout errors are automatically retried instead of failing immediately, improving system robustness.

## Acceptance Criteria

1. **Given** I have an error that cannot be classified into a known category
   **When** The error classification system processes the error
   **Then** The error is classified as `CategoryUnknown`
   **And** The error is marked as `Retryable: true` by default
   **And** The retry logic attempts to retry the operation

2. **Given** I have an unknown HTTP status code (not in the standard classification)
   **When** The system classifies the HTTP error
   **Then** The error is classified as `CategoryUnknown`
   **And** The error is marked as `Retryable: true`
   **And** The retry logic attempts to retry the operation

3. **Given** I have an unknown network error that doesn't match known patterns
   **When** The system classifies the network error
   **Then** The error is classified as `CategoryUnknown`
   **And** The error is marked as `Retryable: true`
   **And** The retry logic attempts to retry the operation

4. **Given** I have a generic error that doesn't match any classification pattern
   **When** The system classifies the error
   **Then** The error is classified as `CategoryUnknown`
   **And** The error is marked as `Retryable: true`
   **And** The retry logic attempts to retry the operation

5. **Given** I run the test suite
   **When** All tests execute
   **Then** All existing tests pass (no regressions)
   **And** New tests verify unknown errors are retryable by default
   **And** New tests verify retry logic attempts retries for unknown errors
   **And** Tests verify backward compatibility (known error categories unchanged)

## Tasks / Subtasks

- [x] Task 1: Update ClassifyHTTPStatus for Unknown Status Codes (AC: #2)
  - [x] Change `CategoryUnknown` errors to `Retryable: true` in `ClassifyHTTPStatus()` default case
  - [x] Update comment to reflect new behavior
  - [x] Verify no impact on known status codes (401, 403, 400, 422, 404, 429, 5xx)

- [x] Task 2: Update ClassifyNetworkError for Unknown Network Errors (AC: #3)
  - [x] Change `CategoryUnknown` errors to `Retryable: true` in `ClassifyNetworkError()` fallback case
  - [x] Update comment to reflect new behavior
  - [x] Verify no impact on known network error patterns (timeout, connection refused, DNS, URL errors)

- [x] Task 3: Update ClassifyError for Generic Unknown Errors (AC: #4)
  - [x] Change `CategoryUnknown` errors to `Retryable: true` in `ClassifyError()` fallback case
  - [x] Update comment to reflect new behavior
  - [x] Verify no impact on known error patterns (classified errors, timeout, context canceled, network errors)

- [x] Task 4: Update Nil Error Handling (AC: #1)
  - [x] Review nil error handling - keep `Retryable: false` for nil errors (correct behavior)
  - [x] Ensure nil errors remain non-retryable (no change needed)

- [x] Task 5: Update Error Category Documentation (AC: #1, #2, #3, #4)
  - [x] Update `CategoryUnknown` constant comment to reflect retryable-by-default behavior
  - [x] Update function documentation comments for `ClassifyHTTPStatus()`, `ClassifyNetworkError()`, `ClassifyError()`
  - [x] Document rationale: unknown errors are more likely to be transient

- [x] Task 6: Add Tests for Unknown Error Retryability (AC: #5)
  - [x] Test: Unknown HTTP status codes are retryable
  - [x] Test: Unknown network errors are retryable
  - [x] Test: Generic unknown errors are retryable
  - [x] Test: Nil errors remain non-retryable (regression test)
  - [x] Test: Known error categories unchanged (regression tests)
  - [x] Test: Retry logic attempts retries for unknown errors
  - [x] Verify existing tests still pass (no regressions)

## Dev Notes

### Context and Rationale

**Why treat unknown errors as retryable by default?**

- **Transient Nature**: Unknown errors are more likely to be transient (network issues, temporary I/O problems, timeouts) than permanent failures
- **Fail-Safe Behavior**: It's safer to retry unknown errors than to fail immediately - retries can recover from transient issues
- **Better Robustness**: Automatic retry for unknown errors improves system resilience without requiring explicit configuration
- **User Control**: Users can still configure retry behavior via `retryConfig` if needed (maxAttempts: 0 to disable retries)

**Current Behavior:**
- Unknown errors are classified as `CategoryUnknown` with `Retryable: false`
- This causes immediate failure for errors that might be transient
- Examples: Unknown HTTP status codes, unclassified network errors, generic errors

**Required Behavior:**
- Unknown errors should be classified as `CategoryUnknown` with `Retryable: true`
- Retry logic should attempt retries for unknown errors (respecting `maxAttempts` configuration)
- Known error categories remain unchanged (authentication, validation, not_found remain non-retryable)

**Impact:**
- Improved robustness: transient unknown errors are automatically retried
- Better user experience: fewer immediate failures for recoverable errors
- Backward compatible: known error categories unchanged, only unknown errors affected

### Technical Requirements

**Code Changes:**

1. **`internal/errhandling/errors.go` - `ClassifyHTTPStatus()` function:**
   - Current: Default case returns `CategoryUnknown` with `Retryable: false` (line 178-183)
   - New: Default case returns `CategoryUnknown` with `Retryable: true`
   - Update comment to reflect new behavior

2. **`internal/errhandling/errors.go` - `ClassifyNetworkError()` function:**
   - Current: Fallback case returns `CategoryUnknown` with `Retryable: false` (line 279-285)
   - New: Fallback case returns `CategoryUnknown` with `Retryable: true`
   - Update comment to reflect new behavior

3. **`internal/errhandling/errors.go` - `ClassifyError()` function:**
   - Current: Fallback case returns `CategoryUnknown` with `Retryable: false` (line 336-342)
   - New: Fallback case returns `CategoryUnknown` with `Retryable: true`
   - Update comment to reflect new behavior

4. **`internal/errhandling/errors.go` - `CategoryUnknown` constant:**
   - Current: Comment says "Unknown errors should be treated cautiously" (line 44-46)
   - New: Update comment to reflect retryable-by-default behavior

5. **Nil Error Handling:**
   - Keep `Retryable: false` for nil errors (correct behavior - no change needed)
   - Nil errors should not be retried

**Files to Modify:**
1. `internal/errhandling/errors.go` - Update classification functions to mark unknown errors as retryable
2. `internal/errhandling/errors_test.go` - Add tests for unknown error retryability

**Files to Review (No Changes Expected):**
- `internal/errhandling/retry.go` - No changes needed (retry logic already respects `Retryable` flag)
- `internal/modules/input/http_polling.go` - No changes needed (uses `IsRetryable()` which will return true for unknown errors)
- `internal/modules/output/http_request.go` - No changes needed (uses `IsRetryable()` which will return true for unknown errors)
- `internal/runtime/pipeline.go` - No changes needed (uses retry logic which respects `Retryable` flag)

### Project Structure Notes

**Files to Modify:**
```
canectors-runtime/
  internal/errhandling/
    errors.go                    # Update classification functions
    errors_test.go              # Add tests for unknown error retryability
```

**No Changes Needed:**
- Retry executor (already respects `Retryable` flag)
- Module implementations (already use `IsRetryable()`)
- Runtime pipeline (already uses retry logic)

### Architecture Compliance

**Error Handling Philosophy:**
- Fail-safe: Retry unknown errors by default (safer than failing immediately)
- Transient-first: Unknown errors are more likely to be transient than permanent
- User control: Users can configure retry behavior via `retryConfig` if needed

**Go Best Practices:**
- Minimal changes: Only update classification functions, no architectural changes
- Backward compatible: Known error categories unchanged
- Clear documentation: Update comments to reflect new behavior

**Error Handling:**
- Unknown errors: Now retryable by default
- Known fatal errors: Remain non-retryable (authentication, validation, not_found)
- Known retryable errors: Remain retryable (network, server, rate_limit)
- Nil errors: Remain non-retryable (correct behavior)

### Library/Framework Requirements

**No new dependencies required:**
- Uses existing error handling infrastructure
- No changes to retry logic (already respects `Retryable` flag)
- No changes to module implementations

### File Structure Requirements

**Code Organization:**
- Keep error classification in `errors.go` with existing classification logic
- Follow existing error category patterns
- Maintain consistency with existing error handling

**Test Organization:**
- Add tests to `errors_test.go` alongside existing classification tests
- Follow existing test naming conventions: `TestClassify*`
- Use existing test patterns for error classification

### Testing Requirements

**Unit Tests:**
- Test unknown HTTP status codes are retryable
- Test unknown network errors are retryable
- Test generic unknown errors are retryable
- Test nil errors remain non-retryable (regression)
- Test known error categories unchanged (regression)
- Test `IsRetryable()` returns true for unknown errors
- Test retry logic attempts retries for unknown errors

**Integration Tests:**
- Test full pipeline with unknown error retry behavior
- Test retry configuration respects `maxAttempts` for unknown errors
- Test retry backoff works for unknown errors

**Test Coverage:**
- Maintain or improve existing test coverage
- Add tests for new unknown error retryability behavior
- Verify no regressions in existing tests

**Linting:**
- Run `golangci-lint run` after implementation
- Fix all linting errors before marking complete

### Previous Story Intelligence

**Epic 12 Stories (In Progress):**
- **Story 12.1:** Removed `schemaVersion` field from pipeline configurations
  - Key Learning: Backward compatibility is important - keep optional fields working
  - Key Learning: Test both old and new formats to ensure compatibility

- **Story 12.2:** Close input module after input execution
  - Key Learning: Resource management timing is critical
  - Key Learning: Explicit cleanup is better than defer for timing-sensitive operations
  - Key Learning: Test resource cleanup thoroughly

- **Story 12.3:** Queue execution when previous still running
  - Key Learning: Channel-based queues are idiomatic Go and thread-safe
  - Key Learning: Bounded queues prevent unbounded memory growth
  - Key Learning: Test queue behavior thoroughly with concurrent scenarios

**Epic 4 Stories (Completed):**
- **Story 4.4:** Implement Error Handling and Retry Logic
  - Key Learning: Error classification is critical for retry behavior
  - Key Learning: `IsRetryable()` function determines retry behavior
  - Key Learning: Retry logic respects `Retryable` flag from classified errors
  - Key Learning: Current implementation marks unknown errors as non-retryable
  - Note: This story changes unknown errors to be retryable by default

**Key Learnings:**
- Current error classification marks unknown errors as non-retryable
- Unknown errors are more likely to be transient (network, I/O, timeouts)
- Retry logic already respects `Retryable` flag - only classification needs to change
- Minimal code changes required (only classification functions)

### Git Intelligence Summary

**Recent Work Patterns:**
- Error handling uses `ClassifiedError` struct with `Category` and `Retryable` fields
- `IsRetryable()` function checks `Retryable` flag from classified errors
- Retry executor respects `Retryable` flag in `Execute()` method
- Classification functions: `ClassifyHTTPStatus()`, `ClassifyNetworkError()`, `ClassifyError()`

**Code Patterns:**
- Error classification: Functions return `*ClassifiedError` with category and retryability
- Retry logic: `RetryExecutor.Execute()` checks `classified.Retryable` before retrying
- Module error handling: Uses `IsRetryable()` to determine retry behavior
- Test patterns: Test classification functions with various error types

### Latest Tech Information

**Error Handling Best Practices:**
- **Fail-Safe Defaults**: When in doubt, retry rather than fail (transient errors are more common than permanent)
- **Exponential Backoff**: Retry logic already implements exponential backoff (no changes needed)
- **Configurable Retries**: Users can configure `maxAttempts: 0` to disable retries if needed
- **Error Classification**: Clear error categories help determine retry strategy

**Go Error Handling:**
- `errors.Is()` and `errors.As()` for error unwrapping and type checking
- `ClassifiedError` implements `error` interface and `Unwrap()` for error chain compatibility
- Error classification functions return `*ClassifiedError` for consistent error handling

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
- [Source: _bmad-output/planning-artifacts/sprint-change-proposal-2026-01-24.md#Story 12.4] - Story 12.4 details: "Treat unknown errors as retryable"
- [Source: internal/errhandling/errors.go] - Current error classification implementation
- [Source: internal/errhandling/errors_test.go] - Existing error classification tests
- [Source: internal/errhandling/retry.go] - Retry logic implementation (respects `Retryable` flag)
- [Source: _bmad-output/implementation-artifacts/4-4-implement-error-handling-and-retry-logic.md] - Story 4.4 that implemented error handling
- [Source: _bmad-output/implementation-artifacts/12-3-queue-execution-when-previous-still-running.md] - Previous story in Epic 12
- [Source: _bmad-output/project-context.md] - Project context and critical rules

## Dev Agent Record

### Agent Model Used

{{agent_model_name_version}}

### Debug Log References

### Completion Notes List

- **Task 1–3:** `ClassifyHTTPStatus` default case, `ClassifyNetworkError` fallback, `ClassifyError` fallback → `CategoryUnknown` with `Retryable: true`. Comments updated.
- **Task 4:** Nil handling verified: `ClassifyError(nil)`, `ClassifyNetworkError(nil)` remain `Retryable: false`. No code change.
- **Task 5:** `CategoryUnknown` constant and function docs updated. Rationale: unknown errors more likely transient.
- **Task 6:** Tests added: `TestClassifyHTTPStatus` 200 unknown, `TestClassifyNetworkError` generic retryable, `TestClassifyError` unknown retryable, `TestIsRetryable` unknown-via-ClassifyError, `TestUnknownErrors_NilRemainsNonRetryable`, `TestRetryExecutor_RetryOnUnknownError`. Output `TestHTTPRequest_IsTransientError` updated for 200/201 → retryable (12-4).
- All ACs satisfied. Full suite + `golangci-lint run` pass.

### File List

- `internal/errhandling/errors.go` (modified)
- `internal/errhandling/errors_test.go` (modified)
- `internal/errhandling/retry_test.go` (modified)
- `internal/modules/output/http_request_test.go` (modified)
- `_bmad-output/implementation-artifacts/sprint-status.yaml` (modified)
- `_bmad-output/implementation-artifacts/12-4-treat-unknown-errors-as-retryable.md` (modified)

### Change Log

- 2026-01-24: Story 12-4 implemented. Unknown errors (HTTP default, network fallback, generic fallback) now `Retryable: true`. Nil unchanged. Docs and tests updated.
