package filter

import (
	"testing"

	"github.com/cannectors/runtime/internal/moduleconfig"
	"github.com/cannectors/runtime/internal/recordpath"
)

// TestMarshalDeterministic_StableForPermutedKeys verifies AC #1 of Story 17.3:
// two records with the same data but keys inserted in different orders must
// produce the same cache key.
func TestMarshalDeterministic_StableForPermutedKeys(t *testing.T) {
	t.Parallel()

	a := map[string]interface{}{"a": 1, "b": 2, "c": "three"}
	b := map[string]interface{}{"c": "three", "a": 1, "b": 2}

	gotA, err := marshalDeterministic(a)
	if err != nil {
		t.Fatalf("marshalDeterministic(a) error = %v", err)
	}
	gotB, err := marshalDeterministic(b)
	if err != nil {
		t.Fatalf("marshalDeterministic(b) error = %v", err)
	}
	if string(gotA) != string(gotB) {
		t.Errorf("permuted keys produced different output:\n  a=%s\n  b=%s", gotA, gotB)
	}
}

// TestMarshalDeterministic_NestedKeysSorted verifies AC #3: nested maps must
// also be canonicalised so that {"u":{"x":1,"y":2}} and {"u":{"y":2,"x":1}}
// share the same serialization.
func TestMarshalDeterministic_NestedKeysSorted(t *testing.T) {
	t.Parallel()

	a := map[string]interface{}{
		"user": map[string]interface{}{"id": 1, "name": "x", "addr": map[string]interface{}{"city": "Paris", "zip": "75001"}},
	}
	b := map[string]interface{}{
		"user": map[string]interface{}{"name": "x", "addr": map[string]interface{}{"zip": "75001", "city": "Paris"}, "id": 1},
	}

	gotA, _ := marshalDeterministic(a)
	gotB, _ := marshalDeterministic(b)
	if string(gotA) != string(gotB) {
		t.Errorf("nested permutations produced different output:\n  a=%s\n  b=%s", gotA, gotB)
	}
}

// TestMarshalDeterministic_StressLoop verifies AC #4: 1000 iterations on a
// permuted record always yield the same cache key.
func TestMarshalDeterministic_StressLoop(t *testing.T) {
	t.Parallel()

	const iterations = 1000
	first, err := marshalDeterministic(map[string]interface{}{
		"a": 1, "b": 2, "c": 3, "d": 4, "e": 5,
		"nested": map[string]interface{}{"x": "X", "y": "Y", "z": "Z"},
	})
	if err != nil {
		t.Fatalf("marshalDeterministic error = %v", err)
	}
	for i := 0; i < iterations; i++ {
		// Each iteration uses a freshly-built map (Go map iteration is randomized).
		got, err := marshalDeterministic(map[string]interface{}{
			"e": 5, "c": 3, "a": 1, "d": 4, "b": 2,
			"nested": map[string]interface{}{"z": "Z", "x": "X", "y": "Y"},
		})
		if err != nil {
			t.Fatalf("iteration %d: marshalDeterministic error = %v", i, err)
		}
		if string(got) != string(first) {
			t.Fatalf("iteration %d produced divergent output:\n  first=%s\n  got=  %s", i, first, got)
		}
	}
}

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
			got, _ := recordpath.Get(record, tt.path)
			if got != tt.want {
				t.Errorf("GetNestedValue(%q) = %v, want %v", tt.path, got, tt.want)
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
