package filter

import (
	"context"
	"testing"
)

func TestParseSetConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  map[string]interface{}
		wantErr bool
		errMsg  string
	}{
		{
			name:    "missing target",
			config:  map[string]interface{}{"value": "test"},
			wantErr: true,
			errMsg:  "'target' is required",
		},
		{
			name:    "empty target",
			config:  map[string]interface{}{"target": "", "value": "test"},
			wantErr: true,
			errMsg:  "'target' is required",
		},
		{
			name:    "missing value",
			config:  map[string]interface{}{"target": "id"},
			wantErr: true,
			errMsg:  "'value' is required",
		},
		{
			name: "valid config with string value",
			config: map[string]interface{}{
				"target": "id",
				"value":  "literal-value",
			},
			wantErr: false,
		},
		{
			name: "valid config with null value",
			config: map[string]interface{}{
				"target": "id",
				"value":  nil,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseSetConfig(tt.config)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
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

func TestNewSetFromConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  SetConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "missing target",
			config:  SetConfig{Value: "test"},
			wantErr: true,
			errMsg:  "target field path is required",
		},
		{
			name: "valid config",
			config: SetConfig{
				Target: "id",
				Value:  "value",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewSetFromConfig(tt.config)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
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

func TestSetModule_Process_LiteralValue(t *testing.T) {
	tests := []struct {
		name     string
		config   SetConfig
		input    []map[string]interface{}
		expected []map[string]interface{}
	}{
		{
			name:   "set string literal on existing field (overwrite)",
			config: SetConfig{Target: "id", Value: "new-id"},
			input: []map[string]interface{}{
				{"id": "old-id", "name": "Alice"},
			},
			expected: []map[string]interface{}{
				{"id": "new-id", "name": "Alice"},
			},
		},
		{
			name:   "set string literal on missing field (create)",
			config: SetConfig{Target: "id", Value: "new-id"},
			input: []map[string]interface{}{
				{"name": "Alice"},
			},
			expected: []map[string]interface{}{
				{"id": "new-id", "name": "Alice"},
			},
		},
		{
			name:   "set integer literal",
			config: SetConfig{Target: "version", Value: 42},
			input: []map[string]interface{}{
				{"name": "Alice"},
			},
			expected: []map[string]interface{}{
				{"name": "Alice", "version": 42},
			},
		},
		{
			name:   "set boolean literal",
			config: SetConfig{Target: "active", Value: true},
			input: []map[string]interface{}{
				{"name": "Alice"},
			},
			expected: []map[string]interface{}{
				{"name": "Alice", "active": true},
			},
		},
		{
			name:   "set null literal",
			config: SetConfig{Target: "deleted_at", Value: nil},
			input: []map[string]interface{}{
				{"name": "Alice", "deleted_at": "2024-01-01"},
			},
			expected: []map[string]interface{}{
				{"name": "Alice", "deleted_at": nil},
			},
		},
		{
			name:   "set float literal",
			config: SetConfig{Target: "score", Value: 3.14},
			input: []map[string]interface{}{
				{"name": "Alice"},
			},
			expected: []map[string]interface{}{
				{"name": "Alice", "score": 3.14},
			},
		},
		{
			name:   "preserve other fields unchanged",
			config: SetConfig{Target: "status", Value: "active"},
			input: []map[string]interface{}{
				{"id": 1, "name": "Alice", "email": "alice@example.com"},
			},
			expected: []map[string]interface{}{
				{"id": 1, "name": "Alice", "email": "alice@example.com", "status": "active"},
			},
		},
		{
			name:   "process multiple records",
			config: SetConfig{Target: "status", Value: "processed"},
			input: []map[string]interface{}{
				{"id": 1},
				{"id": 2},
				{"id": 3},
			},
			expected: []map[string]interface{}{
				{"id": 1, "status": "processed"},
				{"id": 2, "status": "processed"},
				{"id": 3, "status": "processed"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			module, err := NewSetFromConfig(tt.config)
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
				for key, expectedVal := range tt.expected[i] {
					actualVal, ok := rec[key]
					if !ok {
						t.Errorf("record %d: missing key %q", i, key)
						continue
					}
					if !equalValues(actualVal, expectedVal) {
						t.Errorf("record %d: key %q = %v, expected %v", i, key, actualVal, expectedVal)
					}
				}
			}
		})
	}
}

func TestSetModule_Process_NestedTargetPath(t *testing.T) {
	tests := []struct {
		name     string
		config   SetConfig
		input    []map[string]interface{}
		expected []map[string]interface{}
		wantErr  bool
	}{
		{
			name:   "create nested path when intermediate keys missing",
			config: SetConfig{Target: "metadata.version", Value: "1.0.0"},
			input: []map[string]interface{}{
				{"id": 1},
			},
			expected: []map[string]interface{}{
				{"id": 1, "metadata": map[string]interface{}{"version": "1.0.0"}},
			},
		},
		{
			name:   "update existing nested path",
			config: SetConfig{Target: "metadata.version", Value: "2.0.0"},
			input: []map[string]interface{}{
				{"id": 1, "metadata": map[string]interface{}{"version": "1.0.0", "author": "Alice"}},
			},
			expected: []map[string]interface{}{
				{"id": 1, "metadata": map[string]interface{}{"version": "2.0.0", "author": "Alice"}},
			},
		},
		{
			name:   "deeply nested path creation",
			config: SetConfig{Target: "a.b.c.d", Value: "deep"},
			input: []map[string]interface{}{
				{"id": 1},
			},
			expected: []map[string]interface{}{
				{
					"id": 1,
					"a": map[string]interface{}{
						"b": map[string]interface{}{
							"c": map[string]interface{}{
								"d": "deep",
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			module, err := NewSetFromConfig(tt.config)
			if err != nil {
				t.Fatalf("failed to create module: %v", err)
			}

			result, err := module.Process(context.Background(), tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
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

func TestSetModule_Process_EmptyInput(t *testing.T) {
	module, err := NewSetFromConfig(SetConfig{Target: "id", Value: "test"})
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

func TestSetModule_Process_ContextCancellation(t *testing.T) {
	module, err := NewSetFromConfig(SetConfig{Target: "id", Value: "test"})
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = module.Process(ctx, []map[string]interface{}{{"name": "Alice"}})
	if err == nil {
		t.Error("expected context cancellation error, got nil")
	}
}

// Helper functions

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func equalValues(a, b interface{}) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	switch av := a.(type) {
	case map[string]interface{}:
		bv, ok := b.(map[string]interface{})
		if !ok {
			return false
		}
		return deepEqualMap(av, bv)
	case []interface{}:
		bv, ok := b.([]interface{})
		if !ok {
			return false
		}
		if len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !equalValues(av[i], bv[i]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}

func deepEqualMap(a, b map[string]interface{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok {
			return false
		}
		if !equalValues(av, bv) {
			return false
		}
	}
	return true
}
