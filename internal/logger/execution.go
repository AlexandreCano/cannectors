package logger

import (
	"log/slog"
	"time"
)

// ExecutionContext carries the structured fields shared by every log entry
// emitted during a pipeline execution. It is the single source of truth used
// by LogExecutionStart / LogStageStart / LogMetrics / ... — call sites build
// it once and reuse it for every helper.
//
// Only non-empty fields are emitted, so partially-populated values are safe
// (e.g. ModuleType + ModuleName are unset at the pipeline level but set per
// stage).
type ExecutionContext struct {
	TraceID      string // correlation ID shared by all logs of one execution
	PipelineID   string // required
	PipelineName string
	Stage        string // input | filter | output
	ModuleType   string
	ModuleName   string
	DryRun       bool
	FilterIndex  int
}

// ExecutionMetrics captures aggregated timings and record counters at the end
// of an execution.
type ExecutionMetrics struct {
	TotalDuration    time.Duration
	InputDuration    time.Duration
	FilterDuration   time.Duration
	OutputDuration   time.Duration
	RecordsProcessed int
	RecordsFailed    int
	RecordsPerSecond float64
	AvgRecordTime    time.Duration
}

// ExecutionError describes a stage-level failure passed to LogStageEnd. nil
// means the stage succeeded.
type ExecutionError struct {
	Code    string
	Message string
}

// buildContextAttrs flattens an ExecutionContext into a slice of slog
// attributes, skipping any zero-valued field.
func buildContextAttrs(ctx ExecutionContext) []any {
	attrs := make([]any, 0, 14)
	if ctx.TraceID != "" {
		attrs = append(attrs, slog.String(TraceIDField, ctx.TraceID))
	}
	attrs = append(attrs, slog.String("pipeline_id", ctx.PipelineID))
	if ctx.PipelineName != "" {
		attrs = append(attrs, slog.String("pipeline_name", ctx.PipelineName))
	}
	if ctx.Stage != "" {
		attrs = append(attrs, slog.String("stage", ctx.Stage))
	}
	if ctx.ModuleType != "" {
		attrs = append(attrs, slog.String("module_type", ctx.ModuleType))
	}
	if ctx.ModuleName != "" {
		attrs = append(attrs, slog.String("module_name", ctx.ModuleName))
	}
	if ctx.DryRun {
		attrs = append(attrs, slog.Bool("dry_run", true))
	}
	if ctx.FilterIndex > 0 {
		attrs = append(attrs, slog.Int("filter_index", ctx.FilterIndex))
	}
	return attrs
}

// LogExecutionStart emits the "execution started" log entry.
func LogExecutionStart(ctx ExecutionContext) {
	Logger.Info("execution started", buildContextAttrs(ctx)...)
}

// LogExecutionEnd emits the "execution completed" log entry.
func LogExecutionEnd(ctx ExecutionContext, status string, recordsProcessed int, duration time.Duration) {
	attrs := append(buildContextAttrs(ctx),
		slog.String("status", status),
		slog.Int("records_processed", recordsProcessed),
		slog.Duration("duration", duration),
	)
	Logger.Info("execution completed", attrs...)
}

// LogStageStart emits the "stage started" log entry.
func LogStageStart(ctx ExecutionContext) {
	Logger.Info("stage started", buildContextAttrs(ctx)...)
}

// LogStageEnd emits the stage completion log entry. Pass a non-nil err to log
// at error level with the failure code.
func LogStageEnd(ctx ExecutionContext, recordCount int, duration time.Duration, err *ExecutionError) {
	attrs := append(buildContextAttrs(ctx),
		slog.Int("record_count", recordCount),
		slog.Duration("duration", duration),
	)
	if err != nil {
		attrs = append(attrs,
			slog.String("error_code", err.Code),
			slog.String("error", err.Message),
		)
		Logger.Error("stage failed", attrs...)
		return
	}
	Logger.Info("stage completed", attrs...)
}

// LogMetrics emits the per-execution performance summary.
func LogMetrics(ctx ExecutionContext, metrics ExecutionMetrics) {
	attrs := append(buildContextAttrs(ctx),
		slog.Duration("total_duration", metrics.TotalDuration),
		slog.Duration("input_duration", metrics.InputDuration),
		slog.Duration("filter_duration", metrics.FilterDuration),
		slog.Duration("output_duration", metrics.OutputDuration),
		slog.Int("records_processed", metrics.RecordsProcessed),
		slog.Int("records_failed", metrics.RecordsFailed),
		slog.Float64("records_per_second", metrics.RecordsPerSecond),
		slog.Duration("avg_record_time", metrics.AvgRecordTime),
	)
	Logger.Info("execution metrics", attrs...)
}
