package config_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/cannectors/runtime/internal/config"
)

func pipelineWithMapping(filterJSON string) string {
	return fmt.Sprintf(
		`{"name":"x","version":"1.0.0","input":{"type":"webhook","path":"/in"},"filters":[%s],"output":{"type":"httpRequest","endpoint":"https://e.com","method":"POST"}}`,
		filterJSON,
	)
}

func validateMapping(t *testing.T, filterJSON string) bool {
	t.Helper()
	var data map[string]any
	if err := json.Unmarshal([]byte(pipelineWithMapping(filterJSON)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return config.ValidateConfig(data).Valid
}

// TestSchemaMappingTargetMinLength locks Story 24.8 AC1: target must be a
// non-empty string.
func TestSchemaMappingTargetMinLength(t *testing.T) {
	cases := []struct {
		name      string
		mapping   string
		wantValid bool
	}{
		{"target ok", `{"source":"a","target":"b"}`, true},
		{"target empty rejected", `{"source":"a","target":""}`, false},
		{"target missing rejected", `{"source":"a"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := fmt.Sprintf(`{"type":"mapping","mappings":[%s]}`, tc.mapping)
			if got := validateMapping(t, f); got != tc.wantValid {
				t.Errorf("valid=%v want %v", got, tc.wantValid)
			}
		})
	}
}

// TestSchemaMappingOnMissingUseDefault locks Story 24.8 AC4: onMissing
// useDefault requires defaultValue.
func TestSchemaMappingOnMissingUseDefault(t *testing.T) {
	cases := []struct {
		name      string
		mapping   string
		wantValid bool
	}{
		{"useDefault with defaultValue", `{"source":"a","target":"b","onMissing":"useDefault","defaultValue":"x"}`, true},
		{"useDefault without defaultValue rejected", `{"source":"a","target":"b","onMissing":"useDefault"}`, false},
		{"setNull without defaultValue ok", `{"source":"a","target":"b","onMissing":"setNull"}`, true},
		{"unknown onMissing rejected", `{"source":"a","target":"b","onMissing":"bogus"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := fmt.Sprintf(`{"type":"mapping","mappings":[%s]}`, tc.mapping)
			if got := validateMapping(t, f); got != tc.wantValid {
				t.Errorf("valid=%v want %v", got, tc.wantValid)
			}
		})
	}
}

// TestSchemaMappingTransformOpEnum locks Story 24.8 AC5/AC8: transform.op
// is a strict enum of supported operations.
func TestSchemaMappingTransformOpEnum(t *testing.T) {
	supported := []string{"trim", "lowercase", "uppercase", "dateFormat", "replace", "split", "join", "toString", "toInt", "toFloat", "toBool", "toArray", "toObject"}
	for _, op := range supported {
		t.Run("accepts "+op, func(t *testing.T) {
			extra := ""
			if op == "replace" {
				extra = `,"pattern":"x"`
			}
			f := fmt.Sprintf(`{"type":"mapping","mappings":[{"source":"a","target":"b","transforms":[{"op":%q%s}]}]}`, op, extra)
			if !validateMapping(t, f) {
				t.Errorf("expected %q to be valid", op)
			}
		})
	}
	t.Run("rejects unknown op", func(t *testing.T) {
		f := `{"type":"mapping","mappings":[{"source":"a","target":"b","transforms":[{"op":"bogus"}]}]}`
		if validateMapping(t, f) {
			t.Errorf("expected unknown op to be rejected")
		}
	})
}

// TestSchemaMappingTransformReplaceRequiresPattern locks Story 24.8 AC6:
// op:replace requires a non-empty pattern.
func TestSchemaMappingTransformReplaceRequiresPattern(t *testing.T) {
	cases := []struct {
		name      string
		transform string
		wantValid bool
	}{
		{"replace with pattern", `{"op":"replace","pattern":"x","replacement":"y"}`, true},
		{"replace without pattern rejected", `{"op":"replace","replacement":"y"}`, false},
		{"replace with empty pattern rejected", `{"op":"replace","pattern":"","replacement":"y"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := fmt.Sprintf(`{"type":"mapping","mappings":[{"source":"a","target":"b","transforms":[%s]}]}`, tc.transform)
			if got := validateMapping(t, f); got != tc.wantValid {
				t.Errorf("valid=%v want %v", got, tc.wantValid)
			}
		})
	}
}
