// Package config provides functionality for parsing and validating
// pipeline configuration files (JSON/YAML).
package config

import (
	"testing"
)

func TestConvertToPipeline_ValidConfig(t *testing.T) {
	// Arrange
	data := map[string]interface{}{
		"name":        "test-connector",
		"version":     "1.0.0",
		"description": "A test connector",
		"input": map[string]interface{}{
			"type":     "httpPolling",
			"endpoint": "https://api.example.com/data",
			"schedule": "*/5 * * * *",
			"method":   "GET",
		},
		"filters": []interface{}{
			map[string]interface{}{
				"type": "mapping",
				"mappings": []interface{}{
					map[string]interface{}{"source": "id", "target": "externalId"},
				},
			},
		},
		"output": map[string]interface{}{
			"type":     "httpRequest",
			"endpoint": "https://api.destination.com/import",
			"method":   "POST",
		},
	}

	// Act
	pipeline, err := ConvertToPipeline(data)

	// Assert
	if err != nil {
		t.Fatalf("ConvertToPipeline() error = %v", err)
	}

	if pipeline == nil {
		t.Fatal("ConvertToPipeline() returned nil pipeline")
	}

	if pipeline.Name != "test-connector" {
		t.Errorf("Expected name 'test-connector', got '%s'", pipeline.Name)
	}

	if pipeline.Version != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", pipeline.Version)
	}

	if pipeline.Description != "A test connector" {
		t.Errorf("Expected description 'A test connector', got '%s'", pipeline.Description)
	}

	if pipeline.Input == nil {
		t.Fatal("Expected non-nil input")
	}

	if pipeline.Input.Type != "httpPolling" {
		t.Errorf("Expected input type 'httpPolling', got '%s'", pipeline.Input.Type)
	}

	if len(pipeline.Filters) != 1 {
		t.Fatalf("Expected 1 filter, got %d", len(pipeline.Filters))
	}

	if pipeline.Filters[0].Type != "mapping" {
		t.Errorf("Expected filter type 'mapping', got '%s'", pipeline.Filters[0].Type)
	}

	if pipeline.Output == nil {
		t.Fatal("Expected non-nil output")
	}

	if pipeline.Output.Type != "httpRequest" {
		t.Errorf("Expected output type 'httpRequest', got '%s'", pipeline.Output.Type)
	}

	// Schedule should be in input config, not pipeline level
	schedule, ok := pipeline.Input.Config["schedule"].(string)
	if !ok || schedule != "*/5 * * * *" {
		t.Errorf("Expected schedule '*/5 * * * *' in input config, got '%v'", pipeline.Input.Config["schedule"])
	}

	if !pipeline.Enabled {
		t.Error("Expected pipeline to be enabled by default")
	}
}

func TestConvertToPipeline_NilData(t *testing.T) {
	// Act
	pipeline, err := ConvertToPipeline(nil)

	// Assert
	if err == nil {
		t.Error("Expected error for nil data")
	}

	if pipeline != nil {
		t.Error("Expected nil pipeline for nil data")
	}
}

func TestConvertToPipeline_MissingRequiredFields(t *testing.T) {
	// Arrange - empty data
	data := map[string]interface{}{}

	// Act
	pipeline, err := ConvertToPipeline(data)

	// Assert
	if err == nil {
		t.Error("Expected error for missing required fields")
	}

	if pipeline != nil {
		t.Error("Expected nil pipeline for missing required fields")
	}
}

func TestConvertToPipeline_MissingIndividualFields(t *testing.T) {
	tests := []struct {
		name    string
		data    map[string]interface{}
		wantErr string
	}{
		{
			name: "missing name",
			data: map[string]interface{}{
				"input":  map[string]interface{}{"type": "httpPolling"},
				"output": map[string]interface{}{"type": "httpRequest"},
			},
			wantErr: "missing required field 'name'",
		},
		{
			name: "missing input",
			data: map[string]interface{}{
				"name":   "test",
				"output": map[string]interface{}{"type": "httpRequest"},
			},
			wantErr: "missing or invalid 'input' section",
		},
		{
			name: "missing output",
			data: map[string]interface{}{
				"name":  "test",
				"input": map[string]interface{}{"type": "httpPolling"},
			},
			wantErr: "missing or invalid 'output' section",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline, err := ConvertToPipeline(tt.data)

			if err == nil {
				t.Errorf("Expected error containing '%s'", tt.wantErr)
			}

			if pipeline != nil {
				t.Error("Expected nil pipeline for missing required field")
			}
		})
	}
}

func TestConvertToPipeline_NoFilters(t *testing.T) {
	// Arrange
	data := map[string]interface{}{
		"name":    "test-no-filters",
		"version": "1.0.0",
		"input":   map[string]interface{}{"type": "httpPolling"},
		"output":  map[string]interface{}{"type": "httpRequest"},
	}

	// Act
	pipeline, err := ConvertToPipeline(data)

	// Assert
	if err != nil {
		t.Fatalf("ConvertToPipeline() error = %v", err)
	}

	if len(pipeline.Filters) != 0 {
		t.Errorf("Expected 0 filters, got %d", len(pipeline.Filters))
	}
}

func TestConvertToPipeline_WithAuthentication(t *testing.T) {
	// Arrange
	data := map[string]interface{}{
		"name":    "test-auth",
		"version": "1.0.0",
		"input": map[string]interface{}{
			"type":     "httpPolling",
			"endpoint": "https://api.example.com",
			"authentication": map[string]interface{}{
				"type": "bearer",
				"credentials": map[string]interface{}{
					"token": "secret-token",
				},
			},
		},
		"output": map[string]interface{}{
			"type": "httpRequest",
		},
	}

	// Act
	pipeline, err := ConvertToPipeline(data)

	// Assert
	if err != nil {
		t.Fatalf("ConvertToPipeline() error = %v", err)
	}

	if pipeline.Input.Authentication == nil {
		t.Fatal("Expected non-nil authentication")
	}

	if pipeline.Input.Authentication.Type != "bearer" {
		t.Errorf("Expected auth type 'bearer', got '%s'", pipeline.Input.Authentication.Type)
	}

	if pipeline.Input.Authentication.Credentials["token"] != "secret-token" {
		t.Errorf("Expected token 'secret-token', got '%s'", pipeline.Input.Authentication.Credentials["token"])
	}
}

func TestConvertToPipeline_WithErrorHandling(t *testing.T) {
	// Arrange
	data := map[string]interface{}{
		"name":    "test-error-handling",
		"version": "1.0.0",
		"input":   map[string]interface{}{"type": "httpPolling"},
		"output":  map[string]interface{}{"type": "httpRequest"},
		"errorHandling": map[string]interface{}{
			"retry": map[string]interface{}{
				"maxAttempts": float64(3),
				"delayMs":     float64(1000),
			},
			"onError": "stop",
		},
	}

	// Act
	pipeline, err := ConvertToPipeline(data)

	// Assert
	if err != nil {
		t.Fatalf("ConvertToPipeline() error = %v", err)
	}

	if pipeline.ErrorHandling == nil {
		t.Fatal("Expected non-nil error handling")
	}

	if pipeline.ErrorHandling.Retry == nil {
		t.Fatal("Expected non-nil retry config")
	}

	if v, ok := pipeline.ErrorHandling.Retry["maxAttempts"].(float64); !ok || int(v) != 3 {
		t.Errorf("Expected retry maxAttempts 3, got %v", pipeline.ErrorHandling.Retry["maxAttempts"])
	}

	if v, ok := pipeline.ErrorHandling.Retry["delayMs"].(float64); !ok || int(v) != 1000 {
		t.Errorf("Expected retry delayMs 1000, got %v", pipeline.ErrorHandling.Retry["delayMs"])
	}

	if pipeline.ErrorHandling.OnError != "stop" {
		t.Errorf("Expected onError 'stop', got '%s'", pipeline.ErrorHandling.OnError)
	}
}

func TestConvertToPipeline_ModuleConfigFields(t *testing.T) {
	// Arrange - test that module config fields are properly extracted
	data := map[string]interface{}{
		"name":    "test-config-fields",
		"version": "1.0.0",
		"input": map[string]interface{}{
			"type":     "httpPolling",
			"endpoint": "https://api.example.com",
			"schedule": "0 * * * *",
			"method":   "GET",
			"headers": map[string]interface{}{
				"Accept": "application/json",
			},
		},
		"output": map[string]interface{}{"type": "httpRequest"},
	}

	// Act
	pipeline, err := ConvertToPipeline(data)

	// Assert
	if err != nil {
		t.Fatalf("ConvertToPipeline() error = %v", err)
	}

	// Check that config fields are properly stored
	if endpoint, ok := pipeline.Input.Config["endpoint"].(string); !ok || endpoint != "https://api.example.com" {
		t.Errorf("Expected endpoint 'https://api.example.com', got '%v'", pipeline.Input.Config["endpoint"])
	}

	if method, ok := pipeline.Input.Config["method"].(string); !ok || method != "GET" {
		t.Errorf("Expected method 'GET', got '%v'", pipeline.Input.Config["method"])
	}

	// Headers should be included in config
	if _, ok := pipeline.Input.Config["headers"]; !ok {
		t.Error("Expected headers in config")
	}
}

func TestConvertToPipeline_MultipleFilters(t *testing.T) {
	// Arrange
	data := map[string]interface{}{
		"name":    "test-multiple-filters",
		"version": "1.0.0",
		"input":   map[string]interface{}{"type": "httpPolling"},
		"filters": []interface{}{
			map[string]interface{}{"type": "mapping"},
			map[string]interface{}{"type": "condition"},
			map[string]interface{}{"type": "transform"},
		},
		"output": map[string]interface{}{"type": "httpRequest"},
	}

	// Act
	pipeline, err := ConvertToPipeline(data)

	// Assert
	if err != nil {
		t.Fatalf("ConvertToPipeline() error = %v", err)
	}

	if len(pipeline.Filters) != 3 {
		t.Fatalf("Expected 3 filters, got %d", len(pipeline.Filters))
	}

	expectedTypes := []string{"mapping", "condition", "transform"}
	for i, expected := range expectedTypes {
		if pipeline.Filters[i].Type != expected {
			t.Errorf("Filter %d: expected type '%s', got '%s'", i, expected, pipeline.Filters[i].Type)
		}
	}
}

func TestConvertToPipeline_ScheduleInInputConfig(t *testing.T) {
	// Test that schedule defined at input level is stored in input config
	data := map[string]interface{}{
		"name":    "test-schedule-in-input",
		"version": "1.0.0",
		"input": map[string]interface{}{
			"type":     "httpPolling",
			"endpoint": "https://api.example.com",
			"schedule": "*/5 * * * *",
		},
		"filters": []interface{}{},
		"output":  map[string]interface{}{"type": "httpRequest"},
	}

	pipeline, err := ConvertToPipeline(data)
	if err != nil {
		t.Fatalf("ConvertToPipeline() error = %v", err)
	}

	// Schedule should be in input config
	schedule, ok := pipeline.Input.Config["schedule"].(string)
	if !ok || schedule != "*/5 * * * *" {
		t.Errorf("Expected schedule '*/5 * * * *' in input config, got '%v'", pipeline.Input.Config["schedule"])
	}
}

func TestConvertToPipeline_WebhookWithoutSchedule(t *testing.T) {
	// Test that webhook input type works without schedule
	data := map[string]interface{}{
		"name":    "test-webhook",
		"version": "1.0.0",
		"input": map[string]interface{}{
			"type": "webhook",
			"path": "/webhook",
		},
		"filters": []interface{}{},
		"output":  map[string]interface{}{"type": "httpRequest"},
	}

	pipeline, err := ConvertToPipeline(data)
	if err != nil {
		t.Fatalf("ConvertToPipeline() error = %v", err)
	}

	// Webhook should not have schedule
	if _, hasSchedule := pipeline.Input.Config["schedule"]; hasSchedule {
		t.Error("Webhook input should not have schedule")
	}
}

func TestConvertToPipeline_UsesNameAsID(t *testing.T) {
	// When id is not specified, name should be used as ID
	data := map[string]interface{}{
		"name":    "my-connector",
		"version": "1.0.0",
		"input":   map[string]interface{}{"type": "httpPolling"},
		"output":  map[string]interface{}{"type": "httpRequest"},
	}

	pipeline, err := ConvertToPipeline(data)
	if err != nil {
		t.Fatalf("ConvertToPipeline() error = %v", err)
	}

	if pipeline.ID != "my-connector" {
		t.Errorf("Expected ID 'my-connector' (from name), got '%s'", pipeline.ID)
	}
}

func TestConvertToPipeline_ExplicitID(t *testing.T) {
	// When id is specified, it should override name as ID
	data := map[string]interface{}{
		"id":      "explicit-id",
		"name":    "my-connector",
		"version": "1.0.0",
		"input":   map[string]interface{}{"type": "httpPolling"},
		"output":  map[string]interface{}{"type": "httpRequest"},
	}

	pipeline, err := ConvertToPipeline(data)
	if err != nil {
		t.Fatalf("ConvertToPipeline() error = %v", err)
	}

	if pipeline.ID != "explicit-id" {
		t.Errorf("Expected ID 'explicit-id', got '%s'", pipeline.ID)
	}
}

func TestConvertToPipeline_WithAndWithoutVersion(t *testing.T) {
	// Test that version is optional
	tests := []struct {
		name            string
		data            map[string]interface{}
		expectedVersion string
	}{
		{
			name: "without version",
			data: map[string]interface{}{
				"name":    "test-no-version",
				"input":   map[string]interface{}{"type": "httpPolling"},
				"filters": []interface{}{},
				"output":  map[string]interface{}{"type": "httpRequest"},
			},
			expectedVersion: "",
		},
		{
			name: "with version",
			data: map[string]interface{}{
				"name":    "test-with-version",
				"version": "1.0.0",
				"input":   map[string]interface{}{"type": "httpPolling"},
				"filters": []interface{}{},
				"output":  map[string]interface{}{"type": "httpRequest"},
			},
			expectedVersion: "1.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline, err := ConvertToPipeline(tt.data)

			if err != nil {
				t.Fatalf("ConvertToPipeline() error = %v", err)
			}

			if pipeline == nil {
				t.Fatal("ConvertToPipeline() returned nil pipeline")
			}

			if pipeline.Version != tt.expectedVersion {
				t.Errorf("Expected version '%s', got '%s'", tt.expectedVersion, pipeline.Version)
			}

			if pipeline.Input == nil {
				t.Error("Expected non-nil input")
			}
			if pipeline.Output == nil {
				t.Error("Expected non-nil output")
			}
		})
	}
}

func TestResolveRetry_GranularMerge(t *testing.T) {
	tests := []struct {
		name        string
		moduleRetry map[string]interface{}
		defaults    map[string]interface{}
		wantKeys    map[string]interface{}
	}{
		{
			name:        "module overrides single field, inherits rest from defaults",
			moduleRetry: map[string]interface{}{"maxAttempts": float64(5)},
			defaults:    map[string]interface{}{"maxAttempts": float64(3), "delayMs": float64(2000), "backoffMultiplier": float64(1.5)},
			wantKeys:    map[string]interface{}{"maxAttempts": float64(5), "delayMs": float64(2000), "backoffMultiplier": float64(1.5)},
		},
		{
			name:        "no module retry uses defaults",
			moduleRetry: nil,
			defaults:    map[string]interface{}{"maxAttempts": float64(3), "delayMs": float64(1000)},
			wantKeys:    map[string]interface{}{"maxAttempts": float64(3), "delayMs": float64(1000)},
		},
		{
			name:        "module retry without defaults uses module only",
			moduleRetry: map[string]interface{}{"maxAttempts": float64(5)},
			defaults:    nil,
			wantKeys:    map[string]interface{}{"maxAttempts": float64(5)},
		},
		{
			name:        "module overrides multiple fields",
			moduleRetry: map[string]interface{}{"maxAttempts": float64(7), "delayMs": float64(500)},
			defaults:    map[string]interface{}{"maxAttempts": float64(3), "delayMs": float64(2000), "backoffMultiplier": float64(2.0)},
			wantKeys:    map[string]interface{}{"maxAttempts": float64(7), "delayMs": float64(500), "backoffMultiplier": float64(2.0)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := map[string]interface{}{
				"name":  "test-retry-merge",
				"input": map[string]interface{}{"type": "httpPolling"},
				"output": map[string]interface{}{
					"type": "httpRequest",
				},
			}
			if tt.moduleRetry != nil {
				data["input"].(map[string]interface{})["retry"] = tt.moduleRetry
			}
			if tt.defaults != nil {
				data["defaults"] = map[string]interface{}{"retry": tt.defaults}
			}

			pipeline, err := ConvertToPipeline(data)
			if err != nil {
				t.Fatalf("ConvertToPipeline() error = %v", err)
			}

			retryMap, ok := pipeline.Input.Config["retry"].(map[string]interface{})
			if !ok && tt.wantKeys != nil {
				t.Fatal("Expected retry config in input module")
			}

			for k, want := range tt.wantKeys {
				got, exists := retryMap[k]
				if !exists {
					t.Errorf("Missing key %q in merged retry config", k)
					continue
				}
				if got != want {
					t.Errorf("retry[%q] = %v, want %v", k, got, want)
				}
			}
		})
	}
}
