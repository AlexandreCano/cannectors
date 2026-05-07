package recordpath_test

import (
	"testing"

	"github.com/cannectors/runtime/internal/recordpath"
)

func TestGet_Simple(t *testing.T) {
	obj := map[string]interface{}{"name": "Alice"}
	val, ok := recordpath.Get(obj, "name")
	if !ok || val != "Alice" {
		t.Errorf("Get(name) = %v, %v", val, ok)
	}
}

func TestGet_Nested(t *testing.T) {
	obj := map[string]interface{}{
		"user": map[string]interface{}{
			"profile": map[string]interface{}{
				"name": "Bob",
			},
		},
	}
	val, ok := recordpath.Get(obj, "user.profile.name")
	if !ok || val != "Bob" {
		t.Errorf("Get(user.profile.name) = %v, %v", val, ok)
	}
}

func TestGet_Array(t *testing.T) {
	obj := map[string]interface{}{
		"items": []interface{}{"a", "b", "c"},
	}
	val, ok := recordpath.Get(obj, "items[1]")
	if !ok || val != "b" {
		t.Errorf("Get(items[1]) = %v, %v", val, ok)
	}
}

func TestGet_MissingPath(t *testing.T) {
	obj := map[string]interface{}{"name": "Alice"}
	_, ok := recordpath.Get(obj, "missing.path")
	if ok {
		t.Error("Expected missing path to return false")
	}
}

func TestGet_EmptyPath(t *testing.T) {
	obj := map[string]interface{}{"name": "Alice"}
	_, ok := recordpath.Get(obj, "")
	if ok {
		t.Error("Expected empty path to return false")
	}
}

func TestSet_Simple(t *testing.T) {
	obj := map[string]interface{}{}
	if err := recordpath.Set(obj, "name", "Alice"); err != nil {
		t.Fatal(err)
	}
	if obj["name"] != "Alice" {
		t.Errorf("name = %v", obj["name"])
	}
}

func TestSet_Nested(t *testing.T) {
	obj := map[string]interface{}{}
	if err := recordpath.Set(obj, "user.profile.name", "Bob"); err != nil {
		t.Fatal(err)
	}
	val, ok := recordpath.Get(obj, "user.profile.name")
	if !ok || val != "Bob" {
		t.Errorf("user.profile.name = %v, %v", val, ok)
	}
}

func TestSet_EmptyPath(t *testing.T) {
	obj := map[string]interface{}{}
	err := recordpath.Set(obj, "", "value")
	if err == nil {
		t.Error("Expected error for empty path")
	}
}

func TestDelete_Simple(t *testing.T) {
	obj := map[string]interface{}{"name": "Alice", "age": 30}
	recordpath.Delete(obj, "name")
	if _, ok := obj["name"]; ok {
		t.Error("Expected name to be deleted")
	}
	if obj["age"] != 30 {
		t.Error("Expected age to remain")
	}
}

func TestDelete_Nested(t *testing.T) {
	obj := map[string]interface{}{
		"user": map[string]interface{}{
			"name": "Bob",
			"age":  25,
		},
	}
	recordpath.Delete(obj, "user.name")
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

func TestDelete_MissingPath(t *testing.T) {
	obj := map[string]interface{}{"name": "Alice"}
	// Should not panic
	recordpath.Delete(obj, "missing.path")
}

func TestIsNested(t *testing.T) {
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
		if got := recordpath.IsNested(tt.path); got != tt.want {
			t.Errorf("IsNested(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestParsePart(t *testing.T) {
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
		key, idx, has, err := recordpath.ParsePart(tt.part)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParsePart(%q) error = %v, wantErr %v", tt.part, err, tt.wantErr)
			continue
		}
		if err != nil {
			continue
		}
		if key != tt.wantKey || idx != tt.wantIdx || has != tt.wantHas {
			t.Errorf("ParsePart(%q) = (%q, %d, %v), want (%q, %d, %v)",
				tt.part, key, idx, has, tt.wantKey, tt.wantIdx, tt.wantHas)
		}
	}
}
