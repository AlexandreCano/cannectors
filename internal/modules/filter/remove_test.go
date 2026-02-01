package filter

import (
	"context"
	"strings"
	"testing"
)

func TestParseRemoveConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  map[string]interface{}
		wantErr bool
		errMsg  string
	}{
		{
			name:    "missing target and targets",
			config:  map[string]interface{}{},
			wantErr: true,
			errMsg:  "'target' or 'targets' is required",
		},
		{
			name:    "empty target without targets",
			config:  map[string]interface{}{"target": ""},
			wantErr: true,
			errMsg:  "'target' or 'targets' is required",
		},
		{
			name:    "target is not a string",
			config:  map[string]interface{}{"target": 123},
			wantErr: true,
			errMsg:  "'target' or 'targets' is required",
		},
		{
			name: "valid config with simple target",
			config: map[string]interface{}{
				"target": "id",
			},
			wantErr: false,
		},
		{
			name: "valid config with nested target",
			config: map[string]interface{}{
				"target": "metadata.version",
			},
			wantErr: false,
		},
		{
			name: "valid config with targets array",
			config: map[string]interface{}{
				"targets": []interface{}{"id", "password", "internal_id"},
			},
			wantErr: false,
		},
		{
			name: "valid config with both target and targets",
			config: map[string]interface{}{
				"target":  "id",
				"targets": []interface{}{"password"},
			},
			wantErr: false,
		},
		{
			name: "empty targets array without target",
			config: map[string]interface{}{
				"targets": []interface{}{},
			},
			wantErr: true,
			errMsg:  "'target' or 'targets' is required",
		},
		{
			name: "targets array with only empty strings",
			config: map[string]interface{}{
				"targets": []interface{}{"", ""},
			},
			wantErr: true,
			errMsg:  "'target' or 'targets' is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseRemoveConfig(tt.config)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestNewRemoveFromConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  RemoveConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "missing target and targets",
			config:  RemoveConfig{},
			wantErr: true,
			errMsg:  "at least one target field path is required",
		},
		{
			name: "valid config with single target",
			config: RemoveConfig{
				Target: "id",
			},
			wantErr: false,
		},
		{
			name: "valid config with targets",
			config: RemoveConfig{
				Targets: []string{"id", "password"},
			},
			wantErr: false,
		},
		{
			name: "valid config with both target and targets",
			config: RemoveConfig{
				Target:  "id",
				Targets: []string{"password", "secret"},
			},
			wantErr: false,
		},
		{
			name: "targets with empty strings only",
			config: RemoveConfig{
				Targets: []string{"", ""},
			},
			wantErr: true,
			errMsg:  "at least one non-empty target field path is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewRemoveFromConfig(tt.config)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestRemoveModule_Process_FieldPresent(t *testing.T) {
	tests := []struct {
		name     string
		config   RemoveConfig
		input    []map[string]interface{}
		expected []map[string]interface{}
	}{
		{
			name:   "remove existing field (AC#2)",
			config: RemoveConfig{Target: "id"},
			input: []map[string]interface{}{
				{"id": "user-123", "name": "Alice", "email": "alice@example.com"},
			},
			expected: []map[string]interface{}{
				{"name": "Alice", "email": "alice@example.com"},
			},
		},
		{
			name:   "remove field preserves other fields (AC#2)",
			config: RemoveConfig{Target: "password"},
			input: []map[string]interface{}{
				{"username": "alice", "password": "secret123", "role": "admin"},
			},
			expected: []map[string]interface{}{
				{"username": "alice", "role": "admin"},
			},
		},
		{
			name:   "process multiple records (AC#5)",
			config: RemoveConfig{Target: "internal_id"},
			input: []map[string]interface{}{
				{"id": 1, "internal_id": "int-001", "name": "Alice"},
				{"id": 2, "internal_id": "int-002", "name": "Bob"},
				{"id": 3, "internal_id": "int-003", "name": "Charlie"},
			},
			expected: []map[string]interface{}{
				{"id": 1, "name": "Alice"},
				{"id": 2, "name": "Bob"},
				{"id": 3, "name": "Charlie"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			module, err := NewRemoveFromConfig(tt.config)
			if err != nil {
				t.Fatalf("failed to create module: %v", err)
			}

			result, err := module.Process(context.Background(), tt.input)
			if err != nil {
				t.Fatalf("Process() error: %v", err)
			}

			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d records, got %d", len(tt.expected), len(result))
			}

			for i, rec := range result {
				if !deepEqualMap(rec, tt.expected[i]) {
					t.Errorf("record %d mismatch:\ngot:      %v\nexpected: %v", i, rec, tt.expected[i])
				}
			}
		})
	}
}

func TestRemoveModule_Process_FieldAbsent(t *testing.T) {
	tests := []struct {
		name     string
		config   RemoveConfig
		input    []map[string]interface{}
		expected []map[string]interface{}
	}{
		{
			name:   "remove non-existing field is no-op (AC#3)",
			config: RemoveConfig{Target: "nonexistent"},
			input: []map[string]interface{}{
				{"id": 1, "name": "Alice"},
			},
			expected: []map[string]interface{}{
				{"id": 1, "name": "Alice"},
			},
		},
		{
			name:   "remove from empty record is no-op (AC#3)",
			config: RemoveConfig{Target: "id"},
			input: []map[string]interface{}{
				{},
			},
			expected: []map[string]interface{}{
				{},
			},
		},
		{
			name:   "mixed: some records have field, some don't (AC#3)",
			config: RemoveConfig{Target: "optional"},
			input: []map[string]interface{}{
				{"id": 1, "optional": "value"},
				{"id": 2},
				{"id": 3, "optional": "another"},
			},
			expected: []map[string]interface{}{
				{"id": 1},
				{"id": 2},
				{"id": 3},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			module, err := NewRemoveFromConfig(tt.config)
			if err != nil {
				t.Fatalf("failed to create module: %v", err)
			}

			result, err := module.Process(context.Background(), tt.input)
			if err != nil {
				t.Fatalf("Process() error: %v", err)
			}

			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d records, got %d", len(tt.expected), len(result))
			}

			for i, rec := range result {
				if !deepEqualMap(rec, tt.expected[i]) {
					t.Errorf("record %d mismatch:\ngot:      %v\nexpected: %v", i, rec, tt.expected[i])
				}
			}
		})
	}
}

func TestRemoveModule_Process_NestedPath(t *testing.T) {
	tests := []struct {
		name     string
		config   RemoveConfig
		input    []map[string]interface{}
		expected []map[string]interface{}
	}{
		{
			name:   "remove nested field when present (AC#4)",
			config: RemoveConfig{Target: "metadata.version"},
			input: []map[string]interface{}{
				{"id": 1, "metadata": map[string]interface{}{"version": "1.0.0", "author": "Alice"}},
			},
			expected: []map[string]interface{}{
				{"id": 1, "metadata": map[string]interface{}{"author": "Alice"}},
			},
		},
		{
			name:   "remove deeply nested field (AC#4)",
			config: RemoveConfig{Target: "a.b.c"},
			input: []map[string]interface{}{
				{
					"id": 1,
					"a": map[string]interface{}{
						"b": map[string]interface{}{
							"c": "deep",
							"d": "keep",
						},
					},
				},
			},
			expected: []map[string]interface{}{
				{
					"id": 1,
					"a": map[string]interface{}{
						"b": map[string]interface{}{
							"d": "keep",
						},
					},
				},
			},
		},
		{
			name:   "remove nested field when intermediate key missing is no-op (AC#4)",
			config: RemoveConfig{Target: "nonexistent.field"},
			input: []map[string]interface{}{
				{"id": 1, "name": "Alice"},
			},
			expected: []map[string]interface{}{
				{"id": 1, "name": "Alice"},
			},
		},
		{
			name:   "remove nested field when leaf missing is no-op (AC#4)",
			config: RemoveConfig{Target: "metadata.nonexistent"},
			input: []map[string]interface{}{
				{"id": 1, "metadata": map[string]interface{}{"version": "1.0.0"}},
			},
			expected: []map[string]interface{}{
				{"id": 1, "metadata": map[string]interface{}{"version": "1.0.0"}},
			},
		},
		{
			name:   "remove only leaf, keep parent structure (AC#4)",
			config: RemoveConfig{Target: "user.email"},
			input: []map[string]interface{}{
				{"user": map[string]interface{}{"name": "Alice", "email": "alice@example.com"}},
			},
			expected: []map[string]interface{}{
				{"user": map[string]interface{}{"name": "Alice"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			module, err := NewRemoveFromConfig(tt.config)
			if err != nil {
				t.Fatalf("failed to create module: %v", err)
			}

			result, err := module.Process(context.Background(), tt.input)
			if err != nil {
				t.Fatalf("Process() error: %v", err)
			}

			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d records, got %d", len(tt.expected), len(result))
			}

			for i, rec := range result {
				if !deepEqualMap(rec, tt.expected[i]) {
					t.Errorf("record %d mismatch:\ngot:      %v\nexpected: %v", i, rec, tt.expected[i])
				}
			}
		})
	}
}

func TestRemoveModule_Process_MultipleTargets(t *testing.T) {
	tests := []struct {
		name     string
		config   RemoveConfig
		input    []map[string]interface{}
		expected []map[string]interface{}
	}{
		{
			name:   "remove multiple flat fields with targets array",
			config: RemoveConfig{Targets: []string{"password", "internal_id", "secret"}},
			input: []map[string]interface{}{
				{"id": 1, "name": "Alice", "password": "secret123", "internal_id": "int-001", "secret": "key"},
			},
			expected: []map[string]interface{}{
				{"id": 1, "name": "Alice"},
			},
		},
		{
			name:   "remove multiple fields including nested",
			config: RemoveConfig{Targets: []string{"password", "metadata.secret", "internal_id"}},
			input: []map[string]interface{}{
				{
					"id":          1,
					"name":        "Alice",
					"password":    "secret123",
					"internal_id": "int-001",
					"metadata":    map[string]interface{}{"secret": "key", "version": "1.0.0"},
				},
			},
			expected: []map[string]interface{}{
				{
					"id":       1,
					"name":     "Alice",
					"metadata": map[string]interface{}{"version": "1.0.0"},
				},
			},
		},
		{
			name: "combine target and targets (backward compatibility)",
			config: RemoveConfig{
				Target:  "password",
				Targets: []string{"internal_id", "secret"},
			},
			input: []map[string]interface{}{
				{"id": 1, "name": "Alice", "password": "secret123", "internal_id": "int-001", "secret": "key"},
			},
			expected: []map[string]interface{}{
				{"id": 1, "name": "Alice"},
			},
		},
		{
			name:   "remove with some fields missing",
			config: RemoveConfig{Targets: []string{"password", "nonexistent", "internal_id"}},
			input: []map[string]interface{}{
				{"id": 1, "name": "Alice", "password": "secret123"},
			},
			expected: []map[string]interface{}{
				{"id": 1, "name": "Alice"},
			},
		},
		{
			name:   "remove duplicates in targets",
			config: RemoveConfig{Targets: []string{"password", "password", "internal_id", "internal_id"}},
			input: []map[string]interface{}{
				{"id": 1, "password": "secret123", "internal_id": "int-001"},
			},
			expected: []map[string]interface{}{
				{"id": 1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			module, err := NewRemoveFromConfig(tt.config)
			if err != nil {
				t.Fatalf("failed to create module: %v", err)
			}

			result, err := module.Process(context.Background(), tt.input)
			if err != nil {
				t.Fatalf("Process() error: %v", err)
			}

			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d records, got %d", len(tt.expected), len(result))
			}

			for i, rec := range result {
				if !deepEqualMap(rec, tt.expected[i]) {
					t.Errorf("record %d mismatch:\ngot:      %v\nexpected: %v", i, rec, tt.expected[i])
				}
			}
		})
	}
}

func TestRemoveModule_Process_InFilterChain(t *testing.T) {
	// Test that remove filter works correctly in a chain with other filters (AC#5)
	// Simulate: set filter → remove filter
	t.Run("remove after set in filter chain", func(t *testing.T) {
		// First, apply set filter
		setModule, err := NewSetFromConfig(SetConfig{Target: "status", Value: "processed"})
		if err != nil {
			t.Fatalf("failed to create set module: %v", err)
		}

		// Then, apply remove filter with multiple targets
		removeModule, err := NewRemoveFromConfig(RemoveConfig{Targets: []string{"internal_id", "secret"}})
		if err != nil {
			t.Fatalf("failed to create remove module: %v", err)
		}

		input := []map[string]interface{}{
			{"id": 1, "internal_id": "int-001", "secret": "key", "name": "Alice"},
		}

		// Apply set
		afterSet, err := setModule.Process(context.Background(), input)
		if err != nil {
			t.Fatalf("set Process() error: %v", err)
		}

		// Verify set worked
		if afterSet[0]["status"] != "processed" {
			t.Errorf("set filter failed: expected status=processed, got %v", afterSet[0]["status"])
		}

		// Apply remove
		afterRemove, err := removeModule.Process(context.Background(), afterSet)
		if err != nil {
			t.Fatalf("remove Process() error: %v", err)
		}

		// Verify final result
		expected := map[string]interface{}{
			"id":     1,
			"name":   "Alice",
			"status": "processed",
		}

		if !deepEqualMap(afterRemove[0], expected) {
			t.Errorf("final result mismatch:\ngot:      %v\nexpected: %v", afterRemove[0], expected)
		}

		// Verify fields were removed
		if _, exists := afterRemove[0]["internal_id"]; exists {
			t.Error("internal_id should have been removed")
		}
		if _, exists := afterRemove[0]["secret"]; exists {
			t.Error("secret should have been removed")
		}
	})
}

func TestRemoveModule_Process_EmptyInput(t *testing.T) {
	module, err := NewRemoveFromConfig(RemoveConfig{Target: "id"})
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	result, err := module.Process(context.Background(), []map[string]interface{}{})
	if err != nil {
		t.Fatalf("Process() error: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("expected 0 records, got %d", len(result))
	}
}

func TestRemoveModule_Process_ContextCancellation(t *testing.T) {
	module, err := NewRemoveFromConfig(RemoveConfig{Target: "id"})
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = module.Process(ctx, []map[string]interface{}{{"id": 1}})
	if err == nil {
		t.Error("expected context cancellation error, got nil")
	}
}

func TestRemoveModule_Process_Deterministic(t *testing.T) {
	// Verify that execution is deterministic (AC#5)
	module, err := NewRemoveFromConfig(RemoveConfig{Targets: []string{"temp", "secret"}})
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	input := []map[string]interface{}{
		{"id": 1, "temp": "remove-me", "secret": "key", "keep": "value"},
		{"id": 2, "temp": "also-remove", "secret": "key2", "keep": "value2"},
	}

	// Run multiple times and verify same result
	for i := 0; i < 5; i++ {
		// Deep copy input for each run
		inputCopy := make([]map[string]interface{}, len(input))
		for j, rec := range input {
			inputCopy[j] = make(map[string]interface{})
			for k, v := range rec {
				inputCopy[j][k] = v
			}
		}

		result, err := module.Process(context.Background(), inputCopy)
		if err != nil {
			t.Fatalf("run %d: Process() error: %v", i, err)
		}

		// Verify result
		if len(result) != 2 {
			t.Fatalf("run %d: expected 2 records, got %d", i, len(result))
		}

		for j, rec := range result {
			if _, exists := rec["temp"]; exists {
				t.Errorf("run %d, record %d: temp field should have been removed", i, j)
			}
			if _, exists := rec["secret"]; exists {
				t.Errorf("run %d, record %d: secret field should have been removed", i, j)
			}
			if rec["keep"] == nil {
				t.Errorf("run %d, record %d: keep field should be preserved", i, j)
			}
		}
	}
}

func TestRemoveModule_Process_WithArrayIndex(t *testing.T) {
	// Test array index handling in path
	t.Run("remove from array element", func(t *testing.T) {
		module, err := NewRemoveFromConfig(RemoveConfig{Target: "items[0]"})
		if err != nil {
			t.Fatalf("failed to create module: %v", err)
		}

		input := []map[string]interface{}{
			{"id": 1, "items": []interface{}{"first", "second", "third"}},
		}

		result, err := module.Process(context.Background(), input)
		if err != nil {
			t.Fatalf("Process() error: %v", err)
		}

		items, ok := result[0]["items"].([]interface{})
		if !ok {
			t.Fatalf("items should be an array")
		}

		if len(items) != 2 {
			t.Errorf("expected 2 items after removal, got %d", len(items))
		}

		if items[0] != "second" || items[1] != "third" {
			t.Errorf("unexpected items after removal: %v", items)
		}
	})
}
