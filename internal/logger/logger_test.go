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

	"github.com/cannectors/runtime/internal/logger"
)

func TestLoggerInitialization(t *testing.T) {
	if logger.Logger == nil {
		t.Fatal("Logger should be initialized on package load")
	}
}

func TestSetLevelAndFormat(t *testing.T) {
	original := logger.Logger
	defer func() { logger.Logger = original }()

	logger.SetLevelAndFormat(slog.LevelDebug, logger.FormatJSON)
	if logger.Logger == nil {
		t.Fatal("Logger should not be nil after SetLevelAndFormat")
	}
	logger.SetLevelAndFormat(slog.LevelInfo, logger.FormatHuman)
	if logger.Logger == nil {
		t.Fatal("Logger should not be nil after switching format")
	}
}

func TestJSONLogFormat(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	original := logger.Logger
	logger.Logger = handler
	defer func() { logger.Logger = original }()

	logger.Info("info message", "key", "value")

	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &entry); err != nil {
		t.Fatalf("expected valid JSON, got %q: %v", buf.String(), err)
	}
	if entry["msg"] != "info message" {
		t.Errorf("msg = %v, want \"info message\"", entry["msg"])
	}
	if entry["key"] != "value" {
		t.Errorf("key = %v, want \"value\"", entry["key"])
	}
}

// captureLogger swaps logger.Logger for one writing to buf and returns a
// restore func.
func captureLogger(t *testing.T) (*bytes.Buffer, func()) {
	t.Helper()
	var buf bytes.Buffer
	original := logger.Logger
	logger.Logger = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return &buf, func() { logger.Logger = original }
}

// firstEntry parses and returns the first non-empty JSON log line in raw.
func firstEntry(t *testing.T, raw string) map[string]any {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("invalid JSON %q: %v", line, err)
		}
		return entry
	}
	t.Fatalf("no log entries in %q", raw)
	return nil
}

func TestLogExecutionStart(t *testing.T) {
	buf, restore := captureLogger(t)
	defer restore()

	logger.LogExecutionStart(logger.ExecutionContext{
		TraceID:      "trace-1",
		PipelineID:   "pipeline-123",
		PipelineName: "Demo",
		DryRun:       true,
	})
	entry := firstEntry(t, buf.String())
	if entry["msg"] != "execution started" {
		t.Errorf("msg = %v", entry["msg"])
	}
	if entry["trace_id"] != "trace-1" {
		t.Errorf("trace_id = %v", entry["trace_id"])
	}
	if entry["pipeline_id"] != "pipeline-123" {
		t.Errorf("pipeline_id = %v", entry["pipeline_id"])
	}
	if entry["dry_run"] != true {
		t.Errorf("dry_run = %v", entry["dry_run"])
	}
}

func TestLogExecutionEnd(t *testing.T) {
	buf, restore := captureLogger(t)
	defer restore()

	logger.LogExecutionEnd(logger.ExecutionContext{PipelineID: "p"}, "success", 42, 5*time.Second)
	entry := firstEntry(t, buf.String())
	if entry["msg"] != "execution completed" {
		t.Errorf("msg = %v", entry["msg"])
	}
	if entry["status"] != "success" {
		t.Errorf("status = %v", entry["status"])
	}
	if got, _ := entry["records_processed"].(float64); int(got) != 42 {
		t.Errorf("records_processed = %v", entry["records_processed"])
	}
}

func TestLogStageStartEnd(t *testing.T) {
	buf, restore := captureLogger(t)
	defer restore()

	ctx := logger.ExecutionContext{PipelineID: "p", Stage: "input"}
	logger.LogStageStart(ctx)
	logger.LogStageEnd(ctx, 10, time.Second, nil)
	logger.LogStageEnd(ctx, 0, time.Second, &logger.ExecutionError{Code: "X", Message: "boom"})

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 log lines, got %d: %q", len(lines), buf.String())
	}

	var start, ok, fail map[string]any
	_ = json.Unmarshal([]byte(lines[0]), &start)
	_ = json.Unmarshal([]byte(lines[1]), &ok)
	_ = json.Unmarshal([]byte(lines[2]), &fail)

	if start["msg"] != "stage started" || start["stage"] != "input" {
		t.Errorf("stage start mismatch: %v", start)
	}
	if ok["msg"] != "stage completed" {
		t.Errorf("stage success mismatch: %v", ok)
	}
	if fail["msg"] != "stage failed" || fail["error_code"] != "X" || fail["error"] != "boom" {
		t.Errorf("stage failure mismatch: %v", fail)
	}
}

func TestLogMetrics(t *testing.T) {
	buf, restore := captureLogger(t)
	defer restore()

	logger.LogMetrics(logger.ExecutionContext{PipelineID: "p"}, logger.ExecutionMetrics{
		TotalDuration:    5 * time.Second,
		RecordsProcessed: 1000,
		RecordsFailed:    5,
		RecordsPerSecond: 200.0,
	})
	entry := firstEntry(t, buf.String())
	if entry["msg"] != "execution metrics" {
		t.Errorf("msg = %v", entry["msg"])
	}
	if got, _ := entry["records_processed"].(float64); int(got) != 1000 {
		t.Errorf("records_processed = %v", entry["records_processed"])
	}
	if got, _ := entry["records_per_second"].(float64); got != 200.0 {
		t.Errorf("records_per_second = %v", entry["records_per_second"])
	}
}

func TestExecutionContext_OmitsZeroValueFields(t *testing.T) {
	buf, restore := captureLogger(t)
	defer restore()

	logger.LogExecutionStart(logger.ExecutionContext{PipelineID: "minimal"})
	entry := firstEntry(t, buf.String())
	if entry["pipeline_id"] != "minimal" {
		t.Fatalf("pipeline_id = %v", entry["pipeline_id"])
	}
	for _, k := range []string{"pipeline_name", "stage", "module_type", "module_name", "dry_run", "filter_index", "trace_id"} {
		if _, ok := entry[k]; ok {
			t.Errorf("expected %q to be absent for zero-valued field, got %v", k, entry[k])
		}
	}
}

// =============================================================================
// HumanHandler tests
// =============================================================================

func TestHumanHandler_PrintsMessageAndAttrs(t *testing.T) {
	var buf bytes.Buffer
	h := logger.NewHumanHandler(&buf, &logger.HumanHandlerOptions{Level: slog.LevelInfo})

	slog.New(h).Info("test message", "key", "value")
	out := buf.String()

	if !strings.Contains(out, "test message") {
		t.Errorf("missing message in %q", out)
	}
	if !strings.Contains(out, "ℹ") {
		t.Errorf("missing info icon in %q", out)
	}
	if !strings.Contains(out, "key=value") {
		t.Errorf("missing attr in %q", out)
	}
}

func TestHumanHandler_LevelIcons(t *testing.T) {
	cases := []struct {
		level slog.Level
		icon  string
	}{
		{slog.LevelError, "✗"},
		{slog.LevelWarn, "⚠"},
		{slog.LevelInfo, "ℹ"},
		{slog.LevelDebug, "·"},
	}
	for _, tc := range cases {
		t.Run(tc.level.String(), func(t *testing.T) {
			var buf bytes.Buffer
			h := logger.NewHumanHandler(&buf, &logger.HumanHandlerOptions{Level: slog.LevelDebug})
			slog.New(h).Log(context.Background(), tc.level, "test")
			if !strings.Contains(buf.String(), tc.icon) {
				t.Errorf("level %s missing icon %s in %q", tc.level, tc.icon, buf.String())
			}
		})
	}
}

func TestHumanHandler_FormatsDuration(t *testing.T) {
	var buf bytes.Buffer
	h := logger.NewHumanHandler(&buf, &logger.HumanHandlerOptions{Level: slog.LevelInfo})
	slog.New(h).Info("duration test", "duration", 2500*time.Millisecond)
	if !strings.Contains(buf.String(), "duration=2.50s") {
		t.Errorf("missing duration formatting in %q", buf.String())
	}
}

func TestFormatMetricsHuman(t *testing.T) {
	formatted := logger.FormatMetricsHuman(logger.ExecutionMetrics{
		TotalDuration:    5 * time.Second,
		RecordsProcessed: 1000,
		RecordsFailed:    5,
		RecordsPerSecond: 200.0,
	})
	for _, want := range []string{"1000 records", "5.00s", "200.0 records/sec", "5 failed"} {
		if !strings.Contains(formatted, want) {
			t.Errorf("expected %q in %q", want, formatted)
		}
	}
}

// =============================================================================
// Log file output tests
// =============================================================================

func TestSetLogFile_WritesAndCloses(t *testing.T) {
	original := logger.Logger
	defer func() {
		logger.CloseLogFile()
		logger.Logger = original
	}()

	tmp, tmpErr := os.CreateTemp("", "test-log-*.json")
	if tmpErr != nil {
		t.Fatalf("CreateTemp: %v", tmpErr)
	}
	path := tmp.Name()
	_ = tmp.Close()
	defer func() { _ = os.Remove(path) }()

	if err := logger.SetLogFile(path, slog.LevelInfo, logger.FormatJSON); err != nil {
		t.Fatalf("SetLogFile: %v", err)
	}
	logger.Info("disk write", "key", "value")
	logger.CloseLogFile()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(content), "disk write") {
		t.Errorf("log file does not contain expected entry: %s", content)
	}
}

func TestCloseLogFile_NoOpWhenAbsent(t *testing.T) {
	original := logger.Logger
	defer func() { logger.Logger = original }()

	// Should not panic.
	logger.CloseLogFile()

	tmp, tmpErr := os.CreateTemp("", "test-log-*.json")
	if tmpErr != nil {
		t.Fatalf("CreateTemp: %v", tmpErr)
	}
	path := tmp.Name()
	_ = tmp.Close()
	defer func() { _ = os.Remove(path) }()

	if err := logger.SetLogFile(path, slog.LevelInfo, logger.FormatJSON); err != nil {
		t.Fatalf("SetLogFile: %v", err)
	}
	logger.CloseLogFile()
	logger.CloseLogFile() // double close must not panic
}
