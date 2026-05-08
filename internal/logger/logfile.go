package logger

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"
)

// maxLogFileSize is the size threshold beyond which SetLogFile rotates the
// existing log into a timestamped sibling.
const maxLogFileSize = 10 * 1024 * 1024 // 10MB

// logFile holds the currently open log file (if any).
var logFile *os.File

// SetLogFile redirects logging to both stdout and the given file. The file is
// always written in JSON; the console respects consoleFormat. Files larger
// than maxLogFileSize are rotated (renamed with a timestamp suffix) before
// being reopened.
func SetLogFile(path string, level slog.Level, consoleFormat OutputFormat) error {
	CloseLogFile()

	if err := rotateLogFile(path); err != nil {
		// Rotation failure is non-fatal; we keep going with the existing file.
		Warn("log rotation failed", slog.String("error", err.Error()))
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("opening log file: %w", err)
	}
	logFile = f

	fileHandler := slog.NewJSONHandler(f, &slog.HandlerOptions{Level: level})
	consoleHandler := newConsoleHandler(os.Stdout, level, consoleFormat)

	Logger = slog.New(&dualHandler{console: consoleHandler, file: fileHandler})

	formatLabel := "json"
	if consoleFormat == FormatHuman {
		formatLabel = "human"
	}
	Info("log file opened", slog.String("path", path), slog.String("console_format", formatLabel))
	return nil
}

// CloseLogFile syncs and closes the currently-open log file (if any).
func CloseLogFile() {
	if logFile == nil {
		return
	}
	if err := logFile.Sync(); err != nil {
		Warn("failed to sync log file", slog.String("error", err.Error()))
	}
	if err := logFile.Close(); err != nil {
		Warn("failed to close log file", slog.String("error", err.Error()))
	}
	logFile = nil
}

// rotateLogFile renames the existing file to a timestamped sibling when it
// exceeds maxLogFileSize. Returns nil when the file does not exist or is
// below the threshold.
func rotateLogFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("checking log file size: %w", err)
	}
	if info.Size() < maxLogFileSize {
		return nil
	}
	rotated := fmt.Sprintf("%s.%s", path, time.Now().Format("20060102-150405"))
	if err := os.Rename(path, rotated); err != nil {
		return fmt.Errorf("rotating log file: %w", err)
	}
	return nil
}

// dualHandler fans out a record to two handlers (typically console + file).
type dualHandler struct {
	console slog.Handler
	file    slog.Handler
}

func (d *dualHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return d.console.Enabled(ctx, level) || d.file.Enabled(ctx, level)
}

func (d *dualHandler) Handle(ctx context.Context, r slog.Record) error {
	if d.console.Enabled(ctx, r.Level) {
		if err := d.console.Handle(ctx, r); err != nil {
			return err
		}
	}
	if d.file.Enabled(ctx, r.Level) {
		if err := d.file.Handle(ctx, r); err != nil {
			return err
		}
	}
	return nil
}

func (d *dualHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &dualHandler{console: d.console.WithAttrs(attrs), file: d.file.WithAttrs(attrs)}
}

func (d *dualHandler) WithGroup(name string) slog.Handler {
	return &dualHandler{console: d.console.WithGroup(name), file: d.file.WithGroup(name)}
}
