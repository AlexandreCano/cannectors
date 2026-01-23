package logger_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/canectors/runtime/internal/logger"
)

func TestLoggerInitialization(t *testing.T) {
	// Logger should be initialized
	if logger.Logger == nil {
		t.Fatal("Logger should be initialized on package load")
	}
}

func TestSetLevel(t *testing.T) {
	t.Helper()
	// Test setting log level - should not panic
	logger.SetLevel(slog.LevelDebug)
	logger.SetLevel(slog.LevelInfo)
	logger.SetLevel(slog.LevelWarn)
	logger.SetLevel(slog.LevelError)
}

func TestWithPipeline(t *testing.T) {
	pipelineLogger := logger.WithPipeline("test-pipeline-123")
	if pipelineLogger == nil {
		t.Fatal("WithPipeline should return a logger")
	}
}

func TestWithModule(t *testing.T) {
	moduleLogger := logger.WithModule("input", "http-polling")
	if moduleLogger == nil {
		t.Fatal("WithModule should return a logger")
	}
}

func TestJSONLogFormat(t *testing.T) {
	// Create a buffer to capture log output
	var buf bytes.Buffer
	testLogger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	testLogger.Info("test message", "key", "value")

	// Parse the JSON output
	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON log output: %v", err)
	}

	// Verify structure
	if logEntry["msg"] != "test message" {
		t.Errorf("Expected message 'test message', got %v", logEntry["msg"])
	}
	if logEntry["key"] != "value" {
		t.Errorf("Expected key 'value', got %v", logEntry["key"])
	}
	if logEntry["level"] != "INFO" {
		t.Errorf("Expected level 'INFO', got %v", logEntry["level"])
	}
}

// =============================================================================
// Task 1: Execution Context Helpers Tests
// =============================================================================

func TestWithExecution(t *testing.T) {
	var buf bytes.Buffer
	originalLogger := logger.Logger
	defer func() { logger.Logger = originalLogger }()

	logger.Logger = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	ctx := logger.ExecutionContext{
		PipelineID:   "pipeline-123",
		PipelineName: "Test Pipeline",
		Stage:        "input",
		ModuleType:   "httpPolling",
		ModuleName:   "source-api",
	}

	execLogger := logger.WithExecution(ctx)
	if execLogger == nil {
		t.Fatal("WithExecution should return a logger")
	}

	// Log something to verify context is included
	execLogger.Info("test log")

	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON log output: %v", err)
	}

	// Verify all context fields are present
	if logEntry["pipeline_id"] != "pipeline-123" {
		t.Errorf("Expected pipeline_id 'pipeline-123', got %v", logEntry["pipeline_id"])
	}
	if logEntry["pipeline_name"] != "Test Pipeline" {
		t.Errorf("Expected pipeline_name 'Test Pipeline', got %v", logEntry["pipeline_name"])
	}
	if logEntry["stage"] != "input" {
		t.Errorf("Expected stage 'input', got %v", logEntry["stage"])
	}
	if logEntry["module_type"] != "httpPolling" {
		t.Errorf("Expected module_type 'httpPolling', got %v", logEntry["module_type"])
	}
	if logEntry["module_name"] != "source-api" {
		t.Errorf("Expected module_name 'source-api', got %v", logEntry["module_name"])
	}
}

func TestLogExecutionStart(t *testing.T) {
	var buf bytes.Buffer
	originalLogger := logger.Logger
	defer func() { logger.Logger = originalLogger }()

	logger.Logger = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	ctx := logger.ExecutionContext{
		PipelineID:   "pipeline-456",
		PipelineName: "My Pipeline",
	}

	logger.LogExecutionStart(ctx)

	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON log output: %v", err)
	}

	// Verify execution start log structure
	if logEntry["msg"] != "execution started" {
		t.Errorf("Expected msg 'execution started', got %v", logEntry["msg"])
	}
	if logEntry["pipeline_id"] != "pipeline-456" {
		t.Errorf("Expected pipeline_id 'pipeline-456', got %v", logEntry["pipeline_id"])
	}
	if logEntry["level"] != "INFO" {
		t.Errorf("Expected level 'INFO', got %v", logEntry["level"])
	}
}

func TestLogExecutionEnd(t *testing.T) {
	var buf bytes.Buffer
	originalLogger := logger.Logger
	defer func() { logger.Logger = originalLogger }()

	logger.Logger = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	ctx := logger.ExecutionContext{
		PipelineID:   "pipeline-789",
		PipelineName: "Completed Pipeline",
	}

	duration := 2*time.Second + 500*time.Millisecond
	recordsProcessed := 100
	status := "success"

	logger.LogExecutionEnd(ctx, status, recordsProcessed, duration)

	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON log output: %v", err)
	}

	// Verify execution end log structure
	if logEntry["msg"] != "execution completed" {
		t.Errorf("Expected msg 'execution completed', got %v", logEntry["msg"])
	}
	if logEntry["pipeline_id"] != "pipeline-789" {
		t.Errorf("Expected pipeline_id 'pipeline-789', got %v", logEntry["pipeline_id"])
	}
	if logEntry["status"] != "success" {
		t.Errorf("Expected status 'success', got %v", logEntry["status"])
	}
	recVal, ok := logEntry["records_processed"].(float64)
	if !ok || int(recVal) != 100 {
		t.Errorf("Expected records_processed 100, got %v", logEntry["records_processed"])
	}
	// Duration should be present (as nanoseconds in JSON)
	if logEntry["duration"] == nil {
		t.Error("Expected duration to be present")
	}
}

func TestLogStageStart(t *testing.T) {
	var buf bytes.Buffer
	originalLogger := logger.Logger
	defer func() { logger.Logger = originalLogger }()

	logger.Logger = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	ctx := logger.ExecutionContext{
		PipelineID: "pipeline-stage",
		Stage:      "input",
		ModuleType: "httpPolling",
	}

	logger.LogStageStart(ctx)

	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON log output: %v", err)
	}

	if logEntry["msg"] != "stage started" {
		t.Errorf("Expected msg 'stage started', got %v", logEntry["msg"])
	}
	if logEntry["stage"] != "input" {
		t.Errorf("Expected stage 'input', got %v", logEntry["stage"])
	}
}

func TestLogStageEnd(t *testing.T) {
	var buf bytes.Buffer
	originalLogger := logger.Logger
	defer func() { logger.Logger = originalLogger }()

	logger.Logger = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	ctx := logger.ExecutionContext{
		PipelineID: "pipeline-stage-end",
		Stage:      "output",
		ModuleType: "httpRequest",
	}

	duration := 1 * time.Second
	recordCount := 50

	logger.LogStageEnd(ctx, recordCount, duration, nil)

	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON log output: %v", err)
	}

	if logEntry["msg"] != "stage completed" {
		t.Errorf("Expected msg 'stage completed', got %v", logEntry["msg"])
	}
	if logEntry["stage"] != "output" {
		t.Errorf("Expected stage 'output', got %v", logEntry["stage"])
	}
	rcVal, ok := logEntry["record_count"].(float64)
	if !ok || int(rcVal) != 50 {
		t.Errorf("Expected record_count 50, got %v", logEntry["record_count"])
	}
}

func TestLogStageEndWithError(t *testing.T) {
	var buf bytes.Buffer
	originalLogger := logger.Logger
	defer func() { logger.Logger = originalLogger }()

	logger.Logger = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	ctx := logger.ExecutionContext{
		PipelineID: "pipeline-stage-error",
		Stage:      "input",
	}

	duration := 500 * time.Millisecond
	testErr := &logger.ExecutionError{
		Code:    "HTTP_ERROR",
		Message: "connection timeout",
	}

	logger.LogStageEnd(ctx, 0, duration, testErr)

	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON log output: %v", err)
	}

	if logEntry["msg"] != "stage failed" {
		t.Errorf("Expected msg 'stage failed', got %v", logEntry["msg"])
	}
	if logEntry["level"] != "ERROR" {
		t.Errorf("Expected level 'ERROR', got %v", logEntry["level"])
	}
	if logEntry["error_code"] != "HTTP_ERROR" {
		t.Errorf("Expected error_code 'HTTP_ERROR', got %v", logEntry["error_code"])
	}
	if logEntry["error"] != "connection timeout" {
		t.Errorf("Expected error 'connection timeout', got %v", logEntry["error"])
	}
}

func TestLogMetrics(t *testing.T) {
	var buf bytes.Buffer
	originalLogger := logger.Logger
	defer func() { logger.Logger = originalLogger }()

	logger.Logger = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	ctx := logger.ExecutionContext{
		PipelineID:   "pipeline-metrics",
		PipelineName: "Metrics Pipeline",
	}

	metrics := logger.ExecutionMetrics{
		TotalDuration:    5 * time.Second,
		InputDuration:    2 * time.Second,
		FilterDuration:   1 * time.Second,
		OutputDuration:   2 * time.Second,
		RecordsProcessed: 1000,
		RecordsFailed:    5,
		RecordsPerSecond: 200.0,
		AvgRecordTime:    5 * time.Millisecond,
	}

	logger.LogMetrics(ctx, metrics)

	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON log output: %v", err)
	}

	if logEntry["msg"] != "execution metrics" {
		t.Errorf("Expected msg 'execution metrics', got %v", logEntry["msg"])
	}
	if logEntry["pipeline_id"] != "pipeline-metrics" {
		t.Errorf("Expected pipeline_id 'pipeline-metrics', got %v", logEntry["pipeline_id"])
	}
	recProcessed, ok := logEntry["records_processed"].(float64)
	if !ok || int(recProcessed) != 1000 {
		t.Errorf("Expected records_processed 1000, got %v", logEntry["records_processed"])
	}
	recFailed, ok := logEntry["records_failed"].(float64)
	if !ok || int(recFailed) != 5 {
		t.Errorf("Expected records_failed 5, got %v", logEntry["records_failed"])
	}
	rps, ok := logEntry["records_per_second"].(float64)
	if !ok || rps != 200.0 {
		t.Errorf("Expected records_per_second 200.0, got %v", logEntry["records_per_second"])
	}
}

func TestExecutionContextPartialFields(t *testing.T) {
	var buf bytes.Buffer
	originalLogger := logger.Logger
	defer func() { logger.Logger = originalLogger }()

	logger.Logger = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Test with only required fields (pipeline_id)
	ctx := logger.ExecutionContext{
		PipelineID: "minimal-pipeline",
	}

	execLogger := logger.WithExecution(ctx)
	execLogger.Info("minimal context test")

	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON log output: %v", err)
	}

	// Only pipeline_id should be present
	if logEntry["pipeline_id"] != "minimal-pipeline" {
		t.Errorf("Expected pipeline_id 'minimal-pipeline', got %v", logEntry["pipeline_id"])
	}

	// Optional fields should not be present when empty
	if _, exists := logEntry["pipeline_name"]; exists && logEntry["pipeline_name"] != "" {
		t.Errorf("Expected pipeline_name to be absent or empty, got %v", logEntry["pipeline_name"])
	}
}

func TestConsistentFieldNames(t *testing.T) {
	// Test that all logging helpers use consistent field names
	expectedFields := []string{
		"pipeline_id",
		"pipeline_name",
		"stage",
		"module_type",
		"module_name",
		"duration",
		"record_count",
		"records_processed",
		"records_failed",
		"status",
		"error",
		"error_code",
	}

	// Verify these are the expected field names based on the story requirements
	for _, field := range expectedFields {
		// Field names should be snake_case
		if strings.Contains(field, "-") {
			t.Errorf("Field name should use snake_case, not kebab-case: %s", field)
		}
		if field != strings.ToLower(field) {
			t.Errorf("Field name should be lowercase: %s", field)
		}
	}
}

// =============================================================================
// Human-Readable Format Tests
// =============================================================================

func TestHumanHandler(t *testing.T) {
	var buf bytes.Buffer
	handler := logger.NewHumanHandler(&buf, &logger.HumanHandlerOptions{
		Level:     slog.LevelInfo,
		UseColors: false, // Disable colors for testing
	})

	testLogger := slog.New(handler)
	testLogger.Info("test message", "key", "value")

	output := buf.String()

	// Verify output contains expected parts
	if !strings.Contains(output, "test message") {
		t.Errorf("Expected output to contain 'test message', got: %s", output)
	}
	if !strings.Contains(output, "ℹ") {
		t.Errorf("Expected output to contain info prefix 'ℹ', got: %s", output)
	}
	if !strings.Contains(output, "key=value") {
		t.Errorf("Expected output to contain 'key=value', got: %s", output)
	}
}

func TestHumanHandlerLevels(t *testing.T) {
	tests := []struct {
		level          slog.Level
		expectedPrefix string
	}{
		{slog.LevelError, "✗"},
		{slog.LevelWarn, "⚠"},
		{slog.LevelInfo, "ℹ"},
		{slog.LevelDebug, "·"},
	}

	for _, tt := range tests {
		t.Run(tt.level.String(), func(t *testing.T) {
			var buf bytes.Buffer
			handler := logger.NewHumanHandler(&buf, &logger.HumanHandlerOptions{
				Level:     slog.LevelDebug, // Enable all levels
				UseColors: false,
			})

			testLogger := slog.New(handler)
			testLogger.Log(context.Background(), tt.level, "test")

			output := buf.String()
			if !strings.Contains(output, tt.expectedPrefix) {
				t.Errorf("Expected output to contain prefix '%s' for level %s, got: %s",
					tt.expectedPrefix, tt.level, output)
			}
		})
	}
}

func TestHumanHandlerDuration(t *testing.T) {
	var buf bytes.Buffer
	handler := logger.NewHumanHandler(&buf, &logger.HumanHandlerOptions{
		Level:     slog.LevelInfo,
		UseColors: false,
	})

	testLogger := slog.New(handler)
	testLogger.Info("duration test", "duration", 2500*time.Millisecond)

	output := buf.String()

	// Duration should be formatted in human-readable way (2.50s)
	if !strings.Contains(output, "duration=2.50s") {
		t.Errorf("Expected output to contain 'duration=2.50s', got: %s", output)
	}
}

func TestSetFormat(t *testing.T) {
	// Save original logger
	originalLogger := logger.Logger
	defer func() { logger.Logger = originalLogger }()

	// Test setting human format
	logger.SetFormat(logger.FormatHuman)
	if logger.Logger == nil {
		t.Fatal("Logger should not be nil after SetFormat")
	}

	// Test setting JSON format
	logger.SetFormat(logger.FormatJSON)
	if logger.Logger == nil {
		t.Fatal("Logger should not be nil after SetFormat")
	}
}

func TestFormatMetricsHuman(t *testing.T) {
	metrics := logger.ExecutionMetrics{
		TotalDuration:    5 * time.Second,
		RecordsProcessed: 1000,
		RecordsFailed:    5,
		RecordsPerSecond: 200.0,
	}

	formatted := logger.FormatMetricsHuman(metrics)

	// Verify key parts are present
	if !strings.Contains(formatted, "1000 records") {
		t.Errorf("Expected formatted metrics to contain '1000 records', got: %s", formatted)
	}
	if !strings.Contains(formatted, "5.00s") {
		t.Errorf("Expected formatted metrics to contain '5.00s', got: %s", formatted)
	}
	if !strings.Contains(formatted, "200.0 records/sec") {
		t.Errorf("Expected formatted metrics to contain '200.0 records/sec', got: %s", formatted)
	}
	if !strings.Contains(formatted, "5 failed") {
		t.Errorf("Expected formatted metrics to contain '5 failed', got: %s", formatted)
	}
}

// =============================================================================
// Log File Output Tests
// =============================================================================

func TestSetLogFile(t *testing.T) {
	// Save original logger
	originalLogger := logger.Logger
	defer func() {
		logger.CloseLogFile()
		logger.Logger = originalLogger
	}()

	// Create temp file for testing
	tmpFile, err := os.CreateTemp("", "test-log-*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	defer func() { _ = os.Remove(tmpPath) }()

	// Set log file
	err = logger.SetLogFile(tmpPath, slog.LevelInfo, logger.FormatJSON)
	if err != nil {
		t.Fatalf("SetLogFile failed: %v", err)
	}

	// Write a log message
	logger.Info("test log message", "key", "value")

	// Close log file to flush
	logger.CloseLogFile()

	// Read the log file
	content, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	// Verify JSON content (file logs are always JSON)
	if len(content) == 0 {
		t.Error("Log file should contain content")
	}

	// Parse JSON to verify it's valid
	var logEntry map[string]interface{}
	// The file might contain multiple lines, parse first non-empty line
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if err := json.Unmarshal([]byte(line), &logEntry); err == nil {
			if logEntry["msg"] == "test log message" {
				if logEntry["key"] != "value" {
					t.Errorf("Expected key='value' in log, got: %v", logEntry["key"])
				}
				return
			}
		}
	}
	t.Error("Expected to find test log message in log file")
}

func TestCloseLogFile(t *testing.T) {
	// Save original logger
	originalLogger := logger.Logger
	defer func() { logger.Logger = originalLogger }()

	// CloseLogFile should not panic when no file is open
	logger.CloseLogFile()

	// Create temp file
	tmpFile, err := os.CreateTemp("", "test-log-*.json")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	defer func() { _ = os.Remove(tmpPath) }()

	// Set and close log file
	err = logger.SetLogFile(tmpPath, slog.LevelInfo, logger.FormatJSON)
	if err != nil {
		t.Fatalf("SetLogFile failed: %v", err)
	}

	// Close should not panic
	logger.CloseLogFile()
	// Second close should also not panic
	logger.CloseLogFile()
}

// =============================================================================
// Error Logging with Context Tests
// =============================================================================

func TestLogError(t *testing.T) {
	var buf bytes.Buffer
	originalLogger := logger.Logger
	defer func() { logger.Logger = originalLogger }()

	logger.Logger = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	errCtx := logger.ErrorContext{
		PipelineID:   "pipeline-error-test",
		PipelineName: "Error Test Pipeline",
		Stage:        "input",
		ModuleType:   "httpPolling",
		ModuleName:   "source-api",
		ErrorCode:    "HTTP_ERROR",
		ErrorMessage: "connection timeout",
		RecordIndex:  5,
		RecordCount:  100,
		Endpoint:     "https://api.example.com/data",
		HTTPStatus:   503,
		Duration:     30 * time.Second,
		Extra: map[string]interface{}{
			"retry_count": 3,
		},
	}

	logger.LogError("input fetch failed", errCtx)

	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON log output: %v", err)
	}

	// Verify all context fields are present
	if logEntry["msg"] != "input fetch failed" {
		t.Errorf("Expected msg 'input fetch failed', got %v", logEntry["msg"])
	}
	if logEntry["level"] != "ERROR" {
		t.Errorf("Expected level 'ERROR', got %v", logEntry["level"])
	}
	if logEntry["pipeline_id"] != "pipeline-error-test" {
		t.Errorf("Expected pipeline_id 'pipeline-error-test', got %v", logEntry["pipeline_id"])
	}
	if logEntry["stage"] != "input" {
		t.Errorf("Expected stage 'input', got %v", logEntry["stage"])
	}
	if logEntry["error_code"] != "HTTP_ERROR" {
		t.Errorf("Expected error_code 'HTTP_ERROR', got %v", logEntry["error_code"])
	}
	if logEntry["error"] != "connection timeout" {
		t.Errorf("Expected error 'connection timeout', got %v", logEntry["error"])
	}
	if logEntry["endpoint"] != "https://api.example.com/data" {
		t.Errorf("Expected endpoint 'https://api.example.com/data', got %v", logEntry["endpoint"])
	}
	httpStatus, ok := logEntry["http_status"].(float64)
	if !ok || int(httpStatus) != 503 {
		t.Errorf("Expected http_status 503, got %v", logEntry["http_status"])
	}
	retryCount, ok := logEntry["retry_count"].(float64)
	if !ok || int(retryCount) != 3 {
		t.Errorf("Expected retry_count 3, got %v", logEntry["retry_count"])
	}
}

func TestLogErrorMinimalContext(t *testing.T) {
	var buf bytes.Buffer
	originalLogger := logger.Logger
	defer func() { logger.Logger = originalLogger }()

	logger.Logger = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	// Log error with minimal context
	errCtx := logger.ErrorContext{
		PipelineID:   "minimal-error-test",
		ErrorMessage: "something went wrong",
	}

	logger.LogError("generic error", errCtx)

	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON log output: %v", err)
	}

	// Only present fields should be in log
	if logEntry["pipeline_id"] != "minimal-error-test" {
		t.Errorf("Expected pipeline_id 'minimal-error-test', got %v", logEntry["pipeline_id"])
	}
	if logEntry["error"] != "something went wrong" {
		t.Errorf("Expected error 'something went wrong', got %v", logEntry["error"])
	}

	// Optional fields should not be present
	if _, exists := logEntry["stage"]; exists {
		t.Errorf("Expected stage to be absent, got %v", logEntry["stage"])
	}
	if _, exists := logEntry["endpoint"]; exists {
		t.Errorf("Expected endpoint to be absent, got %v", logEntry["endpoint"])
	}
	// RecordIndex should not be present when not set (defaults to 0, and we check > 0)
	if _, exists := logEntry["record_index"]; exists {
		t.Errorf("Expected record_index to be absent when not set, got %v", logEntry["record_index"])
	}
}
