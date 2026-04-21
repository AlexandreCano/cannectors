// Package filter provides implementations for filter modules.
package filter

import (
	"context"
	"strings"
	"testing"
)

// strPtr is a helper to create string pointers for test cases.
func strPtr(s string) *string {
	return &s
}

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
		raw := []interface{}{
			map[string]interface{}{
				"source":       "name",
				"target":       "fullName",
				"defaultValue": "unknown",
				"onMissing":    "useDefault",
				"transforms": []interface{}{
					"trim",
					map[string]interface{}{"op": "lowercase"},
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
		raw := []interface{}{
			map[string]interface{}{
				"source": "name",
				"target": "fullName",
				"transforms": []interface{}{
					map[string]interface{}{"format": "YYYY-MM-DD"}, // missing op
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
		raw := []interface{}{
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
		input    []map[string]interface{}
		want     []map[string]interface{}
		wantErr  bool
	}{
		{
			name: "simple field mapping with source/target",
			mappings: []FieldMapping{
				{Source: strPtr("name"), Target: "fullName"},
				{Source: strPtr("email"), Target: "emailAddress"},
			},
			input: []map[string]interface{}{
				{"name": "John Doe", "email": "john@example.com"},
			},
			want: []map[string]interface{}{
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
			input: []map[string]interface{}{
				{"id": 1, "name": "Alice"},
				{"id": 2, "name": "Bob"},
				{"id": 3, "name": "Charlie"},
			},
			want: []map[string]interface{}{
				{"id": 1, "name": "Alice", "userId": 1, "userName": "Alice"},
				{"id": 2, "name": "Bob", "userId": 2, "userName": "Bob"},
				{"id": 3, "name": "Charlie", "userId": 3, "userName": "Charlie"},
			},
			wantErr: false,
		},
		{
			name:     "empty input returns empty output",
			mappings: []FieldMapping{{Source: strPtr("name"), Target: "fullName"}},
			input:    []map[string]interface{}{},
			want:     []map[string]interface{}{},
			wantErr:  false,
		},
		{
			name:     "nil input returns empty output",
			mappings: []FieldMapping{{Source: strPtr("name"), Target: "fullName"}},
			input:    nil,
			want:     []map[string]interface{}{},
			wantErr:  false,
		},
		{
			name:     "empty mappings preserves all fields",
			mappings: []FieldMapping{},
			input: []map[string]interface{}{
				{"name": "John", "email": "john@example.com"},
			},
			want:    []map[string]interface{}{{"name": "John", "email": "john@example.com"}},
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
			input: []map[string]interface{}{
				{
					"string_field": "hello",
					"int_field":    42,
					"float_field":  3.14,
					"bool_field":   true,
					"null_field":   nil,
				},
			},
			want: []map[string]interface{}{
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
			input: []map[string]interface{}{
				{
					"tags":     []interface{}{"tag1", "tag2"},
					"metadata": map[string]interface{}{"key": "value"},
				},
			},
			want: []map[string]interface{}{
				{
					"tags":     []interface{}{"tag1", "tag2"},
					"metadata": map[string]interface{}{"key": "value"},
					"labels":   []interface{}{"tag1", "tag2"},
					"meta":     map[string]interface{}{"key": "value"},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapper, err := NewMappingFromConfig(tt.mappings, "fail")
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

			if !recordsEqual(got, tt.want) {
				t.Errorf("Process() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMapping_Process_NestedPaths tests nested field path handling.
func TestMapping_Process_NestedPaths(t *testing.T) {
	tests := []struct {
		name     string
		mappings []FieldMapping
		input    []map[string]interface{}
		want     []map[string]interface{}
		wantErr  bool
	}{
		{
			name: "nested source path",
			mappings: []FieldMapping{
				{Source: strPtr("user.name"), Target: "fullName"},
				{Source: strPtr("user.email"), Target: "emailAddress"},
			},
			input: []map[string]interface{}{
				{
					"user": map[string]interface{}{
						"name":  "John Doe",
						"email": "john@example.com",
					},
				},
			},
			want: []map[string]interface{}{
				{
					"user": map[string]interface{}{
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
			input: []map[string]interface{}{
				{
					"data": map[string]interface{}{
						"user": map[string]interface{}{
							"profile": map[string]interface{}{
								"name": "Deep Value",
							},
						},
					},
				},
			},
			want: []map[string]interface{}{
				{
					"data": map[string]interface{}{
						"user": map[string]interface{}{
							"profile": map[string]interface{}{
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
			input: []map[string]interface{}{
				{"firstName": "John", "lastName": "Doe"},
			},
			want: []map[string]interface{}{
				{
					"firstName": "John",
					"lastName":  "Doe",
					"user": map[string]interface{}{
						"name": map[string]interface{}{
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
			input: []map[string]interface{}{
				{
					"input": map[string]interface{}{
						"data": map[string]interface{}{
							"value": 42,
						},
					},
				},
			},
			want: []map[string]interface{}{
				{
					"input": map[string]interface{}{
						"data": map[string]interface{}{
							"value": 42,
						},
					},
					"output": map[string]interface{}{
						"result": map[string]interface{}{
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
			mapper, err := NewMappingFromConfig(tt.mappings, "fail")
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

			if !recordsEqual(got, tt.want) {
				t.Errorf("Process() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMapping_Process_ArrayIndexing tests array indexing in field paths.
func TestMapping_Process_ArrayIndexing(t *testing.T) {
	tests := []struct {
		name     string
		mappings []FieldMapping
		input    []map[string]interface{}
		want     []map[string]interface{}
		wantErr  bool
	}{
		{
			name: "array indexing - first element",
			mappings: []FieldMapping{
				{Source: strPtr("items[0].name"), Target: "firstName"},
			},
			input: []map[string]interface{}{
				{
					"items": []interface{}{
						map[string]interface{}{"name": "Alice"},
						map[string]interface{}{"name": "Bob"},
					},
				},
			},
			want: []map[string]interface{}{
				{
					"items": []interface{}{
						map[string]interface{}{"name": "Alice"},
						map[string]interface{}{"name": "Bob"},
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
			input: []map[string]interface{}{
				{
					"items": []interface{}{
						map[string]interface{}{"name": "Alice"},
						map[string]interface{}{"name": "Bob"},
					},
				},
			},
			want: []map[string]interface{}{
				{
					"items": []interface{}{
						map[string]interface{}{"name": "Alice"},
						map[string]interface{}{"name": "Bob"},
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
			input: []map[string]interface{}{
				{
					"items": []interface{}{
						map[string]interface{}{"name": "Alice"},
					},
				},
			},
			want: []map[string]interface{}{
				{
					"items": []interface{}{
						map[string]interface{}{"name": "Alice"},
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
			input: []map[string]interface{}{
				{
					"data": map[string]interface{}{
						"users": []interface{}{
							map[string]interface{}{
								"address": map[string]interface{}{
									"city": "Paris",
								},
							},
						},
					},
				},
			},
			want: []map[string]interface{}{
				{
					"data": map[string]interface{}{
						"users": []interface{}{
							map[string]interface{}{
								"address": map[string]interface{}{
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
			input: []map[string]interface{}{
				{
					"numbers": []interface{}{1, 2, 3},
				},
			},
			want: []map[string]interface{}{
				{
					"numbers": []interface{}{1, 2, 3},
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
			input: []map[string]interface{}{
				{"name": "Alice"},
			},
			want: []map[string]interface{}{
				{
					"name": "Alice",
					"items": []interface{}{
						map[string]interface{}{"name": "Alice"},
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
			input: []map[string]interface{}{
				{"id": 7},
			},
			want: []map[string]interface{}{
				{
					"id":  7,
					"ids": []interface{}{nil, 7},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapper, err := NewMappingFromConfig(tt.mappings, "fail")
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

			if !recordsEqual(got, tt.want) {
				t.Errorf("Process() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMapping_Process_MissingIntermediatePaths tests handling of missing intermediate objects.
func TestMapping_Process_MissingIntermediatePaths(t *testing.T) {
	tests := []struct {
		name     string
		mappings []FieldMapping
		input    []map[string]interface{}
		want     []map[string]interface{}
		wantErr  bool
	}{
		{
			name: "missing intermediate object - setNull",
			mappings: []FieldMapping{
				{Source: strPtr("user.profile.name"), Target: "name", OnMissing: "setNull"},
			},
			input: []map[string]interface{}{
				{"user": map[string]interface{}{}}, // profile doesn't exist
			},
			want: []map[string]interface{}{
				{"user": map[string]interface{}{}, "name": nil},
			},
			wantErr: false,
		},
		{
			name: "missing intermediate object - skipField",
			mappings: []FieldMapping{
				{Source: strPtr("user.profile.name"), Target: "name", OnMissing: "skipField"},
			},
			input: []map[string]interface{}{
				{"other": "value"},
			},
			want: []map[string]interface{}{
				{"other": "value"},
			},
			wantErr: false,
		},
		{
			name: "intermediate not an object",
			mappings: []FieldMapping{
				{Source: strPtr("user.profile.name"), Target: "name", OnMissing: "setNull"},
			},
			input: []map[string]interface{}{
				{"user": "not an object"}, // user is a string, not an object
			},
			want: []map[string]interface{}{
				{"user": "not an object", "name": nil},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapper, err := NewMappingFromConfig(tt.mappings, "fail")
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

			if !recordsEqual(got, tt.want) {
				t.Errorf("Process() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMapping_Process_OnMissing tests onMissing behavior for missing fields.
func TestMapping_Process_OnMissing(t *testing.T) {
	tests := []struct {
		name     string
		mappings []FieldMapping
		input    []map[string]interface{}
		want     []map[string]interface{}
		wantErr  bool
	}{
		{
			name: "onMissing setNull - missing field becomes null",
			mappings: []FieldMapping{
				{Source: strPtr("name"), Target: "fullName", OnMissing: "setNull"},
				{Source: strPtr("missing"), Target: "missingField", OnMissing: "setNull"},
			},
			input: []map[string]interface{}{
				{"name": "John"},
			},
			want: []map[string]interface{}{
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
			input: []map[string]interface{}{
				{"name": "John"},
			},
			want: []map[string]interface{}{
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
			input: []map[string]interface{}{
				{"name": "John"},
			},
			want: []map[string]interface{}{
				{"name": "John", "fullName": "John", "status": "active"},
			},
			wantErr: false,
		},
		{
			name: "onMissing useDefault with numeric default",
			mappings: []FieldMapping{
				{Source: strPtr("missing"), Target: "count", OnMissing: "useDefault", DefaultValue: 0},
			},
			input: []map[string]interface{}{
				{"other": "value"},
			},
			want: []map[string]interface{}{
				{"other": "value", "count": 0},
			},
			wantErr: false,
		},
		{
			name: "onMissing useDefault with boolean default",
			mappings: []FieldMapping{
				{Source: strPtr("missing"), Target: "enabled", OnMissing: "useDefault", DefaultValue: true},
			},
			input: []map[string]interface{}{
				{"other": "value"},
			},
			want: []map[string]interface{}{
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
			input: []map[string]interface{}{
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
			input: []map[string]interface{}{
				{"other": "value"},
			},
			want: []map[string]interface{}{
				{"other": "value", "field": nil},
			},
			wantErr: false,
		},
		{
			name: "field exists but is null - does not trigger onMissing",
			mappings: []FieldMapping{
				{Source: strPtr("nullField"), Target: "output", OnMissing: "fail"},
			},
			input: []map[string]interface{}{
				{"nullField": nil},
			},
			want: []map[string]interface{}{
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
			input: []map[string]interface{}{
				{"name": "John", "email": "john@example.com"},
				{"name": "Jane"},     // missing email
				{"email": "x@y.com"}, // missing name
			},
			want: []map[string]interface{}{
				{"name": "John", "email": "john@example.com", "fullName": "John", "emailAddress": "john@example.com"},
				{"name": "Jane", "fullName": "Jane"},                             // email skipped
				{"email": "x@y.com", "fullName": nil, "emailAddress": "x@y.com"}, // name is null
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapper, err := NewMappingFromConfig(tt.mappings, "fail")
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

			if !recordsEqual(got, tt.want) {
				t.Errorf("Process() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMapping_Process_OnError tests error handling modes.
func TestMapping_Process_OnError(t *testing.T) {
	tests := []struct {
		name     string
		mappings []FieldMapping
		onError  string
		input    []map[string]interface{}
		wantLen  int
		wantErr  bool
	}{
		{
			name: "onError fail - stops on first error",
			mappings: []FieldMapping{
				{Source: strPtr("required"), Target: "output", OnMissing: "fail"},
			},
			onError: "fail",
			input: []map[string]interface{}{
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
			input: []map[string]interface{}{
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
			input: []map[string]interface{}{
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
		input    []map[string]interface{}
		want     []map[string]interface{}
		wantErr  bool
	}{
		// Single transforms
		{
			name: "transform trim",
			mappings: []FieldMapping{
				{Source: strPtr("name"), Target: "fullName", Transforms: []TransformOp{{Op: "trim"}}},
			},
			input: []map[string]interface{}{
				{"name": "  John Doe  "},
			},
			want: []map[string]interface{}{
				{"name": "  John Doe  ", "fullName": "John Doe"},
			},
		},
		{
			name: "transform lowercase",
			mappings: []FieldMapping{
				{Source: strPtr("name"), Target: "lowername", Transforms: []TransformOp{{Op: "lowercase"}}},
			},
			input: []map[string]interface{}{
				{"name": "John DOE"},
			},
			want: []map[string]interface{}{
				{"name": "John DOE", "lowername": "john doe"},
			},
		},
		{
			name: "transform uppercase",
			mappings: []FieldMapping{
				{Source: strPtr("name"), Target: "uppername", Transforms: []TransformOp{{Op: "uppercase"}}},
			},
			input: []map[string]interface{}{
				{"name": "John Doe"},
			},
			want: []map[string]interface{}{
				{"name": "John Doe", "uppername": "JOHN DOE"},
			},
		},
		{
			name: "transform toInt from string",
			mappings: []FieldMapping{
				{Source: strPtr("count"), Target: "countInt", Transforms: []TransformOp{{Op: "toInt"}}},
			},
			input: []map[string]interface{}{
				{"count": "42"},
			},
			want: []map[string]interface{}{
				{"count": "42", "countInt": 42},
			},
		},
		{
			name: "transform toInt from float64 with no fractional part",
			mappings: []FieldMapping{
				{Source: strPtr("count"), Target: "countInt", Transforms: []TransformOp{{Op: "toInt"}}},
			},
			input: []map[string]interface{}{
				{"count": 42.0},
			},
			want: []map[string]interface{}{
				{"count": 42.0, "countInt": 42},
			},
		},
		{
			name: "transform toFloat from string",
			mappings: []FieldMapping{
				{Source: strPtr("amount"), Target: "amountFloat", Transforms: []TransformOp{{Op: "toFloat"}}},
			},
			input: []map[string]interface{}{
				{"amount": "3.14"},
			},
			want: []map[string]interface{}{
				{"amount": "3.14", "amountFloat": 3.14},
			},
		},
		{
			name: "transform toBool from string",
			mappings: []FieldMapping{
				{Source: strPtr("enabled"), Target: "enabledBool", Transforms: []TransformOp{{Op: "toBool"}}},
			},
			input: []map[string]interface{}{
				{"enabled": "true"},
			},
			want: []map[string]interface{}{
				{"enabled": "true", "enabledBool": true},
			},
		},
		{
			name: "transform toString from int",
			mappings: []FieldMapping{
				{Source: strPtr("id"), Target: "idString", Transforms: []TransformOp{{Op: "toString"}}},
			},
			input: []map[string]interface{}{
				{"id": 12},
			},
			want: []map[string]interface{}{
				{"id": 12, "idString": "12"},
			},
		},
		{
			name: "transform toArray wraps value",
			mappings: []FieldMapping{
				{Source: strPtr("tag"), Target: "tags", Transforms: []TransformOp{{Op: "toArray"}}},
			},
			input: []map[string]interface{}{
				{"tag": "single"},
			},
			want: []map[string]interface{}{
				{"tag": "single", "tags": []interface{}{"single"}},
			},
		},
		{
			name: "transform toObject from map",
			mappings: []FieldMapping{
				{Source: strPtr("meta"), Target: "metadata", Transforms: []TransformOp{{Op: "toObject"}}},
			},
			input: []map[string]interface{}{
				{"meta": map[string]interface{}{"key": "value"}},
			},
			want: []map[string]interface{}{
				{"meta": map[string]interface{}{"key": "value"}, "metadata": map[string]interface{}{"key": "value"}},
			},
		},

		// Split transform
		{
			name: "transform split with default separator",
			mappings: []FieldMapping{
				{Source: strPtr("tags"), Target: "tagList", Transforms: []TransformOp{{Op: "split"}}},
			},
			input: []map[string]interface{}{
				{"tags": "a,b,c"},
			},
			want: []map[string]interface{}{
				{"tags": "a,b,c", "tagList": []interface{}{"a", "b", "c"}},
			},
		},
		{
			name: "transform split with custom separator",
			mappings: []FieldMapping{
				{Source: strPtr("tags"), Target: "tagList", Transforms: []TransformOp{{Op: "split", Separator: "|"}}},
			},
			input: []map[string]interface{}{
				{"tags": "a|b|c"},
			},
			want: []map[string]interface{}{
				{"tags": "a|b|c", "tagList": []interface{}{"a", "b", "c"}},
			},
		},

		// Join transform
		{
			name: "transform join with default separator",
			mappings: []FieldMapping{
				{Source: strPtr("items"), Target: "itemString", Transforms: []TransformOp{{Op: "join"}}},
			},
			input: []map[string]interface{}{
				{"items": []interface{}{"a", "b", "c"}},
			},
			want: []map[string]interface{}{
				{"items": []interface{}{"a", "b", "c"}, "itemString": "a,b,c"},
			},
		},
		{
			name: "transform join with custom separator",
			mappings: []FieldMapping{
				{Source: strPtr("items"), Target: "itemString", Transforms: []TransformOp{{Op: "join", Separator: " - "}}},
			},
			input: []map[string]interface{}{
				{"items": []interface{}{"x", "y", "z"}},
			},
			want: []map[string]interface{}{
				{"items": []interface{}{"x", "y", "z"}, "itemString": "x - y - z"},
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
			input: []map[string]interface{}{
				{"text": "hello123world456"},
			},
			want: []map[string]interface{}{
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
			input: []map[string]interface{}{
				{"created": "2026-01-15T10:30:00Z"},
			},
			want: []map[string]interface{}{
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
			input: []map[string]interface{}{
				{"name": "  JOHN DOE  "},
			},
			want: []map[string]interface{}{
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
			input: []map[string]interface{}{
				{"input": "  Hello   World  "},
			},
			want: []map[string]interface{}{
				{"input": "  Hello   World  ", "output": "hello_world"},
			},
		},

		// Transform on non-string values (should be no-op or handle gracefully)
		{
			name: "transform on number - no-op",
			mappings: []FieldMapping{
				{Source: strPtr("count"), Target: "total", Transforms: []TransformOp{{Op: "trim"}}},
			},
			input: []map[string]interface{}{
				{"count": 42},
			},
			want: []map[string]interface{}{
				{"count": 42, "total": 42},
			},
		},
		{
			name: "transform on nil - no-op",
			mappings: []FieldMapping{
				{Source: strPtr("empty"), Target: "result", Transforms: []TransformOp{{Op: "trim"}}},
			},
			input: []map[string]interface{}{
				{"empty": nil},
			},
			want: []map[string]interface{}{
				{"empty": nil, "result": nil},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapper, err := NewMappingFromConfig(tt.mappings, "fail")
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

			if !recordsEqual(got, tt.want) {
				t.Errorf("Process() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMapping_Process_TypePreservation tests that data types are preserved during mapping.
func TestMapping_Process_TypePreservation(t *testing.T) {
	tests := []struct {
		name     string
		mappings []FieldMapping
		input    []map[string]interface{}
		check    func(t *testing.T, result []map[string]interface{})
	}{
		{
			name: "preserves int type",
			mappings: []FieldMapping{
				{Source: strPtr("count"), Target: "total"},
			},
			input: []map[string]interface{}{
				{"count": 42},
			},
			check: func(t *testing.T, result []map[string]interface{}) {
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
			input: []map[string]interface{}{
				{"price": 19.99},
			},
			check: func(t *testing.T, result []map[string]interface{}) {
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
			input: []map[string]interface{}{
				{"active": true},
			},
			check: func(t *testing.T, result []map[string]interface{}) {
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
			input: []map[string]interface{}{
				{"empty": nil},
			},
			check: func(t *testing.T, result []map[string]interface{}) {
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
			input: []map[string]interface{}{
				{"items": []interface{}{"a", "b", "c"}},
			},
			check: func(t *testing.T, result []map[string]interface{}) {
				if _, ok := result[0]["list"].([]interface{}); !ok {
					t.Errorf("expected []interface{}, got %T", result[0]["list"])
				}
			},
		},
		{
			name: "preserves nested object type",
			mappings: []FieldMapping{
				{Source: strPtr("meta"), Target: "metadata"},
			},
			input: []map[string]interface{}{
				{"meta": map[string]interface{}{"key": "value"}},
			},
			check: func(t *testing.T, result []map[string]interface{}) {
				if _, ok := result[0]["metadata"].(map[string]interface{}); !ok {
					t.Errorf("expected map[string]interface{}, got %T", result[0]["metadata"])
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

// TestMappingModule_IntegrationScenarios tests complete mapping scenarios simulating pipeline integration.
func TestMappingModule_IntegrationScenarios(t *testing.T) {
	t.Run("end-to-end transform scenario", func(t *testing.T) {
		// Simulate data coming from HTTP Polling input module
		inputRecords := []map[string]interface{}{
			{
				"user": map[string]interface{}{
					"firstName": "  JOHN  ",
					"lastName":  "DOE",
					"email":     "JOHN@EXAMPLE.COM",
				},
				"metadata": map[string]interface{}{
					"createdAt": "2026-01-15T10:30:00Z",
					"tags":      "sales,vip,active",
				},
			},
			{
				"user": map[string]interface{}{
					"firstName": "Jane",
					"lastName":  "Smith",
					"email":     "jane.smith@example.com",
				},
				"metadata": map[string]interface{}{
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
		contact, ok := rec1["contact"].(map[string]interface{})
		if !ok {
			t.Fatalf("Expected 'contact' to be a map, got %T", rec1["contact"])
		}

		name, ok := contact["name"].(map[string]interface{})
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
		labels, ok := rec1["labels"].([]interface{})
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
		result, err := mapper.Process(context.Background(), []map[string]interface{}{})
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
		// Verify that output is []map[string]interface{} which Output modules expect
		mappings := []FieldMapping{
			{Source: strPtr("data"), Target: "payload"},
		}

		mapper, _ := NewMappingFromConfig(mappings, "fail")

		input := []map[string]interface{}{
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

		input := []map[string]interface{}{
			{"name": "  JOHN DOE  ", "email": "john@example.com"},
			{"name": "Jane Smith", "email": "jane@example.com"},
		}

		mapper, _ := NewMappingFromConfig(mappings, "fail")

		// Run multiple times
		var results [][]map[string]interface{}
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

		input := []map[string]interface{}{
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

		input := []map[string]interface{}{
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

		input := []map[string]interface{}{
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

		input := []map[string]interface{}{
			{
				"user": map[string]interface{}{
					"profile": map[string]interface{}{
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
			output, ok := result[0]["output"].(map[string]interface{})
			if !ok {
				t.Errorf("Run %d: expected 'output' to be a map", i)
				continue
			}
			nested, ok := output["nested"].(map[string]interface{})
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
	records := []map[string]interface{}{
		{"test": "data"},
	}
	_, _ = mapper.Process(context.Background(), records)
}

// TestMapping_Process_ErrorContext tests that errors include proper context.
func TestMapping_Process_ErrorContext(t *testing.T) {
	tests := []struct {
		name        string
		mappings    []FieldMapping
		input       []map[string]interface{}
		errContains []string
	}{
		{
			name: "missing required field error includes field path and record index",
			mappings: []FieldMapping{
				{Source: strPtr("required_field"), Target: "output", OnMissing: "fail"},
			},
			input: []map[string]interface{}{
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
		input    []map[string]interface{}
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
			input: []map[string]interface{}{
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
			input: []map[string]interface{}{
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

// Helper functions for test assertions

func containsString(s, substr string) bool {
	return strings.Contains(s, substr)
}

func recordsEqual(a, b []map[string]interface{}) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !mapsEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func mapsEqual(a, b map[string]interface{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		bv, ok := b[k]
		if !ok {
			return false
		}
		if !valuesEqual(v, bv) {
			return false
		}
	}
	return true
}

func valuesEqual(a, b interface{}) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Handle maps
	aMap, aIsMap := a.(map[string]interface{})
	bMap, bIsMap := b.(map[string]interface{})
	if aIsMap && bIsMap {
		return mapsEqual(aMap, bMap)
	}

	// Handle slices
	aSlice, aIsSlice := a.([]interface{})
	bSlice, bIsSlice := b.([]interface{})
	if aIsSlice && bIsSlice {
		if len(aSlice) != len(bSlice) {
			return false
		}
		for i := range aSlice {
			if !valuesEqual(aSlice[i], bSlice[i]) {
				return false
			}
		}
		return true
	}

	// Handle primitives - use string comparison for simplicity
	return a == b
}

// Test metadata preservation through mapping filter
func TestMapping_MetadataPreservation(t *testing.T) {
	t.Run("preserves _metadata during transformation", func(t *testing.T) {
		mappings := []FieldMapping{
			{Source: strPtr("name"), Target: "fullName"},
			{Source: strPtr("age"), Target: "userAge"},
		}

		module, err := NewMappingFromConfig(mappings, OnErrorFail)
		if err != nil {
			t.Fatalf("NewMappingFromConfig failed: %v", err)
		}

		records := []map[string]interface{}{
			{
				"name": "John",
				"age":  30,
				"_metadata": map[string]interface{}{
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
		metadata, ok := result[0]["_metadata"].(map[string]interface{})
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

		module, err := NewMappingFromConfig(mappings, OnErrorFail)
		if err != nil {
			t.Fatalf("NewMappingFromConfig failed: %v", err)
		}

		records := []map[string]interface{}{
			{
				"data":      "value",
				"_metadata": map[string]interface{}{"key": "meta"},
				"_custom":   "custom_value",
				"_internal": map[string]interface{}{"debug": true},
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

		module, err := NewMappingFromConfig(mappings, OnErrorFail)
		if err != nil {
			t.Fatalf("NewMappingFromConfig failed: %v", err)
		}

		records := []map[string]interface{}{
			{
				"name": "Test",
				"_metadata": map[string]interface{}{
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

		module, err := NewMappingFromConfig(mappings, OnErrorFail)
		if err != nil {
			t.Fatalf("NewMappingFromConfig failed: %v", err)
		}

		records := []map[string]interface{}{
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

		module, err := NewMappingFromConfig(mappings, OnErrorFail)
		if err != nil {
			t.Fatalf("NewMappingFromConfig failed: %v", err)
		}

		records := []map[string]interface{}{
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

		module, err := NewMappingFromConfig(mappings, OnErrorFail)
		if err != nil {
			t.Fatalf("NewMappingFromConfig failed: %v", err)
		}

		records := []map[string]interface{}{
			{
				"user": map[string]interface{}{
					"name":     "John",
					"password": "secret",
				},
			},
		}

		result, err := module.Process(context.Background(), records)
		if err != nil {
			t.Fatalf("Process failed: %v", err)
		}

		user, ok := result[0]["user"].(map[string]interface{})
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

		module, err := NewMappingFromConfig(mappings, OnErrorFail)
		if err != nil {
			t.Fatalf("NewMappingFromConfig failed: %v", err)
		}

		records := []map[string]interface{}{
			{
				"id": 1,
				"user": map[string]interface{}{
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

		module, err := NewMappingFromConfig(mappings, OnErrorFail)
		if err != nil {
			t.Fatalf("NewMappingFromConfig failed: %v", err)
		}

		records := []map[string]interface{}{
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

		module, err := NewMappingFromConfig(mappings, OnErrorFail)
		if err != nil {
			t.Fatalf("NewMappingFromConfig failed: %v", err)
		}

		records := []map[string]interface{}{
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
