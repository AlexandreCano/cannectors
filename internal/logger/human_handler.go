package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"
)

// HumanHandlerOptions configures the human-readable log handler.
type HumanHandlerOptions struct {
	Level     slog.Level
	UseColors bool
}

// HumanHandler is a slog handler that prints concise, colorized log lines
// suitable for local development.
type HumanHandler struct {
	opts   HumanHandlerOptions
	writer io.Writer
	attrs  []slog.Attr
	groups []string
}

// NewHumanHandler builds a HumanHandler. opts may be nil — in which case the
// level defaults to Info and colors are off.
func NewHumanHandler(w io.Writer, opts *HumanHandlerOptions) *HumanHandler {
	if opts == nil {
		opts = &HumanHandlerOptions{Level: slog.LevelInfo}
	}
	return &HumanHandler{opts: *opts, writer: w}
}

// Enabled reports whether the handler will emit records at the given level.
func (h *HumanHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.opts.Level
}

// Handle formats and writes a record.
func (h *HumanHandler) Handle(_ context.Context, r slog.Record) error {
	var sb strings.Builder
	sb.WriteString(r.Time.Format("15:04:05"))
	sb.WriteString(" ")
	sb.WriteString(h.levelPrefix(r.Level, r.Message))
	sb.WriteString(" ")
	sb.WriteString(r.Message)

	keyAttrs := make([]string, 0, r.NumAttrs()+len(h.attrs))
	r.Attrs(func(a slog.Attr) bool {
		keyAttrs = append(keyAttrs, formatAttr(a))
		return true
	})
	for _, a := range h.attrs {
		keyAttrs = append(keyAttrs, formatAttr(a))
	}

	const maxInline = 5
	if len(keyAttrs) > 0 {
		sb.WriteString(" ")
		shown := keyAttrs
		if len(keyAttrs) > maxInline {
			shown = keyAttrs[:maxInline]
		}
		sb.WriteString(strings.Join(shown, " "))
		if len(keyAttrs) > maxInline {
			fmt.Fprintf(&sb, " (+%d more)", len(keyAttrs)-maxInline)
		}
	}
	sb.WriteString("\n")
	_, err := h.writer.Write([]byte(sb.String()))
	return err
}

// WithAttrs returns a copy of the handler with attrs appended.
func (h *HumanHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	merged := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(merged, h.attrs)
	copy(merged[len(h.attrs):], attrs)
	return &HumanHandler{opts: h.opts, writer: h.writer, attrs: merged, groups: h.groups}
}

// WithGroup returns a copy of the handler with name appended to the group path.
func (h *HumanHandler) WithGroup(name string) slog.Handler {
	return &HumanHandler{opts: h.opts, writer: h.writer, attrs: h.attrs, groups: append(h.groups, name)}
}

// levelPrefix returns a single-character icon for the given log level. Success
// messages get a green check; everything else uses level-colored icons.
func (h *HumanHandler) levelPrefix(level slog.Level, message string) string {
	const (
		colorReset  = "\033[0m"
		colorRed    = "\033[31m"
		colorYellow = "\033[33m"
		colorGreen  = "\033[32m"
		colorCyan   = "\033[36m"
	)

	isSuccess := false
	for _, kw := range []string{"completed", "succeeded", "success"} {
		if strings.Contains(strings.ToLower(message), kw) {
			isSuccess = true
			break
		}
	}

	var prefix, color string
	switch {
	case level >= slog.LevelError:
		prefix, color = "✗", colorRed
	case level >= slog.LevelWarn:
		prefix, color = "⚠", colorYellow
	case level >= slog.LevelInfo:
		if isSuccess {
			prefix, color = "✓", colorGreen
		} else {
			prefix, color = "ℹ", colorCyan
		}
	default:
		prefix, color = "·", colorReset
	}
	if h.opts.UseColors {
		return color + prefix + colorReset
	}
	return prefix
}

// formatAttr renders a single attr as `key=value`, with a small amount of
// type-aware formatting (durations, floats).
func formatAttr(a slog.Attr) string {
	value := a.Value.Any()
	if d, ok := value.(time.Duration); ok {
		return fmt.Sprintf("%s=%s", a.Key, formatDuration(d))
	}
	if f, ok := value.(float64); ok {
		return fmt.Sprintf("%s=%.2f", a.Key, f)
	}
	return fmt.Sprintf("%s=%v", a.Key, value)
}

// formatDuration prints a duration with a human-friendly unit.
func formatDuration(d time.Duration) string {
	switch {
	case d < time.Millisecond:
		return fmt.Sprintf("%dµs", d.Microseconds())
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%.2fs", d.Seconds())
	default:
		return fmt.Sprintf("%.1fm", d.Minutes())
	}
}

// FormatMetricsHuman renders execution metrics for the human format banner.
func FormatMetricsHuman(metrics ExecutionMetrics) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Processed %d records in %s",
		metrics.RecordsProcessed,
		formatDuration(metrics.TotalDuration))
	if metrics.RecordsPerSecond > 0 {
		fmt.Fprintf(&sb, " (%.1f records/sec)", metrics.RecordsPerSecond)
	}
	if metrics.RecordsFailed > 0 {
		fmt.Fprintf(&sb, ", %d failed", metrics.RecordsFailed)
	}
	return sb.String()
}
