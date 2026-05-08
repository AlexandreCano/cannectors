// Package filter provides implementations for filter modules.
package filter

import (
	"context"
	"testing"
)

// TestNewMappingFromConfig tests the constructor with configuration.
func TestNewMappingFromConfig(t *testing.T) {
	tests := []struct {
		name        string
		mappings    []FieldMapping
		onError     string
		wantErr     bool
		errContains string
	}{
		{
			name: "valid mappings with source/target",
			mappings: []FieldMapping{
				{Source: strPtr("name"), Target: "fullName"},
				{Source: strPtr("email"), Target: "emailAddress"},
			},
			onError: "fail",
			wantErr: false,
		},
		{
			name:     "empty mappings array is valid",
			mappings: []FieldMapping{},
			onError:  "fail",
			wantErr:  false,
		},
		{
			name:     "nil mappings array is valid",
			mappings: nil,
			onError:  "fail",
			wantErr:  false,
		},
		{
			name: "invalid mapping - missing target",
			mappings: []FieldMapping{
				{Source: strPtr("name")}, // missing target
			},
			onError:     "fail",
			wantErr:     true,
			errContains: "invalid mapping",
		},
		{
			name: "valid mapping - empty source deletes target field",
			mappings: []FieldMapping{
				{Target: "fullName"}, // empty source = delete field
			},
			onError: "fail",
			wantErr: false,
		},
		{
			name: "valid onError modes",
			mappings: []FieldMapping{
				{Source: strPtr("name"), Target: "fullName"},
			},
			onError: "skip",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapper, err := NewMappingFromConfig(tt.mappings, tt.onError)
			if tt.wantErr {
				if err == nil {
					t.Errorf("NewMappingFromConfig() expected error, got nil")
					return
				}
				if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("NewMappingFromConfig() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}
			if err != nil {
				t.Errorf("NewMappingFromConfig() unexpected error = %v", err)
				return
			}
			if mapper == nil {
				t.Errorf("NewMappingFromConfig() returned nil mapper")
			}
		})
	}
}

func TestParseFieldMappings(t *testing.T) {
	t.Run("valid mapping with transforms", func(t *testing.T) {
		raw := []any{
			map[string]any{
				"source":       "name",
				"target":       "fullName",
				"defaultValue": "unknown",
				"onMissing":    "useDefault",
				"transforms": []any{
					"trim",
					map[string]any{"op": "lowercase"},
				},
			},
		}

		mappings, err := ParseFieldMappings(raw)
		if err != nil {
			t.Fatalf("ParseFieldMappings() error = %v", err)
		}
		if len(mappings) != 1 {
			t.Fatalf("expected 1 mapping, got %d", len(mappings))
		}
		if mappings[0].Source == nil || *mappings[0].Source != "name" || mappings[0].Target != "fullName" {
			t.Fatalf("unexpected mapping: %+v", mappings[0])
		}
		if len(mappings[0].Transforms) != 2 {
			t.Fatalf("expected 2 transforms, got %d", len(mappings[0].Transforms))
		}
		if mappings[0].Transforms[0].Op != "trim" || mappings[0].Transforms[1].Op != "lowercase" {
			t.Fatalf("unexpected transforms: %+v", mappings[0].Transforms)
		}
	})

	t.Run("nil input returns empty array", func(t *testing.T) {
		mappings, err := ParseFieldMappings(nil)
		if err != nil {
			t.Fatalf("ParseFieldMappings(nil) error = %v", err)
		}
		if len(mappings) != 0 {
			t.Fatalf("expected 0 mappings, got %d", len(mappings))
		}
	})

	t.Run("invalid transform op missing", func(t *testing.T) {
		raw := []any{
			map[string]any{
				"source": "name",
				"target": "fullName",
				"transforms": []any{
					map[string]any{"format": "YYYY-MM-DD"}, // missing op
				},
			},
		}
		_, err := ParseFieldMappings(raw)
		if err == nil {
			t.Fatal("expected error for missing transform op")
		}
		if !containsString(err.Error(), "op missing") {
			t.Fatalf("expected error about missing op, got: %v", err)
		}
	})

	t.Run("invalid mapping type returns error", func(t *testing.T) {
		raw := []any{
			"not a mapping object",
		}
		_, err := ParseFieldMappings(raw)
		if err == nil {
			t.Fatal("expected error for invalid mapping type")
		}
	})

	t.Run("non-array input returns error", func(t *testing.T) {
		raw := "not an array"
		_, err := ParseFieldMappings(raw)
		if err == nil {
			t.Fatal("expected error for non-array input")
		}
	})
}

// TestMapping_Process_BasicMappings tests basic field-to-field mapping.
func TestMapping_Process_BasicMappings(t *testing.T) {
	tests := []struct {
		name     string
		mappings []FieldMapping
		input    []map[string]any
		want     []map[string]any
		wantErr  bool
	}{
		{
			name: "simple field mapping with source/target",
			mappings: []FieldMapping{
				{Source: strPtr("name"), Target: "fullName"},
				{Source: strPtr("email"), Target: "emailAddress"},
			},
			input: []map[string]any{
				{"name": "John Doe", "email": "john@example.com"},
			},
			want: []map[string]any{
				{"name": "John Doe", "email": "john@example.com", "fullName": "John Doe", "emailAddress": "john@example.com"},
			},
			wantErr: false,
		},
		{
			name: "multiple records",
			mappings: []FieldMapping{
				{Source: strPtr("id"), Target: "userId"},
				{Source: strPtr("name"), Target: "userName"},
			},
			input: []map[string]any{
				{"id": 1, "name": "Alice"},
				{"id": 2, "name": "Bob"},
				{"id": 3, "name": "Charlie"},
			},
			want: []map[string]any{
				{"id": 1, "name": "Alice", "userId": 1, "userName": "Alice"},
				{"id": 2, "name": "Bob", "userId": 2, "userName": "Bob"},
				{"id": 3, "name": "Charlie", "userId": 3, "userName": "Charlie"},
			},
			wantErr: false,
		},
		{
			name:     "empty input returns empty output",
			mappings: []FieldMapping{{Source: strPtr("name"), Target: "fullName"}},
			input:    []map[string]any{},
			want:     []map[string]any{},
			wantErr:  false,
		},
		{
			name:     "nil input returns empty output",
			mappings: []FieldMapping{{Source: strPtr("name"), Target: "fullName"}},
			input:    nil,
			want:     []map[string]any{},
			wantErr:  false,
		},
		{
			name:     "empty mappings preserves all fields",
			mappings: []FieldMapping{},
			input: []map[string]any{
				{"name": "John", "email": "john@example.com"},
			},
			want:    []map[string]any{{"name": "John", "email": "john@example.com"}},
			wantErr: false,
		},
		{
			name: "preserves various data types",
			mappings: []FieldMapping{
				{Source: strPtr("string_field"), Target: "str"},
				{Source: strPtr("int_field"), Target: "num"},
				{Source: strPtr("float_field"), Target: "decimal"},
				{Source: strPtr("bool_field"), Target: "flag"},
				{Source: strPtr("null_field"), Target: "nil_val"},
			},
			input: []map[string]any{
				{
					"string_field": "hello",
					"int_field":    42,
					"float_field":  3.14,
					"bool_field":   true,
					"null_field":   nil,
				},
			},
			want: []map[string]any{
				{
					"string_field": "hello",
					"int_field":    42,
					"float_field":  3.14,
					"bool_field":   true,
					"null_field":   nil,
					"str":          "hello",
					"num":          42,
					"decimal":      3.14,
					"flag":         true,
					"nil_val":      nil,
				},
			},
			wantErr: false,
		},
		{
			name: "preserves arrays and objects",
			mappings: []FieldMapping{
				{Source: strPtr("tags"), Target: "labels"},
				{Source: strPtr("metadata"), Target: "meta"},
			},
			input: []map[string]any{
				{
					"tags":     []any{"tag1", "tag2"},
					"metadata": map[string]any{"key": "value"},
				},
			},
			want: []map[string]any{
				{
					"tags":     []any{"tag1", "tag2"},
					"metadata": map[string]any{"key": "value"},
					"labels":   []any{"tag1", "tag2"},
					"meta":     map[string]any{"key": "value"},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr {
				assertMappingFails(t, tt.input, tt.mappings)
				return
			}
			assertMapped(t, tt.input, tt.mappings, tt.want)
		})
	}
}

// TestMapping_Process_NestedPaths tests nested field path handling.
func TestMapping_Process_NestedPaths(t *testing.T) {
	tests := []struct {
		name     string
		mappings []FieldMapping
		input    []map[string]any
		want     []map[string]any
		wantErr  bool
	}{
		{
			name: "nested source path",
			mappings: []FieldMapping{
				{Source: strPtr("user.name"), Target: "fullName"},
				{Source: strPtr("user.email"), Target: "emailAddress"},
			},
			input: []map[string]any{
				{
					"user": map[string]any{
						"name":  "John Doe",
						"email": "john@example.com",
					},
				},
			},
			want: []map[string]any{
				{
					"user": map[string]any{
						"name":  "John Doe",
						"email": "john@example.com",
					},
					"fullName":     "John Doe",
					"emailAddress": "john@example.com",
				},
			},
			wantErr: false,
		},
		{
			name: "deeply nested source path",
			mappings: []FieldMapping{
				{Source: strPtr("data.user.profile.name"), Target: "name"},
			},
			input: []map[string]any{
				{
					"data": map[string]any{
						"user": map[string]any{
							"profile": map[string]any{
								"name": "Deep Value",
							},
						},
					},
				},
			},
			want: []map[string]any{
				{
					"data": map[string]any{
						"user": map[string]any{
							"profile": map[string]any{
								"name": "Deep Value",
							},
						},
					},
					"name": "Deep Value",
				},
			},
			wantErr: false,
		},
		{
			name: "nested target path creates structure",
			mappings: []FieldMapping{
				{Source: strPtr("firstName"), Target: "user.name.first"},
				{Source: strPtr("lastName"), Target: "user.name.last"},
			},
			input: []map[string]any{
				{"firstName": "John", "lastName": "Doe"},
			},
			want: []map[string]any{
				{
					"firstName": "John",
					"lastName":  "Doe",
					"user": map[string]any{
						"name": map[string]any{
							"first": "John",
							"last":  "Doe",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "both source and target nested",
			mappings: []FieldMapping{
				{Source: strPtr("input.data.value"), Target: "output.result.value"},
			},
			input: []map[string]any{
				{
					"input": map[string]any{
						"data": map[string]any{
							"value": 42,
						},
					},
				},
			},
			want: []map[string]any{
				{
					"input": map[string]any{
						"data": map[string]any{
							"value": 42,
						},
					},
					"output": map[string]any{
						"result": map[string]any{
							"value": 42,
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr {
				assertMappingFails(t, tt.input, tt.mappings)
				return
			}
			assertMapped(t, tt.input, tt.mappings, tt.want)
		})
	}
}

// TestMapping_Process_ArrayIndexing tests array indexing in field paths.
func TestMapping_Process_ArrayIndexing(t *testing.T) {
	tests := []struct {
		name     string
		mappings []FieldMapping
		input    []map[string]any
		want     []map[string]any
		wantErr  bool
	}{
		{
			name: "array indexing - first element",
			mappings: []FieldMapping{
				{Source: strPtr("items[0].name"), Target: "firstName"},
			},
			input: []map[string]any{
				{
					"items": []any{
						map[string]any{"name": "Alice"},
						map[string]any{"name": "Bob"},
					},
				},
			},
			want: []map[string]any{
				{
					"items": []any{
						map[string]any{"name": "Alice"},
						map[string]any{"name": "Bob"},
					},
					"firstName": "Alice",
				},
			},
			wantErr: false,
		},
		{
			name: "array indexing - second element",
			mappings: []FieldMapping{
				{Source: strPtr("items[1].name"), Target: "secondName"},
			},
			input: []map[string]any{
				{
					"items": []any{
						map[string]any{"name": "Alice"},
						map[string]any{"name": "Bob"},
					},
				},
			},
			want: []map[string]any{
				{
					"items": []any{
						map[string]any{"name": "Alice"},
						map[string]any{"name": "Bob"},
					},
					"secondName": "Bob",
				},
			},
			wantErr: false,
		},
		{
			name: "array indexing - out of bounds returns onMissing",
			mappings: []FieldMapping{
				{Source: strPtr("items[5].name"), Target: "missing", OnMissing: "setNull"},
			},
			input: []map[string]any{
				{
					"items": []any{
						map[string]any{"name": "Alice"},
					},
				},
			},
			want: []map[string]any{
				{
					"items": []any{
						map[string]any{"name": "Alice"},
					},
					"missing": nil,
				},
			},
			wantErr: false,
		},
		{
			name: "nested array indexing",
			mappings: []FieldMapping{
				{Source: strPtr("data.users[0].address.city"), Target: "city"},
			},
			input: []map[string]any{
				{
					"data": map[string]any{
						"users": []any{
							map[string]any{
								"address": map[string]any{
									"city": "Paris",
								},
							},
						},
					},
				},
			},
			want: []map[string]any{
				{
					"data": map[string]any{
						"users": []any{
							map[string]any{
								"address": map[string]any{
									"city": "Paris",
								},
							},
						},
					},
					"city": "Paris",
				},
			},
			wantErr: false,
		},
		{
			name: "simple array value",
			mappings: []FieldMapping{
				{Source: strPtr("numbers[0]"), Target: "first"},
			},
			input: []map[string]any{
				{
					"numbers": []any{1, 2, 3},
				},
			},
			want: []map[string]any{
				{
					"numbers": []any{1, 2, 3},
					"first":   1,
				},
			},
			wantErr: false,
		},
		{
			name: "target array indexing creates nested array",
			mappings: []FieldMapping{
				{Source: strPtr("name"), Target: "items[0].name"},
			},
			input: []map[string]any{
				{"name": "Alice"},
			},
			want: []map[string]any{
				{
					"name": "Alice",
					"items": []any{
						map[string]any{"name": "Alice"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "target array indexing extends array",
			mappings: []FieldMapping{
				{Source: strPtr("id"), Target: "ids[1]"},
			},
			input: []map[string]any{
				{"id": 7},
			},
			want: []map[string]any{
				{
					"id":  7,
					"ids": []any{nil, 7},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr {
				assertMappingFails(t, tt.input, tt.mappings)
				return
			}
			assertMapped(t, tt.input, tt.mappings, tt.want)
		})
	}
}

// TestMapping_Process_MissingIntermediatePaths tests handling of missing intermediate objects.
func TestMapping_Process_MissingIntermediatePaths(t *testing.T) {
	tests := []struct {
		name     string
		mappings []FieldMapping
		input    []map[string]any
		want     []map[string]any
		wantErr  bool
	}{
		{
			name: "missing intermediate object - setNull",
			mappings: []FieldMapping{
				{Source: strPtr("user.profile.name"), Target: "name", OnMissing: "setNull"},
			},
			input: []map[string]any{
				{"user": map[string]any{}},
			},
			want: []map[string]any{
				{"user": map[string]any{}, "name": nil},
			},
			wantErr: false,
		},
		{
			name: "missing intermediate object - skipField",
			mappings: []FieldMapping{
				{Source: strPtr("user.profile.name"), Target: "name", OnMissing: "skipField"},
			},
			input: []map[string]any{
				{"other": "value"},
			},
			want: []map[string]any{
				{"other": "value"},
			},
			wantErr: false,
		},
		{
			name: "intermediate not an object",
			mappings: []FieldMapping{
				{Source: strPtr("user.profile.name"), Target: "name", OnMissing: "setNull"},
			},
			input: []map[string]any{
				{"user": "not an object"}, // user is a string, not an object
			},
			want: []map[string]any{
				{"user": "not an object", "name": nil},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr {
				assertMappingFails(t, tt.input, tt.mappings)
				return
			}
			assertMapped(t, tt.input, tt.mappings, tt.want)
		})
	}
}

// TestMapping_Process_OnMissing tests onMissing behavior for missing fields.
func TestMapping_Process_OnMissing(t *testing.T) {
	tests := []struct {
		name     string
		mappings []FieldMapping
		input    []map[string]any
		want     []map[string]any
		wantErr  bool
	}{
		{
			name: "onMissing setNull - missing field becomes null",
			mappings: []FieldMapping{
				{Source: strPtr("name"), Target: "fullName", OnMissing: "setNull"},
				{Source: strPtr("missing"), Target: "missingField", OnMissing: "setNull"},
			},
			input: []map[string]any{
				{"name": "John"},
			},
			want: []map[string]any{
				{"name": "John", "fullName": "John", "missingField": nil},
			},
			wantErr: false,
		},
		{
			name: "onMissing skipField - missing field is not added",
			mappings: []FieldMapping{
				{Source: strPtr("name"), Target: "fullName"},
				{Source: strPtr("missing"), Target: "missingField", OnMissing: "skipField"},
			},
			input: []map[string]any{
				{"name": "John"},
			},
			want: []map[string]any{
				{"name": "John", "fullName": "John"},
			},
			wantErr: false,
		},
		{
			name: "onMissing useDefault - uses defaultValue",
			mappings: []FieldMapping{
				{Source: strPtr("name"), Target: "fullName"},
				{Source: strPtr("missing"), Target: "status", OnMissing: "useDefault", DefaultValue: "active"},
			},
			input: []map[string]any{
				{"name": "John"},
			},
			want: []map[string]any{
				{"name": "John", "fullName": "John", "status": "active"},
			},
			wantErr: false,
		},
		{
			name: "onMissing useDefault with numeric default",
			mappings: []FieldMapping{
				{Source: strPtr("missing"), Target: "count", OnMissing: "useDefault", DefaultValue: 0},
			},
			input: []map[string]any{
				{"other": "value"},
			},
			want: []map[string]any{
				{"other": "value", "count": 0},
			},
			wantErr: false,
		},
		{
			name: "onMissing useDefault with boolean default",
			mappings: []FieldMapping{
				{Source: strPtr("missing"), Target: "enabled", OnMissing: "useDefault", DefaultValue: true},
			},
			input: []map[string]any{
				{"other": "value"},
			},
			want: []map[string]any{
				{"other": "value", "enabled": true},
			},
			wantErr: false,
		},
		{
			name: "onMissing fail - returns error for missing field",
			mappings: []FieldMapping{
				{Source: strPtr("name"), Target: "fullName"},
				{Source: strPtr("required_field"), Target: "required", OnMissing: "fail"},
			},
			input: []map[string]any{
				{"name": "John"},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "default onMissing is setNull",
			mappings: []FieldMapping{
				{Source: strPtr("missing"), Target: "field"}, // No onMissing specified
			},
			input: []map[string]any{
				{"other": "value"},
			},
			want: []map[string]any{
				{"other": "value", "field": nil},
			},
			wantErr: false,
		},
		{
			name: "field exists but is null - does not trigger onMissing",
			mappings: []FieldMapping{
				{Source: strPtr("nullField"), Target: "output", OnMissing: "fail"},
			},
			input: []map[string]any{
				{"nullField": nil},
			},
			want: []map[string]any{
				{"nullField": nil, "output": nil},
			},
			wantErr: false,
		},
		{
			name: "multiple records with different missing fields",
			mappings: []FieldMapping{
				{Source: strPtr("name"), Target: "fullName", OnMissing: "setNull"},
				{Source: strPtr("email"), Target: "emailAddress", OnMissing: "skipField"},
			},
			input: []map[string]any{
				{"name": "John", "email": "john@example.com"},
				{"name": "Jane"},     // missing email
				{"email": "x@y.com"}, // missing name
			},
			want: []map[string]any{
				{"name": "John", "email": "john@example.com", "fullName": "John", "emailAddress": "john@example.com"},
				{"name": "Jane", "fullName": "Jane"},                             // email skipped
				{"email": "x@y.com", "fullName": nil, "emailAddress": "x@y.com"}, // name is null
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr {
				assertMappingFails(t, tt.input, tt.mappings)
				return
			}
			assertMapped(t, tt.input, tt.mappings, tt.want)
		})
	}
}

// TestMapping_Process_OnError tests error handling modes.
func TestMapping_Process_OnError(t *testing.T) {
	tests := []struct {
		name     string
		mappings []FieldMapping
		onError  string
		input    []map[string]any
		wantLen  int
		wantErr  bool
	}{
		{
			name: "onError fail - stops on first error",
			mappings: []FieldMapping{
				{Source: strPtr("required"), Target: "output", OnMissing: "fail"},
			},
			onError: "fail",
			input: []map[string]any{
				{"other": "value"},
			},
			wantLen: 0,
			wantErr: true,
		},
		{
			name: "onError skip - skips record with error",
			mappings: []FieldMapping{
				{Source: strPtr("required"), Target: "output", OnMissing: "fail"},
			},
			onError: "skip",
			input: []map[string]any{
				{"required": "value1"},
				{"other": "value"}, // This will fail and be skipped
				{"required": "value2"},
			},
			wantLen: 2, // Only successful records
			wantErr: false,
		},
		{
			name: "onError log - continues after error",
			mappings: []FieldMapping{
				{Source: strPtr("name"), Target: "fullName"},
				{Source: strPtr("required"), Target: "output", OnMissing: "fail"},
			},
			onError: "log",
			input: []map[string]any{
				{"name": "John"}, // Will fail on required but continue
			},
			wantLen: 1, // Partial result included
			wantErr: false,
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

// TestMapping_Process_Transforms tests transform operations.
func TestMapping_Process_Transforms(t *testing.T) {
	tests := []struct {
		name     string
		mappings []FieldMapping
		input    []map[string]any
		want     []map[string]any
		wantErr  bool
	}{
		// Single transforms
		{
			name: "transform trim",
			mappings: []FieldMapping{
				{Source: strPtr("name"), Target: "fullName", Transforms: []TransformOp{{Op: "trim"}}},
			},
			input: []map[string]any{
				{"name": "  John Doe  "},
			},
			want: []map[string]any{
				{"name": "  John Doe  ", "fullName": "John Doe"},
			},
		},
		{
			name: "transform lowercase",
			mappings: []FieldMapping{
				{Source: strPtr("name"), Target: "lowername", Transforms: []TransformOp{{Op: "lowercase"}}},
			},
			input: []map[string]any{
				{"name": "John DOE"},
			},
			want: []map[string]any{
				{"name": "John DOE", "lowername": "john doe"},
			},
		},
		{
			name: "transform uppercase",
			mappings: []FieldMapping{
				{Source: strPtr("name"), Target: "uppername", Transforms: []TransformOp{{Op: "uppercase"}}},
			},
			input: []map[string]any{
				{"name": "John Doe"},
			},
			want: []map[string]any{
				{"name": "John Doe", "uppername": "JOHN DOE"},
			},
		},
		{
			name: "transform toInt from string",
			mappings: []FieldMapping{
				{Source: strPtr("count"), Target: "countInt", Transforms: []TransformOp{{Op: "toInt"}}},
			},
			input: []map[string]any{
				{"count": "42"},
			},
			want: []map[string]any{
				{"count": "42", "countInt": 42},
			},
		},
		{
			name: "transform toInt from float64 with no fractional part",
			mappings: []FieldMapping{
				{Source: strPtr("count"), Target: "countInt", Transforms: []TransformOp{{Op: "toInt"}}},
			},
			input: []map[string]any{
				{"count": 42.0},
			},
			want: []map[string]any{
				{"count": 42.0, "countInt": 42},
			},
		},
		{
			name: "transform toFloat from string",
			mappings: []FieldMapping{
				{Source: strPtr("amount"), Target: "amountFloat", Transforms: []TransformOp{{Op: "toFloat"}}},
			},
			input: []map[string]any{
				{"amount": "3.14"},
			},
			want: []map[string]any{
				{"amount": "3.14", "amountFloat": 3.14},
			},
		},
		{
			name: "transform toBool from string",
			mappings: []FieldMapping{
				{Source: strPtr("enabled"), Target: "enabledBool", Transforms: []TransformOp{{Op: "toBool"}}},
			},
			input: []map[string]any{
				{"enabled": "true"},
			},
			want: []map[string]any{
				{"enabled": "true", "enabledBool": true},
			},
		},
		{
			name: "transform toString from int",
			mappings: []FieldMapping{
				{Source: strPtr("id"), Target: "idString", Transforms: []TransformOp{{Op: "toString"}}},
			},
			input: []map[string]any{
				{"id": 12},
			},
			want: []map[string]any{
				{"id": 12, "idString": "12"},
			},
		},
		{
			name: "transform toArray wraps value",
			mappings: []FieldMapping{
				{Source: strPtr("tag"), Target: "tags", Transforms: []TransformOp{{Op: "toArray"}}},
			},
			input: []map[string]any{
				{"tag": "single"},
			},
			want: []map[string]any{
				{"tag": "single", "tags": []any{"single"}},
			},
		},
		{
			name: "transform toObject from map",
			mappings: []FieldMapping{
				{Source: strPtr("meta"), Target: "metadata", Transforms: []TransformOp{{Op: "toObject"}}},
			},
			input: []map[string]any{
				{"meta": map[string]any{"key": "value"}},
			},
			want: []map[string]any{
				{"meta": map[string]any{"key": "value"}, "metadata": map[string]any{"key": "value"}},
			},
		},

		// Split transform
		{
			name: "transform split with default separator",
			mappings: []FieldMapping{
				{Source: strPtr("tags"), Target: "tagList", Transforms: []TransformOp{{Op: "split"}}},
			},
			input: []map[string]any{
				{"tags": "a,b,c"},
			},
			want: []map[string]any{
				{"tags": "a,b,c", "tagList": []any{"a", "b", "c"}},
			},
		},
		{
			name: "transform split with custom separator",
			mappings: []FieldMapping{
				{Source: strPtr("tags"), Target: "tagList", Transforms: []TransformOp{{Op: "split", Separator: "|"}}},
			},
			input: []map[string]any{
				{"tags": "a|b|c"},
			},
			want: []map[string]any{
				{"tags": "a|b|c", "tagList": []any{"a", "b", "c"}},
			},
		},

		// Join transform
		{
			name: "transform join with default separator",
			mappings: []FieldMapping{
				{Source: strPtr("items"), Target: "itemString", Transforms: []TransformOp{{Op: "join"}}},
			},
			input: []map[string]any{
				{"items": []any{"a", "b", "c"}},
			},
			want: []map[string]any{
				{"items": []any{"a", "b", "c"}, "itemString": "a,b,c"},
			},
		},
		{
			name: "transform join with custom separator",
			mappings: []FieldMapping{
				{Source: strPtr("items"), Target: "itemString", Transforms: []TransformOp{{Op: "join", Separator: " - "}}},
			},
			input: []map[string]any{
				{"items": []any{"x", "y", "z"}},
			},
			want: []map[string]any{
				{"items": []any{"x", "y", "z"}, "itemString": "x - y - z"},
			},
		},

		// Replace transform
		{
			name: "transform replace with pattern",
			mappings: []FieldMapping{
				{Source: strPtr("text"), Target: "cleaned", Transforms: []TransformOp{{
					Op:          "replace",
					Pattern:     "[0-9]+",
					Replacement: "X",
				}}},
			},
			input: []map[string]any{
				{"text": "hello123world456"},
			},
			want: []map[string]any{
				{"text": "hello123world456", "cleaned": "helloXworldX"},
			},
		},

		// DateFormat transform
		{
			name: "transform dateFormat ISO to custom",
			mappings: []FieldMapping{
				{Source: strPtr("created"), Target: "date", Transforms: []TransformOp{{
					Op:     "dateFormat",
					Format: "YYYY-MM-DD",
				}}},
			},
			input: []map[string]any{
				{"created": "2026-01-15T10:30:00Z"},
			},
			want: []map[string]any{
				{"created": "2026-01-15T10:30:00Z", "date": "2026-01-15"},
			},
		},

		// Multiple transforms
		{
			name: "multiple transforms in order",
			mappings: []FieldMapping{
				{
					Source: strPtr("name"),
					Target: "cleaned",
					Transforms: []TransformOp{
						{Op: "trim"},
						{Op: "lowercase"},
					},
				},
			},
			input: []map[string]any{
				{"name": "  JOHN DOE  "},
			},
			want: []map[string]any{
				{"name": "  JOHN DOE  ", "cleaned": "john doe"},
			},
		},
		{
			name: "multiple transforms with parameters",
			mappings: []FieldMapping{
				{
					Source: strPtr("input"),
					Target: "output",
					Transforms: []TransformOp{
						{Op: "trim"},
						{Op: "replace", Pattern: "\\s+", Replacement: "_"},
						{Op: "lowercase"},
					},
				},
			},
			input: []map[string]any{
				{"input": "  Hello   World  "},
			},
			want: []map[string]any{
				{"input": "  Hello   World  ", "output": "hello_world"},
			},
		},

		// Transform on non-string values (should be no-op or handle gracefully)
		{
			name: "transform on number - no-op",
			mappings: []FieldMapping{
				{Source: strPtr("count"), Target: "total", Transforms: []TransformOp{{Op: "trim"}}},
			},
			input: []map[string]any{
				{"count": 42},
			},
			want: []map[string]any{
				{"count": 42, "total": 42},
			},
		},
		{
			name: "transform on nil - no-op",
			mappings: []FieldMapping{
				{Source: strPtr("empty"), Target: "result", Transforms: []TransformOp{{Op: "trim"}}},
			},
			input: []map[string]any{
				{"empty": nil},
			},
			want: []map[string]any{
				{"empty": nil, "result": nil},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErr {
				assertMappingFails(t, tt.input, tt.mappings)
				return
			}
			assertMapped(t, tt.input, tt.mappings, tt.want)
		})
	}
}

// TestMapping_Process_TypePreservation tests that data types are preserved during mapping.
func TestMapping_Process_TypePreservation(t *testing.T) {
	tests := []struct {
		name     string
		mappings []FieldMapping
		input    []map[string]any
		check    func(t *testing.T, result []map[string]any)
	}{
		{
			name: "preserves int type",
			mappings: []FieldMapping{
				{Source: strPtr("count"), Target: "total"},
			},
			input: []map[string]any{
				{"count": 42},
			},
			check: func(t *testing.T, result []map[string]any) {
				if _, ok := result[0]["total"].(int); !ok {
					t.Errorf("expected int, got %T", result[0]["total"])
				}
			},
		},
		{
			name: "preserves float type",
			mappings: []FieldMapping{
				{Source: strPtr("price"), Target: "amount"},
			},
			input: []map[string]any{
				{"price": 19.99},
			},
			check: func(t *testing.T, result []map[string]any) {
				if _, ok := result[0]["amount"].(float64); !ok {
					t.Errorf("expected float64, got %T", result[0]["amount"])
				}
			},
		},
		{
			name: "preserves bool type",
			mappings: []FieldMapping{
				{Source: strPtr("active"), Target: "enabled"},
			},
			input: []map[string]any{
				{"active": true},
			},
			check: func(t *testing.T, result []map[string]any) {
				if _, ok := result[0]["enabled"].(bool); !ok {
					t.Errorf("expected bool, got %T", result[0]["enabled"])
				}
			},
		},
		{
			name: "preserves nil value",
			mappings: []FieldMapping{
				{Source: strPtr("empty"), Target: "null_field"},
			},
			input: []map[string]any{
				{"empty": nil},
			},
			check: func(t *testing.T, result []map[string]any) {
				if result[0]["null_field"] != nil {
					t.Errorf("expected nil, got %v", result[0]["null_field"])
				}
			},
		},
		{
			name: "preserves array type",
			mappings: []FieldMapping{
				{Source: strPtr("items"), Target: "list"},
			},
			input: []map[string]any{
				{"items": []any{"a", "b", "c"}},
			},
			check: func(t *testing.T, result []map[string]any) {
				if _, ok := result[0]["list"].([]any); !ok {
					t.Errorf("expected []any, got %T", result[0]["list"])
				}
			},
		},
		{
			name: "preserves nested object type",
			mappings: []FieldMapping{
				{Source: strPtr("meta"), Target: "metadata"},
			},
			input: []map[string]any{
				{"meta": map[string]any{"key": "value"}},
			},
			check: func(t *testing.T, result []map[string]any) {
				if _, ok := result[0]["metadata"].(map[string]any); !ok {
					t.Errorf("expected map[string]any, got %T", result[0]["metadata"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapper, err := NewMappingFromConfig(tt.mappings, "fail")
			if err != nil {
				t.Fatalf("NewMappingFromConfig() error = %v", err)
			}

			result, err := mapper.Process(context.Background(), tt.input)
			if err != nil {
				t.Fatalf("Process() error = %v", err)
			}

			tt.check(t, result)
		})
	}
}
