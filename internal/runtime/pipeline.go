// Package runtime provides the pipeline execution engine.
// It orchestrates the execution of Input, Filter, and Output modules.
package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/cannectors/runtime/internal/errhandling"
	"github.com/cannectors/runtime/internal/logger"
	"github.com/cannectors/runtime/internal/modules/filter"
	"github.com/cannectors/runtime/internal/modules/input"
	"github.com/cannectors/runtime/internal/modules/output"
	"github.com/cannectors/runtime/internal/persistence"
	"github.com/cannectors/runtime/pkg/connector"
)

// Error codes for pipeline execution errors
const (
	ErrCodeInputFailed  = "INPUT_FAILED"
	ErrCodeFilterFailed = "FILTER_FAILED"
	ErrCodeOutputFailed = "OUTPUT_FAILED"
	ErrCodeInvalidInput = "INVALID_INPUT"
)

// Execution status values
const (
	StatusSuccess = "success"
	StatusError   = "error"
)

// filterResult holds the result of filter module execution
type filterResult struct {
	records []map[string]any
	err     error
	errIdx  int
}

// outputResult holds the result of output module execution
type outputResult struct {
	recordsSent   int
	recordsFailed int
	err           error
}

// Common errors
var (
	// ErrNilPipeline is returned when pipeline configuration is nil
	ErrNilPipeline = errors.New("pipeline configuration is nil")

	// ErrNilInputModule is returned when input module is nil
	ErrNilInputModule = errors.New("input module is nil")

	// ErrNilOutputModule is returned when output module is nil
	ErrNilOutputModule = errors.New("output module is nil")
)

// StatePersistentInput is an optional interface for input modules that support state persistence.
// Input modules implementing this interface can have their state persisted between executions.
type StatePersistentInput interface {
	// SetPipelineID sets the pipeline ID for state persistence.
	SetPipelineID(pipelineID string)

	// SetStateStore sets the state store to use for persistence.
	// If not called, the input module will use its own state store.
	SetStateStore(store *persistence.StateStore)

	// LoadState loads the last persisted state for this pipeline.
	LoadState() (*persistence.State, error)

	// GetPersistenceConfig returns the state persistence configuration.
	GetPersistenceConfig() *persistence.StatePersistenceConfig

	// GetLastState returns the last loaded state.
	GetLastState() *persistence.State
}

// Executor is responsible for executing pipeline configurations.
// It orchestrates the execution flow: Input → Filters → Output.
//
// The Executor only interacts with modules through their public interfaces,
// enforcing module boundaries at compile time. This is guaranteed by Go's type system:
// the fields are declared as interface types, so the runtime cannot access concrete
// module types or their internals. This ensures modules can be developed independently
// without depending on runtime internals.
type Executor struct {
	inputModule   input.Module
	filterModules []filter.Module
	outputModule  output.Module
	dryRun        bool

	// State persistence
	stateStore *persistence.StateStore
}

// NewExecutorWithModules creates a new pipeline executor with all modules configured.
// This is the primary constructor for dependency injection.
//
// Parameters:
//   - inputModule: The input module that fetches data from source
//   - filterModules: Optional slice of filter modules for data transformation (can be nil)
//   - outputModule: The output module that sends data to destination
//   - dryRun: If true, skips output module execution (validation only)
func NewExecutorWithModules(
	inputModule input.Module,
	filterModules []filter.Module,
	outputModule output.Module,
	dryRun bool,
) *Executor {
	return &Executor{
		inputModule:   inputModule,
		filterModules: filterModules,
		outputModule:  outputModule,
		dryRun:        dryRun,
	}
}

// SetStateStore sets the state store for state persistence.
// If set and the input module supports state persistence, state will be
// loaded before execution and persisted after successful execution.
func (e *Executor) SetStateStore(store *persistence.StateStore) {
	e.stateStore = store
}

// persistState saves the execution state after successful pipeline execution.
// It persists the execution start timestamp and/or last ID.
// lastID is the ID extracted from raw records (before filters) to ensure the field path
// matches the API response structure, not transformed records.
func (e *Executor) persistState(pipelineID string, executionStart time.Time, lastID *string, config *persistence.StatePersistenceConfig, traceID string) {
	state := &persistence.State{
		PipelineID: pipelineID,
		UpdatedAt:  time.Now(),
	}

	// Set timestamp if enabled
	if config.TimestampEnabled() {
		state.LastTimestamp = &executionStart
		logger.Debug("persisting execution timestamp",
			slog.String(logger.TraceIDField, traceID),
			slog.String("pipeline_id", pipelineID),
			slog.String("timestamp", executionStart.Format(time.RFC3339)),
		)
	}

	// Set last ID if enabled and extracted successfully
	if config.IDEnabled() && lastID != nil {
		state.LastID = lastID
		logger.Debug("persisting last ID from most recent record",
			slog.String(logger.TraceIDField, traceID),
			slog.String("pipeline_id", pipelineID),
			slog.String("id_field", config.ID.Field),
			slog.String("last_id", *lastID),
		)
	}

	// Only save if we have something to persist
	if state.LastTimestamp != nil || state.LastID != nil {
		if err := e.stateStore.Save(pipelineID, state); err != nil {
			logger.Warn("failed to persist state after execution",
				slog.String(logger.TraceIDField, traceID),
				slog.String("pipeline_id", pipelineID),
				slog.String("error", err.Error()),
			)
		} else {
			logger.Debug("state persisted successfully",
				slog.String(logger.TraceIDField, traceID),
				slog.String("pipeline_id", pipelineID),
				slog.Bool("has_timestamp", state.LastTimestamp != nil),
				slog.Bool("has_id", state.LastID != nil),
			)
		}
	}
}

// executeFilters runs all filter modules in sequence on the given records.
// Returns the filtered records and any error that occurred.
func (e *Executor) executeFilters(ctx context.Context, pipelineID string, records []map[string]any) filterResult {
	traceID := logger.TraceIDFrom(ctx)
	currentRecords := records
	for i, filterModule := range e.filterModules {
		if filterModule == nil {
			logger.Warn("nil filter module encountered; skipping",
				slog.String(logger.TraceIDField, traceID),
				slog.String("pipeline_id", pipelineID),
				slog.String("stage", "filter"),
				slog.Int("filter_index", i),
				slog.Int("input_records", len(currentRecords)),
			)
			continue
		}

		logger.Debug("executing filter module",
			slog.String(logger.TraceIDField, traceID),
			slog.String("pipeline_id", pipelineID),
			slog.String("stage", "filter"),
			slog.Int("filter_index", i),
			slog.Int("input_records", len(currentRecords)),
		)

		filterStartTime := time.Now()
		var err error
		currentRecords, err = filterModule.Process(ctx, currentRecords)
		filterDuration := time.Since(filterStartTime)

		if err != nil {
			logger.Error("filter module execution failed",
				slog.String(logger.TraceIDField, traceID),
				slog.String("pipeline_id", pipelineID),
				slog.String("module", "filter"),
				slog.Int("filter_index", i),
				slog.Duration("duration", filterDuration),
				slog.String("error", err.Error()),
			)
			return filterResult{records: nil, err: err, errIdx: i}
		}

		logger.Debug("filter module completed",
			slog.String(logger.TraceIDField, traceID),
			slog.String("pipeline_id", pipelineID),
			slog.String("stage", "filter"),
			slog.Int("filter_index", i),
			slog.Int("output_records", len(currentRecords)),
			slog.Duration("duration", filterDuration),
		)
	}
	return filterResult{records: currentRecords, err: nil, errIdx: -1}
}

// executeOutput runs the output module on the given records.
// In dry-run mode, calls preview if available and returns the record count without actually sending.
func (e *Executor) executeOutput(ctx context.Context, pipelineID string, records []map[string]any) outputResult {
	traceID := logger.TraceIDFrom(ctx)
	if e.dryRun {
		logger.Debug("dry-run mode: skipping output module",
			slog.String(logger.TraceIDField, traceID),
			slog.String("pipeline_id", pipelineID),
			slog.Int("records_would_send", len(records)),
		)
		return outputResult{recordsSent: len(records), recordsFailed: 0, err: nil}
	}

	logger.Debug("executing output module",
		slog.String(logger.TraceIDField, traceID),
		slog.String("pipeline_id", pipelineID),
		slog.String("stage", "output"),
		slog.Int("records_to_send", len(records)),
	)

	outputStartTime := time.Now()
	recordsSent, err := e.outputModule.Send(ctx, records)
	outputDuration := time.Since(outputStartTime)

	if err != nil {
		logger.Error("output module execution failed",
			slog.String(logger.TraceIDField, traceID),
			slog.String("pipeline_id", pipelineID),
			slog.String("module", "output"),
			slog.Int("records_sent", recordsSent),
			slog.Int("records_failed", len(records)-recordsSent),
			slog.Duration("duration", outputDuration),
			slog.String("error", err.Error()),
		)
		return outputResult{recordsSent: recordsSent, recordsFailed: len(records) - recordsSent, err: err}
	}

	logger.Debug("output module completed",
		slog.String(logger.TraceIDField, traceID),
		slog.String("pipeline_id", pipelineID),
		slog.String("stage", "output"),
		slog.Int("records_sent", recordsSent),
		slog.Duration("duration", outputDuration),
	)
	return outputResult{recordsSent: recordsSent, recordsFailed: 0, err: nil}
}

// executeDryRunPreview generates request previews for dry-run mode.
// Returns nil if the output module doesn't implement PreviewableModule.
func (e *Executor) executeDryRunPreview(pipelineID string, records []map[string]any, dryRunOpts *connector.DryRunOptions, traceID string) []connector.RequestPreview {
	if !e.dryRun || e.outputModule == nil {
		return nil
	}

	// Check if output module implements PreviewableModule
	previewable, ok := e.outputModule.(output.PreviewableModule)
	if !ok {
		logger.Debug("output module does not implement PreviewableModule, skipping preview",
			slog.String(logger.TraceIDField, traceID),
			slog.String("pipeline_id", pipelineID),
		)
		return nil
	}

	// Build preview options from pipeline config
	previewOpts := output.PreviewOptions{}
	if dryRunOpts != nil {
		previewOpts.ShowCredentials = dryRunOpts.ShowCredentials
	}

	logger.Debug("generating dry-run preview",
		slog.String(logger.TraceIDField, traceID),
		slog.String("pipeline_id", pipelineID),
		slog.Int("records", len(records)),
		slog.Bool("show_credentials", previewOpts.ShowCredentials),
	)

	// Generate previews
	outputPreviews, err := previewable.PreviewRequest(records, previewOpts)
	if err != nil {
		logger.Error("failed to generate dry-run preview",
			slog.String(logger.TraceIDField, traceID),
			slog.String("pipeline_id", pipelineID),
			slog.Int("record_count", len(records)),
			slog.Bool("show_credentials", previewOpts.ShowCredentials),
			slog.String("error", err.Error()),
			slog.String("error_context", fmt.Sprintf("preview generation failed for %d records", len(records))),
		)
		// Return error preview to inform user that preview generation failed
		// This ensures dry-run mode still provides useful feedback even when preview fails
		return []connector.RequestPreview{
			{
				Endpoint:    "[PREVIEW GENERATION FAILED]",
				Method:      "[UNKNOWN]",
				Headers:     map[string]string{"Error": err.Error()},
				BodyPreview: fmt.Sprintf("Failed to generate preview: %v", err),
				RecordCount: len(records),
			},
		}
	}

	// Convert output.RequestPreview to connector.RequestPreview
	previews := make([]connector.RequestPreview, len(outputPreviews))
	for i, p := range outputPreviews {
		previews[i] = connector.RequestPreview{
			Endpoint:    p.Endpoint,
			Method:      p.Method,
			Headers:     p.Headers,
			BodyPreview: p.BodyPreview,
			RecordCount: p.RecordCount,
		}
	}

	logger.Debug("dry-run preview generated",
		slog.String(logger.TraceIDField, traceID),
		slog.String("pipeline_id", pipelineID),
		slog.Int("preview_count", len(previews)),
	)

	return previews
}

// stageTimings holds timing measurements for each execution stage
type stageTimings struct {
	inputDuration  time.Duration
	filterDuration time.Duration
	outputDuration time.Duration
}

// Execute runs a pipeline configuration with a background context.
// It processes data through Input → Filters → Output modules.
//
// For cancellation support, use ExecuteWithContext instead.
//
// Execution flow:
//  1. Validate pipeline configuration
//  2. Execute Input module to fetch data
//  3. Execute Filter modules in sequence (if any)
//  4. Execute Output module to send data (unless dry-run mode)
//  5. Return ExecutionResult with status and metrics
//
// Returns both result and error for comprehensive error handling.
func (e *Executor) Execute(pipeline *connector.Pipeline) (*connector.ExecutionResult, error) {
	return e.ExecuteWithContext(context.Background(), pipeline)
}

// ExecuteWithContext runs a pipeline configuration with the given context.
// The context can be used to cancel long-running operations.
//
// Execution flow:
//  1. Validate pipeline configuration
//  2. Execute Input module to fetch data
//  3. Execute Filter modules in sequence (if any)
//  4. Execute Output module to send data (unless dry-run mode)
//  5. Return ExecutionResult with status and metrics
//
// Resource Management:
//   - Input module: Closed immediately after input execution completes (even on error).
//     This releases network resources (HTTP connections, connection pools) promptly,
//     before filter and output execution begins. Fetched records remain in memory.
//     HTTP Polling modules release idle connections from their connection pools.
//   - Output module: Closed at end of execution (via defer).
//   - Filter modules: Stateless, no cleanup needed.
//
// Returns both result and error for comprehensive error handling.
func (e *Executor) ExecuteWithContext(ctx context.Context, pipeline *connector.Pipeline) (*connector.ExecutionResult, error) {
	startedAt := time.Now()
	result := e.newErrorResult(startedAt)

	// Ensure a trace ID is propagated for log correlation across the entire
	// execution. If the caller already set one (e.g. scheduler, webhook), we
	// keep it; otherwise we generate a UUID v4.
	ctx, traceID := logger.EnsureTraceID(ctx)

	// Validate pipeline and modules first (before logging, in case pipeline is nil)
	if err := e.validateExecution(pipeline, result); err != nil {
		e.handleValidationError(pipeline, startedAt, traceID)
		return result, err
	}
	result.PipelineID = pipeline.ID

	// Setup execution context and logging
	execCtx := e.createExecutionContext(pipeline, traceID)
	logger.LogExecutionStart(execCtx)

	// Setup output module cleanup (deferred to end of execution)
	if e.outputModule != nil {
		defer e.closeModule(pipeline.ID, "output", e.outputModule, traceID)
	}

	// Setup state persistence if input module supports it
	persistenceConfig := e.setupStatePersistence(pipeline, traceID)

	// Execute pipeline stages (Input → Filter → Output)
	// Extract ID from raw records immediately after input to free memory early
	timings, lastID, err := e.executePipelineStages(ctx, pipeline, result, execCtx, startedAt, persistenceConfig, traceID)
	if err != nil {
		return result, err
	}

	// Persist state after successful execution (Input → Filter → Output all succeeded)
	if persistenceConfig != nil && persistenceConfig.IsEnabled() && e.stateStore != nil {
		e.persistState(pipeline.ID, startedAt, lastID, persistenceConfig, traceID)
	}

	e.finalizeSuccessWithMetrics(result, startedAt, pipeline, timings, traceID)
	return result, nil
}

// createExecutionContext creates the execution context for logging.
func (e *Executor) createExecutionContext(pipeline *connector.Pipeline, traceID string) logger.ExecutionContext {
	return logger.ExecutionContext{
		TraceID:      traceID,
		PipelineID:   pipeline.ID,
		PipelineName: pipeline.Name,
		DryRun:       e.dryRun,
	}
}

// handleValidationError logs execution start and end for validation errors.
func (e *Executor) handleValidationError(pipeline *connector.Pipeline, startedAt time.Time, traceID string) {
	if pipeline != nil {
		execCtx := logger.ExecutionContext{
			TraceID:      traceID,
			PipelineID:   pipeline.ID,
			PipelineName: pipeline.Name,
			DryRun:       e.dryRun,
		}
		logger.LogExecutionStart(execCtx)
		totalDuration := time.Since(startedAt)
		logger.LogExecutionEnd(execCtx, StatusError, 0, totalDuration)
	}
}

// setupStatePersistence configures state persistence for the input module if supported.
// Returns the persistence config if enabled, nil otherwise.
func (e *Executor) setupStatePersistence(pipeline *connector.Pipeline, traceID string) *persistence.StatePersistenceConfig {
	var persistenceConfig *persistence.StatePersistenceConfig
	if spInput, ok := e.inputModule.(StatePersistentInput); ok {
		spInput.SetPipelineID(pipeline.ID)
		// Align the executor's state store with the input module's configured
		// storagePath so that LoadState (input) and Save (executor) use the same
		// directory. Without this, a custom storagePath would be honored on
		// load but the executor would still persist to the default path.
		config := spInput.GetPersistenceConfig()
		if config != nil && config.StoragePath != "" {
			e.stateStore = persistence.NewStateStore(config.StoragePath)
		}
		if e.stateStore != nil {
			spInput.SetStateStore(e.stateStore)
		}
		state, err := spInput.LoadState()
		if err != nil {
			// Log warning but continue - state loading failure is not fatal
			// Pipeline will execute without state-based filtering (first execution behavior)
			logger.Warn("failed to load state for pipeline, continuing without persistence",
				slog.String(logger.TraceIDField, traceID),
				slog.String("pipeline_id", pipeline.ID),
				slog.String("error", err.Error()),
			)
		} else if state != nil {
			logger.Debug("loaded persisted state for pipeline",
				slog.String(logger.TraceIDField, traceID),
				slog.String("pipeline_id", pipeline.ID),
				slog.Bool("has_timestamp", state.LastTimestamp != nil),
				slog.Bool("has_id", state.LastID != nil),
			)
		}
		persistenceConfig = spInput.GetPersistenceConfig()
	}
	return persistenceConfig
}

// executePipelineStages executes Input, Filter, and Output modules in sequence.
// Extracts ID from raw records immediately after input to free memory early.
// Returns timings, last ID (extracted from raw records before filters), and any error encountered.
func (e *Executor) executePipelineStages(
	ctx context.Context,
	pipeline *connector.Pipeline,
	result *connector.ExecutionResult,
	execCtx logger.ExecutionContext,
	startedAt time.Time,
	persistenceConfig *persistence.StatePersistenceConfig,
	traceID string,
) (stageTimings, *string, error) {
	var timings stageTimings
	var lastID *string

	// Execute Input module (returns duration measured inside)
	rawRecords, inputDuration, err := e.executeInput(ctx, pipeline, result, traceID)
	timings.inputDuration = inputDuration

	// Close input module immediately after input execution completes.
	// This releases network resources (HTTP connections, connection pools) promptly,
	// before filter and output execution begins. Fetched records remain in memory.
	// For HTTP Polling modules, this closes idle connections in the connection pool.
	if e.inputModule != nil {
		e.closeModule(pipeline.ID, "input", e.inputModule, traceID)
		e.inputModule = nil // Prevent double-close
	}

	if err != nil {
		e.handleExecutionFailure(execCtx, startedAt, StatusError, 0)
		return timings, nil, err
	}

	// Extract ID from raw records immediately (before filters) to free memory early
	// This ensures the ID field path matches the API response structure, not transformed records
	if persistenceConfig != nil && persistenceConfig.IDEnabled() && persistenceConfig.ID.Field != "" && len(rawRecords) > 0 {
		extractedID, extractErr := persistence.ExtractLastID(rawRecords, persistenceConfig.ID.Field)
		if extractErr != nil {
			logger.Warn("failed to extract last ID for state persistence",
				slog.String(logger.TraceIDField, traceID),
				slog.String("pipeline_id", pipeline.ID),
				slog.String("id_field", persistenceConfig.ID.Field),
				slog.String("error", extractErr.Error()),
			)
			// Continue without ID - state will be persisted with timestamp only if enabled
		} else {
			lastID = &extractedID
			logger.Debug("extracted last ID from raw records",
				slog.String(logger.TraceIDField, traceID),
				slog.String("pipeline_id", pipeline.ID),
				slog.String("id_field", persistenceConfig.ID.Field),
				slog.String("last_id", extractedID),
			)
		}
	}

	// Execute Filter modules (returns duration measured inside)
	filteredRecords, filterDuration, err := e.executeFiltersWithResult(ctx, pipeline, rawRecords, result, traceID)
	timings.filterDuration = filterDuration
	// rawRecords can now be garbage collected after filters start processing
	if err != nil {
		e.handleExecutionFailure(execCtx, startedAt, StatusError, len(rawRecords))
		return timings, nil, err
	}

	// Generate dry-run preview if applicable
	if e.dryRun {
		result.DryRunPreview = e.executeDryRunPreview(pipeline.ID, filteredRecords, pipeline.DryRunOptions, traceID)
	}

	// Execute Output module (returns duration measured inside)
	outputDuration, err := e.executeOutputWithResult(ctx, pipeline, filteredRecords, result, traceID)
	timings.outputDuration = outputDuration
	if err != nil {
		e.handleExecutionFailure(execCtx, startedAt, StatusError, result.RecordsProcessed)
		return timings, nil, err
	}

	return timings, lastID, nil
}

// handleExecutionFailure logs execution end on failure.
func (e *Executor) handleExecutionFailure(execCtx logger.ExecutionContext, startedAt time.Time, status string, recordsProcessed int) {
	totalDuration := time.Since(startedAt)
	logger.LogExecutionEnd(execCtx, status, recordsProcessed, totalDuration)
}

// newErrorResult creates a new ExecutionResult initialized with error status.
func (e *Executor) newErrorResult(startedAt time.Time) *connector.ExecutionResult {
	return &connector.ExecutionResult{
		StartedAt:        startedAt,
		Status:           StatusError,
		RecordsProcessed: 0,
		RecordsFailed:    0,
	}
}

// buildExecutionError creates an ExecutionError with classified category and type.
// When err implements errhandling.ModuleError, its module-specific code,
// module name, and details are surfaced into the ExecutionError so the runtime
// reports rich, uniform context regardless of the source module.
func buildExecutionError(code, module string, err error) *connector.ExecutionError {
	ex := &connector.ExecutionError{
		Code:    code,
		Message: err.Error(),
		Module:  module,
	}
	if me, ok := errhandling.AsModuleError(err); ok {
		if c := me.ErrorCode(); c != "" {
			ex.Code = c
		}
		if m := me.ErrorModule(); m != "" {
			ex.Module = m
		}
		details := me.ErrorDetails()
		idx := me.ErrorRecordIndex()
		if len(details) > 0 || idx >= 0 {
			size := len(details)
			if idx >= 0 {
				size++
			}
			merged := make(map[string]any, size)
			for k, v := range details {
				merged[k] = v
			}
			if idx >= 0 {
				merged["record_index"] = idx
			}
			ex.Details = merged
		}
	}
	cl := errhandling.ClassifyError(err)
	ex.ErrorCategory = string(cl.Category)
	if errhandling.IsFatal(err) {
		ex.ErrorType = "fatal"
	} else {
		ex.ErrorType = "retryable"
	}
	return ex
}

// validateExecution validates the pipeline and modules before execution.
func (e *Executor) validateExecution(pipeline *connector.Pipeline, result *connector.ExecutionResult) error {
	if pipeline == nil {
		logger.Error("pipeline execution failed: nil pipeline configuration")
		result.CompletedAt = time.Now()
		result.Error = buildExecutionError(ErrCodeInvalidInput, "", ErrNilPipeline)
		return ErrNilPipeline
	}

	if e.inputModule == nil {
		logger.Error("pipeline execution failed: input module is nil",
			slog.String("pipeline_id", pipeline.ID))
		result.CompletedAt = time.Now()
		result.Error = buildExecutionError(ErrCodeInvalidInput, "input", ErrNilInputModule)
		return ErrNilInputModule
	}

	if e.outputModule == nil && !e.dryRun {
		logger.Error("pipeline execution failed: output module is nil",
			slog.String("pipeline_id", pipeline.ID))
		result.CompletedAt = time.Now()
		result.Error = buildExecutionError(ErrCodeInvalidInput, "output", ErrNilOutputModule)
		return ErrNilOutputModule
	}

	return nil
}

// moduleCloser interface for modules that can be closed.
type moduleCloser interface {
	Close() error
}

// closeModule closes a module and logs any error.
func (e *Executor) closeModule(pipelineID, moduleName string, m moduleCloser, traceID string) {
	if err := m.Close(); err != nil {
		logger.Warn("failed to close module",
			slog.String(logger.TraceIDField, traceID),
			slog.String("pipeline_id", pipelineID),
			slog.String("module", moduleName),
			slog.String("error", err.Error()),
		)
	}
}

// executeInput executes the input module and returns fetched records and duration.
func (e *Executor) executeInput(ctx context.Context, pipeline *connector.Pipeline, result *connector.ExecutionResult, traceID string) ([]map[string]any, time.Duration, error) {
	// Log stage start
	stageCtx := logger.ExecutionContext{
		TraceID:      traceID,
		PipelineID:   pipeline.ID,
		PipelineName: pipeline.Name,
		Stage:        "input",
		DryRun:       e.dryRun,
	}
	logger.LogStageStart(stageCtx)

	inputStartTime := time.Now()
	records, err := e.inputModule.Fetch(ctx)
	inputDuration := time.Since(inputStartTime)

	if err != nil {
		result.CompletedAt = time.Now()
		result.Error = buildExecutionError(ErrCodeInputFailed, "input", err)
		if p, ok := e.inputModule.(connector.RetryInfoProvider); ok {
			result.RetryInfo = p.GetRetryInfo()
		}
		logger.LogStageEnd(stageCtx, 0, inputDuration, &logger.ExecutionError{
			Code:    ErrCodeInputFailed,
			Message: err.Error(),
		})
		return nil, inputDuration, fmt.Errorf("executing input module: %w", err)
	}

	if p, ok := e.inputModule.(connector.RetryInfoProvider); ok {
		result.RetryInfo = p.GetRetryInfo()
	}
	logger.LogStageEnd(stageCtx, len(records), inputDuration, nil)
	return records, inputDuration, nil
}

// executeFiltersWithResult executes filter modules and updates result on error.
// Returns filtered records, duration, and error.
func (e *Executor) executeFiltersWithResult(ctx context.Context, pipeline *connector.Pipeline, records []map[string]any, result *connector.ExecutionResult, traceID string) ([]map[string]any, time.Duration, error) {
	// Log stage start
	stageCtx := logger.ExecutionContext{
		TraceID:      traceID,
		PipelineID:   pipeline.ID,
		PipelineName: pipeline.Name,
		Stage:        "filter",
		DryRun:       e.dryRun,
	}
	logger.LogStageStart(stageCtx)

	filterStartTime := time.Now()
	filterRes := e.executeFilters(ctx, pipeline.ID, records)
	filterDuration := time.Since(filterStartTime)

	if filterRes.err != nil {
		result.CompletedAt = time.Now()
		errMsg := fmt.Sprintf("filter module %d failed: %v", filterRes.errIdx, filterRes.err)
		result.Error = buildExecutionError(ErrCodeFilterFailed, "filter", filterRes.err)
		result.Error.Message = errMsg
		result.Error.Details = map[string]any{"filterIndex": filterRes.errIdx}
		logger.LogStageEnd(stageCtx, len(records), filterDuration, &logger.ExecutionError{
			Code:    ErrCodeFilterFailed,
			Message: errMsg,
		})
		return nil, filterDuration, fmt.Errorf("executing filter module %d: %w", filterRes.errIdx, filterRes.err)
	}

	// Log stage completion
	logger.LogStageEnd(stageCtx, len(filterRes.records), filterDuration, nil)
	return filterRes.records, filterDuration, nil
}

// executeOutputWithResult executes the output module and updates result.
// Returns duration and error.
func (e *Executor) executeOutputWithResult(ctx context.Context, pipeline *connector.Pipeline, records []map[string]any, result *connector.ExecutionResult, traceID string) (time.Duration, error) {
	// Log stage start
	stageCtx := logger.ExecutionContext{
		TraceID:      traceID,
		PipelineID:   pipeline.ID,
		PipelineName: pipeline.Name,
		Stage:        "output",
		DryRun:       e.dryRun,
	}
	logger.LogStageStart(stageCtx)

	outputStartTime := time.Now()
	outputRes := e.executeOutput(ctx, pipeline.ID, records)
	outputDuration := time.Since(outputStartTime)

	if outputRes.err != nil {
		result.CompletedAt = time.Now()
		result.RecordsProcessed = outputRes.recordsSent
		result.RecordsFailed = outputRes.recordsFailed
		result.Error = buildExecutionError(ErrCodeOutputFailed, "output", outputRes.err)
		if p, ok := e.outputModule.(connector.RetryInfoProvider); ok {
			result.RetryInfo = p.GetRetryInfo()
		}
		logger.LogStageEnd(stageCtx, len(records), outputDuration, &logger.ExecutionError{
			Code:    ErrCodeOutputFailed,
			Message: outputRes.err.Error(),
		})
		return outputDuration, fmt.Errorf("executing output module: %w", outputRes.err)
	}

	if p, ok := e.outputModule.(connector.RetryInfoProvider); ok {
		result.RetryInfo = p.GetRetryInfo()
	}
	logger.LogStageEnd(stageCtx, outputRes.recordsSent, outputDuration, nil)
	result.RecordsProcessed = outputRes.recordsSent
	return outputDuration, nil
}

// finalizeSuccessWithMetrics marks the execution as successful and logs completion with detailed metrics.
func (e *Executor) finalizeSuccessWithMetrics(result *connector.ExecutionResult, startedAt time.Time, pipeline *connector.Pipeline, timings stageTimings, traceID string) {
	result.Status = StatusSuccess
	result.RecordsFailed = 0
	result.CompletedAt = time.Now()
	result.Error = nil

	totalDuration := time.Since(startedAt)

	// Calculate performance metrics
	recordsProcessed := result.RecordsProcessed
	var recordsPerSecond float64
	var avgRecordTime time.Duration
	if recordsProcessed > 0 && totalDuration > 0 {
		recordsPerSecond = float64(recordsProcessed) / totalDuration.Seconds()
		avgRecordTime = totalDuration / time.Duration(recordsProcessed)
	}

	// Log detailed metrics using the new helper
	ctx := logger.ExecutionContext{
		TraceID:      traceID,
		PipelineID:   pipeline.ID,
		PipelineName: pipeline.Name,
		DryRun:       e.dryRun,
	}

	metrics := logger.ExecutionMetrics{
		TotalDuration:    totalDuration,
		InputDuration:    timings.inputDuration,
		FilterDuration:   timings.filterDuration,
		OutputDuration:   timings.outputDuration,
		RecordsProcessed: recordsProcessed,
		RecordsFailed:    0,
		RecordsPerSecond: recordsPerSecond,
		AvgRecordTime:    avgRecordTime,
	}

	// Log execution end (includes metrics via LogMetrics call)
	logger.LogExecutionEnd(ctx, StatusSuccess, recordsProcessed, totalDuration)
	logger.LogMetrics(ctx, metrics)
}

// ExecuteWithRecords runs a pipeline configuration using pre-fetched records.
// This is intended for push-based inputs (e.g. webhooks) that already have data.
// Uses a background context. For cancellation support, use ExecuteWithRecordsContext.
func (e *Executor) ExecuteWithRecords(pipeline *connector.Pipeline, records []map[string]any) (*connector.ExecutionResult, error) {
	return e.ExecuteWithRecordsContext(context.Background(), pipeline, records)
}

// ExecuteWithRecordsContext runs a pipeline configuration using pre-fetched
// records with context. This is intended for push-based inputs (e.g. webhooks)
// that already have data.
//
// The method substitutes the executor's input module with an in-memory
// records source, so the rest of the execution flows through the same path as
// ExecuteWithContext (logging helpers, stage timings, dry-run preview, etc.).
// State persistence is intentionally not engaged: pre-fetched records carry no
// notion of incremental state.
func (e *Executor) ExecuteWithRecordsContext(ctx context.Context, pipeline *connector.Pipeline, records []map[string]any) (*connector.ExecutionResult, error) {
	// Execute with a per-call clone to avoid mutating shared executor state.
	// This keeps ExecuteWithRecordsContext safe when the same executor instance
	// is used concurrently.
	exec := *e
	exec.inputModule = newRecordsInput(records)
	return exec.ExecuteWithContext(ctx, pipeline)
}
