package config_test

import (
	"testing"
)

// TestSchemaScript_RequiresExactlyOneSource locks Story 24.10 AC1/AC2/AC3:
// script and scriptFile are mutually exclusive, exactly one must be set,
// and empty strings are rejected.
func TestSchemaScript_RequiresExactlyOneSource(t *testing.T) {
	cases := []struct {
		name      string
		filter    string
		wantValid bool
	}{
		{"script only ok", `{"type":"script","script":"function transform(r){return r;}"}`, true},
		{"scriptFile only ok", `{"type":"script","scriptFile":"./t.js"}`, true},
		{"both rejected", `{"type":"script","script":"x","scriptFile":"./t.js"}`, false},
		{"neither rejected", `{"type":"script"}`, false},
		{"empty script rejected", `{"type":"script","script":""}`, false},
		{"empty scriptFile rejected", `{"type":"script","scriptFile":""}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validateFilter(t, tc.filter); got != tc.wantValid {
				t.Fatalf("validateFilter(%s) = %v, want %v", tc.filter, got, tc.wantValid)
			}
		})
	}
}

// TestSchemaSet_RequiresTargetAndValue locks Story 24.10 AC5/AC6/AC7: target
// and value are required, value:null is accepted, missing value is rejected.
func TestSchemaSet_RequiresTargetAndValue(t *testing.T) {
	cases := []struct {
		name      string
		filter    string
		wantValid bool
	}{
		{"target+value ok", `{"type":"set","target":"x","value":"y"}`, true},
		{"value null ok", `{"type":"set","target":"x","value":null}`, true},
		{"value string null literal ok", `{"type":"set","target":"x","value":"null"}`, true},
		{"missing value rejected", `{"type":"set","target":"x"}`, false},
		{"missing target rejected", `{"type":"set","value":"y"}`, false},
		{"empty target rejected", `{"type":"set","target":"","value":"y"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validateFilter(t, tc.filter); got != tc.wantValid {
				t.Fatalf("validateFilter(%s) = %v, want %v", tc.filter, got, tc.wantValid)
			}
		})
	}
}

// TestSchemaRemove_AcceptsTargetStringOrList locks Story 24.10 AC8/AC9/AC10:
// `target` is required, accepts string or non-empty list of non-empty strings,
// and the legacy `targets` field is rejected.
func TestSchemaRemove_AcceptsTargetStringOrList(t *testing.T) {
	cases := []struct {
		name      string
		filter    string
		wantValid bool
	}{
		{"single target string ok", `{"type":"remove","target":"x"}`, true},
		{"target list ok", `{"type":"remove","target":["a","b.c"]}`, true},
		{"missing target rejected", `{"type":"remove"}`, false},
		{"empty target string rejected", `{"type":"remove","target":""}`, false},
		{"empty target list rejected", `{"type":"remove","target":[]}`, false},
		{"target list with empty rejected", `{"type":"remove","target":["a",""]}`, false},
		{"legacy targets rejected", `{"type":"remove","targets":["a","b"]}`, false},
		{"legacy targets with target rejected", `{"type":"remove","target":"a","targets":["b"]}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validateFilter(t, tc.filter); got != tc.wantValid {
				t.Fatalf("validateFilter(%s) = %v, want %v", tc.filter, got, tc.wantValid)
			}
		})
	}
}
