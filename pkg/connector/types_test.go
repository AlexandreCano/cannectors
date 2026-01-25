package connector_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/canectors/runtime/pkg/connector"
)

func TestPipelineJSONSerialization(t *testing.T) {
	pipeline := connector.Pipeline{
		ID:          "test-pipeline",
		Name:        "Test Pipeline",
		Description: "A test pipeline for validation",
		Version:     "1.0.0",
		Input: &connector.ModuleConfig{
			Type: "http-polling",
			Config: map[string]interface{}{
				"endpoint": "https://api.example.com/data",
				"schedule": "*/5 * * * *",
			},
		},
		Filters: []connector.ModuleConfig{
			{
				Type: "mapping",
				Config: map[string]interface{}{
					"mappings": map[string]string{
						"source": "target",
					},
				},
			},
		},
		Output: &connector.ModuleConfig{
			Type: "http-request",
			Config: map[string]interface{}{
				"endpoint": "https://api.dest.com/import",
			},
		},
		ErrorHandling: &connector.ErrorHandling{
			RetryCount: 3,
			RetryDelay: 5000,
			OnError:    "notify",
		},
		Enabled:   true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Test JSON marshaling
	data, err := json.Marshal(pipeline)
	if err != nil {
		t.Fatalf("Failed to marshal pipeline to JSON: %v", err)
	}

	// Test JSON unmarshaling
	var decoded connector.Pipeline
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal pipeline from JSON: %v", err)
	}

	// Verify key fields
	if decoded.ID != pipeline.ID {
		t.Errorf("Expected ID %q, got %q", pipeline.ID, decoded.ID)
	}
	if decoded.Name != pipeline.Name {
		t.Errorf("Expected Name %q, got %q", pipeline.Name, decoded.Name)
	}
	if decoded.Enabled != pipeline.Enabled {
		t.Errorf("Expected Enabled %v, got %v", pipeline.Enabled, decoded.Enabled)
	}
	if decoded.Input.Type != pipeline.Input.Type {
		t.Errorf("Expected Input.Type %q, got %q", pipeline.Input.Type, decoded.Input.Type)
	}
	if len(decoded.Filters) != len(pipeline.Filters) {
		t.Errorf("Expected %d filters, got %d", len(pipeline.Filters), len(decoded.Filters))
	}
	if decoded.Output.Type != pipeline.Output.Type {
		t.Errorf("Expected Output.Type %q, got %q", pipeline.Output.Type, decoded.Output.Type)
	}
	if decoded.ErrorHandling.RetryCount != pipeline.ErrorHandling.RetryCount {
		t.Errorf("Expected RetryCount %d, got %d", pipeline.ErrorHandling.RetryCount, decoded.ErrorHandling.RetryCount)
	}
}

func TestModuleConfigWithAuthentication(t *testing.T) {
	module := connector.ModuleConfig{
		Type: "http-polling",
		Config: map[string]interface{}{
			"endpoint": "https://api.example.com/data",
		},
		Authentication: &connector.AuthConfig{
			Type: "bearer",
			Credentials: map[string]string{
				"token": "test-token",
			},
		},
	}

	data, err := json.Marshal(module)
	if err != nil {
		t.Fatalf("Failed to marshal module config: %v", err)
	}

	var decoded connector.ModuleConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal module config: %v", err)
	}

	if decoded.Authentication == nil {
		t.Fatal("Expected authentication to be present")
	}
	if decoded.Authentication.Type != "bearer" {
		t.Errorf("Expected auth type 'bearer', got %q", decoded.Authentication.Type)
	}
	if decoded.Authentication.Credentials["token"] != "test-token" {
		t.Errorf("Expected token 'test-token', got %q", decoded.Authentication.Credentials["token"])
	}
}

func TestExecutionResult(t *testing.T) {
	result := connector.ExecutionResult{
		PipelineID:       "test-pipeline",
		Status:           "success",
		StartedAt:        time.Now().Add(-5 * time.Second),
		CompletedAt:      time.Now(),
		RecordsProcessed: 100,
		RecordsFailed:    2,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal execution result: %v", err)
	}

	var decoded connector.ExecutionResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal execution result: %v", err)
	}

	if decoded.Status != "success" {
		t.Errorf("Expected status 'success', got %q", decoded.Status)
	}
	if decoded.RecordsProcessed != 100 {
		t.Errorf("Expected 100 records processed, got %d", decoded.RecordsProcessed)
	}
}

func TestExecutionResultWithError(t *testing.T) {
	result := connector.ExecutionResult{
		PipelineID:       "test-pipeline",
		Status:           "error",
		StartedAt:        time.Now(),
		CompletedAt:      time.Now(),
		RecordsProcessed: 0,
		RecordsFailed:    0,
		Error: &connector.ExecutionError{
			Code:    "CONNECTION_FAILED",
			Message: "Failed to connect to source API",
			Module:  "http-polling",
			Details: map[string]interface{}{
				"endpoint": "https://api.example.com",
				"timeout":  30,
			},
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal execution result with error: %v", err)
	}

	var decoded connector.ExecutionResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal execution result with error: %v", err)
	}

	if decoded.Error == nil {
		t.Fatal("Expected error to be present")
	}
	if decoded.Error.Code != "CONNECTION_FAILED" {
		t.Errorf("Expected error code 'CONNECTION_FAILED', got %q", decoded.Error.Code)
	}
	if decoded.Error.Module != "http-polling" {
		t.Errorf("Expected module 'http-polling', got %q", decoded.Error.Module)
	}
}
