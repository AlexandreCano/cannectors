package moduleconfig_test

import (
	"testing"

	"github.com/cannectors/runtime/internal/moduleconfig"
)

func TestGetNestedValue_Simple(t *testing.T) {
	obj := map[string]interface{}{"name": "Alice"}
	val, ok := moduleconfig.GetNestedValue(obj, "name")
	if !ok || val != "Alice" {
		t.Errorf("GetNestedValue(name) = %v, %v", val, ok)
	}
}

func TestGetNestedValue_Nested(t *testing.T) {
	obj := map[string]interface{}{
		"user": map[string]interface{}{
			"profile": map[string]interface{}{
				"name": "Bob",
			},
		},
	}
	val, ok := moduleconfig.GetNestedValue(obj, "user.profile.name")
	if !ok || val != "Bob" {
		t.Errorf("GetNestedValue(user.profile.name) = %v, %v", val, ok)
	}
}

func TestGetNestedValue_Array(t *testing.T) {
	obj := map[string]interface{}{
		"items": []interface{}{"a", "b", "c"},
	}
	val, ok := moduleconfig.GetNestedValue(obj, "items[1]")
	if !ok || val != "b" {
		t.Errorf("GetNestedValue(items[1]) = %v, %v", val, ok)
	}
}

func TestGetNestedValue_MissingPath(t *testing.T) {
	obj := map[string]interface{}{"name": "Alice"}
	_, ok := moduleconfig.GetNestedValue(obj, "missing.path")
	if ok {
		t.Error("Expected missing path to return false")
	}
}

func TestGetNestedValue_EmptyPath(t *testing.T) {
	obj := map[string]interface{}{"name": "Alice"}
	_, ok := moduleconfig.GetNestedValue(obj, "")
	if ok {
		t.Error("Expected empty path to return false")
	}
}

func TestSetNestedValue_Simple(t *testing.T) {
	obj := map[string]interface{}{}
	if err := moduleconfig.SetNestedValue(obj, "name", "Alice"); err != nil {
		t.Fatal(err)
	}
	if obj["name"] != "Alice" {
		t.Errorf("name = %v", obj["name"])
	}
}

func TestSetNestedValue_Nested(t *testing.T) {
	obj := map[string]interface{}{}
	if err := moduleconfig.SetNestedValue(obj, "user.profile.name", "Bob"); err != nil {
		t.Fatal(err)
	}
	val, ok := moduleconfig.GetNestedValue(obj, "user.profile.name")
	if !ok || val != "Bob" {
		t.Errorf("user.profile.name = %v, %v", val, ok)
	}
}

func TestSetNestedValue_EmptyPath(t *testing.T) {
	obj := map[string]interface{}{}
	err := moduleconfig.SetNestedValue(obj, "", "value")
	if err == nil {
		t.Error("Expected error for empty path")
	}
}

func TestDeleteNestedValue_Simple(t *testing.T) {
	obj := map[string]interface{}{"name": "Alice", "age": 30}
	moduleconfig.DeleteNestedValue(obj, "name")
	if _, ok := obj["name"]; ok {
		t.Error("Expected name to be deleted")
	}
	if obj["age"] != 30 {
		t.Error("Expected age to remain")
	}
}

func TestDeleteNestedValue_Nested(t *testing.T) {
	obj := map[string]interface{}{
		"user": map[string]interface{}{
			"name": "Bob",
			"age":  25,
		},
	}
	moduleconfig.DeleteNestedValue(obj, "user.name")
	user, ok := obj["user"].(map[string]interface{})
	if !ok {
		t.Fatal("obj[\"user\"] is not map[string]interface{}")
	}
	if _, ok := user["name"]; ok {
		t.Error("Expected user.name to be deleted")
	}
	if user["age"] != 25 {
		t.Error("Expected user.age to remain")
	}
}

func TestDeleteNestedValue_MissingPath(t *testing.T) {
	obj := map[string]interface{}{"name": "Alice"}
	// Should not panic
	moduleconfig.DeleteNestedValue(obj, "missing.path")
}

func TestIsNestedPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"name", false},
		{"user.name", true},
		{"items[0]", true},
		{"data.items[0].name", true},
		{"", false},
	}
	for _, tt := range tests {
		if got := moduleconfig.IsNestedPath(tt.path); got != tt.want {
			t.Errorf("IsNestedPath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestParsePathPart(t *testing.T) {
	tests := []struct {
		part    string
		wantKey string
		wantIdx int
		wantHas bool
		wantErr bool
	}{
		{"name", "name", -1, false, false},
		{"items[0]", "items", 0, true, false},
		{"data[42]", "data", 42, true, false},
		{"bad[-1]", "", -1, false, true},
		{"bad[abc]", "", -1, false, true},
	}
	for _, tt := range tests {
		key, idx, has, err := moduleconfig.ParsePathPart(tt.part)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParsePathPart(%q) error = %v, wantErr %v", tt.part, err, tt.wantErr)
			continue
		}
		if err != nil {
			continue
		}
		if key != tt.wantKey || idx != tt.wantIdx || has != tt.wantHas {
			t.Errorf("ParsePathPart(%q) = (%q, %d, %v), want (%q, %d, %v)",
				tt.part, key, idx, has, tt.wantKey, tt.wantIdx, tt.wantHas)
		}
	}
}
