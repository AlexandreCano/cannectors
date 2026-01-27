package output

import (
	"strings"
	"testing"

	"github.com/canectors/runtime/internal/template"
)

func TestHasTemplateVariables(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "simple template variable",
			input:    "{{record.id}}",
			expected: true,
		},
		{
			name:     "template in URL",
			input:    "/api/users/{{record.user_id}}/orders",
			expected: true,
		},
		{
			name:     "no template variable",
			input:    "/api/users/123/orders",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "only opening braces",
			input:    "{{test",
			expected: false,
		},
		{
			name:     "only closing braces",
			input:    "test}}",
			expected: false,
		},
		{
			name:     "multiple template variables",
			input:    "{{record.id}} and {{record.name}}",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasTemplateVariables(tt.input)
			if result != tt.expected {
				t.Errorf("HasTemplateVariables(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestTemplateEvaluator_ParseTemplateVariables(t *testing.T) {
	evaluator := NewTemplateEvaluator()

	tests := []struct {
		name             string
		template         string
		expectedCount    int
		expectedPaths    []string
		expectedDefaults []string
	}{
		{
			name:             "single variable",
			template:         "{{record.id}}",
			expectedCount:    1,
			expectedPaths:    []string{"record.id"},
			expectedDefaults: []string{""},
		},
		{
			name:             "nested variable",
			template:         "{{record.user.profile.name}}",
			expectedCount:    1,
			expectedPaths:    []string{"record.user.profile.name"},
			expectedDefaults: []string{""},
		},
		{
			name:             "variable with default",
			template:         `{{record.field | default: "fallback"}}`,
			expectedCount:    1,
			expectedPaths:    []string{"record.field"},
			expectedDefaults: []string{"fallback"},
		},
		{
			name:             "multiple variables",
			template:         "/api/{{record.resource}}/{{record.id}}",
			expectedCount:    2,
			expectedPaths:    []string{"record.resource", "record.id"},
			expectedDefaults: []string{"", ""},
		},
		{
			name:             "variable with spaces",
			template:         "{{ record.id }}",
			expectedCount:    1,
			expectedPaths:    []string{"record.id"},
			expectedDefaults: []string{""},
		},
		{
			name:             "array index variable",
			template:         "{{record.items[0].name}}",
			expectedCount:    1,
			expectedPaths:    []string{"record.items[0].name"},
			expectedDefaults: []string{""},
		},
		{
			name:             "no variables",
			template:         "plain text",
			expectedCount:    0,
			expectedPaths:    []string{},
			expectedDefaults: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			variables := evaluator.ParseTemplateVariables(tt.template)

			if len(variables) != tt.expectedCount {
				t.Errorf("ParseTemplateVariables(%q) returned %d variables, want %d",
					tt.template, len(variables), tt.expectedCount)
				return
			}

			for i, v := range variables {
				if i < len(tt.expectedPaths) && v.Path != tt.expectedPaths[i] {
					t.Errorf("variable %d path = %q, want %q", i, v.Path, tt.expectedPaths[i])
				}
				if i < len(tt.expectedDefaults) {
					expectedHasDefault := tt.expectedDefaults[i] != ""
					if v.HasDefault != expectedHasDefault {
						t.Errorf("variable %d HasDefault = %v, want %v", i, v.HasDefault, expectedHasDefault)
					}
					if v.DefaultValue != tt.expectedDefaults[i] {
						t.Errorf("variable %d DefaultValue = %q, want %q", i, v.DefaultValue, tt.expectedDefaults[i])
					}
				}
			}
		})
	}
}

func TestTemplateEvaluator_EvaluateTemplate(t *testing.T) {
	evaluator := NewTemplateEvaluator()

	tests := []struct {
		name     string
		template string
		record   map[string]interface{}
		expected string
	}{
		{
			name:     "simple field substitution",
			template: "{{record.id}}",
			record:   map[string]interface{}{"id": "123"},
			expected: "123",
		},
		{
			name:     "URL with template",
			template: "/api/users/{{record.user_id}}/orders",
			record:   map[string]interface{}{"user_id": "456"},
			expected: "/api/users/456/orders",
		},
		{
			name:     "nested field access",
			template: "{{record.user.profile.name}}",
			record: map[string]interface{}{
				"user": map[string]interface{}{
					"profile": map[string]interface{}{
						"name": "John",
					},
				},
			},
			expected: "John",
		},
		{
			name:     "missing field returns empty",
			template: "{{record.missing}}",
			record:   map[string]interface{}{"id": "123"},
			expected: "",
		},
		{
			name:     "missing field with default",
			template: `{{record.missing | default: "default_value"}}`,
			record:   map[string]interface{}{"id": "123"},
			expected: "default_value",
		},
		{
			name:     "null field with default",
			template: `{{record.nullable | default: "fallback"}}`,
			record:   map[string]interface{}{"nullable": nil},
			expected: "fallback",
		},
		{
			name:     "multiple variables",
			template: "User {{record.name}} has ID {{record.id}}",
			record:   map[string]interface{}{"name": "Alice", "id": 42},
			expected: "User Alice has ID 42",
		},
		{
			name:     "integer value",
			template: "{{record.count}}",
			record:   map[string]interface{}{"count": float64(100)},
			expected: "100",
		},
		{
			name:     "float value",
			template: "{{record.price}}",
			record:   map[string]interface{}{"price": 19.99},
			expected: "19.99",
		},
		{
			name:     "boolean value",
			template: "{{record.active}}",
			record:   map[string]interface{}{"active": true},
			expected: "true",
		},
		{
			name:     "no template variables",
			template: "plain text",
			record:   map[string]interface{}{"id": "123"},
			expected: "plain text",
		},
		{
			name:     "array index access",
			template: "{{record.items[0].name}}",
			record: map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"name": "First"},
					map[string]interface{}{"name": "Second"},
				},
			},
			expected: "First",
		},
		{
			name:     "array index out of bounds returns empty",
			template: "{{record.items[10].name}}",
			record: map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"name": "First"},
				},
			},
			expected: "",
		},
		{
			name:     "without record prefix",
			template: "{{id}}",
			record:   map[string]interface{}{"id": "456"},
			expected: "456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := evaluator.EvaluateTemplate(tt.template, tt.record)
			if result != tt.expected {
				t.Errorf("EvaluateTemplate(%q, %v) = %q, want %q",
					tt.template, tt.record, result, tt.expected)
			}
		})
	}
}

func TestTemplateEvaluator_EvaluateTemplateForURL(t *testing.T) {
	evaluator := NewTemplateEvaluator()

	tests := []struct {
		name     string
		template string
		record   map[string]interface{}
		expected string
	}{
		{
			name:     "simple URL encoding",
			template: "/api/users/{{record.user_id}}",
			record:   map[string]interface{}{"user_id": "123"},
			expected: "/api/users/123",
		},
		{
			name:     "URL encoding with special characters",
			template: "/api/search/{{record.query}}",
			record:   map[string]interface{}{"query": "hello world"},
			expected: "/api/search/hello+world", // QueryEscape uses + for spaces
		},
		{
			name:     "URL encoding with slashes",
			template: "/api/path/{{record.path}}",
			record:   map[string]interface{}{"path": "foo/bar"},
			expected: "/api/path/foo%2Fbar",
		},
		{
			name:     "URL encoding with special URL characters",
			template: "/api/query/{{record.q}}",
			record:   map[string]interface{}{"q": "a=b&c=d"},
			expected: "/api/query/a%3Db%26c%3Dd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := evaluator.EvaluateTemplateForURL(tt.template, tt.record)
			if result != tt.expected {
				t.Errorf("EvaluateTemplateForURL(%q, %v) = %q, want %q",
					tt.template, tt.record, result, tt.expected)
			}
		})
	}
}

func TestValidateTemplateSyntax(t *testing.T) {
	tests := []struct {
		name        string
		template    string
		expectError bool
	}{
		{
			name:        "valid template",
			template:    "{{record.id}}",
			expectError: false,
		},
		{
			name:        "valid template with default",
			template:    `{{record.field | default: "value"}}`,
			expectError: false,
		},
		{
			name:        "valid multiple templates",
			template:    "{{record.a}} {{record.b}}",
			expectError: false,
		},
		{
			name:        "no templates",
			template:    "plain text",
			expectError: false,
		},
		{
			name:        "empty string",
			template:    "",
			expectError: false,
		},
		{
			name:        "unmatched opening brace",
			template:    "{{record.id",
			expectError: true,
		},
		{
			name:        "unmatched closing brace",
			template:    "record.id}}",
			expectError: true,
		},
		{
			name:        "empty variable path",
			template:    "{{}}",
			expectError: true,
		},
		{
			name:        "whitespace only variable path",
			template:    "{{   }}",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTemplateSyntax(tt.template)
			if tt.expectError && err == nil {
				t.Errorf("ValidateTemplateSyntax(%q) expected error, got nil", tt.template)
			}
			if !tt.expectError && err != nil {
				t.Errorf("ValidateTemplateSyntax(%q) unexpected error: %v", tt.template, err)
			}
		})
	}
}

func TestTemplateEvaluator_EvaluateHeaders(t *testing.T) {
	evaluator := NewTemplateEvaluator()

	tests := []struct {
		name     string
		headers  map[string]string
		record   map[string]interface{}
		expected map[string]string
	}{
		{
			name: "single templated header",
			headers: map[string]string{
				"X-User-ID": "{{record.user_id}}",
			},
			record: map[string]interface{}{"user_id": "123"},
			expected: map[string]string{
				"X-User-ID": "123",
			},
		},
		{
			name: "mixed headers",
			headers: map[string]string{
				"X-User-ID":    "{{record.user_id}}",
				"Content-Type": "application/json",
			},
			record: map[string]interface{}{"user_id": "456"},
			expected: map[string]string{
				"X-User-ID":    "456",
				"Content-Type": "application/json",
			},
		},
		{
			name: "missing value in header",
			headers: map[string]string{
				"X-Optional": "{{record.missing}}",
			},
			record: map[string]interface{}{"id": "123"},
			expected: map[string]string{
				"X-Optional": "",
			},
		},
		{
			name:     "empty headers",
			headers:  map[string]string{},
			record:   map[string]interface{}{"id": "123"},
			expected: map[string]string{},
		},
		{
			name:     "nil headers",
			headers:  nil,
			record:   map[string]interface{}{"id": "123"},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := evaluator.EvaluateHeaders(tt.headers, tt.record)

			if tt.expected == nil {
				if len(result) > 0 {
					t.Errorf("EvaluateHeaders expected nil/empty, got %v", result)
				}
				return
			}

			if len(result) != len(tt.expected) {
				t.Errorf("EvaluateHeaders returned %d headers, want %d", len(result), len(tt.expected))
				return
			}

			for k, v := range tt.expected {
				if result[k] != v {
					t.Errorf("Header %q = %q, want %q", k, result[k], v)
				}
			}
		})
	}
}

func TestTemplateEvaluator_EvaluateMapValues(t *testing.T) {
	evaluator := NewTemplateEvaluator()

	tests := []struct {
		name     string
		data     interface{}
		record   map[string]interface{}
		expected interface{}
	}{
		{
			name:     "simple string template",
			data:     "{{record.name}}",
			record:   map[string]interface{}{"name": "John"},
			expected: "John",
		},
		{
			name:     "non-template string",
			data:     "plain text",
			record:   map[string]interface{}{"name": "John"},
			expected: "plain text",
		},
		{
			name: "map with templates",
			data: map[string]interface{}{
				"userId": "{{record.id}}",
				"name":   "{{record.name}}",
				"type":   "user",
			},
			record: map[string]interface{}{"id": "123", "name": "Alice"},
			expected: map[string]interface{}{
				"userId": "123",
				"name":   "Alice",
				"type":   "user",
			},
		},
		{
			name: "nested map with templates",
			data: map[string]interface{}{
				"user": map[string]interface{}{
					"id":   "{{record.user_id}}",
					"name": "{{record.user_name}}",
				},
			},
			record: map[string]interface{}{"user_id": "456", "user_name": "Bob"},
			expected: map[string]interface{}{
				"user": map[string]interface{}{
					"id":   "456",
					"name": "Bob",
				},
			},
		},
		{
			name: "array with templates",
			data: []interface{}{
				"{{record.item1}}",
				"{{record.item2}}",
				"static",
			},
			record: map[string]interface{}{"item1": "first", "item2": "second"},
			expected: []interface{}{
				"first",
				"second",
				"static",
			},
		},
		{
			name:     "non-string value passes through",
			data:     42,
			record:   map[string]interface{}{"id": "123"},
			expected: 42,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := evaluator.EvaluateMapValues(tt.data, tt.record)

			// Compare results based on type
			switch expected := tt.expected.(type) {
			case string:
				if result != expected {
					t.Errorf("EvaluateMapValues = %v, want %v", result, expected)
				}
			case map[string]interface{}:
				resultMap, ok := result.(map[string]interface{})
				if !ok {
					t.Errorf("EvaluateMapValues returned %T, want map[string]interface{}", result)
					return
				}
				compareMapValues(t, resultMap, expected)
			case []interface{}:
				resultArr, ok := result.([]interface{})
				if !ok {
					t.Errorf("EvaluateMapValues returned %T, want []interface{}", result)
					return
				}
				if len(resultArr) != len(expected) {
					t.Errorf("Array length = %d, want %d", len(resultArr), len(expected))
					return
				}
				for i := range expected {
					if resultArr[i] != expected[i] {
						t.Errorf("Array[%d] = %v, want %v", i, resultArr[i], expected[i])
					}
				}
			default:
				if result != expected {
					t.Errorf("EvaluateMapValues = %v, want %v", result, expected)
				}
			}
		})
	}
}

func compareMapValues(t *testing.T, result, expected map[string]interface{}) {
	t.Helper()
	if len(result) != len(expected) {
		t.Errorf("Map length = %d, want %d", len(result), len(expected))
		return
	}
	for k, v := range expected {
		rv, ok := result[k]
		if !ok {
			t.Errorf("Missing key %q", k)
			continue
		}
		switch ev := v.(type) {
		case map[string]interface{}:
			rm, ok := rv.(map[string]interface{})
			if !ok {
				t.Errorf("Key %q: expected map, got %T", k, rv)
				continue
			}
			compareMapValues(t, rm, ev)
		default:
			if rv != v {
				t.Errorf("Key %q = %v, want %v", k, rv, v)
			}
		}
	}
}

func TestGetNestedValueFromPath(t *testing.T) {
	tests := []struct {
		name     string
		obj      map[string]interface{}
		path     string
		expected interface{}
		found    bool
	}{
		{
			name:     "simple field",
			obj:      map[string]interface{}{"id": "123"},
			path:     "id",
			expected: "123",
			found:    true,
		},
		{
			name: "nested field",
			obj: map[string]interface{}{
				"user": map[string]interface{}{
					"name": "John",
				},
			},
			path:     "user.name",
			expected: "John",
			found:    true,
		},
		{
			name: "deeply nested field",
			obj: map[string]interface{}{
				"a": map[string]interface{}{
					"b": map[string]interface{}{
						"c": "value",
					},
				},
			},
			path:     "a.b.c",
			expected: "value",
			found:    true,
		},
		{
			name:     "missing field",
			obj:      map[string]interface{}{"id": "123"},
			path:     "missing",
			expected: nil,
			found:    false,
		},
		{
			name: "missing nested field",
			obj: map[string]interface{}{
				"user": map[string]interface{}{},
			},
			path:     "user.name",
			expected: nil,
			found:    false,
		},
		{
			name: "array index",
			obj: map[string]interface{}{
				"items": []interface{}{"a", "b", "c"},
			},
			path:     "items[1]",
			expected: "b",
			found:    true,
		},
		{
			name: "array index with nested object",
			obj: map[string]interface{}{
				"users": []interface{}{
					map[string]interface{}{"name": "Alice"},
					map[string]interface{}{"name": "Bob"},
				},
			},
			path:     "users[1].name",
			expected: "Bob",
			found:    true,
		},
		{
			name: "array index out of bounds",
			obj: map[string]interface{}{
				"items": []interface{}{"a"},
			},
			path:     "items[5]",
			expected: nil,
			found:    false,
		},
		{
			name:     "empty path",
			obj:      map[string]interface{}{"id": "123"},
			path:     "",
			expected: nil,
			found:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, found := template.GetNestedValue(tt.obj, tt.path)
			if found != tt.found {
				t.Errorf("GetNestedValue(%q) found = %v, want %v", tt.path, found, tt.found)
			}
			if result != tt.expected {
				t.Errorf("GetNestedValue(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestValueToString(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected string
	}{
		{name: "string", value: "hello", expected: "hello"},
		{name: "int", value: 42, expected: "42"},
		{name: "int64", value: int64(123456789), expected: "123456789"},
		{name: "float64 integer", value: float64(100), expected: "100"},
		{name: "float64 decimal", value: 19.99, expected: "19.99"},
		{name: "bool true", value: true, expected: "true"},
		{name: "bool false", value: false, expected: "false"},
		{name: "nil", value: nil, expected: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := template.ValueToString(tt.value)
			if result != tt.expected {
				t.Errorf("valueToString(%v) = %q, want %q", tt.value, result, tt.expected)
			}
		})
	}
}

func TestTemplateEvaluator_Caching(t *testing.T) {
	evaluator := NewTemplateEvaluator()
	templateStr := "{{record.id}} {{record.name}}"

	// First call should parse
	vars1 := evaluator.ParseTemplateVariables(templateStr)

	// Second call should use cache (same results returned)
	vars2 := evaluator.ParseTemplateVariables(templateStr)

	// Both should return same results
	if len(vars1) != len(vars2) {
		t.Errorf("Cached result has different length: %d vs %d", len(vars1), len(vars2))
	}

	// Verify same variable data
	for i, v1 := range vars1 {
		v2 := vars2[i]
		if v1.Path != v2.Path || v1.FullMatch != v2.FullMatch {
			t.Errorf("Variable %d mismatch: %+v vs %+v", i, v1, v2)
		}
	}
}

// Additional comprehensive tests for Task 8

func TestTemplateEvaluator_EdgeCases(t *testing.T) {
	evaluator := NewTemplateEvaluator()

	tests := []struct {
		name     string
		template string
		record   map[string]interface{}
		expected string
	}{
		{
			name:     "empty record",
			template: "{{record.id}}",
			record:   map[string]interface{}{},
			expected: "",
		},
		{
			name:     "nil record value",
			template: "{{record.id}}",
			record:   map[string]interface{}{"id": nil},
			expected: "",
		},
		{
			name:     "deeply nested nil",
			template: "{{record.a.b.c}}",
			record: map[string]interface{}{
				"a": map[string]interface{}{
					"b": nil,
				},
			},
			expected: "",
		},
		{
			name:     "numeric field",
			template: "ID: {{record.id}}",
			record:   map[string]interface{}{"id": float64(12345)},
			expected: "ID: 12345",
		},
		{
			name:     "boolean true",
			template: "Active: {{record.active}}",
			record:   map[string]interface{}{"active": true},
			expected: "Active: true",
		},
		{
			name:     "boolean false",
			template: "Active: {{record.active}}",
			record:   map[string]interface{}{"active": false},
			expected: "Active: false",
		},
		{
			name:     "empty string value",
			template: "Name: {{record.name}}",
			record:   map[string]interface{}{"name": ""},
			expected: "Name: ",
		},
		{
			name:     "special characters in value",
			template: "Query: {{record.q}}",
			record:   map[string]interface{}{"q": "<script>alert('xss')</script>"},
			expected: "Query: <script>alert('xss')</script>",
		},
		{
			name:     "unicode value",
			template: "Name: {{record.name}}",
			record:   map[string]interface{}{"name": "日本語テスト"},
			expected: "Name: 日本語テスト",
		},
		{
			name:     "multiple same variable",
			template: "{{record.id}}-{{record.id}}",
			record:   map[string]interface{}{"id": "abc"},
			expected: "abc-abc",
		},
		{
			name:     "adjacent variables",
			template: "{{record.a}}{{record.b}}",
			record:   map[string]interface{}{"a": "X", "b": "Y"},
			expected: "XY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := evaluator.EvaluateTemplate(tt.template, tt.record)
			if result != tt.expected {
				t.Errorf("EvaluateTemplate(%q, %v) = %q, want %q",
					tt.template, tt.record, result, tt.expected)
			}
		})
	}
}

func TestTemplateEvaluator_DefaultValues(t *testing.T) {
	evaluator := NewTemplateEvaluator()

	tests := []struct {
		name     string
		template string
		record   map[string]interface{}
		expected string
	}{
		{
			name:     "default used when missing",
			template: `{{record.missing | default: "N/A"}}`,
			record:   map[string]interface{}{},
			expected: "N/A",
		},
		{
			name:     "default used when nil",
			template: `{{record.nullable | default: "none"}}`,
			record:   map[string]interface{}{"nullable": nil},
			expected: "none",
		},
		{
			name:     "default not used when present",
			template: `{{record.id | default: "unknown"}}`,
			record:   map[string]interface{}{"id": "123"},
			expected: "123",
		},
		{
			name:     "empty string default - uses empty default",
			template: `{{record.missing | default: ""}}`,
			record:   map[string]interface{}{},
			expected: "", // Empty string is a valid default value
		},
		{
			name:     "default with spaces",
			template: `{{record.status | default: "not set"}}`,
			record:   map[string]interface{}{},
			expected: "not set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := evaluator.EvaluateTemplate(tt.template, tt.record)
			if result != tt.expected {
				t.Errorf("EvaluateTemplate(%q, %v) = %q, want %q",
					tt.template, tt.record, result, tt.expected)
			}
		})
	}
}

// Note: TestTruncateForLog was removed as truncateForLog is an internal utility
// function in the shared template package and is tested implicitly through other tests.

// TestTemplateEvaluator_EdgeCases_WhitespaceOnly tests edge cases with whitespace
func TestTemplateEvaluator_EdgeCases_WhitespaceOnly(t *testing.T) {
	evaluator := NewTemplateEvaluator()

	// Template with only whitespace should be caught by ValidateTemplateSyntax
	err := ValidateTemplateSyntax("{{   }}")
	if err == nil {
		t.Error("ValidateTemplateSyntax should reject template with only whitespace")
	}

	// Template with whitespace around variable should work
	result := evaluator.EvaluateTemplate("{{ record.id }}", map[string]interface{}{"id": "123"})
	if result != "123" {
		t.Errorf("Expected '123', got %q", result)
	}
}

// TestTemplateEvaluator_EdgeCases_SpecialRegexCharacters tests special regex characters in field names
func TestTemplateEvaluator_EdgeCases_SpecialRegexCharacters(t *testing.T) {
	evaluator := NewTemplateEvaluator()

	tests := []struct {
		name     string
		template string
		record   map[string]interface{}
		expected string
	}{
		{
			name:     "field with dots",
			template: "{{record.field.with.dots}}",
			record: map[string]interface{}{
				"field": map[string]interface{}{
					"with": map[string]interface{}{
						"dots": "value",
					},
				},
			},
			expected: "value",
		},
		{
			name:     "field with brackets",
			template: "{{record.items[0]}}",
			record: map[string]interface{}{
				"items": []interface{}{"first"},
			},
			expected: "first",
		},
		{
			name:     "field with pipe character in value",
			template: "{{record.field}}",
			record:   map[string]interface{}{"field": "value|with|pipes"},
			expected: "value|with|pipes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := evaluator.EvaluateTemplate(tt.template, tt.record)
			if result != tt.expected {
				t.Errorf("EvaluateTemplate(%q, %v) = %q, want %q",
					tt.template, tt.record, result, tt.expected)
			}
		})
	}
}

// TestTemplateEvaluator_EdgeCases_VeryLongTemplate tests very long template strings (DoS protection)
func TestTemplateEvaluator_EdgeCases_VeryLongTemplate(t *testing.T) {
	evaluator := NewTemplateEvaluator()

	// Create a very long template (10KB)
	longTemplate := "{{record.id}}" + strings.Repeat(" and {{record.id}}", 1000)
	longRecord := map[string]interface{}{"id": "123"}

	// Should not panic or hang
	result := evaluator.EvaluateTemplate(longTemplate, longRecord)

	// Result should contain many "123" values
	expectedCount := 1001 // 1 + 1000
	actualCount := strings.Count(result, "123")
	if actualCount != expectedCount {
		t.Errorf("Expected %d occurrences of '123', got %d", expectedCount, actualCount)
	}
}

// TestTemplateEvaluator_EdgeCases_ConcurrentAccess tests concurrent template evaluation.
// Each goroutine uses its own Evaluator because the template cache is not thread-safe.
func TestTemplateEvaluator_EdgeCases_ConcurrentAccess(t *testing.T) {
	template := "{{record.id}}"
	record := map[string]interface{}{"id": "123"}

	const numGoroutines = 10
	const numIterations = 100
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer func() { done <- true }()
			evaluator := NewTemplateEvaluator()
			for j := 0; j < numIterations; j++ {
				result := evaluator.EvaluateTemplate(template, record)
				if result != "123" {
					t.Errorf("Concurrent evaluation failed: expected '123', got %q", result)
				}
			}
		}()
	}

	for i := 0; i < numGoroutines; i++ {
		<-done
	}
}

// TestValidateTemplateSyntax_EdgeCases tests edge cases for template syntax validation
func TestValidateTemplateSyntax_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		template    string
		expectError bool
	}{
		{
			name:        "whitespace only variable",
			template:    "{{   }}",
			expectError: true,
		},
		{
			name:        "empty braces",
			template:    "{{}}",
			expectError: true,
		},
		{
			name:        "nested braces",
			template:    "{{record.{{field}}}}",
			expectError: true, // stray {{/}} remain after removing {{field}} match
		},
		{
			name:        "balanced but invalid pairing",
			template:    "}}{{",
			expectError: true,
		},
		{
			name:        "very long template",
			template:    strings.Repeat("{{record.id}}", 1000),
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTemplateSyntax(tt.template)
			if tt.expectError && err == nil {
				t.Errorf("ValidateTemplateSyntax(%q) expected error, got nil", tt.template)
			}
			if !tt.expectError && err != nil {
				t.Errorf("ValidateTemplateSyntax(%q) unexpected error: %v", tt.template, err)
			}
		})
	}
}
