package logger_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/canectors/runtime/internal/logger"
)

func TestLoggerInitialization(t *testing.T) {
	// Logger should be initialized
	if logger.Logger == nil {
		t.Fatal("Logger should be initialized on package load")
	}
}

func TestSetLevel(t *testing.T) {
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
