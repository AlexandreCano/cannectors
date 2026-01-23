// Package logger provides structured logging functionality.
// It wraps the standard log/slog package for consistent logging across the runtime.
//
// This package provides execution context helpers for consistent pipeline logging,
// including helpers for execution start/end, stage start/end, and metrics logging.
// All helpers use structured logging with consistent field names (snake_case).
//
// The package supports two output formats:
//   - JSON (default): Machine-readable structured logging
//   - Human: Human-readable console output with colors and prefixes
package logger

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"
)

// Logger is the default logger instance.
var Logger *slog.Logger

func init() {
	// Initialize with JSON handler for structured logging
	Logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

// SetLevel configures the logging level.
func SetLevel(level slog.Level) {
	Logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
}

// Info logs an informational message.
func Info(msg string, args ...any) {
	Logger.Info(msg, args...)
}

// Debug logs a debug message.
func Debug(msg string, args ...any) {
	Logger.Debug(msg, args...)
}

// Warn logs a warning message.
func Warn(msg string, args ...any) {
	Logger.Warn(msg, args...)
}

// Error logs an error message.
func Error(msg string, args ...any) {
	Logger.Error(msg, args...)
}

// WithPipeline returns a logger with pipeline context.
func WithPipeline(pipelineID string) *slog.Logger {
	return Logger.With("pipelineId", pipelineID)
}

// WithModule returns a logger with module context.
func WithModule(moduleType string, moduleName string) *slog.Logger {
	return Logger.With("moduleType", moduleType, "moduleName", moduleName)
}

// =============================================================================
// Execution Context Types
// =============================================================================

// ExecutionContext contains context information for pipeline execution logging.
// Use this struct with WithExecution() and other execution logging helpers.
type ExecutionContext struct {
	// PipelineID is the unique identifier for the pipeline (required)
	PipelineID string
	// PipelineName is the human-readable name of the pipeline
	PipelineName string
	// Stage is the current execution stage (input, filter, output)
	Stage string
	// ModuleType is the type of module being executed (httpPolling, mapping, etc.)
	ModuleType string
	// ModuleName is the name/identifier of the module
	ModuleName string
	// DryRun indicates if this is a dry-run execution
	DryRun bool
	// FilterIndex is the index of the current filter (for filter stage)
	FilterIndex int
}

// ExecutionError contains structured error information for logging.
type ExecutionError struct {
	// Code is the error code (e.g., HTTP_ERROR, INPUT_FAILED)
	Code string
	// Message is the human-readable error message
	Message string
	// Details contains additional error context
	Details map[string]interface{}
}

// ErrorContext contains structured context for error logging.
// Use this with LogError() for consistent, actionable error logs.
type ErrorContext struct {
	// Execution context (inherited from ExecutionContext)
	PipelineID   string
	PipelineName string
	Stage        string // input, filter, output
	ModuleType   string
	ModuleName   string

	// Error details
	ErrorCode    string
	ErrorMessage string
	Err          error // underlying error (for stack trace/chain)

	// Contextual information
	RecordIndex int
	RecordCount int
	Endpoint    string
	HTTPStatus  int
	Duration    time.Duration

	// Additional context as key-value pairs
	Extra map[string]interface{}
}

// ExecutionMetrics contains performance metrics for execution logging.
type ExecutionMetrics struct {
	// TotalDuration is the total execution time
	TotalDuration time.Duration
	// InputDuration is the time spent in the input stage
	InputDuration time.Duration
	// FilterDuration is the total time spent in all filter stages
	FilterDuration time.Duration
	// OutputDuration is the time spent in the output stage
	OutputDuration time.Duration
	// RecordsProcessed is the number of records successfully processed
	RecordsProcessed int
	// RecordsFailed is the number of records that failed processing
	RecordsFailed int
	// RecordsPerSecond is the throughput (records per second)
	RecordsPerSecond float64
	// AvgRecordTime is the average processing time per record
	AvgRecordTime time.Duration
}

// =============================================================================
// Execution Context Helpers
// =============================================================================

// WithExecution returns a logger with execution context attached.
// This creates a logger with all relevant context fields for pipeline execution.
// Only non-empty fields are included in the log output.
func WithExecution(ctx ExecutionContext) *slog.Logger {
	attrs := make([]any, 0, 12)

	// Always include pipeline_id (required)
	attrs = append(attrs, slog.String("pipeline_id", ctx.PipelineID))

	// Include optional fields only if non-empty
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
	if ctx.FilterIndex >= 0 {
		attrs = append(attrs, slog.Int("filter_index", ctx.FilterIndex))
	}

	return Logger.With(attrs...)
}

// LogExecutionStart logs the start of a pipeline execution.
// This should be called at the beginning of Execute().
func LogExecutionStart(ctx ExecutionContext) {
	attrs := buildContextAttrs(ctx)
	Logger.Info("execution started", attrs...)
}

// LogExecutionEnd logs the completion of a pipeline execution.
// This should be called at the end of Execute() with the final status and metrics.
func LogExecutionEnd(ctx ExecutionContext, status string, recordsProcessed int, duration time.Duration) {
	attrs := buildContextAttrs(ctx)
	attrs = append(attrs,
		slog.String("status", status),
		slog.Int("records_processed", recordsProcessed),
		slog.Duration("duration", duration),
	)
	Logger.Info("execution completed", attrs...)
}

// LogStageStart logs the start of a pipeline stage (input, filter, output).
// This provides visibility into stage-level progress.
func LogStageStart(ctx ExecutionContext) {
	attrs := buildContextAttrs(ctx)
	Logger.Info("stage started", attrs...)
}

// LogStageEnd logs the completion of a pipeline stage.
// If err is non-nil, logs as an error with error details.
func LogStageEnd(ctx ExecutionContext, recordCount int, duration time.Duration, err *ExecutionError) {
	attrs := buildContextAttrs(ctx)
	attrs = append(attrs,
		slog.Int("record_count", recordCount),
		slog.Duration("duration", duration),
	)

	if err != nil {
		attrs = append(attrs,
			slog.String("error_code", err.Code),
			slog.String("error", err.Message),
		)
		Logger.Error("stage failed", attrs...)
	} else {
		Logger.Info("stage completed", attrs...)
	}
}

// LogMetrics logs execution performance metrics.
// This should be called after execution completion with collected metrics.
func LogMetrics(ctx ExecutionContext, metrics ExecutionMetrics) {
	attrs := buildContextAttrs(ctx)
	attrs = append(attrs,
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

// LogError logs an error with full execution context.
// This ensures all error logs are actionable and include sufficient context for debugging (NFR32).
func LogError(message string, errCtx ErrorContext) {
	attrs := make([]any, 0, 20)

	// Always include pipeline context
	if errCtx.PipelineID != "" {
		attrs = append(attrs, slog.String("pipeline_id", errCtx.PipelineID))
	}
	if errCtx.PipelineName != "" {
		attrs = append(attrs, slog.String("pipeline_name", errCtx.PipelineName))
	}

	// Stage and module info
	if errCtx.Stage != "" {
		attrs = append(attrs, slog.String("stage", errCtx.Stage))
	}
	if errCtx.ModuleType != "" {
		attrs = append(attrs, slog.String("module_type", errCtx.ModuleType))
	}
	if errCtx.ModuleName != "" {
		attrs = append(attrs, slog.String("module_name", errCtx.ModuleName))
	}

	// Error details
	if errCtx.ErrorCode != "" {
		attrs = append(attrs, slog.String("error_code", errCtx.ErrorCode))
	}
	if errCtx.ErrorMessage != "" {
		attrs = append(attrs, slog.String("error", errCtx.ErrorMessage))
	}
	if errCtx.Err != nil {
		// Include error chain/type for debugging
		attrs = append(attrs, slog.String("error_type", fmt.Sprintf("%T", errCtx.Err)))

		// Include error chain (Unwrap) if available
		errorChain := []string{errCtx.Err.Error()}
		currentErr := errCtx.Err
		for {
			unwrapped := errors.Unwrap(currentErr)
			if unwrapped == nil {
				break
			}
			errorChain = append(errorChain, unwrapped.Error())
			currentErr = unwrapped
		}
		if len(errorChain) > 1 {
			attrs = append(attrs, slog.String("error_chain", strings.Join(errorChain, " -> ")))
		}
	}

	// Contextual information
	if errCtx.RecordIndex >= 0 {
		attrs = append(attrs, slog.Int("record_index", errCtx.RecordIndex))
	}
	if errCtx.RecordCount > 0 {
		attrs = append(attrs, slog.Int("record_count", errCtx.RecordCount))
	}
	if errCtx.Endpoint != "" {
		attrs = append(attrs, slog.String("endpoint", errCtx.Endpoint))
	}
	if errCtx.HTTPStatus > 0 {
		attrs = append(attrs, slog.Int("http_status", errCtx.HTTPStatus))
	}
	if errCtx.Duration > 0 {
		attrs = append(attrs, slog.Duration("duration", errCtx.Duration))
	}

	// Extra context
	for k, v := range errCtx.Extra {
		attrs = append(attrs, slog.Any(k, v))
	}

	Logger.Error(message, attrs...)
}

// buildContextAttrs builds a slice of slog attributes from an ExecutionContext.
// Only non-empty fields are included.
func buildContextAttrs(ctx ExecutionContext) []any {
	attrs := make([]any, 0, 12)

	// Always include pipeline_id
	attrs = append(attrs, slog.String("pipeline_id", ctx.PipelineID))

	// Include optional fields only if non-empty
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
	if ctx.FilterIndex >= 0 {
		attrs = append(attrs, slog.Int("filter_index", ctx.FilterIndex))
	}

	return attrs
}

// =============================================================================
// Human-Readable Log Format Support
// =============================================================================

// OutputFormat represents the log output format
type OutputFormat int

const (
	// FormatJSON is the default machine-readable JSON format
	FormatJSON OutputFormat = iota
	// FormatHuman is a human-readable console format with colors and prefixes
	FormatHuman
)

// SetFormat sets the log output format.
// FormatJSON uses structured JSON output (default, machine-readable)
// FormatHuman uses human-readable console output with colors and prefixes
func SetFormat(format OutputFormat) {
	switch format {
	case FormatHuman:
		Logger = slog.New(NewHumanHandler(os.Stdout, &HumanHandlerOptions{
			Level:     slog.LevelInfo,
			UseColors: isTerminal(os.Stdout),
		}))
	default:
		Logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}))
	}
}

// SetLevelAndFormat sets both the log level and format.
func SetLevelAndFormat(level slog.Level, format OutputFormat) {
	switch format {
	case FormatHuman:
		Logger = slog.New(NewHumanHandler(os.Stdout, &HumanHandlerOptions{
			Level:     level,
			UseColors: isTerminal(os.Stdout),
		}))
	default:
		Logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		}))
	}
}

// isTerminal returns true if the writer is a terminal (supports colors)
func isTerminal(w io.Writer) bool {
	if f, ok := w.(*os.File); ok {
		fi, err := f.Stat()
		if err != nil {
			return false
		}
		return (fi.Mode() & os.ModeCharDevice) != 0
	}
	return false
}

// HumanHandlerOptions configures the human-readable log handler.
type HumanHandlerOptions struct {
	// Level is the minimum log level to output
	Level slog.Level
	// UseColors enables ANSI color codes (auto-detected by default)
	UseColors bool
}

// HumanHandler is a slog handler that outputs human-readable log messages.
type HumanHandler struct {
	opts   HumanHandlerOptions
	writer io.Writer
	attrs  []slog.Attr
	groups []string
}

// NewHumanHandler creates a new human-readable log handler.
func NewHumanHandler(w io.Writer, opts *HumanHandlerOptions) *HumanHandler {
	if opts == nil {
		opts = &HumanHandlerOptions{Level: slog.LevelInfo}
	}
	return &HumanHandler{
		opts:   *opts,
		writer: w,
	}
}

// Enabled returns true if the handler is enabled for the given level.
func (h *HumanHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.opts.Level
}

// Handle outputs a log record in human-readable format.
func (h *HumanHandler) Handle(_ context.Context, r slog.Record) error {
	// Build the log line
	var sb strings.Builder

	// Timestamp in readable format
	timestamp := r.Time.Format("15:04:05")
	sb.WriteString(timestamp)
	sb.WriteString(" ")

	// Level prefix with optional color (use ✓ for success messages)
	prefix := h.levelPrefixWithMessage(r.Level, r.Message)
	sb.WriteString(prefix)
	sb.WriteString(" ")

	// Message
	sb.WriteString(r.Message)

	// Collect key attributes for inline display
	var keyAttrs []string
	r.Attrs(func(a slog.Attr) bool {
		keyAttrs = append(keyAttrs, h.formatAttr(a))
		return true
	})

	// Add pre-stored attrs
	for _, a := range h.attrs {
		keyAttrs = append(keyAttrs, h.formatAttr(a))
	}

	// Append important attributes inline (up to 5)
	if len(keyAttrs) > 0 {
		sb.WriteString(" ")
		maxInline := 5
		if len(keyAttrs) < maxInline {
			maxInline = len(keyAttrs)
		}
		sb.WriteString(strings.Join(keyAttrs[:maxInline], " "))
		if len(keyAttrs) > 5 {
			sb.WriteString(fmt.Sprintf(" (+%d more)", len(keyAttrs)-5))
		}
	}

	sb.WriteString("\n")
	_, err := h.writer.Write([]byte(sb.String()))
	return err
}

// WithAttrs returns a new handler with the given attributes added.
func (h *HumanHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandler := &HumanHandler{
		opts:   h.opts,
		writer: h.writer,
		attrs:  make([]slog.Attr, len(h.attrs)+len(attrs)),
		groups: h.groups,
	}
	copy(newHandler.attrs, h.attrs)
	copy(newHandler.attrs[len(h.attrs):], attrs)
	return newHandler
}

// WithGroup returns a new handler with the given group name.
func (h *HumanHandler) WithGroup(name string) slog.Handler {
	newHandler := &HumanHandler{
		opts:   h.opts,
		writer: h.writer,
		attrs:  h.attrs,
		groups: append(h.groups, name),
	}
	return newHandler
}

// levelPrefixWithMessage returns a human-readable prefix for the log level, using ✓ for success messages.
func (h *HumanHandler) levelPrefixWithMessage(level slog.Level, message string) string {
	// Check if message indicates success
	isSuccess := strings.Contains(strings.ToLower(message), "completed") ||
		strings.Contains(strings.ToLower(message), "succeeded") ||
		strings.Contains(strings.ToLower(message), "success") ||
		strings.Contains(strings.ToLower(message), "execution completed") ||
		strings.Contains(strings.ToLower(message), "stage completed")

	// ANSI color codes
	const (
		colorReset  = "\033[0m"
		colorRed    = "\033[31m"
		colorYellow = "\033[33m"
		colorGreen  = "\033[32m"
		colorCyan   = "\033[36m"
	)

	var prefix, color string
	switch {
	case level >= slog.LevelError:
		prefix = "✗"
		color = colorRed
	case level >= slog.LevelWarn:
		prefix = "⚠"
		color = colorYellow
	case level >= slog.LevelInfo:
		if isSuccess {
			prefix = "✓"
			color = colorGreen
		} else {
			prefix = "ℹ"
			color = colorCyan
		}
	default:
		prefix = "·"
		color = colorReset
	}

	if h.opts.UseColors {
		return color + prefix + colorReset
	}
	return prefix
}

// formatAttr formats a single attribute for display.
func (h *HumanHandler) formatAttr(a slog.Attr) string {
	key := a.Key
	value := a.Value.Any()

	// Format durations in human-readable way
	if d, ok := value.(time.Duration); ok {
		return fmt.Sprintf("%s=%s", key, formatDuration(d))
	}

	// Format floats with limited precision
	if f, ok := value.(float64); ok {
		return fmt.Sprintf("%s=%.2f", key, f)
	}

	return fmt.Sprintf("%s=%v", key, value)
}

// formatDuration formats a duration in a human-readable way.
func formatDuration(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
	return fmt.Sprintf("%.1fm", d.Minutes())
}

// FormatMetricsHuman formats execution metrics in a human-readable way.
func FormatMetricsHuman(metrics ExecutionMetrics) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Processed %d records in %s",
		metrics.RecordsProcessed,
		formatDuration(metrics.TotalDuration)))

	if metrics.RecordsPerSecond > 0 {
		sb.WriteString(fmt.Sprintf(" (%.1f records/sec)", metrics.RecordsPerSecond))
	}

	if metrics.RecordsFailed > 0 {
		sb.WriteString(fmt.Sprintf(", %d failed", metrics.RecordsFailed))
	}

	return sb.String()
}

// =============================================================================
// Log File Output Support
// =============================================================================

// logFile holds the currently open log file (if any)
var logFile *os.File

const (
	// maxLogFileSize is the maximum size of a log file before rotation (10MB)
	maxLogFileSize = 10 * 1024 * 1024
)

// rotateLogFile rotates the log file if it exceeds the maximum size.
// It renames the current file with a timestamp suffix and creates a new file.
func rotateLogFile(path string) error {
	// Check if file exists and get its size
	info, err := os.Stat(path)
	if err != nil {
		// File doesn't exist, no rotation needed
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("checking log file size: %w", err)
	}

	// Rotate if file exceeds maximum size
	if info.Size() >= maxLogFileSize {
		// Generate rotated filename with timestamp
		timestamp := time.Now().Format("20060102-150405")
		rotatedPath := fmt.Sprintf("%s.%s", path, timestamp)

		// Rename current file to rotated filename
		if err := os.Rename(path, rotatedPath); err != nil {
			return fmt.Errorf("rotating log file: %w", err)
		}
	}

	return nil
}

// SetLogFile configures logging to write to both stdout and the specified file.
// File logs are always in JSON format (machine-readable).
// Basic log rotation is performed if the file exceeds 10MB (renamed with timestamp).
// Returns an error if the file cannot be opened/created.
func SetLogFile(path string, level slog.Level, consoleFormat OutputFormat) error {
	// Close any existing log file
	CloseLogFile()

	// Rotate log file if it exceeds maximum size
	if err := rotateLogFile(path); err != nil {
		// Log rotation error but continue (non-fatal)
		Warn("log rotation failed", slog.String("error", err.Error()))
	}

	// Open/create the log file
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("opening log file: %w", err)
	}
	logFile = f

	// Create file handler (always JSON)
	fileHandler := slog.NewJSONHandler(f, &slog.HandlerOptions{
		Level: level,
	})

	// Create console handler based on format
	var consoleHandler slog.Handler
	switch consoleFormat {
	case FormatHuman:
		consoleHandler = NewHumanHandler(os.Stdout, &HumanHandlerOptions{
			Level:     level,
			UseColors: isTerminal(os.Stdout),
		})
	default:
		consoleHandler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		})
	}

	// Create a combined handler that writes to both
	Logger = slog.New(&dualHandler{
		console: consoleHandler,
		file:    fileHandler,
	})

	Info("log file opened",
		slog.String("path", path),
		slog.String("console_format", formatName(consoleFormat)),
	)

	return nil
}

// CloseLogFile closes the current log file if one is open.
func CloseLogFile() {
	if logFile != nil {
		if err := logFile.Sync(); err != nil {
			Warn("failed to sync log file", slog.String("error", err.Error()))
		}
		if err := logFile.Close(); err != nil {
			Warn("failed to close log file", slog.String("error", err.Error()))
		}
		logFile = nil
	}
}

// formatName returns the name of the output format.
func formatName(f OutputFormat) string {
	switch f {
	case FormatHuman:
		return "human"
	default:
		return "json"
	}
}

// dualHandler is a slog.Handler that writes to both console and file handlers.
type dualHandler struct {
	console slog.Handler
	file    slog.Handler
}

func (d *dualHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return d.console.Enabled(ctx, level) || d.file.Enabled(ctx, level)
}

func (d *dualHandler) Handle(ctx context.Context, r slog.Record) error {
	// Write to console
	if d.console.Enabled(ctx, r.Level) {
		if err := d.console.Handle(ctx, r); err != nil {
			return err
		}
	}
	// Write to file
	if d.file.Enabled(ctx, r.Level) {
		if err := d.file.Handle(ctx, r); err != nil {
			return err
		}
	}
	return nil
}

func (d *dualHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &dualHandler{
		console: d.console.WithAttrs(attrs),
		file:    d.file.WithAttrs(attrs),
	}
}

func (d *dualHandler) WithGroup(name string) slog.Handler {
	return &dualHandler{
		console: d.console.WithGroup(name),
		file:    d.file.WithGroup(name),
	}
}
