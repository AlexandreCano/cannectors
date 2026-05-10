package filter

import (
	"context"
	"strings"
	"testing"
)

func TestParseSetConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  map[string]any
		wantErr bool
		errMsg  string
	}{
		{
			name:    "missing target",
			config:  map[string]any{"value": "test"},
			wantErr: true,
			errMsg:  "'target' is required",
		},
		{
			name:    "empty target",
			config:  map[string]any{"target": "", "value": "test"},
			wantErr: true,
			errMsg:  "'target' is required",
		},
		{
			name:    "missing value",
			config:  map[string]any{"target": "id"},
			wantErr: true,
			errMsg:  "'value' is required",
		},
		{
			name: "valid config with string value",
			config: map[string]any{
				"target": "id",
				"value":  "literal-value",
			},
			wantErr: false,
		},
		{
			name: "valid config with null value",
			config: map[string]any{
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

func TestNewSetFromConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  SetConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "missing target",
			config:  SetConfig{Value: "test", HasValue: true},
			wantErr: true,
			errMsg:  "target field path is required",
		},
		{
			name:    "missing value",
			config:  SetConfig{Target: "id"},
			wantErr: true,
			errMsg:  "'value' is required",
		},
		{
			name: "valid config",
			config: SetConfig{
				Target:   "id",
				Value:    "value",
				HasValue: true,
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

func TestSetModule_Process_LiteralValue(t *testing.T) {
	tests := []struct {
		name     string
		config   SetConfig
		input    []map[string]any
		expected []map[string]any
	}{
		{
			name:   "set string literal on existing field (overwrite)",
			config: SetConfig{Target: "id", Value: "new-id", HasValue: true},
			input: []map[string]any{
				{"id": "old-id", "name": "Alice"},
			},
			expected: []map[string]any{
				{"id": "new-id", "name": "Alice"},
			},
		},
		{
			name:   "set string literal on missing field (create)",
			config: SetConfig{Target: "id", Value: "new-id", HasValue: true},
			input: []map[string]any{
				{"name": "Alice"},
			},
			expected: []map[string]any{
				{"id": "new-id", "name": "Alice"},
			},
		},
		{
			name:   "set integer literal",
			config: SetConfig{Target: "version", Value: 42, HasValue: true},
			input: []map[string]any{
				{"name": "Alice"},
			},
			expected: []map[string]any{
				{"name": "Alice", "version": 42},
			},
		},
		{
			name:   "set boolean literal",
			config: SetConfig{Target: "active", Value: true, HasValue: true},
			input: []map[string]any{
				{"name": "Alice"},
			},
			expected: []map[string]any{
				{"name": "Alice", "active": true},
			},
		},
		{
			name:   "set null literal",
			config: SetConfig{Target: "deleted_at", Value: nil, HasValue: true},
			input: []map[string]any{
				{"name": "Alice", "deleted_at": "2024-01-01"},
			},
			expected: []map[string]any{
				{"name": "Alice", "deleted_at": nil},
			},
		},
		{
			name:   "set float literal",
			config: SetConfig{Target: "score", Value: 3.14, HasValue: true},
			input: []map[string]any{
				{"name": "Alice"},
			},
			expected: []map[string]any{
				{"name": "Alice", "score": 3.14},
			},
		},
		{
			name:   "preserve other fields unchanged",
			config: SetConfig{Target: "status", Value: "active", HasValue: true},
			input: []map[string]any{
				{"id": 1, "name": "Alice", "email": "alice@example.com"},
			},
			expected: []map[string]any{
				{"id": 1, "name": "Alice", "email": "alice@example.com", "status": "active"},
			},
		},
		{
			name:   "process multiple records",
			config: SetConfig{Target: "status", Value: "processed", HasValue: true},
			input: []map[string]any{
				{"id": 1},
				{"id": 2},
				{"id": 3},
			},
			expected: []map[string]any{
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
		input    []map[string]any
		expected []map[string]any
		wantErr  bool
	}{
		{
			name:   "create nested path when intermediate keys missing",
			config: SetConfig{Target: "metadata.version", Value: "1.0.0", HasValue: true},
			input: []map[string]any{
				{"id": 1},
			},
			expected: []map[string]any{
				{"id": 1, "metadata": map[string]any{"version": "1.0.0"}},
			},
		},
		{
			name:   "update existing nested path",
			config: SetConfig{Target: "metadata.version", Value: "2.0.0", HasValue: true},
			input: []map[string]any{
				{"id": 1, "metadata": map[string]any{"version": "1.0.0", "author": "Alice"}},
			},
			expected: []map[string]any{
				{"id": 1, "metadata": map[string]any{"version": "2.0.0", "author": "Alice"}},
			},
		},
		{
			name:   "deeply nested path creation",
			config: SetConfig{Target: "a.b.c.d", Value: "deep", HasValue: true},
			input: []map[string]any{
				{"id": 1},
			},
			expected: []map[string]any{
				{
					"id": 1,
					"a": map[string]any{
						"b": map[string]any{
							"c": map[string]any{
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
	module, err := NewSetFromConfig(SetConfig{Target: "id", Value: "test", HasValue: true})
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	result, err := module.Process(context.Background(), []map[string]any{})
	if err != nil {
		t.Fatalf("Process() error: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("expected 0 records, got %d", len(result))
	}
}

func TestSetModule_Process_ContextCancellation(t *testing.T) {
	module, err := NewSetFromConfig(SetConfig{Target: "id", Value: "test", HasValue: true})
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = module.Process(ctx, []map[string]any{{"name": "Alice"}})
	if err == nil {
		t.Error("expected context cancellation error, got nil")
	}
}

// Helper functions

func equalValues(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok {
			return false
		}
		return deepEqualMap(av, bv)
	case []any:
		bv, ok := b.([]any)
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

func deepEqualMap(a, b map[string]any) bool {
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
