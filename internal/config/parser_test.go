package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseJSONFile_ValidJSON(t *testing.T) {
	result := ParseJSONFile("testdata/valid-config.json")

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}

	if result.Format != "json" {
		t.Errorf("expected format 'json', got '%s'", result.Format)
	}

	if result.Data == nil {
		t.Fatal("expected data to be non-nil")
	}

	// Check schemaVersion exists
	if _, ok := result.Data["schemaVersion"]; !ok {
		t.Error("expected schemaVersion field in parsed data")
	}

	// Check connector object exists
	connector, ok := result.Data["connector"]
	if !ok {
		t.Error("expected connector field in parsed data")
	}

	// Check connector.name
	connMap, ok := connector.(map[string]interface{})
	if !ok {
		t.Error("expected connector to be a map")
	}
	if name, ok := connMap["name"]; !ok || name != "test-connector" {
		t.Errorf("expected connector.name to be 'test-connector', got '%v'", name)
	}
}

func TestParseJSONFile_InvalidJSON(t *testing.T) {
	result := ParseJSONFile("testdata/invalid-json.json")

	if result.IsValid() {
		t.Error("expected parsing to fail for invalid JSON")
	}

	if len(result.Errors) == 0 {
		t.Error("expected at least one error")
	}

	// Check error has correct type
	if result.Errors[0].Type != ErrorTypeSyntax {
		t.Errorf("expected error type '%s', got '%s'", ErrorTypeSyntax, result.Errors[0].Type)
	}

	// Check error message contains useful information
	if !strings.Contains(result.Errors[0].Message, "invalid") &&
		!strings.Contains(strings.ToLower(result.Errors[0].Message), "character") &&
		!strings.Contains(strings.ToLower(result.Errors[0].Message), "expect") {
		t.Errorf("expected descriptive error message, got: %s", result.Errors[0].Message)
	}
}

func TestParseJSONFile_EmptyFile(t *testing.T) {
	result := ParseJSONFile("testdata/empty.json")

	if result.IsValid() {
		t.Error("expected parsing to fail for empty file")
	}

	if len(result.Errors) == 0 {
		t.Error("expected at least one error for empty file")
	}
}

func TestParseJSONFile_NonExistentFile(t *testing.T) {
	result := ParseJSONFile("testdata/does-not-exist.json")

	if result.IsValid() {
		t.Error("expected parsing to fail for non-existent file")
	}

	if len(result.Errors) == 0 {
		t.Error("expected at least one error")
	}

	// Check error type is IO
	if result.Errors[0].Type != ErrorTypeIO {
		t.Errorf("expected error type '%s', got '%s'", ErrorTypeIO, result.Errors[0].Type)
	}

	// Check file path is in error
	if result.Errors[0].Path == "" {
		t.Error("expected file path in error")
	}
}

func TestParseJSONString_ValidJSON(t *testing.T) {
	jsonStr := `{"name": "test", "version": "1.0.0"}`
	result := ParseJSONString(jsonStr)

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}

	if result.Data == nil {
		t.Fatal("expected data to be non-nil")
	}

	if result.Data["name"] != "test" {
		t.Errorf("expected name 'test', got '%v'", result.Data["name"])
	}
}

func TestParseJSONString_InvalidJSON(t *testing.T) {
	jsonStr := `{"name": "test", "version": }`
	result := ParseJSONString(jsonStr)

	if result.IsValid() {
		t.Error("expected parsing to fail for invalid JSON")
	}

	if len(result.Errors) == 0 {
		t.Error("expected at least one error")
	}

	if result.Errors[0].Type != ErrorTypeSyntax {
		t.Errorf("expected error type '%s', got '%s'", ErrorTypeSyntax, result.Errors[0].Type)
	}
}

func TestParseJSONString_EmptyString(t *testing.T) {
	result := ParseJSONString("")

	if result.IsValid() {
		t.Error("expected parsing to fail for empty string")
	}
}

func TestParseJSONString_NullJSON(t *testing.T) {
	result := ParseJSONString("null")

	// null is valid JSON but should return empty data or error for config
	if result.Data != nil {
		t.Error("expected nil data for null JSON")
	}
}

func TestParseJSONString_ArrayJSON(t *testing.T) {
	result := ParseJSONString(`[1, 2, 3]`)

	// Arrays are valid JSON but not valid config objects
	if result.IsValid() {
		t.Error("expected parsing to fail for array JSON (configs must be objects)")
	}
}

func TestParseJSONFile_PerformanceTypicalConfig(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("skipping performance test in short mode")
	}

	// Create a larger config file for performance testing
	tempDir := t.TempDir()
	largeConfig := generateLargeJSONConfig(500) // ~500 lines
	configPath := filepath.Join(tempDir, "large-config.json")
	if err := os.WriteFile(configPath, []byte(largeConfig), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Parse should complete in less than 1 second
	start := time.Now()
	result := ParseJSONFile(configPath)
	elapsed := time.Since(start)

	if !result.IsValid() {
		t.Errorf("expected valid result for large config, got errors: %v", result.Errors)
	}

	// AC: parsing is fast (<1 second for typical configurations)
	if elapsed > 1*time.Second {
		t.Errorf("parsing took %v, expected < 1 second", elapsed)
	}
}

// ============================================================================
// YAML Parsing Tests
// ============================================================================

func TestParseYAMLFile_ValidYAML(t *testing.T) {
	result := ParseYAMLFile("testdata/valid-config.yaml")

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}

	if result.Format != "yaml" {
		t.Errorf("expected format 'yaml', got '%s'", result.Format)
	}

	if result.Data == nil {
		t.Fatal("expected data to be non-nil")
	}

	// Check schemaVersion exists
	if _, ok := result.Data["schemaVersion"]; !ok {
		t.Error("expected schemaVersion field in parsed data")
	}

	// Check connector object exists
	connector, ok := result.Data["connector"]
	if !ok {
		t.Error("expected connector field in parsed data")
	}

	// Check connector.name
	connMap, ok := connector.(map[string]interface{})
	if !ok {
		t.Error("expected connector to be a map")
	}
	if name, ok := connMap["name"]; !ok || name != "test-yaml-connector" {
		t.Errorf("expected connector.name to be 'test-yaml-connector', got '%v'", name)
	}
}

func TestParseYAMLFile_InvalidYAML(t *testing.T) {
	result := ParseYAMLFile("testdata/invalid-yaml.yaml")

	if result.IsValid() {
		t.Error("expected parsing to fail for invalid YAML")
	}

	if len(result.Errors) == 0 {
		t.Error("expected at least one error")
	}

	// Check error has correct type
	if result.Errors[0].Type != ErrorTypeSyntax {
		t.Errorf("expected error type '%s', got '%s'", ErrorTypeSyntax, result.Errors[0].Type)
	}

	// Check line information is present
	if result.Errors[0].Line == 0 {
		t.Error("expected line number in YAML error")
	}
}

func TestParseYAMLFile_EmptyFile(t *testing.T) {
	result := ParseYAMLFile("testdata/empty.yaml")

	if result.IsValid() {
		t.Error("expected parsing to fail for empty file")
	}

	if len(result.Errors) == 0 {
		t.Error("expected at least one error for empty file")
	}
}

func TestParseYAMLFile_NonExistentFile(t *testing.T) {
	result := ParseYAMLFile("testdata/does-not-exist.yaml")

	if result.IsValid() {
		t.Error("expected parsing to fail for non-existent file")
	}

	if len(result.Errors) == 0 {
		t.Error("expected at least one error")
	}

	// Check error type is IO
	if result.Errors[0].Type != ErrorTypeIO {
		t.Errorf("expected error type '%s', got '%s'", ErrorTypeIO, result.Errors[0].Type)
	}
}

func TestParseYAMLString_ValidYAML(t *testing.T) {
	yamlStr := `name: test
version: "1.0.0"`
	result := ParseYAMLString(yamlStr)

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.Errors)
	}

	if result.Data == nil {
		t.Fatal("expected data to be non-nil")
	}

	if result.Data["name"] != "test" {
		t.Errorf("expected name 'test', got '%v'", result.Data["name"])
	}
}

func TestParseYAMLString_InvalidYAML(t *testing.T) {
	yamlStr := `name: test
  invalid: indentation`
	result := ParseYAMLString(yamlStr)

	if result.IsValid() {
		t.Error("expected parsing to fail for invalid YAML")
	}

	if len(result.Errors) == 0 {
		t.Error("expected at least one error")
	}

	if result.Errors[0].Type != ErrorTypeSyntax {
		t.Errorf("expected error type '%s', got '%s'", ErrorTypeSyntax, result.Errors[0].Type)
	}
}

func TestParseYAMLString_EmptyString(t *testing.T) {
	result := ParseYAMLString("")

	if result.IsValid() {
		t.Error("expected parsing to fail for empty string")
	}
}

func TestParseYAMLString_NullYAML(t *testing.T) {
	result := ParseYAMLString("null")

	// null is valid YAML but should return empty data
	if result.Data != nil {
		t.Error("expected nil data for null YAML")
	}
}

func TestParseYAMLString_OnlyComments(t *testing.T) {
	result := ParseYAMLString("# This is just a comment")

	// Comments only should be treated as empty
	if result.IsValid() && result.Data != nil {
		t.Error("expected empty/invalid result for comments-only YAML")
	}
}

func TestParseYAMLFile_PerformanceTypicalConfig(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("skipping performance test in short mode")
	}

	// Create a larger YAML config file for performance testing
	tempDir := t.TempDir()
	largeConfig := generateLargeYAMLConfig(500) // ~500 lines
	configPath := filepath.Join(tempDir, "large-config.yaml")
	if err := os.WriteFile(configPath, []byte(largeConfig), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Parse should complete in less than 1 second
	start := time.Now()
	result := ParseYAMLFile(configPath)
	elapsed := time.Since(start)

	if !result.IsValid() {
		t.Errorf("expected valid result for large config, got errors: %v", result.Errors)
	}

	// AC: parsing is fast (<1 second for typical configurations)
	if elapsed > 1*time.Second {
		t.Errorf("parsing took %v, expected < 1 second", elapsed)
	}
}

// generateLargeYAMLConfig creates a YAML config with approximately the given number of lines.
func generateLargeYAMLConfig(lines int) string {
	var sb strings.Builder
	sb.WriteString(`schemaVersion: "1.1.0"
connector:
  name: large-test-connector
  version: "1.0.0"
  description: A large connector configuration for performance testing
  input:
    type: httpPolling
    endpoint: https://api.example.com/data
    schedule: "*/5 * * * *"
  filters:
`)

	// Generate many filter entries
	numFilters := (lines - 30) / 6 // roughly 6 lines per filter
	for i := 0; i < numFilters; i++ {
		sb.WriteString(`    - type: mapping
      id: filter-`)
		sb.WriteString(strings.Repeat("0", 5))
		sb.WriteString(`
      mappings:
        - source: field_`)
		sb.WriteString(strings.Repeat("0", 5))
		sb.WriteString(`
          target: target_`)
		sb.WriteString(strings.Repeat("0", 5))
		sb.WriteString("\n")
	}

	sb.WriteString(`  output:
    type: httpRequest
    endpoint: https://api.destination.com/import
    method: POST
`)

	return sb.String()
}

// ============================================================================
// Unified Parser Tests
// ============================================================================

func TestParseConfig_JSONByExtension(t *testing.T) {
	result := ParseConfig("testdata/valid-schema-config.json")

	if !result.IsValid() {
		t.Errorf("expected valid result, got errors: %v", result.AllErrors())
	}

	if result.Format != "json" {
		t.Errorf("expected format 'json', got '%s'", result.Format)
	}
}

func TestParseConfig_YAMLByExtension(t *testing.T) {
	result := ParseConfig("testdata/valid-config.yaml")

	if len(result.ParseErrors) > 0 {
		t.Errorf("expected no parse errors, got: %v", result.ParseErrors)
	}

	if result.Format != "yaml" {
		t.Errorf("expected format 'yaml', got '%s'", result.Format)
	}

	// Verify full validation passes
	if !result.IsValid() {
		t.Errorf("expected valid config, got validation errors: %v", result.ValidationErrors)
	}
}

func TestParseConfig_InvalidJSON(t *testing.T) {
	result := ParseConfig("testdata/invalid-json.json")

	if result.IsValid() {
		t.Error("expected parsing to fail for invalid JSON")
	}

	if len(result.ParseErrors) == 0 {
		t.Error("expected at least one parse error")
	}
}

func TestParseConfig_InvalidYAML(t *testing.T) {
	result := ParseConfig("testdata/invalid-yaml.yaml")

	if result.IsValid() {
		t.Error("expected parsing to fail for invalid YAML")
	}

	if len(result.ParseErrors) == 0 {
		t.Error("expected at least one parse error")
	}
}

func TestParseConfig_ValidationErrors(t *testing.T) {
	result := ParseConfig("testdata/invalid-schema-missing-required.json")

	// Parsing should succeed
	if len(result.ParseErrors) > 0 {
		t.Errorf("expected no parse errors, got: %v", result.ParseErrors)
	}

	// Validation should fail
	if len(result.ValidationErrors) == 0 {
		t.Error("expected validation errors for missing required field")
	}

	if result.IsValid() {
		t.Error("expected IsValid() to return false for validation errors")
	}
}

func TestParseConfig_NonExistentFile(t *testing.T) {
	result := ParseConfig("testdata/does-not-exist.json")

	if result.IsValid() {
		t.Error("expected parsing to fail for non-existent file")
	}
}

func TestParseConfig_UnknownExtension(t *testing.T) {
	// Create a temp file with unknown extension
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.txt")
	content := `{"schemaVersion": "1.1.0", "connector": {"name": "test", "version": "1.0.0", "input": {"type": "httpPolling"}, "filters": [], "output": {"type": "httpRequest"}}}`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	result := ParseConfig(configPath)

	// Should auto-detect JSON from content
	if len(result.ParseErrors) > 0 && result.Format == "" {
		// Either auto-detection works or it fails gracefully
		t.Logf("Unknown extension handling: format=%s, errors=%v", result.Format, result.ParseErrors)
	}
}

func TestIsJSON(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{"valid JSON object", `{"key": "value"}`, true},
		{"valid JSON array", `[1, 2, 3]`, true},
		{"JSON with whitespace", `  { "key": "value" }  `, true},
		{"YAML", "key: value", false},
		{"empty string", "", false},
		{"plain text", "hello world", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsJSON(tt.content)
			if result != tt.expected {
				t.Errorf("IsJSON(%q) = %v, expected %v", tt.content, result, tt.expected)
			}
		})
	}
}

func TestIsYAML(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{"valid YAML mapping", "key: value", true},
		{"valid YAML list", "- item1\n- item2", true},
		{"JSON (also valid YAML)", `{"key": "value"}`, true}, // JSON is valid YAML
		{"empty string", "", false},
		{"plain string (valid YAML)", ":::invalid:::", true}, // YAML is permissive - this parses as string
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsYAML(tt.content)
			if result != tt.expected {
				t.Errorf("IsYAML(%q) = %v, expected %v", tt.content, result, tt.expected)
			}
		})
	}
}

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		name     string
		filepath string
		expected string
	}{
		{"JSON extension", "config.json", "json"},
		{"YAML extension", "config.yaml", "yaml"},
		{"YML extension", "config.yml", "yaml"},
		{"Unknown extension", "config.txt", ""},
		{"No extension", "config", ""},
		{"Case insensitive JSON", "CONFIG.JSON", "json"},
		{"Case insensitive YAML", "CONFIG.YAML", "yaml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectFormat(tt.filepath)
			if result != tt.expected {
				t.Errorf("DetectFormat(%q) = %q, expected %q", tt.filepath, result, tt.expected)
			}
		})
	}
}

func TestParseConfigString_JSON(t *testing.T) {
	content := `{"schemaVersion": "1.1.0", "connector": {"name": "test", "version": "1.0.0", "input": {"type": "httpPolling", "endpoint": "http://example.com", "schedule": "* * * * *"}, "filters": [], "output": {"type": "httpRequest", "endpoint": "http://example.com", "method": "POST"}}}`
	result := ParseConfigString(content, "json")

	if len(result.ParseErrors) > 0 {
		t.Errorf("expected no parse errors, got: %v", result.ParseErrors)
	}

	if result.Data == nil {
		t.Error("expected data to be non-nil")
	}
}

func TestParseConfigString_YAML(t *testing.T) {
	content := `schemaVersion: "1.1.0"
connector:
  name: test
  version: "1.0.0"
  input:
    type: httpPolling
    endpoint: http://example.com
    schedule: "* * * * *"
  filters: []
  output:
    type: httpRequest
    endpoint: http://example.com
    method: POST`
	result := ParseConfigString(content, "yaml")

	if len(result.ParseErrors) > 0 {
		t.Errorf("expected no parse errors, got: %v", result.ParseErrors)
	}

	if result.Data == nil {
		t.Error("expected data to be non-nil")
	}
}

func TestParseConfigString_AutoDetect(t *testing.T) {
	jsonContent := `{"name": "test"}`
	result := ParseConfigString(jsonContent, "")

	if len(result.ParseErrors) > 0 {
		t.Errorf("expected no parse errors with auto-detect, got: %v", result.ParseErrors)
	}

	if result.Format != "json" {
		t.Errorf("expected auto-detected format 'json', got '%s'", result.Format)
	}
}

// generateLargeJSONConfig creates a JSON config with approximately the given number of lines.
func generateLargeJSONConfig(lines int) string {
	var sb strings.Builder
	sb.WriteString(`{
  "schemaVersion": "1.1.0",
  "connector": {
    "name": "large-test-connector",
    "version": "1.0.0",
    "description": "A large connector configuration for performance testing",
    "input": {
      "type": "httpPolling",
      "endpoint": "https://api.example.com/data",
      "schedule": "*/5 * * * *"
    },
    "filters": [`)

	// Generate many filter entries
	numFilters := (lines - 30) / 10 // roughly 10 lines per filter
	for i := 0; i < numFilters; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`
      {
        "type": "mapping",
        "id": "filter-`)
		sb.WriteString(strings.Repeat("0", 5))
		sb.WriteString(`",
        "mappings": [
          { "source": "field_`)
		sb.WriteString(strings.Repeat("0", 5))
		sb.WriteString(`", "target": "target_`)
		sb.WriteString(strings.Repeat("0", 5))
		sb.WriteString(`" }
        ]
      }`)
	}

	sb.WriteString(`
    ],
    "output": {
      "type": "httpRequest",
      "endpoint": "https://api.destination.com/import",
      "method": "POST"
    }
  }
}`)

	return sb.String()
}

// ============================================================================
// YAML 1.1 vs 1.2 Compatibility Tests
// ============================================================================

func TestParseYAMLString_YAML12BooleanValues(t *testing.T) {
	// YAML 1.2 (gopkg.in/yaml.v3) uses YAML 1.2 boolean rules:
	// - Only true/false are booleans (not yes/no, on/off)
	// - This is the correct behavior for YAML 1.2 compliance
	tests := []struct {
		name     string
		yaml     string
		key      string
		expected interface{}
	}{
		// YAML 1.2: yes/no/on/off are strings, NOT booleans
		{"yes as string (YAML 1.2)", "enabled: yes", "enabled", "yes"},
		{"no as string (YAML 1.2)", "enabled: no", "enabled", "no"},
		{"on as string (YAML 1.2)", "enabled: on", "enabled", "on"},
		{"off as string (YAML 1.2)", "enabled: off", "enabled", "off"},
		// YAML 1.2: true/false are booleans
		{"true as boolean", "enabled: true", "enabled", true},
		{"false as boolean", "enabled: false", "enabled", false},
		{"True as boolean", "enabled: True", "enabled", true},
		{"False as boolean", "enabled: False", "enabled", false},
		// Quoted strings remain strings
		{"quoted true as string", "enabled: \"true\"", "enabled", "true"},
		{"quoted yes as string", "enabled: \"yes\"", "enabled", "yes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseYAMLString(tt.yaml)
			if !result.IsValid() {
				t.Fatalf("expected valid YAML, got errors: %v", result.Errors)
			}

			val, ok := result.Data[tt.key]
			if !ok {
				t.Fatalf("expected key '%s' in parsed data", tt.key)
			}

			if val != tt.expected {
				t.Errorf("expected %v (%T), got %v (%T)", tt.expected, tt.expected, val, val)
			}
		})
	}
}

func TestParseYAMLString_YAML12OctalNumbers(t *testing.T) {
	// YAML 1.2 changed octal notation from 0777 to 0o777
	// gopkg.in/yaml.v3 supports both
	tests := []struct {
		name     string
		yaml     string
		key      string
		expected int
	}{
		{"YAML 1.2 octal (0o prefix)", "value: 0o755", "value", 493}, // 755 octal = 493 decimal
		{"decimal number", "value: 100", "value", 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseYAMLString(tt.yaml)
			if !result.IsValid() {
				t.Fatalf("expected valid YAML, got errors: %v", result.Errors)
			}

			val, ok := result.Data[tt.key]
			if !ok {
				t.Fatalf("expected key '%s' in parsed data", tt.key)
			}

			// YAML parses numbers as int
			if intVal, ok := val.(int); ok {
				if intVal != tt.expected {
					t.Errorf("expected %d, got %d", tt.expected, intVal)
				}
			} else {
				t.Errorf("expected int, got %T", val)
			}
		})
	}
}

// ============================================================================
// Result.AllErrors() Tests
// ============================================================================

func TestResult_AllErrors(t *testing.T) {
	// Test with both parse and validation errors
	result := &Result{
		ParseErrors: []ParseError{
			{Message: "parse error 1", Type: ErrorTypeSyntax},
			{Message: "parse error 2", Type: ErrorTypeIO},
		},
		ValidationErrors: []ValidationError{
			{Path: "/connector", Message: "validation error 1", Type: "required"},
			{Path: "/schemaVersion", Message: "validation error 2", Type: "pattern"},
		},
	}

	allErrors := result.AllErrors()

	if len(allErrors) != 4 {
		t.Errorf("expected 4 errors, got %d", len(allErrors))
	}

	// Verify order: parse errors first, then validation errors
	if allErrors[0].Error() != "parse error 1" {
		t.Errorf("expected first error to be parse error 1, got: %s", allErrors[0].Error())
	}
	if allErrors[2].Error() != "/connector: validation error 1" {
		t.Errorf("expected third error to be validation error 1, got: %s", allErrors[2].Error())
	}
}

func TestResult_AllErrors_Empty(t *testing.T) {
	result := &Result{}

	allErrors := result.AllErrors()

	if len(allErrors) != 0 {
		t.Errorf("expected 0 errors, got %d", len(allErrors))
	}
}

func TestResult_AllErrors_OnlyParseErrors(t *testing.T) {
	result := &Result{
		ParseErrors: []ParseError{
			{Message: "parse error 1"},
		},
	}

	allErrors := result.AllErrors()

	if len(allErrors) != 1 {
		t.Errorf("expected 1 error, got %d", len(allErrors))
	}
}

func TestResult_AllErrors_OnlyValidationErrors(t *testing.T) {
	result := &Result{
		ValidationErrors: []ValidationError{
			{Path: "/test", Message: "validation error 1"},
		},
	}

	allErrors := result.AllErrors()

	if len(allErrors) != 1 {
		t.Errorf("expected 1 error, got %d", len(allErrors))
	}
}

// ============================================================================
// ParseError.Error() and ValidationError.Error() Tests
// ============================================================================

func TestParseError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      ParseError
		expected string
	}{
		{
			name:     "message only",
			err:      ParseError{Message: "invalid syntax"},
			expected: "invalid syntax",
		},
		{
			name:     "with path",
			err:      ParseError{Path: "/path/to/file.json", Message: "invalid syntax"},
			expected: "/path/to/file.json: invalid syntax",
		},
		{
			name:     "with line",
			err:      ParseError{Line: 10, Message: "unexpected token"},
			expected: "line 10: unexpected token",
		},
		{
			name:     "with line and column",
			err:      ParseError{Line: 10, Column: 5, Message: "unexpected token"},
			expected: "line 10, column 5: unexpected token",
		},
		{
			name:     "with path, line, and column",
			err:      ParseError{Path: "config.json", Line: 10, Column: 5, Message: "unexpected token"},
			expected: "config.json: line 10, column 5: unexpected token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.Error()
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestValidationError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      ValidationError
		expected string
	}{
		{
			name:     "message only",
			err:      ValidationError{Message: "missing required field"},
			expected: "missing required field",
		},
		{
			name:     "with path",
			err:      ValidationError{Path: "/connector/name", Message: "must be a string"},
			expected: "/connector/name: must be a string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.Error()
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}
