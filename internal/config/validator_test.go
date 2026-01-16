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
