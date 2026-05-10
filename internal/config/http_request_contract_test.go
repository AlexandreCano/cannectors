package config_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/cannectors/runtime/internal/config"
)

func pipelineWithOutput(outputJSON string) string {
	return fmt.Sprintf(
		`{"name":"x","version":"1.0.0","input":{"type":"webhook","path":"/in"},"filters":[],"output":%s}`,
		outputJSON,
	)
}

func validateOutput(t *testing.T, outputJSON string) bool {
	t.Helper()
	var data map[string]any
	if err := json.Unmarshal([]byte(pipelineWithOutput(outputJSON)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return config.ValidateConfig(data).Valid
}

// TestSchemaHTTPRequestMethodOptional locks Story 24.12 AC1: method is
// optional at the schema level (the runtime defaults it to POST).
func TestSchemaHTTPRequestMethodOptional(t *testing.T) {
	if !validateOutput(t, `{"type":"httpRequest","endpoint":"https://e/x"}`) {
		t.Fatal("method should be optional in the schema")
	}
}

// TestSchemaHTTPRequestSuccessLangRejected locks Story 24.12 AC6.
func TestSchemaHTTPRequestSuccessLangRejected(t *testing.T) {
	cases := []struct {
		name      string
		output    string
		wantValid bool
	}{
		{
			"lang rejected",
			`{"type":"httpRequest","endpoint":"https://e/x","success":{"lang":"cel","expression":"statusCode == 200"}}`,
			false,
		},
		{
			"expression alone ok",
			`{"type":"httpRequest","endpoint":"https://e/x","success":{"expression":"statusCode == 200"}}`,
			true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validateOutput(t, tc.output); got != tc.wantValid {
				t.Fatalf("valid=%v want %v (output=%s)", got, tc.wantValid, tc.output)
			}
		})
	}
}

// TestSchemaHTTPRequestSuccessStatusCodes locks Story 24.12 AC14.
func TestSchemaHTTPRequestSuccessStatusCodes(t *testing.T) {
	cases := []struct {
		name      string
		success   string
		wantValid bool
	}{
		{"valid list", `{"statusCodes":[200,201]}`, true},
		{"empty list rejected", `{"statusCodes":[]}`, false},
		{"duplicates rejected", `{"statusCodes":[200,200]}`, false},
		{"out-of-range low rejected", `{"statusCodes":[99]}`, false},
		{"out-of-range high rejected", `{"statusCodes":[600]}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := fmt.Sprintf(`{"type":"httpRequest","endpoint":"https://e/x","success":%s}`, tc.success)
			if got := validateOutput(t, out); got != tc.wantValid {
				t.Fatalf("valid=%v want %v (success=%s)", got, tc.wantValid, tc.success)
			}
		})
	}
}

// TestSchemaHTTPRequestSuccessRequiresAtLeastOne locks the schema rule that
// `success` must declare at least one of `expression` or `statusCodes`.
func TestSchemaHTTPRequestSuccessRequiresAtLeastOne(t *testing.T) {
	if validateOutput(t, `{"type":"httpRequest","endpoint":"https://e/x","success":{}}`) {
		t.Fatal("empty success object should be rejected")
	}
}

// TestSchemaHTTPRequestSuccessExpressionMinLength locks Story 24.12 AC7
// (`expression` must not be empty when present).
func TestSchemaHTTPRequestSuccessExpressionMinLength(t *testing.T) {
	if validateOutput(t, `{"type":"httpRequest","endpoint":"https://e/x","success":{"expression":""}}`) {
		t.Fatal("empty expression should be rejected")
	}
}

// TestSchemaHTTPRequestRequestModeEnum locks Story 24.12 AC15.
func TestSchemaHTTPRequestRequestModeEnum(t *testing.T) {
	cases := []struct {
		name      string
		mode      string
		wantValid bool
	}{
		{"batch ok", "batch", true},
		{"single ok", "single", true},
		{"unknown rejected", "stream", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := fmt.Sprintf(`{"type":"httpRequest","endpoint":"https://e/x","requestMode":%q}`, tc.mode)
			if got := validateOutput(t, out); got != tc.wantValid {
				t.Fatalf("valid=%v want %v", got, tc.wantValid)
			}
		})
	}
}

// TestSchemaHTTPRequestKeysStrict locks Story 24.12 AC16.
func TestSchemaHTTPRequestKeysStrict(t *testing.T) {
	cases := []struct {
		name      string
		keys      string
		wantValid bool
	}{
		{"valid", `[{"field":"id","paramType":"query","paramName":"id"}]`, true},
		{"empty field", `[{"field":"","paramType":"query","paramName":"id"}]`, false},
		{"empty paramName", `[{"field":"id","paramType":"query","paramName":""}]`, false},
		{"invalid paramType", `[{"field":"id","paramType":"body","paramName":"id"}]`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := fmt.Sprintf(`{"type":"httpRequest","endpoint":"https://e/x","keys":%s}`, tc.keys)
			if got := validateOutput(t, out); got != tc.wantValid {
				t.Fatalf("valid=%v want %v", got, tc.wantValid)
			}
		})
	}
}
