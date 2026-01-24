# Story 12.3: Queue Execution When Previous Still Running

Status: done

<!-- Note: Validation is optional. Run validate-create-story for quality check before dev-story. -->

## Story

As a developer,
I want new pipeline executions to be queued when a previous execution is still running,
so that executions are serialized per pipeline and executed deterministically without implicit concurrency.

## Acceptance Criteria

1. **Given** I have a pipeline with a scheduled execution
   **When** A new scheduled execution is triggered while a previous execution is still running
   **Then** The new execution is queued (not skipped)
   **And** The queued execution waits for the previous execution to complete
   **And** The queued execution starts automatically when the previous execution finishes
   **And** Executions are processed in FIFO order (first in, first out)

2. **Given** I have multiple queued executions for the same pipeline
   **When** The current execution completes
   **Then** The next queued execution starts immediately
   **And** Only one execution runs at a time per pipeline
   **And** All queued executions eventually execute (no starvation)

3. **Given** I have a queued execution
   **When** The scheduler is stopped (Stop() called)
   **Then** Queued executions are canceled gracefully
   **And** Currently running execution completes (if possible)
   **And** No new executions start from the queue

4. **Given** I have a pipeline with queued executions
   **When** I check the execution status
   **Then** I can see how many executions are queued
   **And** I can see which execution is currently running
   **And** The queue status is logged appropriately

5. **Given** I run the test suite
   **When** All tests execute
   **Then** All existing tests pass (no regressions)
   **And** New tests verify queued executions wait for previous to complete
   **And** New tests verify FIFO ordering of queued executions
   **And** New tests verify queue cancellation on scheduler stop

## Tasks / Subtasks

- [x] Task 1: Add Queue Structure to registeredPipeline (AC: #1, #2)
  - [x] Add `queue` field to `registeredPipeline` struct (channel or slice-based queue)
  - [x] Add `queueMu` mutex to protect queue access
  - [x] Initialize queue in `registeredPipeline` creation
  - [x] Choose queue implementation: channel-based (bounded) or slice-based (unbounded)

- [x] Task 2: Modify tryStartExecution to Queue Instead of Skip (AC: #1)
  - [x] Change `tryStartExecution()` to queue execution if pipeline is running
  - [x] Add execution request to queue when `reg.running == true`
  - [x] Return true if execution can start immediately, false if queued
  - [x] Log queue addition with queue position

- [x] Task 3: Implement Queue Processing After Execution Completes (AC: #1, #2)
  - [x] Modify `finishExecution()` to check queue after marking as not running
  - [x] If queue has items, start next execution from queue
  - [x] Process queue in FIFO order
  - [x] Ensure only one execution processes queue at a time (mutex protection)

- [x] Task 4: Handle Queue Cancellation on Scheduler Stop (AC: #3)
  - [x] Cancel queued executions when scheduler Stop() is called
  - [x] Clear queue for all pipelines during shutdown
  - [x] Log cancellation of queued executions
  - [x] Ensure currently running execution can complete (if context allows)

- [x] Task 5: Add Queue Status Methods (AC: #4)
  - [x] Add `GetQueueLength(pipelineID string) int` method
  - [x] Add `IsQueued(pipelineID string) bool` method (if needed)
  - [x] Update logging to include queue status
  - [x] Add queue position in execution start logs

- [x] Task 6: Add Tests for Queue Functionality (AC: #5)
  - [x] Test: Queue execution when previous still running
  - [x] Test: FIFO ordering of queued executions
  - [x] Test: Queue processing after execution completes
  - [x] Test: Queue cancellation on scheduler stop
  - [x] Test: Multiple queued executions processed sequentially
  - [x] Verify existing tests still pass (no regressions)

## Dev Notes

### Context and Rationale

**Why queue executions instead of skipping?**

- **Deterministic Behavior**: Queuing ensures all scheduled executions eventually run, providing predictable behavior
- **Avoid Implicit Concurrency**: Serializing executions per pipeline prevents race conditions and data corruption
- **No Lost Executions**: Skipping means some scheduled executions never run, which can cause data inconsistencies
- **Better Resource Management**: Controlled execution order prevents resource contention

**Current Behavior:**
- When a new execution is triggered while previous is running, it's **skipped** (line 419 in `scheduler.go`)
- Log message: "skipping overlapping execution"
- This means scheduled executions can be lost if they overlap

**Required Behavior:**
- When a new execution is triggered while previous is running, it should be **queued**
- Queued executions wait for previous to complete
- Executions are processed in FIFO order (first scheduled, first executed)
- All scheduled executions eventually run (no starvation)

### Technical Requirements

**Code Changes:**

1. **`internal/scheduler/scheduler.go` - `registeredPipeline` struct:**
   - Add queue structure to hold pending executions
   - Add mutex to protect queue access
   - Choose queue implementation:
     - **Option A: Channel-based (bounded)**: `queue chan struct{}` with fixed capacity
     - **Option B: Slice-based (unbounded)**: `queue []executionRequest` with append/remove
   - **Recommendation**: Use channel-based for simplicity and Go idioms

2. **`internal/scheduler/scheduler.go` - `tryStartExecution()` method:**
   - Current: Returns `false` if `reg.running == true` (skips execution)
   - New: If `reg.running == true`, add to queue and return `false`
   - If `reg.running == false`, set `reg.running = true` and return `true`

3. **`internal/scheduler/scheduler.go` - `finishExecution()` method:**
   - Current: Just sets `reg.running = false`
   - New: After setting `reg.running = false`, check queue
   - If queue has items, start next execution (remove from queue, call `executePipeline()`)
   - Process queue atomically (hold mutex during check and start)

4. **`internal/scheduler/scheduler.go` - `Stop()` method:**
   - Clear queues for all registered pipelines
   - Cancel any queued executions (close queue channels or clear slice)
   - Log queue cancellation

5. **Queue Implementation Details:**
   - **Channel-based approach**:
     ```go
     type registeredPipeline struct {
         pipeline *connector.Pipeline
         entryID  cron.EntryID
         running  bool
         mu       sync.Mutex
         queue    chan struct{}  // Bounded queue for execution requests
     }
     ```
   - Queue size: Bounded queue with capacity 100 (DefaultQueueCapacity) to prevent unbounded growth
   - Execution request: Simple `struct{}` signal (channel-based queue is thread-safe by design)
   - Note: Channel operations are thread-safe, so no separate queueMu mutex is needed
   - Queue full behavior: When queue is full, requests are dropped (logged as warning) to prevent memory exhaustion

**Files to Modify:**
1. `internal/scheduler/scheduler.go` - Add queue to `registeredPipeline`, modify `tryStartExecution()`, `finishExecution()`, `Stop()`
2. `internal/scheduler/scheduler_test.go` - Add tests for queue functionality

**Files to Review (No Changes Expected):**
- `internal/runtime/pipeline.go` - No changes needed (executor interface unchanged)
- `cmd/canectors/main.go` - No changes needed (scheduler usage unchanged)

### Project Structure Notes

**Files to Modify:**
```
canectors-runtime/
  internal/scheduler/
    scheduler.go                    # Add queue, modify execution logic
    scheduler_test.go             # Add queue tests
```

**No Changes Needed:**
- Pipeline executor (no interface changes)
- Main CLI (scheduler usage unchanged)
- Module implementations (no changes)

### Architecture Compliance

**Execution Serialization:**
- One execution per pipeline at a time (deterministic)
- FIFO queue ensures ordered execution
- No implicit concurrency (prevents race conditions)

**Go Best Practices:**
- Use channels for queue (idiomatic Go)
- Protect queue access with mutex
- Use bounded channels to prevent unbounded growth
- Handle context cancellation for graceful shutdown

**Error Handling:**
- Queue full: Log warning, consider dropping oldest or rejecting new
- Queue cancellation: Log info, don't fail scheduler stop
- Execution errors: Don't block queue processing (errors handled in `doExecutePipeline()`)

### Library/Framework Requirements

**No new dependencies required:**
- Uses standard Go `sync` package (mutex, channels)
- Uses existing `context` package for cancellation
- Uses existing `robfig/cron` library (no changes)

### File Structure Requirements

**Code Organization:**
- Keep queue logic in `scheduler.go` with existing execution logic
- Use channel-based queue for Go idioms
- Follow existing mutex patterns for thread safety

**Test Organization:**
- Add tests to `scheduler_test.go` alongside existing execution tests
- Follow existing test naming conventions: `TestScheduler_*`
- Use existing mock executor for testing

### Testing Requirements

**Unit Tests:**
- Test execution queued when previous still running
- Test FIFO ordering of queued executions
- Test queue processing after execution completes
- Test queue cancellation on scheduler stop
- Test multiple queued executions processed sequentially
- Test queue bounded capacity (if using bounded channel)
- Test no starvation (all queued executions eventually run)

**Integration Tests:**
- Test full scheduler with multiple pipelines (some queued, some not)
- Test scheduler stop with queued executions
- Test rapid scheduling (multiple triggers before first completes)

**Test Coverage:**
- Maintain or improve existing test coverage
- Add tests for new queue behavior
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

**Epic 4 Stories (Completed):**
- **Story 4.1:** Implement CRON scheduler for polling
  - Key Learning: Current implementation **skips** overlapping executions (line 419)
  - Key Learning: `tryStartExecution()` returns false if already running
  - Key Learning: This story changes that behavior to queue instead of skip
  - Note: Story 4.1 mentioned "skip if busy" - this story changes to "queue if busy"

**Key Learnings:**
- Current scheduler skips overlapping executions (deterministic but loses executions)
- Queue approach ensures all executions run (better for data consistency)
- Channel-based queues are idiomatic Go and thread-safe
- Bounded queues prevent unbounded memory growth

### Git Intelligence Summary

**Recent Work Patterns:**
- Scheduler uses mutex-protected `running` flag per pipeline
- `tryStartExecution()` and `finishExecution()` handle execution state
- Current pattern: Skip if running (this story changes to queue)
- Tests exist for overlap detection (`TestScheduler_OverlappingExecution`)

**Code Patterns:**
- Mutex protection: `reg.mu` protects `reg.running` flag
- Execution tracking: `s.wg` WaitGroup tracks in-flight executions
- Context cancellation: `s.ctx` used for graceful shutdown
- Logging: Structured logging with `slog` for execution events

### References

- [Source: _bmad-output/planning-artifacts/sprint-change-proposal-2026-01-24.md#Epic 12] - Epic 12 rationale and story definition
- [Source: _bmad-output/planning-artifacts/sprint-change-proposal-2026-01-24.md#Story 12.3] - Story 12.3 details: "Queue execution when previous still running"
- [Source: internal/scheduler/scheduler.go] - Current scheduler implementation with skip behavior
- [Source: internal/scheduler/scheduler_test.go] - Existing scheduler tests
- [Source: _bmad-output/implementation-artifacts/12-2-close-input-module-after-input-execution.md] - Previous story in Epic 12
- [Source: _bmad-output/implementation-artifacts/4-1-implement-cron-scheduler-for-polling.md] - Story 4.1 that implemented current skip behavior
- [Source: _bmad-output/project-context.md] - Project context and critical rules

## Dev Agent Record

### Agent Model Used

Claude Opus 4.5 (claude-opus-4-5-20251101)

### Debug Log References

- All tests pass: `go test ./internal/scheduler/...` (48+ seconds, all pass)
- Linting passes: `golangci-lint run ./...` (0 issues)
- Full regression suite: `go test ./...` (all pass)

### Completion Notes List

- **Task 1**: Added `queue chan struct{}` field to `registeredPipeline` struct with `DefaultQueueCapacity = 100`. Queue is initialized in `Register()` when creating new `registeredPipeline`.

- **Task 2**: Modified `tryStartExecution()` to queue execution requests instead of skipping. When `reg.running == true`, adds to queue with non-blocking send. Logs queue position. Falls back to warning if queue is full.

- **Task 3**: Modified `finishExecution()` to process queued executions. After execution completes, checks queue for pending items. If found, immediately starts next execution (keeps `running = true`). Implements FIFO via channel semantics.

- **Task 4**: Enhanced `Stop()` to explicitly drain queues before clearing pipelines map. Uses labeled loop with break to drain all queued items. Logs number of canceled executions per pipeline.

- **Task 5**: Added `GetQueueLength(pipelineID string) int` and `IsQueued(pipelineID string) bool` methods. Queue status is also logged in execution start and queue processing messages.

- **Task 6**: Added comprehensive tests:
  - `TestScheduler_GetQueueLength_EmptyQueue` / `_NotFound`
  - `TestScheduler_QueuesExecutionWhenPreviousRunning`
  - `TestScheduler_ProcessesQueueAfterExecutionCompletes`
  - `TestScheduler_FIFOQueueOrder`
  - `TestScheduler_ClearsQueueOnStop`
  - `TestScheduler_StopDoesNotStartNewQueuedExecutions`
  - `TestScheduler_IsQueued` / `_NotFound`
  - `TestScheduler_MultipleQueuedExecutionsProcessedSequentially`
  - `TestScheduler_NoStarvation`
  - Renamed `TestScheduler_SkipsOverlappingExecution` to `TestScheduler_SerializesExecution`

### File List

- `internal/scheduler/scheduler.go` - Modified: added queue to registeredPipeline, modified tryStartExecution, finishExecution, Stop, added GetQueueLength, IsQueued, executeQueuedPipeline
- `internal/scheduler/scheduler_test.go` - Modified: added 10+ new tests for queue functionality, renamed 1 test

### Code Review Fixes (2026-01-24)

**CRITICAL Fixes:**
- Added WaitGroup tracking in `executeQueuedPipeline()` to ensure graceful shutdown waits for queued executions
- Documented queue full behavior: requests are dropped (not blocked) to prevent memory exhaustion

**HIGH Severity Fixes:**
- Fixed race conditions with `len(reg.queue)` by protecting all reads with `reg.mu`
- Protected `GetQueueLength()` with pipeline mutex
- Captured `queueLen` before releasing mutex in `finishExecution()`

**MEDIUM Severity Fixes:**
- Added tests for queue full scenario: `TestScheduler_QueueFullDropsRequest` and `TestScheduler_QueueFullDoesNotBlock`
- Updated story documentation to remove incorrect `queueMu` mention (channels are thread-safe by design)
