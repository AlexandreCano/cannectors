package config

import (
	"strings"
	"testing"
)

func TestValidateConfig_ValidConfig(t *testing.T) {
	// Parse valid config first
	parseResult := ParseJSONFile("testdata/valid-schema-config.json")
	if !parseResult.IsValid() {
		t.Fatalf("failed to parse valid config: %v", parseResult.Errors)
	}

	// Validate against schema
	result := ValidateConfig(parseResult.Data)

	if !result.Valid {
		t.Errorf("expected valid config, got errors: %v", result.Errors)
	}
}

func TestValidateConfig_ValidConfigWithoutVersion(t *testing.T) {
	// Config without version field should be valid
	data := map[string]interface{}{
		"name":    "test-no-version",
		"input":   map[string]interface{}{"type": "httpPolling", "endpoint": "https://example.com", "schedule": "* * * * *"},
		"filters": []interface{}{},
		"output":  map[string]interface{}{"type": "httpRequest", "endpoint": "https://example.com", "method": "POST"},
	}

	result := ValidateConfig(data)

	if !result.Valid {
		t.Errorf("expected valid config without version, got errors: %v", result.Errors)
	}
}

func TestValidateConfig_MissingRequiredField(t *testing.T) {
	// Parse config missing required "filters" field
	parseResult := ParseJSONFile("testdata/invalid-schema-missing-required.json")
	if !parseResult.IsValid() {
		t.Fatalf("failed to parse config: %v", parseResult.Errors)
	}

	// Validate against schema
	result := ValidateConfig(parseResult.Data)

	if result.Valid {
		t.Error("expected validation to fail for config missing required field")
	}

	if len(result.Errors) == 0 {
		t.Error("expected at least one validation error")
	}

	// Check that error mentions the missing field
	found := false
	for _, err := range result.Errors {
		if err.Type == "required" || strings.Contains(strings.ToLower(err.Message), strings.ToLower("required")) || strings.Contains(strings.ToLower(err.Message), strings.ToLower("filters")) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error about missing required field, got: %v", result.Errors)
	}
}

func TestValidateConfig_WrongType(t *testing.T) {
	// Parse config with wrong type (name should be string, not number)
	parseResult := ParseJSONFile("testdata/invalid-schema-wrong-type.json")
	if !parseResult.IsValid() {
		t.Fatalf("failed to parse config: %v", parseResult.Errors)
	}

	// Validate against schema
	result := ValidateConfig(parseResult.Data)

	if result.Valid {
		t.Error("expected validation to fail for config with wrong type")
	}

	if len(result.Errors) == 0 {
		t.Error("expected at least one validation error")
	}

	// Check that error mentions type issue
	found := false
	for _, err := range result.Errors {
		if err.Type == "type" || strings.Contains(strings.ToLower(err.Message), strings.ToLower("type")) || strings.Contains(strings.ToLower(err.Message), strings.ToLower("string")) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error about type mismatch, got: %v", result.Errors)
	}
}

func TestValidateConfig_NilData(t *testing.T) {
	result := ValidateConfig(nil)

	if result.Valid {
		t.Error("expected validation to fail for nil data")
	}
}

func TestValidateConfig_EmptyData(t *testing.T) {
	result := ValidateConfig(map[string]interface{}{})

	if result.Valid {
		t.Error("expected validation to fail for empty data")
	}
}

func TestValidationError_Path(t *testing.T) {
	// Parse config with wrong type
	parseResult := ParseJSONFile("testdata/invalid-schema-wrong-type.json")
	if !parseResult.IsValid() {
		t.Fatalf("failed to parse config: %v", parseResult.Errors)
	}

	// Validate against schema
	result := ValidateConfig(parseResult.Data)

	if result.Valid {
		t.Skip("validation passed unexpectedly, cannot test error path")
	}

	// Check that at least one error has a path
	hasPath := false
	for _, err := range result.Errors {
		if err.Path != "" {
			hasPath = true
			break
		}
	}
	if !hasPath {
		t.Error("expected at least one validation error with a JSON path")
	}
}

func TestGetSchema_ReturnsSchema(t *testing.T) {
	schema := GetEmbeddedSchema()
	if len(schema) == 0 {
		t.Error("expected embedded schema to be non-empty")
	}
}

func TestValidateConfig_WebhookWithSchedule_Rejected(t *testing.T) {
	// Webhook input must not have schedule (AC#2, #4). Schema rejects it.
	data := map[string]interface{}{
		"name":    "webhook-schedule-test",
		"version": "1.0.0",
		"input": map[string]interface{}{
			"type":     "webhook",
			"path":     "/webhook",
			"schedule": "0 * * * *",
		},
		"filters": []interface{}{},
		"output": map[string]interface{}{
			"type":     "httpRequest",
			"endpoint": "https://example.com",
			"method":   "POST",
		},
	}
	result := ValidateConfig(data)
	if result.Valid {
		t.Error("expected validation to fail for webhook input with schedule")
	}
	if len(result.Errors) == 0 {
		t.Error("expected at least one validation error")
	}
}

func TestValidateConfig_ConnectorLevelSchedule_Rejected(t *testing.T) {
	// Schedule must not be at root; only in input module (polling types).
	data := map[string]interface{}{
		"name":     "connector-schedule-test",
		"version":  "1.0.0",
		"schedule": "0 * * * *",
		"input": map[string]interface{}{
			"type":     "httpPolling",
			"endpoint": "https://example.com",
			"schedule": "*/5 * * * *",
		},
		"filters": []interface{}{},
		"output": map[string]interface{}{
			"type":     "httpRequest",
			"endpoint": "https://example.com",
			"method":   "POST",
		},
	}
	result := ValidateConfig(data)
	if result.Valid {
		t.Error("expected validation to fail for schedule at connector level")
	}
	if len(result.Errors) == 0 {
		t.Error("expected at least one validation error")
	}
}

func TestValidateConfig_ActionableMessages(t *testing.T) {
	tests := []struct {
		name       string
		data       map[string]interface{}
		wantType   string
		wantSubstr string
	}{
		{
			name: "missing required field shows property name",
			data: map[string]interface{}{
				"name":   "test",
				"input":  map[string]interface{}{"type": "httpPolling", "endpoint": "https://example.com", "schedule": "* * * * *"},
				"output": map[string]interface{}{"type": "httpRequest", "endpoint": "https://example.com", "method": "POST"},
				// missing "filters"
			},
			wantType:   "required",
			wantSubstr: "filters",
		},
		{
			name: "wrong type shows expected and actual",
			data: map[string]interface{}{
				"name":    123, // should be string
				"input":   map[string]interface{}{"type": "httpPolling", "endpoint": "https://example.com", "schedule": "* * * * *"},
				"filters": []interface{}{},
				"output":  map[string]interface{}{"type": "httpRequest", "endpoint": "https://example.com", "method": "POST"},
			},
			wantType:   "type",
			wantSubstr: "string",
		},
		{
			name: "additional property shows property name",
			data: map[string]interface{}{
				"name":     "test",
				"schedule": "* * * * *", // not allowed at root
				"input":    map[string]interface{}{"type": "httpPolling", "endpoint": "https://example.com", "schedule": "* * * * *"},
				"filters":  []interface{}{},
				"output":   map[string]interface{}{"type": "httpRequest", "endpoint": "https://example.com", "method": "POST"},
			},
			wantType:   "additionalProperties",
			wantSubstr: "schedule",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateConfig(tt.data)
			if result.Valid {
				t.Fatal("expected validation to fail")
			}

			found := false
			for _, err := range result.Errors {
				if err.Type == tt.wantType && strings.Contains(err.Message, tt.wantSubstr) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected error of type %q containing %q, got: %v", tt.wantType, tt.wantSubstr, result.Errors)
			}
		})
	}
}

func TestFormatInstanceLocation_ReadablePaths(t *testing.T) {
	tests := []struct {
		loc  []string
		want string
	}{
		{nil, "/"},
		{[]string{"input"}, "input"},
		{[]string{"filters", "0", "type"}, "filters[0].type"},
		{[]string{"output", "endpoint"}, "output.endpoint"},
		{[]string{"filters", "2", "mappings", "0"}, "filters[2].mappings[0]"},
	}

	for _, tt := range tests {
		got := formatInstanceLocation(tt.loc)
		if got != tt.want {
			t.Errorf("formatInstanceLocation(%v) = %q, want %q", tt.loc, got, tt.want)
		}
	}
}
