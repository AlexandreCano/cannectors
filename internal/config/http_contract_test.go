package config_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/cannectors/runtime/internal/config"
)

// TestSchemaAcceptsAnyHTTPMethod locks Story 24.3 AC1: any RFC 7230 token is
// accepted as an HTTP method (GET, DELETE, HEAD, OPTIONS, custom verbs…).
// Non-token values must be rejected.
func TestSchemaAcceptsAnyHTTPMethod(t *testing.T) {
	cases := []struct {
		method    string
		wantValid bool
	}{
		{"GET", true},
		{"POST", true},
		{"DELETE", true},
		{"PATCH", true},
		{"HEAD", true},
		{"OPTIONS", true},
		{"PURGE", true},
		{"X-CUSTOM", true},
		{"", false},
		{"BAD METHOD", false},
		{"GET\r\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			raw := fmt.Sprintf(
				`{"name":"x","version":"1.0.0","input":{"type":"httpPolling","endpoint":"https://e.com","schedule":"* * * * * *","method":%q},"output":{"type":"httpRequest","endpoint":"https://e.com","method":"POST"}}`,
				tc.method,
			)
			var data map[string]any
			if err := json.Unmarshal([]byte(raw), &data); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			res := config.ValidateConfig(data)
			if res.Valid != tc.wantValid {
				t.Errorf("schema validation for method=%q: got valid=%v, want %v (errors: %v)",
					tc.method, res.Valid, tc.wantValid, res.Errors)
			}
		})
	}
}

// TestSchemaAcceptsBodyOnAnyMethod locks Story 24.3 AC2/AC3: a request body
// (inline string) is accepted regardless of the HTTP method, including on
// httpPolling.
func TestSchemaAcceptsBodyOnAnyMethod(t *testing.T) {
	cases := []struct {
		method string
		body   string
	}{
		{"GET", `{"q":"x"}`},
		{"DELETE", `{"reason":"x"}`},
		{"POST", `{"x":1}`},
	}
	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			raw := fmt.Sprintf(
				`{"name":"x","version":"1.0.0","input":{"type":"httpPolling","endpoint":"https://e.com","schedule":"* * * * * *","method":%q,"body":%q},"output":{"type":"httpRequest","endpoint":"https://e.com","method":"POST"}}`,
				tc.method, tc.body,
			)
			var data map[string]any
			if err := json.Unmarshal([]byte(raw), &data); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			res := config.ValidateConfig(data)
			if !res.Valid {
				t.Errorf("expected valid for method=%q with body, got errors: %v", tc.method, res.Errors)
			}
		})
	}
}

// TestSchemaAcceptsTemplatedEndpoint locks Story 24.3 AC4: format:uri is
// removed so endpoints with template placeholders validate at the schema
// layer (the final URL is checked at runtime).
func TestSchemaAcceptsTemplatedEndpoint(t *testing.T) {
	raw := `{"name":"x","version":"1.0.0","input":{"type":"httpPolling","endpoint":"https://e.com/{{record.id}}","schedule":"* * * * * *"},"output":{"type":"httpRequest","endpoint":"https://e.com","method":"POST"}}`
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	res := config.ValidateConfig(data)
	if !res.Valid {
		t.Errorf("expected valid for templated endpoint, got: %v", res.Errors)
	}
}

// TestSchemaTimeoutMsMinimum locks Story 24.3 AC7: timeoutMs must be >= 1.
// Omitting timeoutMs is allowed (AC8): the runtime fills in its default.
func TestSchemaTimeoutMsMinimum(t *testing.T) {
	cases := []struct {
		name      string
		fragment  string
		wantValid bool
	}{
		{"omitted", `"endpoint":"https://e.com"`, true},
		{"one", `"endpoint":"https://e.com","timeoutMs":1`, true},
		{"thirty thousand", `"endpoint":"https://e.com","timeoutMs":30000`, true},
		{"zero", `"endpoint":"https://e.com","timeoutMs":0`, false},
		{"negative", `"endpoint":"https://e.com","timeoutMs":-1`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := fmt.Sprintf(
				`{"name":"x","version":"1.0.0","input":{"type":"httpPolling",%s,"schedule":"* * * * * *"},"output":{"type":"httpRequest","endpoint":"https://e.com","method":"POST"}}`,
				tc.fragment,
			)
			var data map[string]any
			if err := json.Unmarshal([]byte(raw), &data); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			res := config.ValidateConfig(data)
			if res.Valid != tc.wantValid {
				t.Errorf("got valid=%v, want %v (errors: %v)", res.Valid, tc.wantValid, res.Errors)
			}
		})
	}
}

// TestSchemaRejectsEmptyHTTPCallKey locks Story 24.3 AC11: httpCallKeyConfig
// `field` and `paramName` cannot be empty strings.
func TestSchemaRejectsEmptyHTTPCallKey(t *testing.T) {
	cases := []struct {
		name      string
		field     string
		paramName string
		wantValid bool
	}{
		{"both set", "id", "id", true},
		{"empty field", "", "id", false},
		{"empty paramName", "id", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := fmt.Sprintf(
				`{"name":"x","version":"1.0.0","input":{"type":"webhook","path":"/in"},"filters":[{"type":"http_call","endpoint":"https://e.com/x","keys":[{"field":%q,"paramType":"query","paramName":%q}]}],"output":{"type":"httpRequest","endpoint":"https://e.com","method":"POST"}}`,
				tc.field, tc.paramName,
			)
			var data map[string]any
			if err := json.Unmarshal([]byte(raw), &data); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			res := config.ValidateConfig(data)
			if res.Valid != tc.wantValid {
				t.Errorf("got valid=%v, want %v (errors: %v)", res.Valid, tc.wantValid, res.Errors)
			}
		})
	}
}

// TestSchemaAcceptsRetryOnHTTPModules locks Story 24.3 AC9: retry config is
// accepted on httpPolling, http_call, and httpRequest.
func TestSchemaAcceptsRetryOnHTTPModules(t *testing.T) {
	retryFragment := `"retry":{"maxAttempts":3,"delayMs":100}`
	cases := []struct {
		name string
		raw  string
	}{
		{
			"httpPolling",
			fmt.Sprintf(`{"name":"x","version":"1.0.0","input":{"type":"httpPolling","endpoint":"https://e.com","schedule":"* * * * * *",%s},"output":{"type":"httpRequest","endpoint":"https://e.com","method":"POST"}}`, retryFragment),
		},
		{
			"http_call",
			fmt.Sprintf(`{"name":"x","version":"1.0.0","input":{"type":"webhook","path":"/in"},"filters":[{"type":"http_call","endpoint":"https://e.com/x","keys":[{"field":"id","paramType":"query","paramName":"id"}],%s}],"output":{"type":"httpRequest","endpoint":"https://e.com","method":"POST"}}`, retryFragment),
		},
		{
			"httpRequest",
			fmt.Sprintf(`{"name":"x","version":"1.0.0","input":{"type":"webhook","path":"/in"},"output":{"type":"httpRequest","endpoint":"https://e.com","method":"POST",%s}}`, retryFragment),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var data map[string]any
			if err := json.Unmarshal([]byte(tc.raw), &data); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			res := config.ValidateConfig(data)
			if !res.Valid {
				t.Errorf("expected valid retry on %s, got: %v", tc.name, res.Errors)
			}
		})
	}
}
