// Package logger provides structured logging functionality.
// It wraps the standard log/slog package for consistent logging across the runtime.
//
// This package will be enhanced in Story 4.3: Implement Execution Logging.
package logger

import (
	"log/slog"
	"os"
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
