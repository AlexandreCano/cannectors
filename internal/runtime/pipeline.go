// Package runtime provides the pipeline execution engine.
// It orchestrates the execution of Input, Filter, and Output modules.
package runtime

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/canectors/runtime/internal/logger"
	"github.com/canectors/runtime/internal/modules/filter"
	"github.com/canectors/runtime/internal/modules/input"
	"github.com/canectors/runtime/internal/modules/output"
	"github.com/canectors/runtime/pkg/connector"
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
	StatusPartial = "partial"
)

// filterResult holds the result of filter module execution
type filterResult struct {
	records []map[string]interface{}
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

// Executor is responsible for executing pipeline configurations.
// It orchestrates the execution flow: Input → Filters → Output.
type Executor struct {
	inputModule   input.Module
	filterModules []filter.Module
	outputModule  output.Module
	dryRun        bool
}

// NewExecutor creates a new pipeline executor with only dry-run flag.
// This is the basic constructor - modules must be set separately.
func NewExecutor(dryRun bool) *Executor {
	return &Executor{
		dryRun: dryRun,
	}
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

// executeFilters runs all filter modules in sequence on the given records.
// Returns the filtered records and any error that occurred.
func (e *Executor) executeFilters(pipelineID string, records []map[string]interface{}) filterResult {
	currentRecords := records
	for i, filterModule := range e.filterModules {
		if filterModule == nil {
			logger.Warn("nil filter module encountered; skipping",
				slog.String("pipeline_id", pipelineID),
				slog.String("stage", "filter"),
				slog.Int("filter_index", i),
				slog.Int("input_records", len(currentRecords)),
			)
			continue
		}

		logger.Debug("executing filter module",
			slog.String("pipeline_id", pipelineID),
			slog.String("stage", "filter"),
			slog.Int("filter_index", i),
			slog.Int("input_records", len(currentRecords)),
		)

		filterStartTime := time.Now()
		var err error
		currentRecords, err = filterModule.Process(currentRecords)
		filterDuration := time.Since(filterStartTime)

		if err != nil {
			logger.Error("filter module execution failed",
				slog.String("pipeline_id", pipelineID),
				slog.String("module", "filter"),
				slog.Int("filter_index", i),
				slog.Duration("duration", filterDuration),
				slog.String("error", err.Error()),
			)
			return filterResult{records: nil, err: err, errIdx: i}
		}

		logger.Debug("filter module completed",
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
func (e *Executor) executeOutput(pipelineID string, records []map[string]interface{}) outputResult {
	if e.dryRun {
		logger.Debug("dry-run mode: skipping output module",
			slog.String("pipeline_id", pipelineID),
			slog.Int("records_would_send", len(records)),
		)
		return outputResult{recordsSent: len(records), recordsFailed: 0, err: nil}
	}

	logger.Debug("executing output module",
		slog.String("pipeline_id", pipelineID),
		slog.String("stage", "output"),
		slog.Int("records_to_send", len(records)),
	)

	outputStartTime := time.Now()
	recordsSent, err := e.outputModule.Send(records)
	outputDuration := time.Since(outputStartTime)

	if err != nil {
		logger.Error("output module execution failed",
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
		slog.String("pipeline_id", pipelineID),
		slog.String("stage", "output"),
		slog.Int("records_sent", recordsSent),
		slog.Duration("duration", outputDuration),
	)
	return outputResult{recordsSent: recordsSent, recordsFailed: 0, err: nil}
}

// executeDryRunPreview generates request previews for dry-run mode.
// Returns nil if the output module doesn't implement PreviewableModule.
func (e *Executor) executeDryRunPreview(pipelineID string, records []map[string]interface{}, dryRunOpts *connector.DryRunOptions) []connector.RequestPreview {
	if !e.dryRun || e.outputModule == nil {
		return nil
	}

	// Check if output module implements PreviewableModule
	previewable, ok := e.outputModule.(output.PreviewableModule)
	if !ok {
		logger.Debug("output module does not implement PreviewableModule, skipping preview",
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
		slog.String("pipeline_id", pipelineID),
		slog.Int("records", len(records)),
		slog.Bool("show_credentials", previewOpts.ShowCredentials),
	)

	// Generate previews
	outputPreviews, err := previewable.PreviewRequest(records, previewOpts)
	if err != nil {
		logger.Error("failed to generate dry-run preview",
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

// Execute runs a pipeline configuration.
// It processes data through Input → Filters → Output modules.
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
	startedAt := time.Now()
	result := e.newErrorResult(startedAt)
	var timings stageTimings

	// Validate pipeline and modules first (before logging, in case pipeline is nil)
	if err := e.validateExecution(pipeline, result); err != nil {
		// Log execution start and end on validation error (if pipeline is valid)
		if pipeline != nil {
			execCtx := logger.ExecutionContext{
				PipelineID:   pipeline.ID,
				PipelineName: pipeline.Name,
				DryRun:       e.dryRun,
			}
			logger.LogExecutionStart(execCtx)
			totalDuration := time.Since(startedAt)
			logger.LogExecutionEnd(execCtx, StatusError, 0, totalDuration)
		}
		return result, err
	}
	result.PipelineID = pipeline.ID

	// Log execution start using helper
	execCtx := logger.ExecutionContext{
		PipelineID:   pipeline.ID,
		PipelineName: pipeline.Name,
		DryRun:       e.dryRun,
	}
	logger.LogExecutionStart(execCtx)

	// Setup module cleanup (must be in Execute for defers to work)
	if e.inputModule != nil {
		defer e.closeModule(pipeline.ID, "input", e.inputModule)
	}
	if e.outputModule != nil {
		defer e.closeModule(pipeline.ID, "output", e.outputModule)
	}

	// Execute Input module (returns duration measured inside)
	records, inputDuration, err := e.executeInput(pipeline, result)
	timings.inputDuration = inputDuration
	if err != nil {
		// Log execution end on input failure
		totalDuration := time.Since(startedAt)
		logger.LogExecutionEnd(execCtx, StatusError, 0, totalDuration)
		return result, err
	}

	// Execute Filter modules (returns duration measured inside)
	filteredRecords, filterDuration, err := e.executeFiltersWithResult(pipeline, records, result)
	timings.filterDuration = filterDuration
	if err != nil {
		// Log execution end on filter failure
		totalDuration := time.Since(startedAt)
		logger.LogExecutionEnd(execCtx, StatusError, len(records), totalDuration)
		return result, err
	}

	// Generate dry-run preview if applicable
	if e.dryRun {
		result.DryRunPreview = e.executeDryRunPreview(pipeline.ID, filteredRecords, pipeline.DryRunOptions)
	}

	// Execute Output module (returns duration measured inside)
	outputDuration, err := e.executeOutputWithResult(pipeline, filteredRecords, result)
	timings.outputDuration = outputDuration
	if err != nil {
		// Log execution end on output failure
		totalDuration := time.Since(startedAt)
		logger.LogExecutionEnd(execCtx, StatusError, result.RecordsProcessed, totalDuration)
		return result, err
	}

	e.finalizeSuccessWithMetrics(result, startedAt, pipeline, timings)
	return result, nil
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

// validateExecution validates the pipeline and modules before execution.
func (e *Executor) validateExecution(pipeline *connector.Pipeline, result *connector.ExecutionResult) error {
	if pipeline == nil {
		logger.Error("pipeline execution failed: nil pipeline configuration")
		result.CompletedAt = time.Now()
		result.Error = &connector.ExecutionError{
			Code:    ErrCodeInvalidInput,
			Message: ErrNilPipeline.Error(),
		}
		return ErrNilPipeline
	}

	if e.inputModule == nil {
		logger.Error("pipeline execution failed: input module is nil",
			slog.String("pipeline_id", pipeline.ID))
		result.CompletedAt = time.Now()
		result.Error = &connector.ExecutionError{
			Code:    ErrCodeInvalidInput,
			Message: ErrNilInputModule.Error(),
			Module:  "input",
		}
		return ErrNilInputModule
	}

	if e.outputModule == nil && !e.dryRun {
		logger.Error("pipeline execution failed: output module is nil",
			slog.String("pipeline_id", pipeline.ID))
		result.CompletedAt = time.Now()
		result.Error = &connector.ExecutionError{
			Code:    ErrCodeInvalidInput,
			Message: ErrNilOutputModule.Error(),
			Module:  "output",
		}
		return ErrNilOutputModule
	}

	return nil
}

// moduleCloser interface for modules that can be closed.
type moduleCloser interface {
	Close() error
}

// closeModule closes a module and logs any error.
func (e *Executor) closeModule(pipelineID, moduleName string, m moduleCloser) {
	if err := m.Close(); err != nil {
		logger.Warn("failed to close module",
			slog.String("pipeline_id", pipelineID),
			slog.String("module", moduleName),
			slog.String("error", err.Error()),
		)
	}
}

// executeInput executes the input module and returns fetched records and duration.
func (e *Executor) executeInput(pipeline *connector.Pipeline, result *connector.ExecutionResult) ([]map[string]interface{}, time.Duration, error) {
	// Log stage start
	stageCtx := logger.ExecutionContext{
		PipelineID:   pipeline.ID,
		PipelineName: pipeline.Name,
		Stage:        "input",
		DryRun:       e.dryRun,
	}
	logger.LogStageStart(stageCtx)

	inputStartTime := time.Now()
	records, err := e.inputModule.Fetch()
	inputDuration := time.Since(inputStartTime)

	if err != nil {
		result.CompletedAt = time.Now()
		result.Error = &connector.ExecutionError{
			Code:    ErrCodeInputFailed,
			Message: fmt.Sprintf("input module failed: %v", err),
			Module:  "input",
		}
		// Log stage failure
		logger.LogStageEnd(stageCtx, 0, inputDuration, &logger.ExecutionError{
			Code:    ErrCodeInputFailed,
			Message: err.Error(),
		})
		return nil, inputDuration, fmt.Errorf("executing input module: %w", err)
	}

	// Log stage completion
	logger.LogStageEnd(stageCtx, len(records), inputDuration, nil)

	return records, inputDuration, nil
}

// executeFiltersWithResult executes filter modules and updates result on error.
// Returns filtered records, duration, and error.
func (e *Executor) executeFiltersWithResult(pipeline *connector.Pipeline, records []map[string]interface{}, result *connector.ExecutionResult) ([]map[string]interface{}, time.Duration, error) {
	// Log stage start
	stageCtx := logger.ExecutionContext{
		PipelineID:   pipeline.ID,
		PipelineName: pipeline.Name,
		Stage:        "filter",
		DryRun:       e.dryRun,
	}
	logger.LogStageStart(stageCtx)

	filterStartTime := time.Now()
	filterRes := e.executeFilters(pipeline.ID, records)
	filterDuration := time.Since(filterStartTime)

	if filterRes.err != nil {
		result.CompletedAt = time.Now()
		result.Error = &connector.ExecutionError{
			Code:    ErrCodeFilterFailed,
			Message: fmt.Sprintf("filter module %d failed: %v", filterRes.errIdx, filterRes.err),
			Module:  "filter",
			Details: map[string]interface{}{
				"filterIndex": filterRes.errIdx,
			},
		}
		// Log stage failure
		logger.LogStageEnd(stageCtx, len(records), filterDuration, &logger.ExecutionError{
			Code:    ErrCodeFilterFailed,
			Message: filterRes.err.Error(),
		})
		return nil, filterDuration, fmt.Errorf("executing filter module %d: %w", filterRes.errIdx, filterRes.err)
	}

	// Log stage completion
	logger.LogStageEnd(stageCtx, len(filterRes.records), filterDuration, nil)
	return filterRes.records, filterDuration, nil
}

// executeOutputWithResult executes the output module and updates result.
// Returns duration and error.
func (e *Executor) executeOutputWithResult(pipeline *connector.Pipeline, records []map[string]interface{}, result *connector.ExecutionResult) (time.Duration, error) {
	// Log stage start
	stageCtx := logger.ExecutionContext{
		PipelineID:   pipeline.ID,
		PipelineName: pipeline.Name,
		Stage:        "output",
		DryRun:       e.dryRun,
	}
	logger.LogStageStart(stageCtx)

	outputStartTime := time.Now()
	outputRes := e.executeOutput(pipeline.ID, records)
	outputDuration := time.Since(outputStartTime)

	if outputRes.err != nil {
		result.CompletedAt = time.Now()
		result.RecordsProcessed = outputRes.recordsSent
		result.RecordsFailed = outputRes.recordsFailed
		result.Error = &connector.ExecutionError{
			Code:    ErrCodeOutputFailed,
			Message: fmt.Sprintf("output module failed: %v", outputRes.err),
			Module:  "output",
		}
		// Log stage failure
		logger.LogStageEnd(stageCtx, len(records), outputDuration, &logger.ExecutionError{
			Code:    ErrCodeOutputFailed,
			Message: outputRes.err.Error(),
		})
		return outputDuration, fmt.Errorf("executing output module: %w", outputRes.err)
	}

	// Log stage completion
	logger.LogStageEnd(stageCtx, outputRes.recordsSent, outputDuration, nil)
	result.RecordsProcessed = outputRes.recordsSent
	return outputDuration, nil
}

// finalizeSuccessWithMetrics marks the execution as successful and logs completion with detailed metrics.
func (e *Executor) finalizeSuccessWithMetrics(result *connector.ExecutionResult, startedAt time.Time, pipeline *connector.Pipeline, timings stageTimings) {
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
// This is intended for push-based inputs (e.g., webhooks) that already have data.
func (e *Executor) ExecuteWithRecords(pipeline *connector.Pipeline, records []map[string]interface{}) (*connector.ExecutionResult, error) {
	startedAt := time.Now()

	result := &connector.ExecutionResult{
		StartedAt:        startedAt,
		Status:           StatusError,
		RecordsProcessed: 0,
		RecordsFailed:    0,
	}

	if pipeline == nil {
		logger.Error("pipeline execution failed: nil pipeline configuration")
		result.CompletedAt = time.Now()
		result.Error = &connector.ExecutionError{
			Code:    ErrCodeInvalidInput,
			Message: ErrNilPipeline.Error(),
		}
		return result, ErrNilPipeline
	}

	result.PipelineID = pipeline.ID

	logger.Info("starting pipeline execution (pre-fetched records)",
		slog.String("pipeline_id", pipeline.ID),
		slog.String("pipeline_name", pipeline.Name),
		slog.String("version", pipeline.Version),
		slog.Bool("dry_run", e.dryRun),
		slog.Int("filter_count", len(e.filterModules)),
		slog.Int("input_records", len(records)),
	)

	// Output module is required unless in dry-run mode
	if e.outputModule == nil && !e.dryRun {
		logger.Error("pipeline execution failed: output module is nil",
			slog.String("pipeline_id", pipeline.ID),
		)
		result.CompletedAt = time.Now()
		result.Error = &connector.ExecutionError{
			Code:    ErrCodeInvalidInput,
			Message: ErrNilOutputModule.Error(),
			Module:  "output",
		}
		return result, ErrNilOutputModule
	}

	if e.outputModule != nil {
		defer func() {
			if err := e.outputModule.Close(); err != nil {
				logger.Warn("failed to close output module",
					slog.String("pipeline_id", pipeline.ID),
					slog.String("error", err.Error()),
				)
			}
		}()
	}

	// Step 1: Skip input module, use provided records
	// Step 2: Execute Filter modules in sequence
	filterRes := e.executeFilters(pipeline.ID, records)
	if filterRes.err != nil {
		result.CompletedAt = time.Now()
		result.Error = &connector.ExecutionError{
			Code:    ErrCodeFilterFailed,
			Message: fmt.Sprintf("filter module %d failed: %v", filterRes.errIdx, filterRes.err),
			Module:  "filter",
			Details: map[string]interface{}{
				"filterIndex": filterRes.errIdx,
			},
		}
		return result, fmt.Errorf("executing filter module %d: %w", filterRes.errIdx, filterRes.err)
	}

	// Step 3: Generate dry-run preview if applicable
	if e.dryRun {
		result.DryRunPreview = e.executeDryRunPreview(pipeline.ID, filterRes.records, pipeline.DryRunOptions)
	}

	// Step 4: Execute Output module (skip in dry-run mode)
	outputRes := e.executeOutput(pipeline.ID, filterRes.records)
	if outputRes.err != nil {
		result.CompletedAt = time.Now()
		result.RecordsProcessed = outputRes.recordsSent
		result.RecordsFailed = outputRes.recordsFailed
		result.Error = &connector.ExecutionError{
			Code:    ErrCodeOutputFailed,
			Message: fmt.Sprintf("output module failed: %v", outputRes.err),
			Module:  "output",
		}
		return result, fmt.Errorf("executing output module: %w", outputRes.err)
	}

	result.Status = StatusSuccess
	result.RecordsProcessed = outputRes.recordsSent
	result.RecordsFailed = 0
	result.CompletedAt = time.Now()
	result.Error = nil

	totalDuration := time.Since(startedAt)

	logger.Info("pipeline execution completed",
		slog.String("pipeline_id", pipeline.ID),
		slog.String("status", StatusSuccess),
		slog.Int("records_processed", outputRes.recordsSent),
		slog.Int("records_failed", 0),
		slog.Duration("total_duration", totalDuration),
		slog.Bool("dry_run", e.dryRun),
	)

	return result, nil
}
