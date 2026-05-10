package filter

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestSet_AcceptsExplicitNullValue locks Story 24.10 AC5: value:null is
// preserved as a real Go nil, not treated as "absent".
func TestSet_AcceptsExplicitNullValue(t *testing.T) {
	var cfg SetConfig
	if err := json.Unmarshal([]byte(`{"target":"x","value":null}`), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !cfg.HasValue {
		t.Fatal("HasValue should be true for explicit null")
	}
	mod, err := NewSetFromConfig(cfg)
	if err != nil {
		t.Fatalf("NewSetFromConfig: %v", err)
	}
	out, err := mod.Process(context.Background(), []map[string]any{{"x": "before"}})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	v, ok := out[0]["x"]
	if !ok {
		t.Fatal("target key missing after set")
	}
	if v != nil {
		t.Fatalf("expected nil, got %v", v)
	}
}

// TestSet_RejectsAbsentValue locks Story 24.10 AC6: missing value at runtime
// must surface a clear error, even when the schema layer is bypassed.
func TestSet_RejectsAbsentValue(t *testing.T) {
	_, err := NewSetFromConfig(SetConfig{Target: "x"})
	if err == nil || !strings.Contains(err.Error(), "'value' is required") {
		t.Fatalf("want 'value' is required error, got %v", err)
	}
}

// TestSet_PreservesStringNullLiteral locks Story 24.10 AC7: value:"null"
// remains the literal string, not converted to nil.
func TestSet_PreservesStringNullLiteral(t *testing.T) {
	var cfg SetConfig
	if err := json.Unmarshal([]byte(`{"target":"x","value":"null"}`), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	mod, err := NewSetFromConfig(cfg)
	if err != nil {
		t.Fatalf("NewSetFromConfig: %v", err)
	}
	out, err := mod.Process(context.Background(), []map[string]any{{}})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if got := out[0]["x"]; got != "null" {
		t.Fatalf("expected literal string \"null\", got %v (%T)", got, got)
	}
}

// TestRemove_AcceptsTargetString locks Story 24.10 AC8.
func TestRemove_AcceptsTargetString(t *testing.T) {
	var cfg RemoveConfig
	if err := json.Unmarshal([]byte(`{"target":"x"}`), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	mod, err := NewRemoveFromConfig(cfg)
	if err != nil {
		t.Fatalf("NewRemoveFromConfig: %v", err)
	}
	out, err := mod.Process(context.Background(), []map[string]any{{"x": 1, "y": 2}})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if _, present := out[0]["x"]; present {
		t.Fatal("expected x removed")
	}
	if out[0]["y"] != 2 {
		t.Fatal("expected y preserved")
	}
}

// TestRemove_AcceptsTargetList locks Story 24.10 AC9.
func TestRemove_AcceptsTargetList(t *testing.T) {
	var cfg RemoveConfig
	if err := json.Unmarshal([]byte(`{"target":["a","b.c"]}`), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	mod, err := NewRemoveFromConfig(cfg)
	if err != nil {
		t.Fatalf("NewRemoveFromConfig: %v", err)
	}
	out, err := mod.Process(context.Background(), []map[string]any{
		{"a": 1, "b": map[string]any{"c": 2, "d": 3}, "keep": 4},
	})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if _, present := out[0]["a"]; present {
		t.Fatal("expected a removed")
	}
	inner, _ := out[0]["b"].(map[string]any)
	if _, present := inner["c"]; present {
		t.Fatal("expected b.c removed")
	}
	if inner["d"] != 3 || out[0]["keep"] != 4 {
		t.Fatal("expected unrelated fields preserved")
	}
}

// TestRemove_RejectsInvalidTargetTypes locks Story 24.10 AC10 (programmatic).
func TestRemove_RejectsInvalidTargetTypes(t *testing.T) {
	cases := []string{
		`{}`,
		`{"target":""}`,
		`{"target":[]}`,
		`{"target":["",""]}`,
		`{"target":123}`,
	}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			var cfg RemoveConfig
			if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
				return // unmarshal-level rejection is acceptable
			}
			if _, err := NewRemoveFromConfig(cfg); err == nil {
				t.Fatalf("expected error for %s", raw)
			}
		})
	}
}

// TestScript_RejectsWhitespaceOnly locks Story 24.10 AC4 (runtime).
func TestScript_RejectsWhitespaceOnly(t *testing.T) {
	_, err := NewScriptFromConfig(ScriptConfig{Script: "   \n\t  "})
	if err == nil {
		t.Fatal("expected error for whitespace-only script")
	}
}

// TestScript_RejectsMissingTransform locks Story 24.10 AC4 (runtime).
func TestScript_RejectsMissingTransform(t *testing.T) {
	_, err := NewScriptFromConfig(ScriptConfig{Script: "var x = 1;"})
	if err == nil {
		t.Fatal("expected error for missing transform()")
	}
}
