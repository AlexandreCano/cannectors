// Package filter provides implementations for filter modules.
// Console provides a lightweight console implementation for Goja JavaScript runtime.
package filter

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"unsafe"

	"github.com/dop251/goja"

	"github.com/canectors/runtime/internal/logger"
)

// Console configuration limits
const (
	// MaxLogMessageLength is the maximum length of a single log message (8KB)
	MaxLogMessageLength = 8 * 1024
	// MaxObjectDepth is the maximum depth for object serialization
	MaxObjectDepth = 10
)

// Placeholders used when formatting values for logging.
const (
	placeholderCircular = "[Circular]"
	placeholderObject   = "[Object]"
)

// jsConsole provides console.log/error/warn/info/debug methods for Goja runtime.
// It routes JavaScript console output to the Go slog logger with appropriate log levels.
type jsConsole struct {
	runtime   *goja.Runtime
	moduleID  string // Optional identifier for the script module
	recordIdx *int   // Pointer to current record index (updated during processing)
}

// newJSConsole creates a new jsConsole instance and registers it in the Goja runtime.
// The moduleID is used to identify log messages from this script module.
func newJSConsole(runtime *goja.Runtime, moduleID string) (*jsConsole, error) {
	c := &jsConsole{
		runtime:  runtime,
		moduleID: moduleID,
	}

	console := runtime.NewObject()
	for name, fn := range map[string]func(goja.FunctionCall) goja.Value{
		"log":   c.log,
		"info":  c.info,
		"warn":  c.warn,
		"error": c.consoleError,
		"debug": c.debug,
	} {
		if err := console.Set(name, fn); err != nil {
			return nil, fmt.Errorf("console.Set(%q): %w", name, err)
		}
	}
	if err := runtime.Set("console", console); err != nil {
		return nil, fmt.Errorf("runtime.Set(console): %w", err)
	}
	return c, nil
}

// SetRecordIndex updates the current record index for log context.
// This should be called before processing each record.
func (c *jsConsole) SetRecordIndex(idx int) {
	c.recordIdx = &idx
}

// ClearRecordIndex clears the record index after processing.
func (c *jsConsole) ClearRecordIndex() {
	c.recordIdx = nil
}

// log implements console.log() - logs at Info level
func (c *jsConsole) log(call goja.FunctionCall) goja.Value {
	return c.info(call)
}

// info implements console.info() - logs at Info level
func (c *jsConsole) info(call goja.FunctionCall) goja.Value {
	c.logWithLevel(slog.LevelInfo, call.Arguments)
	return goja.Undefined()
}

// warn implements console.warn() - logs at Warn level
func (c *jsConsole) warn(call goja.FunctionCall) goja.Value {
	c.logWithLevel(slog.LevelWarn, call.Arguments)
	return goja.Undefined()
}

// consoleError implements console.error() - logs at Error level
func (c *jsConsole) consoleError(call goja.FunctionCall) goja.Value {
	c.logWithLevel(slog.LevelError, call.Arguments)
	return goja.Undefined()
}

// debug implements console.debug() - logs at Debug level
func (c *jsConsole) debug(call goja.FunctionCall) goja.Value {
	c.logWithLevel(slog.LevelDebug, call.Arguments)
	return goja.Undefined()
}

// logWithLevel formats arguments and logs at the specified level.
func (c *jsConsole) logWithLevel(level slog.Level, args []goja.Value) {
	message := c.formatArgs(args)

	// Truncate if too long
	if len(message) > MaxLogMessageLength {
		message = message[:MaxLogMessageLength-3] + "..."
	}

	// Build attributes
	attrs := []any{
		slog.String("source", "javascript"),
		slog.String("module_type", "script"),
	}
	if c.moduleID != "" {
		attrs = append(attrs, slog.String("module_id", c.moduleID))
	}
	if c.recordIdx != nil {
		attrs = append(attrs, slog.Int("record_index", *c.recordIdx))
	}

	// Log using appropriate level
	switch level {
	case slog.LevelDebug:
		logger.Debug(message, attrs...)
	case slog.LevelInfo:
		logger.Info(message, attrs...)
	case slog.LevelWarn:
		logger.Warn(message, attrs...)
	case slog.LevelError:
		logger.Error(message, attrs...)
	default:
		logger.Info(message, attrs...)
	}
}

// formatArgs converts JavaScript arguments to a formatted string.
// Handles multiple arguments similar to Node.js console.log behavior.
func (c *jsConsole) formatArgs(args []goja.Value) string {
	if len(args) == 0 {
		return ""
	}

	parts := make([]string, 0, len(args))
	for _, arg := range args {
		parts = append(parts, c.formatValue(arg, 0, make(map[uintptr]bool)))
	}

	return strings.Join(parts, " ")
}

// formatValue converts a single JavaScript value to string.
// Handles objects, arrays, and primitives with circular reference detection.
func (c *jsConsole) formatValue(val goja.Value, depth int, seen map[uintptr]bool) string {
	if val == nil || goja.IsUndefined(val) {
		return "undefined"
	}
	if goja.IsNull(val) {
		return "null"
	}

	// Check depth limit
	if depth > MaxObjectDepth {
		return placeholderObject
	}

	// Export to Go value for type checking
	exported := val.Export()

	switch v := exported.(type) {
	case string:
		return v
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return fmt.Sprintf("%v", v)
	case []interface{}:
		return c.formatArray(val, depth, seen)
	case map[string]interface{}:
		return c.formatObject(v, depth, seen)
	default:
		// Try to get the object for further inspection
		if obj, ok := val.(*goja.Object); ok {
			return c.formatGojaObject(obj, depth, seen)
		}
		// Fallback to string representation
		return val.String()
	}
}

// formatArray formats a JavaScript array to string.
// Uses object identity (uintptr of *goja.Object) for circular reference detection.
func (c *jsConsole) formatArray(val goja.Value, depth int, seen map[uintptr]bool) string {
	obj, ok := val.(*goja.Object)
	if !ok {
		return "[]"
	}

	ptr := uintptr(unsafe.Pointer(obj))
	if seen[ptr] && depth > 0 {
		return placeholderCircular
	}
	seen[ptr] = true

	lengthVal := obj.Get("length")
	if lengthVal == nil || goja.IsUndefined(lengthVal) {
		return "[]"
	}

	length := int(lengthVal.ToInteger())
	if length == 0 {
		return "[]"
	}

	maxElements := 100
	if length > maxElements {
		parts := make([]string, 0, maxElements+1)
		for i := 0; i < maxElements; i++ {
			elem := obj.Get(fmt.Sprintf("%d", i))
			parts = append(parts, c.formatValue(elem, depth+1, seen))
		}
		parts = append(parts, fmt.Sprintf("... %d more items", length-maxElements))
		return "[" + strings.Join(parts, ", ") + "]"
	}

	parts := make([]string, 0, length)
	for i := 0; i < length; i++ {
		elem := obj.Get(fmt.Sprintf("%d", i))
		parts = append(parts, c.formatValue(elem, depth+1, seen))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// formatObject formats a Go map to JSON-like string with circular reference handling.
func (c *jsConsole) formatObject(obj map[string]interface{}, depth int, seen map[uintptr]bool) string {
	return c.formatMapSafe(obj, depth, seen)
}

// formatMapSafe serializes a map with circular reference detection.
func (c *jsConsole) formatMapSafe(m map[string]interface{}, depth int, seen map[uintptr]bool) string {
	if depth > MaxObjectDepth {
		return placeholderObject
	}
	ptr := reflect.ValueOf(m).Pointer()
	if seen[ptr] {
		return placeholderCircular
	}
	seen[ptr] = true

	var b strings.Builder
	b.WriteByte('{')
	first := true
	for k, v := range m {
		if !first {
			b.WriteString(", ")
		}
		first = false
		// Escape key for JSON-like output
		b.WriteString(quoteJSONKey(k))
		b.WriteString(": ")
		b.WriteString(c.formatGoValue(v, depth+1, seen))
	}
	b.WriteByte('}')
	return b.String()
}

func quoteJSONKey(s string) string {
	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Sprintf("%q", s)
	}
	return string(data)
}

// formatGoValue formats a Go value (from Export) for logging.
func (c *jsConsole) formatGoValue(v interface{}, depth int, seen map[uintptr]bool) string {
	if depth > MaxObjectDepth {
		return placeholderObject
	}
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		data, _ := json.Marshal(x)
		return string(data)
	case bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return fmt.Sprintf("%v", x)
	case []interface{}:
		return c.formatSliceSafe(x, depth, seen)
	case map[string]interface{}:
		return c.formatMapSafe(x, depth, seen)
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("[Object %T]", v)
		}
		return string(data)
	}
}

// formatSliceSafe serializes a slice with circular reference detection.
func (c *jsConsole) formatSliceSafe(s []interface{}, depth int, seen map[uintptr]bool) string {
	if depth > MaxObjectDepth {
		return placeholderObject
	}
	ptr := reflect.ValueOf(s).Pointer()
	if seen[ptr] {
		return placeholderCircular
	}
	seen[ptr] = true

	parts := make([]string, 0, len(s))
	for _, v := range s {
		parts = append(parts, c.formatGoValue(v, depth+1, seen))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// formatGojaObject formats a Goja object to string.
func (c *jsConsole) formatGojaObject(obj *goja.Object, depth int, seen map[uintptr]bool) string {
	if obj.ClassName() == "Array" {
		return c.formatArray(obj, depth, seen)
	}

	exported := obj.Export()
	if m, ok := exported.(map[string]interface{}); ok {
		return c.formatObject(m, depth, seen)
	}
	// Use formatGoValue for other types (handles slices, cycles)
	return c.formatGoValue(exported, depth, seen)
}
