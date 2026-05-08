package filter

import (
	"context"
	"testing"

	"github.com/cannectors/runtime/internal/errhandling"
)

// TestMappingModule_IntegrationScenarios tests complete mapping scenarios simulating pipeline integration.
func TestMappingModule_IntegrationScenarios(t *testing.T) {
	t.Run("end-to-end transform scenario", func(t *testing.T) {
		// Simulate data coming from HTTP Polling input module
		inputRecords := []map[string]any{
			{
				"user": map[string]any{
					"firstName": "  JOHN  ",
					"lastName":  "DOE",
					"email":     "JOHN@EXAMPLE.COM",
				},
				"metadata": map[string]any{
					"createdAt": "2026-01-15T10:30:00Z",
					"tags":      "sales,vip,active",
				},
			},
			{
				"user": map[string]any{
					"firstName": "Jane",
					"lastName":  "Smith",
					"email":     "jane.smith@example.com",
				},
				"metadata": map[string]any{
					"createdAt": "2026-01-14T08:00:00Z",
					"tags":      "support",
				},
			},
		}

		// Configure mappings as they would come from connector config
		mappings := []FieldMapping{
			{Source: strPtr("user.firstName"), Target: "contact.name.first", Transforms: []TransformOp{
				{Op: "trim"},
				{Op: "lowercase"},
			}},
			{Source: strPtr("user.lastName"), Target: "contact.name.last", Transforms: []TransformOp{{Op: "lowercase"}}},
			{Source: strPtr("user.email"), Target: "contact.email", Transforms: []TransformOp{{Op: "lowercase"}}},
			{Source: strPtr("metadata.createdAt"), Target: "created", Transforms: []TransformOp{{
				Op:     "dateFormat",
				Format: "YYYY-MM-DD",
			}}},
			{Source: strPtr("metadata.tags"), Target: "labels", Transforms: []TransformOp{{
				Op:        "split",
				Separator: ",",
			}}},
			{Source: strPtr("metadata.priority"), Target: "priority", OnMissing: "useDefault", DefaultValue: "normal"},
		}

		mapper, err := NewMappingFromConfig(mappings, "fail")
		if err != nil {
			t.Fatalf("Failed to create mapper: %v", err)
		}

		// Process records (as pipeline executor would)
		result, err := mapper.Process(context.Background(), inputRecords)
		if err != nil {
			t.Fatalf("Process() failed: %v", err)
		}

		// Verify output format matches what Output module expects
		if len(result) != 2 {
			t.Fatalf("Expected 2 records, got %d", len(result))
		}

		// Verify first record transformation
		rec1 := result[0]

		// Check nested structure created correctly
		contact, ok := rec1["contact"].(map[string]any)
		if !ok {
			t.Fatalf("Expected 'contact' to be a map, got %T", rec1["contact"])
		}

		name, ok := contact["name"].(map[string]any)
		if !ok {
			t.Fatalf("Expected 'contact.name' to be a map, got %T", contact["name"])
		}

		if name["first"] != "john" {
			t.Errorf("Expected first name 'john', got %v", name["first"])
		}
		if name["last"] != "doe" {
			t.Errorf("Expected last name 'doe', got %v", name["last"])
		}

		if contact["email"] != "john@example.com" {
			t.Errorf("Expected email 'john@example.com', got %v", contact["email"])
		}

		// Check date formatting
		if rec1["created"] != "2026-01-15" {
			t.Errorf("Expected created '2026-01-15', got %v", rec1["created"])
		}

		// Check split transform
		labels, ok := rec1["labels"].([]any)
		if !ok {
			t.Fatalf("Expected 'labels' to be an array, got %T", rec1["labels"])
		}
		if len(labels) != 3 {
			t.Errorf("Expected 3 labels, got %d", len(labels))
		}

		// Check default value
		if rec1["priority"] != "normal" {
			t.Errorf("Expected priority 'normal', got %v", rec1["priority"])
		}
	})

	t.Run("handles empty input gracefully", func(t *testing.T) {
		mappings := []FieldMapping{
			{Source: strPtr("name"), Target: "fullName"},
		}

		mapper, _ := NewMappingFromConfig(mappings, "fail")

		// Empty input
		result, err := mapper.Process(context.Background(), []map[string]any{})
		if err != nil {
			t.Fatalf("Process() failed on empty input: %v", err)
		}
		if len(result) != 0 {
			t.Errorf("Expected 0 records, got %d", len(result))
		}

		// Nil input
		result, err = mapper.Process(context.Background(), nil)
		if err != nil {
			t.Fatalf("Process() failed on nil input: %v", err)
		}
		if len(result) != 0 {
			t.Errorf("Expected 0 records for nil input, got %d", len(result))
		}
	})

	t.Run("output format compatible with Output module", func(t *testing.T) {
		// Verify that output is []map[string]any which Output modules expect
		mappings := []FieldMapping{
			{Source: strPtr("data"), Target: "payload"},
		}

		mapper, _ := NewMappingFromConfig(mappings, "fail")

		input := []map[string]any{
			{"data": "test"},
		}

		result, err := mapper.Process(context.Background(), input)
		if err != nil {
			t.Fatalf("Process() failed: %v", err)
		}

		// Verify we can iterate and access values
		for _, rec := range result {
			for k, v := range rec {
				_ = k
				_ = v
			}
		}
	})
}

// TestMapping_Deterministic verifies that mapping execution is deterministic.
func TestMapping_Deterministic(t *testing.T) {
	t.Run("same input same output - multiple runs", func(t *testing.T) {
		mappings := []FieldMapping{
			{Source: strPtr("name"), Target: "fullName", Transforms: []TransformOp{
				{Op: "trim"},
				{Op: "lowercase"},
			}},
			{Source: strPtr("email"), Target: "contact.email"},
			{Source: strPtr("missing"), Target: "optional", OnMissing: "useDefault", DefaultValue: "default"},
		}

		input := []map[string]any{
			{"name": "  JOHN DOE  ", "email": "john@example.com"},
			{"name": "Jane Smith", "email": "jane@example.com"},
		}

		mapper, _ := NewMappingFromConfig(mappings, "fail")

		// Run multiple times
		var results [][]map[string]any
		for i := 0; i < 10; i++ {
			result, err := mapper.Process(context.Background(), input)
			if err != nil {
				t.Fatalf("Run %d failed: %v", i, err)
			}
			results = append(results, result)
		}

		// All results should be identical
		for i := 1; i < len(results); i++ {
			if !recordsEqual(results[0], results[i]) {
				t.Errorf("Run %d produced different result than run 0", i)
				t.Errorf("Run 0: %v", results[0])
				t.Errorf("Run %d: %v", i, results[i])
			}
		}
	})

	t.Run("deterministic transform operations", func(t *testing.T) {
		mappings := []FieldMapping{
			{Source: strPtr("text"), Target: "result", Transforms: []TransformOp{
				{Op: "trim"},
				{Op: "lowercase"},
				{Op: "replace", Pattern: "\\s+", Replacement: "_"},
			}},
		}

		input := []map[string]any{
			{"text": "  Hello   World  "},
		}

		mapper, _ := NewMappingFromConfig(mappings, "fail")

		// Run multiple times
		for i := 0; i < 10; i++ {
			result, err := mapper.Process(context.Background(), input)
			if err != nil {
				t.Fatalf("Run %d failed: %v", i, err)
			}
			if result[0]["result"] != "hello_world" {
				t.Errorf("Run %d: expected 'hello_world', got '%v'", i, result[0]["result"])
			}
		}
	})

	t.Run("deterministic date formatting", func(t *testing.T) {
		mappings := []FieldMapping{
			{Source: strPtr("date"), Target: "formatted", Transforms: []TransformOp{{
				Op:     "dateFormat",
				Format: "YYYY-MM-DD",
			}}},
		}

		input := []map[string]any{
			{"date": "2026-01-15T10:30:00Z"},
		}

		mapper, _ := NewMappingFromConfig(mappings, "fail")

		for i := 0; i < 10; i++ {
			result, err := mapper.Process(context.Background(), input)
			if err != nil {
				t.Fatalf("Run %d failed: %v", i, err)
			}
			if result[0]["formatted"] != "2026-01-15" {
				t.Errorf("Run %d: expected '2026-01-15', got '%v'", i, result[0]["formatted"])
			}
		}
	})

	t.Run("deterministic error handling", func(t *testing.T) {
		mappings := []FieldMapping{
			{Source: strPtr("required"), Target: "output", OnMissing: "fail"},
		}

		input := []map[string]any{
			{"other": "value"},
		}

		mapper, _ := NewMappingFromConfig(mappings, "fail")

		// Same error should occur every time
		for i := 0; i < 5; i++ {
			_, err := mapper.Process(context.Background(), input)
			if err == nil {
				t.Errorf("Run %d: expected error, got nil", i)
			}
			// Error message should be consistent
			if !containsString(err.Error(), "required") {
				t.Errorf("Run %d: error should mention 'required' field", i)
			}
		}
	})

	t.Run("deterministic nested path resolution", func(t *testing.T) {
		mappings := []FieldMapping{
			{Source: strPtr("user.profile.name"), Target: "output.nested.value"},
		}

		input := []map[string]any{
			{
				"user": map[string]any{
					"profile": map[string]any{
						"name": "test",
					},
				},
			},
		}

		mapper, _ := NewMappingFromConfig(mappings, "fail")

		// Run multiple times and verify structure is identical
		for i := 0; i < 10; i++ {
			result, err := mapper.Process(context.Background(), input)
			if err != nil {
				t.Fatalf("Run %d failed: %v", i, err)
			}

			// Verify nested structure
			output, ok := result[0]["output"].(map[string]any)
			if !ok {
				t.Errorf("Run %d: expected 'output' to be a map", i)
				continue
			}
			nested, ok := output["nested"].(map[string]any)
			if !ok {
				t.Errorf("Run %d: expected 'output.nested' to be a map", i)
				continue
			}
			if nested["value"] != "test" {
				t.Errorf("Run %d: expected value 'test', got '%v'", i, nested["value"])
			}
		}
	})
}

// TestMappingModule_Interface verifies MappingModule implements filter.Module.
func TestMappingModule_Interface(t *testing.T) {
	mapper, _ := NewMappingFromConfig([]FieldMapping{}, "fail")

	// Verify interface implementation at compile time
	var _ Module = mapper

	// Verify Process method signature
	records := []map[string]any{
		{"test": "data"},
	}
	_, _ = mapper.Process(context.Background(), records)
}

// TestMapping_Process_ErrorContext tests that errors include proper context.
func TestMapping_Process_ErrorContext(t *testing.T) {
	tests := []struct {
		name        string
		mappings    []FieldMapping
		input       []map[string]any
		errContains []string
	}{
		{
			name: "missing required field error includes field path and record index",
			mappings: []FieldMapping{
				{Source: strPtr("required_field"), Target: "output", OnMissing: "fail"},
			},
			input: []map[string]any{
				{"other": "value"},
			},
			errContains: []string{"required_field", "record 0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapper, err := NewMappingFromConfig(tt.mappings, "fail")
			if err != nil {
				t.Fatalf("NewMappingFromConfig() error = %v", err)
			}

			_, err = mapper.Process(context.Background(), tt.input)
			if err == nil {
				t.Errorf("Process() expected error, got nil")
				return
			}

			errStr := err.Error()
			for _, contains := range tt.errContains {
				if !containsString(errStr, contains) {
					t.Errorf("Error %q should contain %q", errStr, contains)
				}
			}
		})
	}

	// Invalid regex patterns are now caught at config parsing time (fail fast)
	t.Run("invalid regex pattern detected at config time", func(t *testing.T) {
		_, err := NewMappingFromConfig([]FieldMapping{
			{Source: strPtr("text"), Target: "output", Transforms: []TransformOp{{
				Op:      "replace",
				Pattern: "[invalid", // Invalid regex
			}}},
		}, "fail")
		if err == nil {
			t.Fatal("expected error for invalid regex pattern")
		}
		if !containsString(err.Error(), "invalid regex pattern") {
			t.Errorf("error should mention invalid regex pattern, got: %v", err)
		}
	})
}

// TestMapping_Process_TransformErrors tests transform error handling.
func TestMapping_Process_TransformErrors(t *testing.T) {
	// Invalid regex patterns are now caught at config parsing time (fail fast)
	t.Run("invalid regex detected at config time - fail mode", func(t *testing.T) {
		_, err := NewMappingFromConfig([]FieldMapping{
			{Source: strPtr("text"), Target: "output", Transforms: []TransformOp{{
				Op:      "replace",
				Pattern: "[invalid",
			}}},
		}, "fail")
		if err == nil {
			t.Fatal("expected error for invalid regex pattern")
		}
		if !containsString(err.Error(), "invalid regex pattern") {
			t.Errorf("error should mention invalid regex pattern, got: %v", err)
		}
	})

	t.Run("invalid regex detected at config time - skip mode", func(t *testing.T) {
		_, err := NewMappingFromConfig([]FieldMapping{
			{Source: strPtr("text"), Target: "output", Transforms: []TransformOp{{
				Op:      "replace",
				Pattern: "[invalid",
			}}},
		}, "skip")
		if err == nil {
			t.Fatal("expected error for invalid regex pattern even in skip mode")
		}
	})

	tests := []struct {
		name     string
		mappings []FieldMapping
		onError  string
		input    []map[string]any
		wantLen  int
		wantErr  bool
	}{
		{
			name: "invalid int conversion - fail mode",
			mappings: []FieldMapping{
				{Source: strPtr("count"), Target: "countInt", Transforms: []TransformOp{{
					Op: "toInt",
				}}},
			},
			onError: "fail",
			input: []map[string]any{
				{"count": "abc"},
			},
			wantLen: 0,
			wantErr: true,
		},
		{
			name: "unparseable date - fail mode",
			mappings: []FieldMapping{
				{Source: strPtr("date"), Target: "formatted", Transforms: []TransformOp{{
					Op: "dateFormat",
				}}},
			},
			onError: "fail",
			input: []map[string]any{
				{"date": "not-a-date"},
			},
			wantLen: 0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapper, err := NewMappingFromConfig(tt.mappings, tt.onError)
			if err != nil {
				t.Fatalf("NewMappingFromConfig() error = %v", err)
			}

			got, err := mapper.Process(context.Background(), tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Process() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("Process() unexpected error = %v", err)
				return
			}

			if len(got) != tt.wantLen {
				t.Errorf("Process() returned %d records, want %d", len(got), tt.wantLen)
			}
		})
	}
}

// Test metadata preservation through mapping filter
func TestMapping_MetadataPreservation(t *testing.T) {
	t.Run("preserves _metadata during transformation", func(t *testing.T) {
		mappings := []FieldMapping{
			{Source: strPtr("name"), Target: "fullName"},
			{Source: strPtr("age"), Target: "userAge"},
		}

		module, err := NewMappingFromConfig(mappings, string(errhandling.OnErrorFail))
		if err != nil {
			t.Fatalf("NewMappingFromConfig failed: %v", err)
		}

		records := []map[string]any{
			{
				"name": "John",
				"age":  30,
				"_metadata": map[string]any{
					"processed_at": "2024-01-01T12:00:00Z",
					"source":       "api",
				},
			},
		}

		result, err := module.Process(context.Background(), records)
		if err != nil {
			t.Fatalf("Process failed: %v", err)
		}

		if len(result) != 1 {
			t.Fatalf("expected 1 record, got %d", len(result))
		}

		// Check that mapped fields exist
		if result[0]["fullName"] != "John" {
			t.Errorf("expected fullName='John', got %v", result[0]["fullName"])
		}
		if result[0]["userAge"] != 30 {
			t.Errorf("expected userAge=30, got %v", result[0]["userAge"])
		}

		// Check that _metadata is preserved
		metadata, ok := result[0]["_metadata"].(map[string]any)
		if !ok {
			t.Fatal("expected _metadata to be preserved")
		}
		if metadata["processed_at"] != "2024-01-01T12:00:00Z" {
			t.Errorf("expected processed_at preserved, got %v", metadata["processed_at"])
		}
		if metadata["source"] != "api" {
			t.Errorf("expected source preserved, got %v", metadata["source"])
		}
	})

	t.Run("preserves all fields including underscore fields (in-place modification)", func(t *testing.T) {
		mappings := []FieldMapping{
			{Source: strPtr("data"), Target: "output"},
		}

		module, err := NewMappingFromConfig(mappings, string(errhandling.OnErrorFail))
		if err != nil {
			t.Fatalf("NewMappingFromConfig failed: %v", err)
		}

		records := []map[string]any{
			{
				"data":      "value",
				"_metadata": map[string]any{"key": "meta"},
				"_custom":   "custom_value",
				"_internal": map[string]any{"debug": true},
			},
		}

		result, err := module.Process(context.Background(), records)
		if err != nil {
			t.Fatalf("Process failed: %v", err)
		}

		// All fields should be preserved (in-place modification)
		if _, ok := result[0]["_metadata"]; !ok {
			t.Error("expected _metadata to be preserved")
		}
		if _, ok := result[0]["_custom"]; !ok {
			t.Error("expected _custom to be preserved (in-place modification)")
		}
		if _, ok := result[0]["_internal"]; !ok {
			t.Error("expected _internal to be preserved (in-place modification)")
		}
		if result[0]["output"] != "value" {
			t.Errorf("expected output='value', got %v", result[0]["output"])
		}
	})

	t.Run("_metadata available for mapping source", func(t *testing.T) {
		// This test verifies _metadata can be accessed via mapping if needed
		mappings := []FieldMapping{
			{Source: strPtr("_metadata.source"), Target: "original_source"},
			{Source: strPtr("name"), Target: "fullName"},
		}

		module, err := NewMappingFromConfig(mappings, string(errhandling.OnErrorFail))
		if err != nil {
			t.Fatalf("NewMappingFromConfig failed: %v", err)
		}

		records := []map[string]any{
			{
				"name": "Test",
				"_metadata": map[string]any{
					"source": "external-api",
				},
			},
		}

		result, err := module.Process(context.Background(), records)
		if err != nil {
			t.Fatalf("Process failed: %v", err)
		}

		// _metadata.source should be mapped to output field
		if result[0]["original_source"] != "external-api" {
			t.Errorf("expected original_source='external-api', got %v", result[0]["original_source"])
		}
		// Original fields should be preserved
		if result[0]["name"] != "Test" {
			t.Errorf("expected name='Test', got %v", result[0]["name"])
		}
	})
}

// TestMapping_FieldDeletion tests the field deletion feature (empty source).
func TestMapping_FieldDeletion(t *testing.T) {
	t.Run("empty source deletes target field", func(t *testing.T) {
		mappings := []FieldMapping{
			{Target: "password"}, // Delete password field
		}

		module, err := NewMappingFromConfig(mappings, string(errhandling.OnErrorFail))
		if err != nil {
			t.Fatalf("NewMappingFromConfig failed: %v", err)
		}

		records := []map[string]any{
			{
				"name":     "John",
				"email":    "john@example.com",
				"password": "secret123",
			},
		}

		result, err := module.Process(context.Background(), records)
		if err != nil {
			t.Fatalf("Process failed: %v", err)
		}

		// password should be deleted
		if _, ok := result[0]["password"]; ok {
			t.Error("expected password to be deleted")
		}
		// Other fields should be preserved
		if result[0]["name"] != "John" {
			t.Errorf("expected name='John', got %v", result[0]["name"])
		}
		if result[0]["email"] != "john@example.com" {
			t.Errorf("expected email='john@example.com', got %v", result[0]["email"])
		}
	})

	t.Run("delete multiple fields", func(t *testing.T) {
		mappings := []FieldMapping{
			{Target: "password"},
			{Target: "token"},
			{Target: "secret"},
		}

		module, err := NewMappingFromConfig(mappings, string(errhandling.OnErrorFail))
		if err != nil {
			t.Fatalf("NewMappingFromConfig failed: %v", err)
		}

		records := []map[string]any{
			{
				"name":     "John",
				"password": "pass123",
				"token":    "abc123",
				"secret":   "shhh",
			},
		}

		result, err := module.Process(context.Background(), records)
		if err != nil {
			t.Fatalf("Process failed: %v", err)
		}

		// Sensitive fields should be deleted
		if _, ok := result[0]["password"]; ok {
			t.Error("expected password to be deleted")
		}
		if _, ok := result[0]["token"]; ok {
			t.Error("expected token to be deleted")
		}
		if _, ok := result[0]["secret"]; ok {
			t.Error("expected secret to be deleted")
		}
		// name should be preserved
		if result[0]["name"] != "John" {
			t.Errorf("expected name='John', got %v", result[0]["name"])
		}
	})

	t.Run("delete nested field", func(t *testing.T) {
		mappings := []FieldMapping{
			{Target: "user.password"},
		}

		module, err := NewMappingFromConfig(mappings, string(errhandling.OnErrorFail))
		if err != nil {
			t.Fatalf("NewMappingFromConfig failed: %v", err)
		}

		records := []map[string]any{
			{
				"user": map[string]any{
					"name":     "John",
					"password": "secret",
				},
			},
		}

		result, err := module.Process(context.Background(), records)
		if err != nil {
			t.Fatalf("Process failed: %v", err)
		}

		user, ok := result[0]["user"].(map[string]any)
		if !ok {
			t.Fatal("expected user to be a map")
		}
		// password should be deleted from user object
		if _, ok := user["password"]; ok {
			t.Error("expected user.password to be deleted")
		}
		// name should be preserved
		if user["name"] != "John" {
			t.Errorf("expected user.name='John', got %v", user["name"])
		}
	})

	t.Run("delete entire nested object", func(t *testing.T) {
		mappings := []FieldMapping{
			{Target: "user"},
		}

		module, err := NewMappingFromConfig(mappings, string(errhandling.OnErrorFail))
		if err != nil {
			t.Fatalf("NewMappingFromConfig failed: %v", err)
		}

		records := []map[string]any{
			{
				"id": 1,
				"user": map[string]any{
					"name":     "John",
					"password": "secret",
				},
			},
		}

		result, err := module.Process(context.Background(), records)
		if err != nil {
			t.Fatalf("Process failed: %v", err)
		}

		// user object should be deleted entirely
		if _, ok := result[0]["user"]; ok {
			t.Error("expected user to be deleted entirely")
		}
		// id should be preserved
		if result[0]["id"] != 1 {
			t.Errorf("expected id=1, got %v", result[0]["id"])
		}
	})

	t.Run("delete non-existent field is no-op", func(t *testing.T) {
		mappings := []FieldMapping{
			{Target: "nonexistent"},
		}

		module, err := NewMappingFromConfig(mappings, string(errhandling.OnErrorFail))
		if err != nil {
			t.Fatalf("NewMappingFromConfig failed: %v", err)
		}

		records := []map[string]any{
			{
				"name": "John",
			},
		}

		result, err := module.Process(context.Background(), records)
		if err != nil {
			t.Fatalf("Process failed: %v", err)
		}

		// No error, record should be unchanged
		if result[0]["name"] != "John" {
			t.Errorf("expected name='John', got %v", result[0]["name"])
		}
	})

	t.Run("combine deletion and mapping", func(t *testing.T) {
		mappings := []FieldMapping{
			{Source: strPtr("name"), Target: "fullName"},
			{Target: "password"},
			{Source: strPtr("email"), Target: "contact"},
		}

		module, err := NewMappingFromConfig(mappings, string(errhandling.OnErrorFail))
		if err != nil {
			t.Fatalf("NewMappingFromConfig failed: %v", err)
		}

		records := []map[string]any{
			{
				"name":     "John",
				"email":    "john@example.com",
				"password": "secret",
			},
		}

		result, err := module.Process(context.Background(), records)
		if err != nil {
			t.Fatalf("Process failed: %v", err)
		}

		// Mapped fields should exist
		if result[0]["fullName"] != "John" {
			t.Errorf("expected fullName='John', got %v", result[0]["fullName"])
		}
		if result[0]["contact"] != "john@example.com" {
			t.Errorf("expected contact='john@example.com', got %v", result[0]["contact"])
		}
		// Deleted field should not exist
		if _, ok := result[0]["password"]; ok {
			t.Error("expected password to be deleted")
		}
		// Original fields preserved
		if result[0]["name"] != "John" {
			t.Errorf("expected name='John', got %v", result[0]["name"])
		}
		if result[0]["email"] != "john@example.com" {
			t.Errorf("expected email='john@example.com', got %v", result[0]["email"])
		}
	})
}
