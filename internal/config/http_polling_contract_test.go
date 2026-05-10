package config_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/cannectors/runtime/internal/config"
)

// pipelineWithHTTPPolling wraps an input fragment into a minimal valid pipeline.
func pipelineWithHTTPPolling(inputJSON string) string {
	return fmt.Sprintf(
		`{"name":"x","version":"1.0.0","input":%s,"output":{"type":"httpRequest","endpoint":"https://e.com","method":"POST"}}`,
		inputJSON,
	)
}

// TestSchemaAcceptsHTTPPollingWithoutSchedule locks Story 24.6 AC1: schedule
// is optional on httpPolling (one-shot when omitted).
func TestSchemaAcceptsHTTPPollingWithoutSchedule(t *testing.T) {
	input := `{"type":"httpPolling","endpoint":"https://e.com","method":"GET"}`
	var data map[string]any
	if err := json.Unmarshal([]byte(pipelineWithHTTPPolling(input)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	res := config.ValidateConfig(data)
	if !res.Valid {
		t.Errorf("expected valid pipeline without schedule, got errors: %v", res.Errors)
	}
}

// TestSchemaAcceptsHTTPPollingWithSchedule locks Story 24.6 AC2: schedule is
// still accepted when provided.
func TestSchemaAcceptsHTTPPollingWithSchedule(t *testing.T) {
	input := `{"type":"httpPolling","endpoint":"https://e.com","method":"GET","schedule":"* * * * * *"}`
	var data map[string]any
	if err := json.Unmarshal([]byte(pipelineWithHTTPPolling(input)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	res := config.ValidateConfig(data)
	if !res.Valid {
		t.Errorf("expected valid pipeline with schedule, got errors: %v", res.Errors)
	}
}

// TestSchemaHTTPPaginationCanonicalParam locks Story 24.6 AC6/AC8: `param` is the
// canonical field; legacy `cursorParam`/`pageParam`/`offsetParam` are rejected.
func TestSchemaHTTPPaginationCanonicalParam(t *testing.T) {
	cases := []struct {
		name      string
		pagJSON   string
		wantValid bool
	}{
		{"cursor with param", `{"type":"cursor","param":"after","nextCursorField":"next"}`, true},
		{"page with param", `{"type":"page","param":"page","totalPagesField":"total"}`, true},
		{"offset with param", `{"type":"offset","param":"offset","limitParam":"limit","totalField":"total"}`, true},
		{"cursor legacy cursorParam", `{"type":"cursor","cursorParam":"after","nextCursorField":"next"}`, false},
		{"page legacy pageParam", `{"type":"page","pageParam":"page","totalPagesField":"total"}`, false},
		{"offset legacy offsetParam", `{"type":"offset","offsetParam":"offset","limitParam":"limit","totalField":"total"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := fmt.Sprintf(
				`{"type":"httpPolling","endpoint":"https://e.com","method":"GET","pagination":%s}`,
				tc.pagJSON,
			)
			var data map[string]any
			if err := json.Unmarshal([]byte(pipelineWithHTTPPolling(input)), &data); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			res := config.ValidateConfig(data)
			if res.Valid != tc.wantValid {
				t.Errorf("got valid=%v want %v (errors: %v)", res.Valid, tc.wantValid, res.Errors)
			}
		})
	}
}

// TestSchemaPaginationLimit locks Story 24.6 AC4/AC5: limit can be omitted
// (runtime applies its default) but limit:0 must be rejected.
func TestSchemaPaginationLimit(t *testing.T) {
	cases := []struct {
		name      string
		pagJSON   string
		wantValid bool
	}{
		{"limit omitted", `{"type":"offset","param":"offset","limitParam":"limit","totalField":"total"}`, true},
		{"limit positive", `{"type":"offset","param":"offset","limitParam":"limit","limit":50,"totalField":"total"}`, true},
		{"limit zero", `{"type":"offset","param":"offset","limitParam":"limit","limit":0,"totalField":"total"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := fmt.Sprintf(
				`{"type":"httpPolling","endpoint":"https://e.com","method":"GET","pagination":%s}`,
				tc.pagJSON,
			)
			var data map[string]any
			if err := json.Unmarshal([]byte(pipelineWithHTTPPolling(input)), &data); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			res := config.ValidateConfig(data)
			if res.Valid != tc.wantValid {
				t.Errorf("got valid=%v want %v (errors: %v)", res.Valid, tc.wantValid, res.Errors)
			}
		})
	}
}
