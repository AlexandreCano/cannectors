// Package config provides functionality for parsing and validating
// pipeline configuration files (JSON/YAML).
package config

import (
	"testing"
)

func TestConvertToPipeline_ValidConfig(t *testing.T) {
	// Arrange
	data := map[string]interface{}{
		"schemaVersion": "1.1.0",
		"connector": map[string]interface{}{
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

	if pipeline.Schedule != "*/5 * * * *" {
		t.Errorf("Expected schedule '*/5 * * * *', got '%s'", pipeline.Schedule)
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

func TestConvertToPipeline_MissingConnectorSection(t *testing.T) {
	// Arrange
	data := map[string]interface{}{
		"schemaVersion": "1.1.0",
	}

	// Act
	pipeline, err := ConvertToPipeline(data)

	// Assert
	if err == nil {
		t.Error("Expected error for missing connector section")
	}

	if pipeline != nil {
		t.Error("Expected nil pipeline for missing connector section")
	}
}

func TestConvertToPipeline_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		data    map[string]interface{}
		wantErr string
	}{
		{
			name: "missing name",
			data: map[string]interface{}{
				"connector": map[string]interface{}{
					"version": "1.0.0",
					"input":   map[string]interface{}{"type": "httpPolling"},
					"output":  map[string]interface{}{"type": "httpRequest"},
				},
			},
			wantErr: "missing required field 'connector.name'",
		},
		{
			name: "missing version",
			data: map[string]interface{}{
				"connector": map[string]interface{}{
					"name":   "test",
					"input":  map[string]interface{}{"type": "httpPolling"},
					"output": map[string]interface{}{"type": "httpRequest"},
				},
			},
			wantErr: "missing required field 'connector.version'",
		},
		{
			name: "missing input",
			data: map[string]interface{}{
				"connector": map[string]interface{}{
					"name":    "test",
					"version": "1.0.0",
					"output":  map[string]interface{}{"type": "httpRequest"},
				},
			},
			wantErr: "missing or invalid 'connector.input' section",
		},
		{
			name: "missing output",
			data: map[string]interface{}{
				"connector": map[string]interface{}{
					"name":    "test",
					"version": "1.0.0",
					"input":   map[string]interface{}{"type": "httpPolling"},
				},
			},
			wantErr: "missing or invalid 'connector.output' section",
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
		"connector": map[string]interface{}{
			"name":    "test-no-filters",
			"version": "1.0.0",
			"input":   map[string]interface{}{"type": "httpPolling"},
			"output":  map[string]interface{}{"type": "httpRequest"},
		},
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
		"connector": map[string]interface{}{
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
		"connector": map[string]interface{}{
			"name":    "test-error-handling",
			"version": "1.0.0",
			"input":   map[string]interface{}{"type": "httpPolling"},
			"output":  map[string]interface{}{"type": "httpRequest"},
			"errorHandling": map[string]interface{}{
				"retryCount": float64(3),
				"retryDelay": float64(1000),
				"onError":    "stop",
			},
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

	if pipeline.ErrorHandling.RetryCount != 3 {
		t.Errorf("Expected retryCount 3, got %d", pipeline.ErrorHandling.RetryCount)
	}

	if pipeline.ErrorHandling.RetryDelay != 1000 {
		t.Errorf("Expected retryDelay 1000, got %d", pipeline.ErrorHandling.RetryDelay)
	}

	if pipeline.ErrorHandling.OnError != "stop" {
		t.Errorf("Expected onError 'stop', got '%s'", pipeline.ErrorHandling.OnError)
	}
}

func TestConvertToPipeline_ModuleConfigFields(t *testing.T) {
	// Arrange - test that module config fields are properly extracted
	data := map[string]interface{}{
		"connector": map[string]interface{}{
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
		},
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
		"connector": map[string]interface{}{
			"name":    "test-multiple-filters",
			"version": "1.0.0",
			"input":   map[string]interface{}{"type": "httpPolling"},
			"filters": []interface{}{
				map[string]interface{}{"type": "mapping"},
				map[string]interface{}{"type": "condition"},
				map[string]interface{}{"type": "transform"},
			},
			"output": map[string]interface{}{"type": "httpRequest"},
		},
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

func TestConvertToPipeline_UsesNameAsID(t *testing.T) {
	// When id is not specified, name should be used as ID
	data := map[string]interface{}{
		"connector": map[string]interface{}{
			"name":    "my-connector",
			"version": "1.0.0",
			"input":   map[string]interface{}{"type": "httpPolling"},
			"output":  map[string]interface{}{"type": "httpRequest"},
		},
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
		"connector": map[string]interface{}{
			"id":      "explicit-id",
			"name":    "my-connector",
			"version": "1.0.0",
			"input":   map[string]interface{}{"type": "httpPolling"},
			"output":  map[string]interface{}{"type": "httpRequest"},
		},
	}

	pipeline, err := ConvertToPipeline(data)
	if err != nil {
		t.Fatalf("ConvertToPipeline() error = %v", err)
	}

	if pipeline.ID != "explicit-id" {
		t.Errorf("Expected ID 'explicit-id', got '%s'", pipeline.ID)
	}
}
