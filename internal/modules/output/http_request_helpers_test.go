package output

import "testing"

func TestGetRecordFieldString_DotNotation(t *testing.T) {
	record := map[string]interface{}{
		"user": map[string]interface{}{
			"profile": map[string]interface{}{
				"name": "Alice",
				"age":  float64(42),
			},
		},
	}

	if got := getRecordFieldString(record, "user.profile.name"); got != "Alice" {
		t.Errorf("got %q, want %q", got, "Alice")
	}
	if got := getRecordFieldString(record, "user.profile.age"); got != "42" {
		t.Errorf("got %q, want %q", got, "42")
	}
	if got := getRecordFieldString(record, "user.profile.missing"); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestGetRecordFieldString_WithArrayIndex(t *testing.T) {
	record := map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{"id": "abc", "name": "first"},
			map[string]interface{}{"id": "def", "name": "second"},
		},
	}

	tests := []struct {
		path string
		want string
	}{
		{"items[0].id", "abc"},
		{"items[0].name", "first"},
		{"items[1].id", "def"},
		{"items[1].name", "second"},
		{"items[2].id", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := getRecordFieldString(record, tt.path); got != tt.want {
				t.Errorf("getRecordFieldString(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestGetRecordFieldString_TypeConversions(t *testing.T) {
	record := map[string]interface{}{
		"str":   "hello",
		"int":   42,
		"int64": int64(123456789),
		"flt":   3.14,
		"bool":  true,
		"null":  nil,
	}

	tests := []struct {
		path string
		want string
	}{
		{"str", "hello"},
		{"int", "42"},
		{"int64", "123456789"},
		{"flt", "3.14"},
		{"bool", "true"},
		{"null", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := getRecordFieldString(record, tt.path); got != tt.want {
				t.Errorf("getRecordFieldString(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
