// Package filter provides implementations for filter modules.
// Tests for the script filter module that executes JavaScript using Goja.
package filter

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cannectors/runtime/pkg/connector"
)

// strPtrScript is a helper to create string pointers for test cases.
func strPtrScript(s string) *string {
	return &s
}

// TestScriptModuleCreation tests creating a script module with valid script.
func TestScriptModuleCreation(t *testing.T) {
	config := ScriptConfig{
		Script: `function transform(record) { return record; }`,
	}

	module, err := NewScriptFromConfig(config)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if module == nil {
		t.Fatal("expected module to be created")
	}
}

// TestScriptModuleCreation_WithOnError tests creating a script module with onError config.
func TestScriptModuleCreation_WithOnError(t *testing.T) {
	testCases := []struct {
		name     string
		onError  string
		expected string
	}{
		{"default", "", OnErrorFail},
		{"fail", "fail", OnErrorFail},
		{"skip", "skip", OnErrorSkip},
		{"log", "log", OnErrorLog},
		{"invalid defaults to fail", "invalid", OnErrorFail},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := ScriptConfig{
				Script:     `function transform(record) { return record; }`,
				ModuleBase: connector.ModuleBase{OnError: tc.onError},
			}

			module, err := NewScriptFromConfig(config)
			if err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
			if module.onError != tc.expected {
				t.Errorf("expected onError=%q, got %q", tc.expected, module.onError)
			}
		})
	}
}

// TestScriptModuleCreation_EmptyScript tests that empty script returns an error.
func TestScriptModuleCreation_EmptyScript(t *testing.T) {
	config := ScriptConfig{
		Script: "",
	}

	_, err := NewScriptFromConfig(config)
	if err == nil {
		t.Fatal("expected error for empty script")
	}
	// Empty string is treated as "not provided", so we get the "either script or scriptFile" error
	if !strings.Contains(err.Error(), "either 'script' or 'scriptFile' must be provided") {
		t.Errorf("expected error about missing script/scriptFile, got: %v", err)
	}
}

// TestScriptModuleCreation_WhitespaceOnlyScript tests that whitespace-only script returns error.
func TestScriptModuleCreation_WhitespaceOnlyScript(t *testing.T) {
	config := ScriptConfig{
		Script: "   \n\t  ",
	}

	_, err := NewScriptFromConfig(config)
	if err == nil {
		t.Fatal("expected error for whitespace-only script")
	}
}

// TestScriptModuleCreation_SyntaxError tests that invalid JavaScript syntax returns error.
func TestScriptModuleCreation_SyntaxError(t *testing.T) {
	config := ScriptConfig{
		Script: `function transform(record { return record; }`, // Missing )
	}

	_, err := NewScriptFromConfig(config)
	if err == nil {
		t.Fatal("expected error for syntax error in script")
	}
	if !strings.Contains(err.Error(), "compilation failed") {
		t.Errorf("expected error to mention 'compilation failed', got: %v", err)
	}
}

// TestScriptModuleCreation_MissingTransformFunction tests that missing transform function returns error.
func TestScriptModuleCreation_MissingTransformFunction(t *testing.T) {
	config := ScriptConfig{
		Script: `function process(record) { return record; }`, // Wrong function name
	}

	_, err := NewScriptFromConfig(config)
	if err == nil {
		t.Fatal("expected error for missing transform function")
	}
	if !strings.Contains(err.Error(), "transform function not found") {
		t.Errorf("expected error to mention 'transform function not found', got: %v", err)
	}
}

// TestScriptModuleCreation_TransformNotFunction tests that non-function transform returns error.
func TestScriptModuleCreation_TransformNotFunction(t *testing.T) {
	config := ScriptConfig{
		Script: `var transform = "not a function";`,
	}

	_, err := NewScriptFromConfig(config)
	if err == nil {
		t.Fatal("expected error when transform is not a function")
	}
	if !strings.Contains(err.Error(), "transform is not a function") {
		t.Errorf("expected error to mention 'transform is not a function', got: %v", err)
	}
}

// TestScriptModuleCreation_ScriptTooLong tests that very long scripts are rejected.
func TestScriptModuleCreation_ScriptTooLong(t *testing.T) {
	// Create a script longer than 100KB
	longScript := "function transform(record) { " + strings.Repeat("var x = 1; ", 20000) + " return record; }"

	config := ScriptConfig{
		Script: longScript,
	}

	_, err := NewScriptFromConfig(config)
	if err == nil {
		t.Fatal("expected error for script exceeding length limit")
	}
	if !strings.Contains(err.Error(), "exceeds maximum length") {
		t.Errorf("expected error to mention 'exceeds maximum length', got: %v", err)
	}
}

// TestScriptModuleProcess_SimpleTransform tests basic record transformation.
func TestScriptModuleProcess_SimpleTransform(t *testing.T) {
	config := ScriptConfig{
		Script: `function transform(record) {
			record.processed = true;
			return record;
		}`,
	}

	module, err := NewScriptFromConfig(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	input := []map[string]interface{}{
		{"id": 1, "name": "test"},
	}

	result, err := module.Process(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result))
	}

	if result[0]["processed"] != true {
		t.Errorf("expected processed=true, got %v", result[0]["processed"])
	}
	// Goja may return int64 or float64 depending on the value
	id := result[0]["id"]
	switch v := id.(type) {
	case int64:
		if v != 1 {
			t.Errorf("expected id=1, got %v", v)
		}
	case float64:
		if v != 1 {
			t.Errorf("expected id=1, got %v", v)
		}
	case int:
		if v != 1 {
			t.Errorf("expected id=1, got %v", v)
		}
	default:
		t.Errorf("expected id to be numeric, got %T: %v", id, id)
	}
}

// TestScriptModuleProcess_ComputedFields tests computed field transformation.
func TestScriptModuleProcess_ComputedFields(t *testing.T) {
	config := ScriptConfig{
		Script: `function transform(record) {
			record.total = record.price * record.quantity;
			return record;
		}`,
	}

	module, err := NewScriptFromConfig(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	input := []map[string]interface{}{
		{"price": 10.5, "quantity": 2},
	}

	result, err := module.Process(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result))
	}

	// Goja may return int64 if the result is a whole number
	var total float64
	switch v := result[0]["total"].(type) {
	case float64:
		total = v
	case int64:
		total = float64(v)
	case int:
		total = float64(v)
	default:
		t.Fatalf("expected total to be numeric, got %T", result[0]["total"])
	}
	if total != 21.0 {
		t.Errorf("expected total=21.0, got %v", total)
	}
}

// TestScriptModuleProcess_NestedObjects tests transformation with nested objects.
func TestScriptModuleProcess_NestedObjects(t *testing.T) {
	config := ScriptConfig{
		Script: `function transform(record) {
			record.user.fullName = record.user.firstName + " " + record.user.lastName;
			return record;
		}`,
	}

	module, err := NewScriptFromConfig(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	input := []map[string]interface{}{
		{
			"user": map[string]interface{}{
				"firstName": "John",
				"lastName":  "Doe",
			},
		},
	}

	result, err := module.Process(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result))
	}

	user, ok := result[0]["user"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected user to be map, got %T", result[0]["user"])
	}
	if user["fullName"] != "John Doe" {
		t.Errorf("expected fullName='John Doe', got %v", user["fullName"])
	}
}

// TestScriptModuleProcess_Arrays tests transformation with arrays.
func TestScriptModuleProcess_Arrays(t *testing.T) {
	config := ScriptConfig{
		Script: `function transform(record) {
			record.count = record.items.length;
			record.first = record.items[0];
			return record;
		}`,
	}

	module, err := NewScriptFromConfig(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	input := []map[string]interface{}{
		{
			"items": []interface{}{"a", "b", "c"},
		},
	}

	result, err := module.Process(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result))
	}

	count, ok := result[0]["count"].(int64)
	if !ok {
		countFloat, ok2 := result[0]["count"].(float64)
		if !ok2 {
			t.Fatalf("expected count to be numeric, got %T", result[0]["count"])
		}
		count = int64(countFloat)
	}
	if count != 3 {
		t.Errorf("expected count=3, got %v", count)
	}
	if result[0]["first"] != "a" {
		t.Errorf("expected first='a', got %v", result[0]["first"])
	}
}

// TestScriptModuleProcess_MultipleRecords tests processing multiple records.
func TestScriptModuleProcess_MultipleRecords(t *testing.T) {
	config := ScriptConfig{
		Script: `function transform(record) {
			record.doubled = record.value * 2;
			return record;
		}`,
	}

	module, err := NewScriptFromConfig(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	input := []map[string]interface{}{
		{"value": 1},
		{"value": 2},
		{"value": 3},
	}

	result, err := module.Process(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 3 {
		t.Fatalf("expected 3 records, got %d", len(result))
	}

	expected := []float64{2, 4, 6}
	for i, rec := range result {
		doubled, ok := rec["doubled"].(float64)
		if !ok {
			dInt, ok2 := rec["doubled"].(int64)
			if !ok2 {
				t.Fatalf("record %d: expected doubled to be numeric, got %T", i, rec["doubled"])
			}
			doubled = float64(dInt)
		}
		if doubled != expected[i] {
			t.Errorf("record %d: expected doubled=%v, got %v", i, expected[i], doubled)
		}
	}
}

// TestScriptModuleProcess_ReturnNewRecord tests returning a completely new record.
func TestScriptModuleProcess_ReturnNewRecord(t *testing.T) {
	config := ScriptConfig{
		Script: `function transform(record) {
			return {
				newId: record.id + 100,
				newName: record.name.toUpperCase()
			};
		}`,
	}

	module, err := NewScriptFromConfig(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	input := []map[string]interface{}{
		{"id": 1, "name": "test"},
	}

	result, err := module.Process(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result))
	}

	// Original fields should not be present
	if _, exists := result[0]["id"]; exists {
		t.Error("expected 'id' to not exist in result")
	}
	if _, exists := result[0]["name"]; exists {
		t.Error("expected 'name' to not exist in result")
	}

	// New fields should exist
	if result[0]["newName"] != "TEST" {
		t.Errorf("expected newName='TEST', got %v", result[0]["newName"])
	}
}

// TestScriptModuleProcess_NullAndUndefined tests handling of null and undefined values.
func TestScriptModuleProcess_NullAndUndefined(t *testing.T) {
	config := ScriptConfig{
		Script: `function transform(record) {
			record.nullField = null;
			record.undefinedCheck = record.nonExistent === undefined;
			return record;
		}`,
	}

	module, err := NewScriptFromConfig(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	input := []map[string]interface{}{
		{"existing": "value"},
	}

	result, err := module.Process(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result[0]["nullField"] != nil {
		t.Errorf("expected nullField=nil, got %v", result[0]["nullField"])
	}
	if result[0]["undefinedCheck"] != true {
		t.Errorf("expected undefinedCheck=true, got %v", result[0]["undefinedCheck"])
	}
}

// TestScriptModuleProcess_EmptyInput tests processing empty input.
func TestScriptModuleProcess_EmptyInput(t *testing.T) {
	config := ScriptConfig{
		Script: `function transform(record) { return record; }`,
	}

	module, err := NewScriptFromConfig(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, err := module.Process(context.Background(), []map[string]interface{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("expected 0 records, got %d", len(result))
	}
}

// TestScriptModuleProcess_NilInput tests processing nil input.
func TestScriptModuleProcess_NilInput(t *testing.T) {
	config := ScriptConfig{
		Script: `function transform(record) { return record; }`,
	}

	module, err := NewScriptFromConfig(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, err := module.Process(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result) != 0 {
		t.Errorf("expected 0 records, got %d", len(result))
	}
}

// TestScriptModuleProcess_JavaScriptException tests handling of JavaScript exceptions.
func TestScriptModuleProcess_JavaScriptException(t *testing.T) {
	config := ScriptConfig{
		Script: `function transform(record) {
			throw new Error("test error");
		}`,
		ModuleBase: connector.ModuleBase{OnError: OnErrorFail},
	}

	module, err := NewScriptFromConfig(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	input := []map[string]interface{}{
		{"id": 1},
	}

	_, err = module.Process(context.Background(), input)
	if err == nil {
		t.Fatal("expected error from JavaScript exception")
	}
	if !strings.Contains(err.Error(), "test error") {
		t.Errorf("expected error message to contain 'test error', got: %v", err)
	}
}

// TestScriptModuleProcess_OnErrorSkip tests skip behavior on error.
func TestScriptModuleProcess_OnErrorSkip(t *testing.T) {
	config := ScriptConfig{
		Script: `function transform(record) {
			if (record.fail) {
				throw new Error("intentional error");
			}
			return record;
		}`,
		ModuleBase: connector.ModuleBase{OnError: OnErrorSkip},
	}

	module, err := NewScriptFromConfig(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	input := []map[string]interface{}{
		{"id": 1, "fail": false},
		{"id": 2, "fail": true},
		{"id": 3, "fail": false},
	}

	result, err := module.Process(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Record 2 should be skipped
	if len(result) != 2 {
		t.Fatalf("expected 2 records (one skipped), got %d", len(result))
	}
}

// TestScriptModuleProcess_OnErrorLog tests log behavior on error.
func TestScriptModuleProcess_OnErrorLog(t *testing.T) {
	config := ScriptConfig{
		Script: `function transform(record) {
			if (record.fail) {
				throw new Error("intentional error");
			}
			record.processed = true;
			return record;
		}`,
		ModuleBase: connector.ModuleBase{OnError: OnErrorLog},
	}

	module, err := NewScriptFromConfig(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	input := []map[string]interface{}{
		{"id": 1, "fail": false},
		{"id": 2, "fail": true},
		{"id": 3, "fail": false},
	}

	result, err := module.Process(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// All records should be in result (failed one without processed flag)
	if len(result) != 3 {
		t.Fatalf("expected 3 records, got %d", len(result))
	}

	// Record 1 and 3 should have processed=true
	if result[0]["processed"] != true {
		t.Error("record 0 should have processed=true")
	}
	// Record 2 failed, so it should not have processed flag
	if result[1]["processed"] == true {
		t.Error("record 1 should not have processed=true (it failed)")
	}
	if result[2]["processed"] != true {
		t.Error("record 2 should have processed=true")
	}
}

// TestScriptModuleProcess_RuntimeError tests handling of JavaScript runtime errors.
func TestScriptModuleProcess_RuntimeError(t *testing.T) {
	config := ScriptConfig{
		Script: `function transform(record) {
			return record.nonExistent.property; // TypeError: Cannot read property of undefined
		}`,
		ModuleBase: connector.ModuleBase{OnError: OnErrorFail},
	}

	module, err := NewScriptFromConfig(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	input := []map[string]interface{}{
		{"id": 1},
	}

	_, err = module.Process(context.Background(), input)
	if err == nil {
		t.Fatal("expected error from JavaScript runtime error")
	}
}

// TestScriptModuleProcess_ContextCancellation tests respecting context cancellation.
func TestScriptModuleProcess_ContextCancellation(t *testing.T) {
	config := ScriptConfig{
		Script: `function transform(record) { return record; }`,
	}

	module, err := NewScriptFromConfig(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	input := []map[string]interface{}{
		{"id": 1},
	}

	_, err = module.Process(ctx, input)
	if err == nil {
		t.Fatal("expected error from canceled context")
	}
	if err != context.Canceled {
		t.Errorf("expected context.Canceled error, got: %v", err)
	}
}

// TestScriptModuleProcess_ContextCancellationDuringExecution tests that long-running JavaScript can be interrupted.
// Uses context.WithTimeout to make the test deterministic and avoid timing dependencies.
func TestScriptModuleProcess_ContextCancellationDuringExecution(t *testing.T) {
	config := ScriptConfig{
		Script: `function transform(record) {
			// Intentional infinite loop to test interruption
			// This will run until interrupted by context cancellation
			while (true) {
				// Busy loop that will be interrupted
				var sum = 0;
				for (var i = 0; i < 100000; i++) {
					sum += i;
				}
			}
		}`,
	}

	module, err := NewScriptFromConfig(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Use context.WithTimeout to make the test deterministic
	// The timeout ensures the context is canceled, and the infinite loop
	// guarantees the JavaScript is still running when interrupted
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	input := []map[string]interface{}{
		{"id": 1},
	}

	// Process with timeout - should be interrupted
	_, err = module.Process(ctx, input)
	if err == nil {
		t.Fatal("expected error from canceled context during execution")
	}
	// Check for either context.DeadlineExceeded (from timeout) or context.Canceled
	// The error may be wrapped in a ScriptError, so check with errors.Is
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		// Also check if the error message contains the context error
		errStr := err.Error()
		if !strings.Contains(errStr, "context deadline exceeded") && !strings.Contains(errStr, "context canceled") {
			t.Errorf("expected context.DeadlineExceeded or context.Canceled error, got: %v", err)
		}
	}
}

// TestScriptConfig_Parse tests parsing script config from map.
func TestScriptConfig_Parse(t *testing.T) {
	testCases := []struct {
		name        string
		config      map[string]interface{}
		expectError bool
		errorMsg    string
		expected    ScriptConfig
	}{
		{
			name: "valid inline script config",
			config: map[string]interface{}{
				"script":  "function transform(r) { return r; }",
				"onError": "skip",
			},
			expectError: false,
			expected: ScriptConfig{
				Script:     "function transform(r) { return r; }",
				ModuleBase: connector.ModuleBase{OnError: "skip"},
			},
		},
		{
			name: "valid script file config",
			config: map[string]interface{}{
				"scriptFile": "/path/to/script.js",
				"onError":    "fail",
			},
			expectError: false,
			expected: ScriptConfig{
				ScriptFile: "/path/to/script.js",
				ModuleBase: connector.ModuleBase{OnError: "fail"},
			},
		},
		{
			name: "missing both script and scriptFile",
			config: map[string]interface{}{
				"onError": "fail",
			},
			expectError: true,
			errorMsg:    "either 'script' or 'scriptFile' is required",
		},
		{
			name: "both script and scriptFile provided",
			config: map[string]interface{}{
				"script":     "function transform(r) { return r; }",
				"scriptFile": "/path/to/script.js",
			},
			expectError: true,
			errorMsg:    "cannot specify both",
		},
		{
			name: "script not string",
			config: map[string]interface{}{
				"script": 123,
			},
			expectError: true,
			errorMsg:    "must be a string",
		},
		{
			name: "scriptFile not string",
			config: map[string]interface{}{
				"scriptFile": 123,
			},
			expectError: true,
			errorMsg:    "must be a string",
		},
		{
			name: "minimal inline config",
			config: map[string]interface{}{
				"script": "function transform(r) { return r; }",
			},
			expectError: false,
			expected: ScriptConfig{
				Script: "function transform(r) { return r; }",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := ParseScriptConfig(tc.config)
			if tc.expectError {
				if err == nil {
					t.Fatal("expected error")
				}
				if tc.errorMsg != "" && !strings.Contains(err.Error(), tc.errorMsg) {
					t.Errorf("expected error to contain %q, got: %v", tc.errorMsg, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.Script != tc.expected.Script {
				t.Errorf("expected script=%q, got %q", tc.expected.Script, cfg.Script)
			}
			if cfg.ScriptFile != tc.expected.ScriptFile {
				t.Errorf("expected scriptFile=%q, got %q", tc.expected.ScriptFile, cfg.ScriptFile)
			}
			if cfg.OnError != tc.expected.OnError {
				t.Errorf("expected onError=%q, got %q", tc.expected.OnError, cfg.OnError)
			}
		})
	}
}

// TestScriptModule_VerifyInterfaceCompliance verifies ScriptModule implements filter.Module.
func TestScriptModule_VerifyInterfaceCompliance(t *testing.T) {
	var _ Module = (*ScriptModule)(nil)
}

// TestScriptModuleCreation_FromFile tests creating a script module from a file.
func TestScriptModuleCreation_FromFile(t *testing.T) {
	// Create a temporary script file
	tmpFile, err := os.CreateTemp("", "test-script-*.js")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	scriptContent := `function transform(record) {
		record.fromFile = true;
		record.doubled = record.value * 2;
		return record;
	}`

	if _, writeErr := tmpFile.WriteString(scriptContent); writeErr != nil {
		t.Fatalf("failed to write script: %v", writeErr)
	}
	_ = tmpFile.Close()

	// Create module from file
	config := ScriptConfig{
		ScriptFile: tmpFile.Name(),
	}

	module, err := NewScriptFromConfig(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if module == nil {
		t.Fatal("expected module to be created")
	}

	// Test that it works
	input := []map[string]interface{}{
		{"value": 5},
	}

	result, err := module.Process(context.Background(), input)
	if err != nil {
		t.Fatalf("processing failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result))
	}

	if result[0]["fromFile"] != true {
		t.Errorf("expected fromFile=true, got %v", result[0]["fromFile"])
	}
}

// TestScriptModuleCreation_FromFile_NotFound tests error when script file doesn't exist.
func TestScriptModuleCreation_FromFile_NotFound(t *testing.T) {
	config := ScriptConfig{
		ScriptFile: "/nonexistent/path/to/script.js",
	}

	_, err := NewScriptFromConfig(config)
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
	// Error can come from Stat() or ReadFile() - check for either
	if !strings.Contains(err.Error(), "failed to stat script file") && !strings.Contains(err.Error(), "failed to read script file") && !strings.Contains(err.Error(), "no such file") {
		t.Errorf("expected error about file not found, got: %v", err)
	}
}

// TestScriptModuleCreation_FromFile_Empty tests error when script file is empty.
func TestScriptModuleCreation_FromFile_Empty(t *testing.T) {
	// Create an empty temporary file
	tmpFile, err := os.CreateTemp("", "test-empty-*.js")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	_ = tmpFile.Close()

	config := ScriptConfig{
		ScriptFile: tmpFile.Name(),
	}

	_, err = NewScriptFromConfig(config)
	if err == nil {
		t.Fatal("expected error for empty script file")
	}
	if !strings.Contains(err.Error(), "script cannot be empty") {
		t.Errorf("expected error about empty script, got: %v", err)
	}
}

// TestScriptModuleCreation_FromFile_InvalidSyntax tests error when script file has invalid JS.
func TestScriptModuleCreation_FromFile_InvalidSyntax(t *testing.T) {
	// Create a temporary file with invalid JavaScript
	tmpFile, err := os.CreateTemp("", "test-invalid-*.js")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	if _, writeErr := tmpFile.WriteString("function transform(record { return record; }"); writeErr != nil {
		t.Fatalf("failed to write script: %v", writeErr)
	}
	_ = tmpFile.Close()

	config := ScriptConfig{
		ScriptFile: tmpFile.Name(),
	}

	_, err = NewScriptFromConfig(config)
	if err == nil {
		t.Fatal("expected error for invalid JavaScript syntax")
	}
	if !strings.Contains(err.Error(), "compilation failed") {
		t.Errorf("expected compilation error, got: %v", err)
	}
}

// TestScriptModuleCreation_FromFile_MissingTransform tests error when file lacks transform function.
func TestScriptModuleCreation_FromFile_MissingTransform(t *testing.T) {
	// Create a temporary file without transform function
	tmpFile, err := os.CreateTemp("", "test-notransform-*.js")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	if _, writeErr := tmpFile.WriteString("function process(record) { return record; }"); writeErr != nil {
		t.Fatalf("failed to write script: %v", writeErr)
	}
	_ = tmpFile.Close()

	config := ScriptConfig{
		ScriptFile: tmpFile.Name(),
	}

	_, err = NewScriptFromConfig(config)
	if err == nil {
		t.Fatal("expected error for missing transform function")
	}
	if !strings.Contains(err.Error(), "transform function not found") {
		t.Errorf("expected error about missing transform, got: %v", err)
	}
}

// TestScriptModule_InPipeline tests script filter combined with other filters in a pipeline.
func TestScriptModule_InPipeline(t *testing.T) {
	// Create a script module
	scriptConfig := ScriptConfig{
		Script: `function transform(record) {
			record.total = record.price * record.quantity;
			record.currency = "EUR";
			return record;
		}`,
	}
	scriptModule, err := NewScriptFromConfig(scriptConfig)
	if err != nil {
		t.Fatalf("failed to create script module: %v", err)
	}

	// Create a mapping module to rename fields after script processing
	mappingConfig := []FieldMapping{
		{Source: strPtrScript("total"), Target: "amount"},
		{Source: strPtrScript("currency"), Target: "currencyCode"},
	}
	mappingModule, err := NewMappingFromConfig(mappingConfig, OnErrorFail)
	if err != nil {
		t.Fatalf("failed to create mapping module: %v", err)
	}

	// Process through script module first
	input := []map[string]interface{}{
		{"price": 10.0, "quantity": 2},
		{"price": 25.5, "quantity": 4},
	}

	ctx := context.Background()

	scriptResult, err := scriptModule.Process(ctx, input)
	if err != nil {
		t.Fatalf("script processing failed: %v", err)
	}

	// Then process through mapping module
	finalResult, err := mappingModule.Process(ctx, scriptResult)
	if err != nil {
		t.Fatalf("mapping processing failed: %v", err)
	}

	if len(finalResult) != 2 {
		t.Fatalf("expected 2 records, got %d", len(finalResult))
	}

	// Verify first record
	var amount1 float64
	switch v := finalResult[0]["amount"].(type) {
	case float64:
		amount1 = v
	case int64:
		amount1 = float64(v)
	default:
		t.Fatalf("expected amount to be numeric, got %T", finalResult[0]["amount"])
	}
	if amount1 != 20.0 {
		t.Errorf("expected amount=20.0, got %v", amount1)
	}
	if finalResult[0]["currencyCode"] != "EUR" {
		t.Errorf("expected currencyCode=EUR, got %v", finalResult[0]["currencyCode"])
	}

	// Verify second record
	var amount2 float64
	switch v := finalResult[1]["amount"].(type) {
	case float64:
		amount2 = v
	case int64:
		amount2 = float64(v)
	default:
		t.Fatalf("expected amount to be numeric, got %T", finalResult[1]["amount"])
	}
	if amount2 != 102.0 {
		t.Errorf("expected amount=102.0, got %v", amount2)
	}
}

// TestScriptModuleCreation_FromFile_LargeFile tests that large files are rejected before reading.
func TestScriptModuleCreation_FromFile_LargeFile(t *testing.T) {
	// Create a temporary file larger than MaxScriptLength
	tmpFile, err := os.CreateTemp("", "test-large-*.js")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	// Write content larger than MaxScriptLength (100KB)
	largeContent := "function transform(record) { return record; }" + strings.Repeat("x", MaxScriptLength+1)
	if _, writeErr := tmpFile.WriteString(largeContent); writeErr != nil {
		t.Fatalf("failed to write script: %v", writeErr)
	}
	_ = tmpFile.Close()

	config := ScriptConfig{
		ScriptFile: tmpFile.Name(),
	}

	_, err = NewScriptFromConfig(config)
	if err == nil {
		t.Fatal("expected error for file exceeding size limit")
	}
	if !strings.Contains(err.Error(), "exceeds maximum length") {
		t.Errorf("expected error about exceeding maximum length, got: %v", err)
	}
}

// TestScriptModuleCreation_FromFile_PathTraversal tests that path traversal attacks are prevented.
func TestScriptModuleCreation_FromFile_PathTraversal(t *testing.T) {
	testCases := []struct {
		name          string
		filePath      string
		expectError   bool
		errorContains string
	}{
		{
			name:          "path traversal with ..",
			filePath:      "../../../etc/passwd",
			expectError:   true,
			errorContains: "path traversal",
		},
		{
			name:          "path traversal with .. in middle",
			filePath:      "scripts/../../etc/passwd",
			expectError:   true,
			errorContains: "path traversal",
		},
		{
			name:          "null byte in path",
			filePath:      "script\x00.js",
			expectError:   true,
			errorContains: "invalid characters",
		},
		{
			name:          "empty path",
			filePath:      "",
			expectError:   true,
			errorContains: "must be provided", // Empty path triggers "either script or scriptFile must be provided"
		},
		{
			name:        "valid relative path",
			filePath:    "scripts/transform.js",
			expectError: false,
		},
		{
			name:        "valid absolute path",
			filePath:    "/tmp/script.js",
			expectError: false, // Absolute paths are allowed but logged
		},
		{
			name:        "filename with .. in middle (not traversal)",
			filePath:    "scripts/..hidden/transform.js",
			expectError: false, // Should not be rejected - .. is part of filename, not traversal
		},
		{
			name:        "filename starting with .. (not traversal)",
			filePath:    "scripts/..transform.js",
			expectError: false, // Should not be rejected - .. is part of filename
		},
		{
			name:          "path traversal bypass attempt",
			filePath:      "scripts/../etc/passwd",
			expectError:   true,
			errorContains: "path traversal",
		},
		{
			name:          "path traversal with multiple ..",
			filePath:      "scripts/../../etc/passwd",
			expectError:   true,
			errorContains: "path traversal",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := ScriptConfig{
				ScriptFile: tc.filePath,
			}

			_, err := NewScriptFromConfig(config)
			if tc.expectError {
				if err == nil {
					t.Fatal("expected error for path traversal attempt")
				}
				if tc.errorContains != "" && !strings.Contains(err.Error(), tc.errorContains) {
					t.Errorf("expected error to contain %q, got: %v", tc.errorContains, err)
				}
			} else {
				// For valid paths, we expect either file not found (if file doesn't exist)
				// or other validation errors, but not path traversal errors
				if err != nil {
					// Check that it's not a path traversal error
					if strings.Contains(err.Error(), "path traversal") || strings.Contains(err.Error(), "invalid characters") {
						t.Errorf("unexpected path validation error: %v", err)
					}
					// File not found is acceptable for these test cases
				}
			}
		})
	}
}

// TestScriptModule_ExportToGoMap_Array tests that arrays returned from transform are explicitly rejected.
func TestScriptModule_ExportToGoMap_Array(t *testing.T) {
	config := ScriptConfig{
		Script: `function transform(record) {
			return [record.id, record.name];
		}`,
	}

	module, err := NewScriptFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	input := []map[string]interface{}{
		{"id": 1, "name": "test"},
	}

	// Arrays should be explicitly rejected
	_, err = module.Process(context.Background(), input)
	if err == nil {
		t.Fatal("expected error when transform returns array")
	}
	if !strings.Contains(err.Error(), "returned an array") {
		t.Errorf("expected error about returning array, got: %v", err)
	}
}

// TestScriptModule_ExportToGoMap_Primitive tests that primitives returned from transform are handled correctly.
func TestScriptModule_ExportToGoMap_Primitive(t *testing.T) {
	testCases := []struct {
		name                  string
		script                string
		expectedErrorContains string
	}{
		{
			name:                  "returns string",
			script:                `function transform(record) { return "string"; }`,
			expectedErrorContains: "must return an object",
		},
		{
			name:                  "returns number",
			script:                `function transform(record) { return 42; }`,
			expectedErrorContains: "must return an object",
		},
		{
			name:                  "returns boolean",
			script:                `function transform(record) { return true; }`,
			expectedErrorContains: "must return an object",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := ScriptConfig{
				Script: tc.script,
			}

			module, err := NewScriptFromConfig(config)
			if err != nil {
				t.Fatalf("failed to create module: %v", err)
			}

			input := []map[string]interface{}{
				{"id": 1},
			}

			_, err = module.Process(context.Background(), input)
			if err == nil {
				t.Fatal("expected error when transform returns primitive")
			}
			if tc.expectedErrorContains != "" && !strings.Contains(err.Error(), tc.expectedErrorContains) {
				t.Errorf("expected error to contain %q, got: %v", tc.expectedErrorContains, err)
			}
		})
	}
}

// TestScriptModule_ExportToGoMap_ComplexNested tests complex nested object structures.
func TestScriptModule_ExportToGoMap_ComplexNested(t *testing.T) {
	config := ScriptConfig{
		Script: `function transform(record) {
			return {
				id: record.id,
				nested: {
					level1: {
						level2: {
							value: record.value * 2
						}
					},
					array: [1, 2, 3],
					objects: [
						{a: 1},
						{b: 2}
					]
				}
			};
		}`,
	}

	module, err := NewScriptFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	input := []map[string]interface{}{
		{"id": 1, "value": 10},
	}

	result, err := module.Process(context.Background(), input)
	if err != nil {
		t.Fatalf("processing failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result))
	}

	// Verify nested structure
	nested, ok := result[0]["nested"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected nested to be map, got %T", result[0]["nested"])
	}

	level1, ok := nested["level1"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected level1 to be map, got %T", nested["level1"])
	}

	level2, ok := level1["level2"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected level2 to be map, got %T", level1["level2"])
	}

	value, ok := level2["value"].(float64)
	if !ok {
		valueInt, ok2 := level2["value"].(int64)
		if !ok2 {
			t.Fatalf("expected value to be numeric, got %T", level2["value"])
		}
		value = float64(valueInt)
	}
	if value != 20.0 {
		t.Errorf("expected value=20.0, got %v", value)
	}
}

// TestScriptModule_ComplexBusinessLogic tests complex transformation scenarios.
func TestScriptModule_ComplexBusinessLogic(t *testing.T) {
	config := ScriptConfig{
		Script: `function transform(record) {
			// Calculate discount based on quantity
			var discount = 0;
			if (record.quantity >= 10) {
				discount = 0.15; // 15% for bulk orders
			} else if (record.quantity >= 5) {
				discount = 0.10; // 10% for medium orders
			} else {
				discount = 0.05; // 5% for small orders
			}

			var subtotal = record.price * record.quantity;
			var discountAmount = subtotal * discount;

			return {
				orderId: record.id,
				subtotal: subtotal,
				discountPercent: discount * 100,
				discountAmount: discountAmount,
				total: subtotal - discountAmount
			};
		}`,
	}

	module, err := NewScriptFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	input := []map[string]interface{}{
		{"id": "ORD-001", "price": 100.0, "quantity": 3},  // Small: 5% discount
		{"id": "ORD-002", "price": 100.0, "quantity": 7},  // Medium: 10% discount
		{"id": "ORD-003", "price": 100.0, "quantity": 15}, // Bulk: 15% discount
	}

	result, err := module.Process(context.Background(), input)
	if err != nil {
		t.Fatalf("processing failed: %v", err)
	}

	if len(result) != 3 {
		t.Fatalf("expected 3 records, got %d", len(result))
	}

	// Verify small order (5% discount on 300 = 15)
	if result[0]["orderId"] != "ORD-001" {
		t.Errorf("expected orderId=ORD-001, got %v", result[0]["orderId"])
	}
	assertNumericEqual(t, result[0]["total"], 285.0, "small order total")

	// Verify medium order (10% discount on 700 = 70)
	assertNumericEqual(t, result[1]["total"], 630.0, "medium order total")

	// Verify bulk order (15% discount on 1500 = 225)
	assertNumericEqual(t, result[2]["total"], 1275.0, "bulk order total")
}

// assertNumericEqual compares numeric values handling both int64 and float64.
func assertNumericEqual(t *testing.T, actual interface{}, expected float64, name string) {
	t.Helper()
	var actualFloat float64
	switch v := actual.(type) {
	case float64:
		actualFloat = v
	case int64:
		actualFloat = float64(v)
	case int:
		actualFloat = float64(v)
	default:
		t.Errorf("%s: expected numeric type, got %T", name, actual)
		return
	}
	if actualFloat != expected {
		t.Errorf("%s: expected %v, got %v", name, expected, actualFloat)
	}
}
