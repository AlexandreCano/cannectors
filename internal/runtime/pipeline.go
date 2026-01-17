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
// In dry-run mode, returns the record count without actually sending.
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
// Error handling:
//   - Input errors: Stop immediately, return error result
//   - Filter errors: Stop immediately, no data sent to output
//   - Output errors: Stop immediately, return error result
//
// Returns both result and error for comprehensive error handling.
func (e *Executor) Execute(pipeline *connector.Pipeline) (*connector.ExecutionResult, error) {
	startedAt := time.Now()

	// Initialize result with error status (will be updated on success)
	result := &connector.ExecutionResult{
		StartedAt:        startedAt,
		Status:           StatusError,
		RecordsProcessed: 0,
		RecordsFailed:    0,
	}

	// Validate pipeline
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

	// Log pipeline execution start
	logger.Info("starting pipeline execution",
		slog.String("pipeline_id", pipeline.ID),
		slog.String("pipeline_name", pipeline.Name),
		slog.String("version", pipeline.Version),
		slog.Bool("dry_run", e.dryRun),
		slog.Int("filter_count", len(e.filterModules)),
	)

	// Validate modules
	if e.inputModule == nil {
		logger.Error("pipeline execution failed: input module is nil",
			slog.String("pipeline_id", pipeline.ID),
		)
		result.CompletedAt = time.Now()
		result.Error = &connector.ExecutionError{
			Code:    ErrCodeInvalidInput,
			Message: ErrNilInputModule.Error(),
			Module:  "input",
		}
		return result, ErrNilInputModule
	}

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

	// Ensure output module is properly closed after execution (resource cleanup)
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

	// Step 1: Execute Input module
	logger.Debug("executing input module",
		slog.String("pipeline_id", pipeline.ID),
		slog.String("stage", "input"),
	)

	inputStartTime := time.Now()
	records, err := e.inputModule.Fetch()
	inputDuration := time.Since(inputStartTime)

	if err != nil {
		logger.Error("input module execution failed",
			slog.String("pipeline_id", pipeline.ID),
			slog.String("module", "input"),
			slog.Duration("duration", inputDuration),
			slog.String("error", err.Error()),
		)
		result.CompletedAt = time.Now()
		result.Error = &connector.ExecutionError{
			Code:    ErrCodeInputFailed,
			Message: fmt.Sprintf("input module failed: %v", err),
			Module:  "input",
		}
		return result, fmt.Errorf("executing input module: %w", err)
	}

	logger.Debug("input module completed",
		slog.String("pipeline_id", pipeline.ID),
		slog.String("stage", "input"),
		slog.Int("records_fetched", len(records)),
		slog.Duration("duration", inputDuration),
	)

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

	// Step 3: Execute Output module (skip in dry-run mode)
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

	// Success
	result.Status = StatusSuccess
	result.RecordsProcessed = outputRes.recordsSent
	result.RecordsFailed = 0
	result.CompletedAt = time.Now()
	result.Error = nil

	totalDuration := time.Since(startedAt)

	// Log execution completion summary
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

	// Step 3: Execute Output module (skip in dry-run mode)
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
