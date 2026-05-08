// Package logger provides structured logging on top of log/slog.
//
// The package offers:
//   - A package-global *slog.Logger (Logger) used as the default sink.
//   - Helpers to switch between JSON (machine-readable) and human-readable
//     output formats — see SetLevelAndFormat / SetLogFile.
//   - Pipeline-execution helpers in execution.go.
//   - Trace-ID propagation primitives in trace.go.
package logger

import (
	"io"
	"log/slog"
	"os"
)

// Logger is the default logger instance used by the package-level helpers
// (Info / Debug / Warn / Error). Tests may swap this for a custom *slog.Logger
// to capture output.
var Logger *slog.Logger

func init() {
	Logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

// Info logs an informational message via Logger.
func Info(msg string, args ...any) { Logger.Info(msg, args...) }

// Debug logs a debug message via Logger.
func Debug(msg string, args ...any) { Logger.Debug(msg, args...) }

// Warn logs a warning message via Logger.
func Warn(msg string, args ...any) { Logger.Warn(msg, args...) }

// Error logs an error message via Logger.
func Error(msg string, args ...any) { Logger.Error(msg, args...) }

// OutputFormat selects between JSON and the human-readable console format.
type OutputFormat int

const (
	// FormatJSON is the default machine-readable JSON format.
	FormatJSON OutputFormat = iota
	// FormatHuman is a colorized console format intended for local dev.
	FormatHuman
)

// SetLevelAndFormat reconfigures Logger with the given level and format.
// FormatHuman is colorized when stdout is a terminal.
func SetLevelAndFormat(level slog.Level, format OutputFormat) {
	Logger = slog.New(newConsoleHandler(os.Stdout, level, format))
}

// newConsoleHandler returns the slog.Handler that matches the requested
// format, applied to the given writer with the given level.
func newConsoleHandler(w io.Writer, level slog.Level, format OutputFormat) slog.Handler {
	if format == FormatHuman {
		return NewHumanHandler(w, &HumanHandlerOptions{
			Level:     level,
			UseColors: isTerminal(w),
		})
	}
	return slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})
}

// isTerminal returns true when the writer is a character device (terminal).
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
