package config_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/cannectors/runtime/internal/config"
)

func pipelineWithFilter(filterJSON string) string {
	return fmt.Sprintf(
		`{"name":"x","version":"1.0.0","input":{"type":"webhook","path":"/in"},"filters":[%s],"output":{"type":"httpRequest","endpoint":"https://e.com","method":"POST"}}`,
		filterJSON,
	)
}

func validateFilter(t *testing.T, filterJSON string) bool {
	t.Helper()
	var data map[string]any
	if err := json.Unmarshal([]byte(pipelineWithFilter(filterJSON)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return config.ValidateConfig(data).Valid
}

// TestSchemaConditionDropsLegacyFields locks Story 24.9 AC1/AC2: legacy
// fields lang/onTrue/onFalse are rejected by the schema.
func TestSchemaConditionDropsLegacyFields(t *testing.T) {
	cases := []struct {
		name      string
		filter    string
		wantValid bool
	}{
		{"plain condition ok", `{"type":"condition","expression":"x > 0"}`, true},
		{"with then/else ok", `{"type":"condition","expression":"x > 0","then":[{"type":"drop"}],"else":[]}`, true},
		{"lang rejected", `{"type":"condition","expression":"x > 0","lang":"simple"}`, false},
		{"onTrue rejected", `{"type":"condition","expression":"x > 0","onTrue":"continue"}`, false},
		{"onFalse rejected", `{"type":"condition","expression":"x > 0","onFalse":"skip"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validateFilter(t, tc.filter); got != tc.wantValid {
				t.Errorf("valid=%v want %v", got, tc.wantValid)
			}
		})
	}
}

// TestSchemaConditionExpression locks Story 24.9 AC3: expression is required
// and must not be empty.
func TestSchemaConditionExpression(t *testing.T) {
	cases := []struct {
		name      string
		filter    string
		wantValid bool
	}{
		{"missing expression rejected", `{"type":"condition"}`, false},
		{"empty expression rejected", `{"type":"condition","expression":""}`, false},
		{"non-empty expression accepted", `{"type":"condition","expression":"true"}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validateFilter(t, tc.filter); got != tc.wantValid {
				t.Errorf("valid=%v want %v", got, tc.wantValid)
			}
		})
	}
}

// TestSchemaDropFilterAccepted locks Story 24.9 AC8: the drop filter type is
// recognized by the schema (no extra config required).
func TestSchemaDropFilterAccepted(t *testing.T) {
	if !validateFilter(t, `{"type":"drop"}`) {
		t.Errorf("expected drop filter to be valid")
	}
}

// TestSchemaConditionAllowsArbitraryNesting locks Story 24.9 AC9: the schema
// imposes no artificial nesting depth on conditions.
func TestSchemaConditionAllowsArbitraryNesting(t *testing.T) {
	// Build a condition tree of depth 60 (well past the previously-hardcoded 50)
	leaf := `{"type":"drop"}`
	for i := 0; i < 60; i++ {
		leaf = fmt.Sprintf(`{"type":"condition","expression":"x > %d","then":[%s]}`, i, leaf)
	}
	if !validateFilter(t, leaf) {
		t.Errorf("expected 60-deep condition tree to be valid")
	}
}
