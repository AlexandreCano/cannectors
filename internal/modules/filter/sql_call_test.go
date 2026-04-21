package filter

import (
	"testing"

	"github.com/cannectors/runtime/internal/moduleconfig"
)

func TestNewSQLCallFromConfig_Validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  SQLCallConfig
		wantErr error
	}{
		{
			name:    "missing connection string",
			config:  SQLCallConfig{SQLRequestBase: moduleconfig.SQLRequestBase{Query: "SELECT 1"}},
			wantErr: ErrSQLCallMissingConnection,
		},
		{
			name:    "missing query",
			config:  SQLCallConfig{SQLRequestBase: moduleconfig.SQLRequestBase{ConnectionString: "postgres://localhost/db"}},
			wantErr: ErrSQLCallMissingQuery,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewSQLCallFromConfig(tt.config)
			if err == nil {
				t.Error("expected error, got nil")
				return
			}
			if err != tt.wantErr {
				t.Errorf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestGetSQLNestedValue(t *testing.T) {
	t.Parallel()

	record := map[string]interface{}{
		"id":   123,
		"name": "test",
		"nested": map[string]interface{}{
			"field": "value",
			"deep": map[string]interface{}{
				"key": "deep_value",
			},
		},
	}

	tests := []struct {
		name string
		path string
		want interface{}
	}{
		{
			name: "top-level field",
			path: "id",
			want: 123,
		},
		{
			name: "top-level string",
			path: "name",
			want: "test",
		},
		{
			name: "nested field",
			path: "nested.field",
			want: "value",
		},
		{
			name: "deeply nested field",
			path: "nested.deep.key",
			want: "deep_value",
		},
		{
			name: "non-existent field",
			path: "nonexistent",
			want: nil,
		},
		{
			name: "non-existent nested field",
			path: "nested.nonexistent",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getSQLNestedValue(record, tt.path)
			if got != tt.want {
				t.Errorf("getSQLNestedValue(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestDeepMerge(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a    map[string]interface{}
		b    map[string]interface{}
		want map[string]interface{}
	}{
		{
			name: "simple merge",
			a:    map[string]interface{}{"a": 1, "b": 2},
			b:    map[string]interface{}{"c": 3},
			want: map[string]interface{}{"a": 1, "b": 2, "c": 3},
		},
		{
			name: "override value",
			a:    map[string]interface{}{"a": 1, "b": 2},
			b:    map[string]interface{}{"b": 10},
			want: map[string]interface{}{"a": 1, "b": 10},
		},
		{
			name: "nested merge",
			a: map[string]interface{}{
				"outer": map[string]interface{}{
					"a": 1,
					"b": 2,
				},
			},
			b: map[string]interface{}{
				"outer": map[string]interface{}{
					"c": 3,
				},
			},
			want: map[string]interface{}{
				"outer": map[string]interface{}{
					"a": 1,
					"b": 2,
					"c": 3,
				},
			},
		},
		{
			name: "b nil map",
			a:    map[string]interface{}{"a": 1},
			b:    map[string]interface{}{},
			want: map[string]interface{}{"a": 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deepMerge(tt.a, tt.b)
			// Compare top-level keys
			for k, v := range tt.want {
				gotV, exists := got[k]
				if !exists {
					t.Errorf("key %q missing from result", k)
					continue
				}
				// For nested maps, just check they exist
				if _, ok := v.(map[string]interface{}); ok {
					if _, ok := gotV.(map[string]interface{}); !ok {
						t.Errorf("key %q should be a map", k)
					}
				} else if gotV != v {
					t.Errorf("got[%q] = %v, want %v", k, gotV, v)
				}
			}
		})
	}
}
