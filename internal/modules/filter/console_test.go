package filter

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/dop251/goja"

	"github.com/canectors/runtime/internal/logger"
)

// testLogHandler captures log records for testing.
type testLogHandler struct {
	records []slog.Record
	attrs   []slog.Attr
}

func (h *testLogHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *testLogHandler) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, r.Clone())
	return nil
}
func (h *testLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &testLogHandler{records: h.records, attrs: append(h.attrs, attrs...)}
}
func (h *testLogHandler) WithGroup(name string) slog.Handler { return h }

func mustJSConsole(t *testing.T, vm *goja.Runtime) *jsConsole {
	t.Helper()
	c, err := newJSConsole(vm, "test-module")
	if err != nil {
		t.Fatalf("newJSConsole: %v", err)
	}
	return c
}

func TestJSConsole_BasicLog(t *testing.T) {
	vm := goja.New()
	_ = mustJSConsole(t, vm)

	// Test console.log exists
	consoleVal := vm.Get("console")
	if consoleVal == nil || goja.IsUndefined(consoleVal) {
		t.Fatal("console should be defined")
	}

	// Execute console.log - should not panic
	_, err := vm.RunString(`console.log("hello world")`)
	if err != nil {
		t.Fatalf("console.log failed: %v", err)
	}
}

func TestJSConsole_AllMethods(t *testing.T) {
	vm := goja.New()
	_ = mustJSConsole(t, vm)

	methods := []string{"log", "info", "warn", "error", "debug"}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			script := `console.` + method + `("test message")`
			_, err := vm.RunString(script)
			if err != nil {
				t.Fatalf("console.%s failed: %v", method, err)
			}
		})
	}
}

// TestJSConsole_LogLevels verifies console.error/warn/info/log/debug map to correct slog levels (AC2, Task 6).
func TestJSConsole_LogLevels(t *testing.T) {
	handler := &testLogHandler{records: make([]slog.Record, 0)}
	orig := logger.Logger
	defer func() { logger.Logger = orig }()
	logger.Logger = slog.New(handler)

	vm := goja.New()
	_ = mustJSConsole(t, vm)

	// Call in order: error, warn, info, log, debug
	for _, s := range []string{`console.error("e")`, `console.warn("w")`, `console.info("i")`, `console.log("l")`, `console.debug("d")`} {
		_, err := vm.RunString(s)
		if err != nil {
			t.Fatalf("run %q: %v", s, err)
		}
	}

	want := []slog.Level{slog.LevelError, slog.LevelWarn, slog.LevelInfo, slog.LevelInfo, slog.LevelDebug}
	if len(handler.records) != len(want) {
		t.Fatalf("got %d log records, want %d", len(handler.records), len(want))
	}
	for i, w := range want {
		got := handler.records[i].Level
		if got != w {
			t.Errorf("record %d: level = %v, want %v", i, got, w)
		}
	}
}

func TestJSConsole_MultipleArguments(t *testing.T) {
	vm := goja.New()
	c := mustJSConsole(t, vm)

	// Test formatting with multiple arguments
	args := []goja.Value{
		vm.ToValue("hello"),
		vm.ToValue(42),
		vm.ToValue(true),
	}
	result := c.formatArgs(args)
	if result != "hello 42 true" {
		t.Errorf("expected 'hello 42 true', got '%s'", result)
	}
}

func TestJSConsole_ObjectFormatting(t *testing.T) {
	vm := goja.New()
	c := mustJSConsole(t, vm)

	// Create a JavaScript object
	_, err := vm.RunString(`var testObj = {name: "test", value: 123}`)
	if err != nil {
		t.Fatal(err)
	}

	objVal := vm.Get("testObj")
	result := c.formatValue(objVal, 0, make(map[uintptr]bool))

	if !strings.Contains(result, "name") || !strings.Contains(result, "test") {
		t.Errorf("expected object to contain 'name' and 'test', got '%s'", result)
	}
}

func TestJSConsole_ArrayFormatting(t *testing.T) {
	vm := goja.New()
	c := mustJSConsole(t, vm)

	// Create a JavaScript array
	_, err := vm.RunString(`var testArr = [1, 2, 3, "four"]`)
	if err != nil {
		t.Fatal(err)
	}

	arrVal := vm.Get("testArr")
	result := c.formatValue(arrVal, 0, make(map[uintptr]bool))

	if !strings.Contains(result, "1") || !strings.Contains(result, "four") {
		t.Errorf("expected array to contain '1' and 'four', got '%s'", result)
	}
}

func TestJSConsole_NullUndefined(t *testing.T) {
	vm := goja.New()
	c := mustJSConsole(t, vm)

	tests := []struct {
		name     string
		value    goja.Value
		expected string
	}{
		{"nil", nil, "undefined"},
		{"undefined", goja.Undefined(), "undefined"},
		{"null", goja.Null(), "null"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := c.formatValue(tt.value, 0, make(map[uintptr]bool))
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestJSConsole_CircularReference(t *testing.T) {
	vm := goja.New()
	c := mustJSConsole(t, vm)
	_, err := vm.RunString(`var o = {}; o.self = o;`)
	if err != nil {
		t.Fatal(err)
	}
	obj := vm.Get("o")
	seen := make(map[uintptr]bool)
	out := c.formatValue(obj, 0, seen)
	if !strings.Contains(out, placeholderCircular) {
		t.Errorf("expected %q in output for self-ref object, got %q", placeholderCircular, out)
	}
}

func TestJSConsole_DepthLimit(t *testing.T) {
	vm := goja.New()
	c := mustJSConsole(t, vm)

	// Create deeply nested object
	_, err := vm.RunString(`
		var deep = {level: 0};
		var current = deep;
		for (var i = 1; i <= 15; i++) {
			current.nested = {level: i};
			current = current.nested;
		}
	`)
	if err != nil {
		t.Fatal(err)
	}

	deepVal := vm.Get("deep")
	result := c.formatValue(deepVal, 0, make(map[uintptr]bool))

	// Should not cause infinite recursion or stack overflow
	if result == "" {
		t.Error("expected non-empty result for deep object")
	}
}

func TestJSConsole_RecordIndex(t *testing.T) {
	vm := goja.New()
	c := mustJSConsole(t, vm)

	// Initially no record index
	if c.recordIdx != nil {
		t.Error("recordIdx should initially be nil")
	}

	// Set record index
	c.SetRecordIndex(5)
	if c.recordIdx == nil || *c.recordIdx != 5 {
		t.Error("recordIdx should be 5")
	}

	// Clear record index
	c.ClearRecordIndex()
	if c.recordIdx != nil {
		t.Error("recordIdx should be nil after clear")
	}
}

func TestJSConsole_EmptyArgs(t *testing.T) {
	vm := goja.New()
	c := mustJSConsole(t, vm)

	result := c.formatArgs([]goja.Value{})
	if result != "" {
		t.Errorf("expected empty string, got '%s'", result)
	}
}

func TestJSConsole_LongMessageTruncation(t *testing.T) {
	vm := goja.New()
	_ = mustJSConsole(t, vm)

	// Create a very long string
	var buf bytes.Buffer
	for i := 0; i < MaxLogMessageLength+1000; i++ {
		buf.WriteByte('a')
	}
	longStr := buf.String()

	// Should not panic when logging very long message
	script := `console.log("` + longStr + `")`
	_, err := vm.RunString(script)
	if err != nil {
		t.Fatalf("console.log with long message failed: %v", err)
	}
}

func TestJSConsole_Integration(t *testing.T) {
	// Test that console works in a complete script execution
	script := `
		function transform(record) {
			console.log("Processing record:", record.name);
			console.debug("Debug info");
			console.warn("Warning message");
			record.processed = true;
			return record;
		}
	`

	module, err := NewScriptFromConfig(ScriptConfig{Script: script})
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	if module.console == nil {
		t.Fatal("console should be initialized")
	}
}

func TestJSConsole_SpecialCharacters(t *testing.T) {
	vm := goja.New()
	_ = mustJSConsole(t, vm)

	// Test with special characters that might cause issues
	testCases := []string{
		`console.log("hello\nworld")`,
		`console.log("tab\there")`,
		`console.log("quote: \"test\"")`,
		`console.log("unicode: 日本語")`,
	}

	for _, tc := range testCases {
		_, err := vm.RunString(tc)
		if err != nil {
			t.Errorf("failed for script '%s': %v", tc, err)
		}
	}
}
