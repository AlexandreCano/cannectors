// Package config provides functionality for parsing and validating
// pipeline configuration files (JSON/YAML).
package config

import (
	"encoding/json"
	"testing"
)

func rawToMap(t *testing.T, raw json.RawMessage) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("Failed to unmarshal Raw: %v", err)
	}
	return m
}

func TestConvertToPipeline_ValidConfig(t *testing.T) {
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

	pipeline, err := ConvertToPipeline(data)
	if err != nil {
		t.Fatalf("ConvertToPipeline() error = %v", err)
	}
	if pipeline == nil {
		t.Fatal("ConvertToPipeline() returned nil pipeline")
		return
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

	// Schedule should be in Raw
	inputRaw := rawToMap(t, pipeline.Input.Raw)
	if inputRaw["schedule"] != "*/5 * * * *" {
		t.Errorf("Expected schedule '*/5 * * * *' in Raw, got '%v'", inputRaw["schedule"])
	}

	if !pipeline.Enabled {
		t.Error("Expected pipeline to be enabled by default")
	}
}

func TestConvertToPipeline_NilData(t *testing.T) {
	pipeline, err := ConvertToPipeline(nil)
	if err == nil {
		t.Error("Expected error for nil data")
	}
	if pipeline != nil {
		t.Error("Expected nil pipeline for nil data")
	}
}

func TestConvertToPipeline_MissingRequiredFields(t *testing.T) {
	data := map[string]interface{}{}
	pipeline, err := ConvertToPipeline(data)
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
	data := map[string]interface{}{
		"name":    "test-no-filters",
		"version": "1.0.0",
		"input":   map[string]interface{}{"type": "httpPolling"},
		"output":  map[string]interface{}{"type": "httpRequest"},
	}

	pipeline, err := ConvertToPipeline(data)
	if err != nil {
		t.Fatalf("ConvertToPipeline() error = %v", err)
	}
	if len(pipeline.Filters) != 0 {
		t.Errorf("Expected 0 filters, got %d", len(pipeline.Filters))
	}
}

func TestConvertToPipeline_WithAuthentication(t *testing.T) {
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

	pipeline, err := ConvertToPipeline(data)
	if err != nil {
		t.Fatalf("ConvertToPipeline() error = %v", err)
	}

	// Authentication is now inside Raw
	inputRaw := rawToMap(t, pipeline.Input.Raw)
	authRaw, ok := inputRaw["authentication"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected authentication in Raw")
	}
	if authRaw["type"] != "bearer" {
		t.Errorf("Expected auth type 'bearer', got '%v'", authRaw["type"])
	}
	creds, ok := authRaw["credentials"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected credentials in authentication")
	}
	if creds["token"] != "secret-token" {
		t.Errorf("Expected token 'secret-token', got '%v'", creds["token"])
	}
}

func TestConvertToPipeline_WithDefaults(t *testing.T) {
	data := map[string]interface{}{
		"name":    "test-defaults",
		"version": "1.0.0",
		"input":   map[string]interface{}{"type": "httpPolling"},
		"output":  map[string]interface{}{"type": "httpRequest"},
		"defaults": map[string]interface{}{
			"retry": map[string]interface{}{
				"maxAttempts": float64(3),
				"delayMs":     float64(1000),
			},
			"onError": "stop",
		},
	}

	pipeline, err := ConvertToPipeline(data)
	if err != nil {
		t.Fatalf("ConvertToPipeline() error = %v", err)
	}
	if pipeline.Defaults == nil {
		t.Fatal("Expected non-nil defaults")
	}
	if pipeline.Defaults.Retry == nil {
		t.Fatal("Expected non-nil retry config")
	}
	if pipeline.Defaults.Retry.MaxAttempts != 3 {
		t.Errorf("Expected retry maxAttempts 3, got %d", pipeline.Defaults.Retry.MaxAttempts)
	}
	if pipeline.Defaults.Retry.DelayMs != 1000 {
		t.Errorf("Expected retry delayMs 1000, got %d", pipeline.Defaults.Retry.DelayMs)
	}
	if pipeline.Defaults.OnError != "stop" {
		t.Errorf("Expected onError 'stop', got '%s'", pipeline.Defaults.OnError)
	}
}

func TestConvertToPipeline_ModuleConfigFields(t *testing.T) {
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

	pipeline, err := ConvertToPipeline(data)
	if err != nil {
		t.Fatalf("ConvertToPipeline() error = %v", err)
	}

	inputRaw := rawToMap(t, pipeline.Input.Raw)
	if inputRaw["endpoint"] != "https://api.example.com" {
		t.Errorf("Expected endpoint 'https://api.example.com', got '%v'", inputRaw["endpoint"])
	}
	if inputRaw["method"] != "GET" {
		t.Errorf("Expected method 'GET', got '%v'", inputRaw["method"])
	}
	if _, ok := inputRaw["headers"]; !ok {
		t.Error("Expected headers in Raw")
	}
}

func TestConvertToPipeline_MultipleFilters(t *testing.T) {
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

	pipeline, err := ConvertToPipeline(data)
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

	inputRaw := rawToMap(t, pipeline.Input.Raw)
	if inputRaw["schedule"] != "*/5 * * * *" {
		t.Errorf("Expected schedule '*/5 * * * *' in Raw, got '%v'", inputRaw["schedule"])
	}
}

func TestConvertToPipeline_WebhookWithoutSchedule(t *testing.T) {
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

	inputRaw := rawToMap(t, pipeline.Input.Raw)
	if _, hasSchedule := inputRaw["schedule"]; hasSchedule {
		t.Error("Webhook input should not have schedule")
	}
}

func TestConvertToPipeline_UsesNameAsID(t *testing.T) {
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

func TestConvertToPipeline_RawPopulated(t *testing.T) {
	data := map[string]interface{}{
		"name": "test-raw",
		"input": map[string]interface{}{
			"type":     "httpPolling",
			"endpoint": "https://api.example.com",
			"schedule": "*/5 * * * *",
		},
		"output": map[string]interface{}{
			"type":     "httpRequest",
			"endpoint": "https://dest.example.com",
			"method":   "POST",
		},
	}

	pipeline, err := ConvertToPipeline(data)
	if err != nil {
		t.Fatalf("ConvertToPipeline() error = %v", err)
	}

	if pipeline.Input.Raw == nil {
		t.Fatal("Expected Input.Raw to be populated")
	}

	inputRaw := rawToMap(t, pipeline.Input.Raw)
	if inputRaw["endpoint"] != "https://api.example.com" {
		t.Errorf("Expected endpoint in Raw, got %v", inputRaw["endpoint"])
	}
	if inputRaw["schedule"] != "*/5 * * * *" {
		t.Errorf("Expected schedule in Raw, got %v", inputRaw["schedule"])
	}
	if _, hasType := inputRaw["type"]; hasType {
		t.Error("Raw should not contain 'type'")
	}

	if pipeline.Output.Raw == nil {
		t.Fatal("Expected Output.Raw to be populated")
	}
}

func TestConvertToPipeline_RawWithAuthentication(t *testing.T) {
	data := map[string]interface{}{
		"name": "test-raw-auth",
		"input": map[string]interface{}{
			"type":     "httpPolling",
			"endpoint": "https://api.example.com",
			"authentication": map[string]interface{}{
				"type": "bearer",
				"credentials": map[string]interface{}{
					"token": "secret",
				},
			},
		},
		"output": map[string]interface{}{"type": "httpRequest"},
	}

	pipeline, err := ConvertToPipeline(data)
	if err != nil {
		t.Fatalf("ConvertToPipeline() error = %v", err)
	}

	inputRaw := rawToMap(t, pipeline.Input.Raw)
	authRaw, ok := inputRaw["authentication"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected authentication in Raw")
	}
	if authRaw["type"] != "bearer" {
		t.Errorf("Expected auth type 'bearer' in Raw, got %v", authRaw["type"])
	}
}

func TestConvertToPipeline_RawWithResolvedDefaults(t *testing.T) {
	data := map[string]interface{}{
		"name": "test-raw-defaults",
		"input": map[string]interface{}{
			"type":     "httpPolling",
			"endpoint": "https://api.example.com",
		},
		"defaults": map[string]interface{}{
			"onError":   "skip",
			"timeoutMs": float64(60000),
		},
		"output": map[string]interface{}{"type": "httpRequest"},
	}

	pipeline, err := ConvertToPipeline(data)
	if err != nil {
		t.Fatalf("ConvertToPipeline() error = %v", err)
	}

	inputRaw := rawToMap(t, pipeline.Input.Raw)
	if inputRaw["onError"] != "skip" {
		t.Errorf("Expected onError 'skip' in Raw, got %v", inputRaw["onError"])
	}
	if inputRaw["timeoutMs"] != float64(60000) {
		t.Errorf("Expected timeoutMs 60000 in Raw, got %v", inputRaw["timeoutMs"])
	}
}

func TestConvertToPipeline_RawFilterPopulated(t *testing.T) {
	data := map[string]interface{}{
		"name":  "test-raw-filter",
		"input": map[string]interface{}{"type": "httpPolling"},
		"filters": []interface{}{
			map[string]interface{}{
				"type":   "set",
				"target": "status",
				"value":  "active",
			},
		},
		"output": map[string]interface{}{"type": "httpRequest"},
	}

	pipeline, err := ConvertToPipeline(data)
	if err != nil {
		t.Fatalf("ConvertToPipeline() error = %v", err)
	}
	if len(pipeline.Filters) != 1 {
		t.Fatalf("Expected 1 filter, got %d", len(pipeline.Filters))
	}
	if pipeline.Filters[0].Raw == nil {
		t.Fatal("Expected filter Raw to be populated")
	}

	filterRaw := rawToMap(t, pipeline.Filters[0].Raw)
	if filterRaw["target"] != "status" {
		t.Errorf("Expected target 'status' in Raw, got %v", filterRaw["target"])
	}
}

func TestConvertToPipeline_WithAndWithoutVersion(t *testing.T) {
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
				return
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
				inputMap, ok := data["input"].(map[string]interface{})
				if !ok {
					t.Fatalf("data[\"input\"] is not map[string]interface{}")
				}
				inputMap["retry"] = tt.moduleRetry
			}
			if tt.defaults != nil {
				data["defaults"] = map[string]interface{}{"retry": tt.defaults}
			}

			pipeline, err := ConvertToPipeline(data)
			if err != nil {
				t.Fatalf("ConvertToPipeline() error = %v", err)
			}

			// Unmarshal Raw and check retry values
			inputRaw := rawToMap(t, pipeline.Input.Raw)
			retryRaw, ok := inputRaw["retry"].(map[string]interface{})
			if !ok && tt.wantKeys != nil {
				t.Fatal("Expected retry config in input Raw")
			}

			for k, want := range tt.wantKeys {
				got, exists := retryRaw[k]
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
