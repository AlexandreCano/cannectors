// Package scheduler provides CRON-based scheduling for pipeline execution.
// It allows pipelines to be executed on a recurring schedule using CRON expressions.
//
// The scheduler supports:
//   - Standard 5-field CRON format (minute, hour, day, month, weekday)
//   - Extended 6-field CRON format (second, minute, hour, day, month, weekday)
//   - Overlap handling (skips execution if previous is still running)
//   - Graceful shutdown with timeout
//   - Dynamic pipeline registration/unregistration
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/canectors/runtime/internal/logger"
	"github.com/canectors/runtime/pkg/connector"
)

// Error definitions
var (
	// ErrNotImplemented is returned when a feature is not yet implemented.
	ErrNotImplemented = errors.New("not implemented")

	// ErrNilPipeline is returned when a nil pipeline is provided.
	ErrNilPipeline = errors.New("pipeline is nil")

	// ErrPipelineDisabled is returned when attempting to register a disabled pipeline.
	ErrPipelineDisabled = errors.New("pipeline is disabled")

	// ErrEmptySchedule is returned when a pipeline has no schedule configured.
	ErrEmptySchedule = errors.New("pipeline schedule is empty")

	// ErrInvalidCronExpression is returned when the CRON expression is invalid.
	ErrInvalidCronExpression = errors.New("invalid CRON expression")

	// ErrSchedulerAlreadyRunning is returned when Start() is called on a running scheduler.
	ErrSchedulerAlreadyRunning = errors.New("scheduler is already running")

	// ErrPipelineNotFound is returned when the pipeline is not registered.
	ErrPipelineNotFound = errors.New("pipeline not found")

	// ErrPipelineRunning is returned when attempting to update a pipeline that is currently executing.
	ErrPipelineRunning = errors.New("pipeline is currently executing")
)

// Executor defines the interface for pipeline execution.
// This allows dependency injection for testing.
type Executor interface {
	Execute(pipeline *connector.Pipeline) (*connector.ExecutionResult, error)
}

// registeredPipeline holds a pipeline and its CRON entry ID.
type registeredPipeline struct {
	pipeline *connector.Pipeline
	entryID  cron.EntryID
	running  bool
	mu       sync.Mutex
}

// Scheduler manages scheduled pipeline executions using CRON expressions.
type Scheduler struct {
	// cron is the underlying CRON scheduler
	cron *cron.Cron

	// pipelines holds registered pipelines with their schedules
	pipelines map[string]*registeredPipeline

	// executor executes pipelines (nil for stub mode)
	executor Executor

	// mu protects the pipelines map, started flag, and ctx
	mu sync.RWMutex

	// started indicates if the scheduler is running
	started bool

	// ctx is the context for cancellation (set in Start())
	ctx context.Context

	// cancel cancels the context (set in Start())
	cancel context.CancelFunc

	// wg tracks in-flight executions for graceful shutdown
	wg sync.WaitGroup

	// stopMu protects stopChan access
	stopMu sync.RWMutex

	// stopChan signals shutdown
	stopChan chan struct{}
}

// New creates a new scheduler instance with a stub executor.
// Use NewWithExecutor for production use with a real executor.
func New() *Scheduler {
	return NewWithExecutor(nil)
}

// NewWithExecutor creates a new scheduler instance with the given executor.
func NewWithExecutor(executor Executor) *Scheduler {
	// Create cron with support for seconds (6-field format)
	c := cron.New(cron.WithParser(cron.NewParser(
		cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
	)))

	return &Scheduler{
		cron:      c,
		pipelines: make(map[string]*registeredPipeline),
		executor:  executor,
		stopChan:  make(chan struct{}),
	}
}

// ValidateCronExpression validates a CRON expression string.
// Supports both standard 5-field and extended 6-field formats.
//
// Standard format: minute hour day month weekday
// Extended format: second minute hour day month weekday
//
// Returns nil if valid, error with details if invalid.
func ValidateCronExpression(expr string) error {
	if expr == "" {
		return fmt.Errorf("%w: expression is empty", ErrInvalidCronExpression)
	}

	parser := cron.NewParser(
		cron.SecondOptional | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
	)

	_, err := parser.Parse(expr)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidCronExpression, err)
	}

	return nil
}

// Register adds a pipeline to the scheduler.
// The pipeline must be enabled and have a valid Schedule field.
// If a pipeline with the same ID is already registered, it will be updated.
//
// Returns an error if:
//   - Pipeline is nil
//   - Pipeline is disabled (Enabled == false)
//   - Pipeline has no schedule (Schedule == "")
//   - Pipeline has invalid CRON expression
func (s *Scheduler) Register(pipeline *connector.Pipeline) error {
	if pipeline == nil {
		return ErrNilPipeline
	}

	if !pipeline.Enabled {
		return fmt.Errorf("%w: pipeline %s is not enabled", ErrPipelineDisabled, pipeline.ID)
	}

	if pipeline.Schedule == "" {
		return fmt.Errorf("%w: pipeline %s has no schedule", ErrEmptySchedule, pipeline.ID)
	}

	// Validate CRON expression
	if err := ValidateCronExpression(pipeline.Schedule); err != nil {
		return fmt.Errorf("pipeline %s: %w", pipeline.ID, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if already registered (update case)
	if existing, ok := s.pipelines[pipeline.ID]; ok {
		// Check if pipeline is currently executing - prevent update during execution
		// to avoid race condition where executePipeline holds a reference to the old entry
		existing.mu.Lock()
		isRunning := existing.running
		existing.mu.Unlock()

		if isRunning {
			return fmt.Errorf("%w: cannot update pipeline %s while it is executing", ErrPipelineRunning, pipeline.ID)
		}

		// Remove old CRON job
		s.cron.Remove(existing.entryID)
		delete(s.pipelines, pipeline.ID)

		logger.Info("updating existing pipeline in scheduler",
			slog.String("pipeline_id", pipeline.ID),
			slog.String("schedule", pipeline.Schedule),
		)
	}

	// Create registered pipeline entry
	reg := &registeredPipeline{
		pipeline: pipeline,
	}

	// Add CRON job
	entryID, err := s.cron.AddFunc(pipeline.Schedule, func() {
		s.executePipeline(reg)
	})
	if err != nil {
		return fmt.Errorf("failed to add CRON job for pipeline %s: %w", pipeline.ID, err)
	}

	reg.entryID = entryID
	s.pipelines[pipeline.ID] = reg

	logger.Info("pipeline registered in scheduler",
		slog.String("pipeline_id", pipeline.ID),
		slog.String("pipeline_name", pipeline.Name),
		slog.String("schedule", pipeline.Schedule),
	)

	return nil
}

// Unregister removes a pipeline from the scheduler.
// Returns ErrPipelineNotFound if the pipeline is not registered.
func (s *Scheduler) Unregister(pipelineID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	reg, ok := s.pipelines[pipelineID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrPipelineNotFound, pipelineID)
	}

	// Remove CRON job
	s.cron.Remove(reg.entryID)
	delete(s.pipelines, pipelineID)

	logger.Info("pipeline unregistered from scheduler",
		slog.String("pipeline_id", pipelineID),
	)

	return nil
}

// Start begins executing scheduled pipelines.
// The provided context is used for cancellation when Stop() is called.
// Returns ErrSchedulerAlreadyRunning if the scheduler is already started.
func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return ErrSchedulerAlreadyRunning
	}

	// Create a cancellable context from the provided context
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.started = true

	// Ensure stopChan is initialized (it should normally be set in the constructor)
	s.stopMu.Lock()
	if s.stopChan == nil {
		s.stopChan = make(chan struct{})
	}
	s.stopMu.Unlock()

	// Start the CRON scheduler
	s.cron.Start()

	logger.Info("scheduler started",
		slog.Int("pipeline_count", s.PipelineCount()),
	)

	return nil
}

// Stop halts all scheduled executions gracefully.
// It waits for in-flight executions to complete or until the context is canceled.
// After Stop(), all registered pipelines are cleared.
func (s *Scheduler) Stop(ctx context.Context) error {
	// First, stop the CRON scheduler to prevent new job triggers
	cronCtx := s.cron.Stop()

	// Now set started to false and cancel the context while holding the lock
	// This ensures no new executions can start (they check s.started)
	s.mu.Lock()
	wasStarted := s.started
	s.started = false
	if s.cancel != nil {
		s.cancel() // Cancel the context to signal all executions
	}
	s.mu.Unlock()

	if wasStarted {
		// Wait for CRON to fully stop (with timeout to prevent indefinite blocking)
		// Use a timeout context to ensure we don't block forever
		timeoutCtx, timeoutCancel := context.WithTimeout(ctx, 2*time.Second)
		defer timeoutCancel()

		select {
		case <-cronCtx.Done():
			// CRON stopped
		case <-ctx.Done():
			// Context canceled
		case <-timeoutCtx.Done():
			// Timeout to prevent indefinite blocking
			logger.Warn("cron stop context timeout - continuing with shutdown")
		}

		// Signal any running executions to stop (protected by mutex)
		s.stopMu.Lock()
		if s.stopChan != nil {
			select {
			case <-s.stopChan:
				// Already closed
			default:
				close(s.stopChan)
			}
		}
		s.stopMu.Unlock()
	}

	// Wait for in-flight executions with timeout
	// Use a channel-based approach to allow timeout
	waitDone := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
		// All executions completed
	case <-ctx.Done():
		logger.Warn("scheduler stop timeout - some executions may still be running")
		return ctx.Err()
	}

	// Clear all pipelines
	s.mu.Lock()
	s.pipelines = make(map[string]*registeredPipeline)
	s.mu.Unlock()

	logger.Info("scheduler stopped")

	return nil
}

// executePipeline runs a single pipeline execution.
// Handles overlap detection and logging.
func (s *Scheduler) executePipeline(reg *registeredPipeline) {
	// Atomically check if started and add to WaitGroup while holding the lock.
	// This ensures Stop() cannot complete wg.Wait() before we've registered this execution.
	// The invariant is: Add(1) is only called if started is true, and both operations
	// happen under the same lock acquisition.
	s.mu.RLock()
	if !s.started {
		s.mu.RUnlock()
		return
	}
	s.wg.Add(1)
	ctx := s.ctx // Capture context while holding lock
	s.mu.RUnlock()

	defer s.wg.Done()

	// Check for overlap - acquire pipeline-specific lock
	if !s.tryStartExecution(reg) {
		return
	}
	defer s.finishExecution(reg)

	// Copy pipeline reference while we still have a valid reg
	pipeline := reg.pipeline
	startTime := time.Now()

	logger.Info("scheduled pipeline execution starting",
		slog.String("pipeline_id", pipeline.ID),
		slog.String("pipeline_name", pipeline.Name),
		slog.String("schedule", pipeline.Schedule),
		slog.Time("scheduled_time", startTime),
	)

	// Check if context is canceled before executing
	if ctx != nil {
		select {
		case <-ctx.Done():
			logger.Info("scheduled pipeline execution canceled",
				slog.String("pipeline_id", pipeline.ID),
				slog.String("pipeline_name", pipeline.Name),
			)
			return
		default:
			// Continue execution
		}
	}

	// Execute pipeline
	s.doExecutePipeline(pipeline, startTime)
}

// tryStartExecution attempts to mark the pipeline as running.
// Returns false if the pipeline is already running (overlap).
func (s *Scheduler) tryStartExecution(reg *registeredPipeline) bool {
	reg.mu.Lock()
	defer reg.mu.Unlock()

	if reg.running {
		logger.Warn("skipping overlapping execution",
			slog.String("pipeline_id", reg.pipeline.ID),
			slog.String("pipeline_name", reg.pipeline.Name),
		)
		return false
	}
	reg.running = true
	return true
}

// finishExecution marks the pipeline as no longer running.
func (s *Scheduler) finishExecution(reg *registeredPipeline) {
	reg.mu.Lock()
	reg.running = false
	reg.mu.Unlock()
}

// doExecutePipeline performs the actual pipeline execution.
func (s *Scheduler) doExecutePipeline(pipeline *connector.Pipeline, startTime time.Time) {
	if s.executor == nil {
		// Stub mode - just log
		logger.Info("scheduler stub: pipeline would be executed",
			slog.String("pipeline_id", pipeline.ID),
		)
		return
	}

	result, err := s.executor.Execute(pipeline)
	duration := time.Since(startTime)

	if err != nil {
		logger.Error("scheduled pipeline execution failed",
			slog.String("pipeline_id", pipeline.ID),
			slog.String("pipeline_name", pipeline.Name),
			slog.Duration("duration", duration),
			slog.String("error", err.Error()),
		)
		return
	}

	logger.Info("scheduled pipeline execution completed",
		slog.String("pipeline_id", pipeline.ID),
		slog.String("pipeline_name", pipeline.Name),
		slog.String("status", result.Status),
		slog.Int("records_processed", result.RecordsProcessed),
		slog.Int("records_failed", result.RecordsFailed),
		slog.Duration("duration", duration),
	)
}

// HasPipeline checks if a pipeline is registered.
func (s *Scheduler) HasPipeline(pipelineID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.pipelines[pipelineID]
	return ok
}

// IsRunning checks if a specific pipeline is currently executing.
func (s *Scheduler) IsRunning(pipelineID string) bool {
	s.mu.RLock()
	reg, ok := s.pipelines[pipelineID]
	s.mu.RUnlock()

	if !ok {
		return false
	}

	reg.mu.Lock()
	defer reg.mu.Unlock()
	return reg.running
}

// IsStarted returns true if the scheduler is running.
func (s *Scheduler) IsStarted() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.started
}

// PipelineCount returns the number of registered pipelines.
func (s *Scheduler) PipelineCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.pipelines)
}

// GetPipelineIDs returns a list of all registered pipeline IDs.
func (s *Scheduler) GetPipelineIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := make([]string, 0, len(s.pipelines))
	for id := range s.pipelines {
		ids = append(ids, id)
	}
	return ids
}

// GetNextRun returns the next scheduled run time for a pipeline.
// Returns zero time and error if:
//   - Pipeline is not found
//   - Scheduler is not started
func (s *Scheduler) GetNextRun(pipelineID string) (time.Time, error) {
	s.mu.RLock()
	started := s.started
	reg, ok := s.pipelines[pipelineID]
	s.mu.RUnlock()

	if !ok {
		return time.Time{}, fmt.Errorf("%w: %s", ErrPipelineNotFound, pipelineID)
	}

	if !started {
		return time.Time{}, fmt.Errorf("scheduler is not started")
	}

	// Get entry from cron - Entry() should not block, but we need to ensure
	// the cron scheduler has processed the job registration
	entry := s.cron.Entry(reg.entryID)

	// If Next is zero, the entry might not be ready yet
	if entry.Next.IsZero() {
		return time.Time{}, fmt.Errorf("next run time not yet calculated for pipeline %s", pipelineID)
	}

	return entry.Next, nil
}
