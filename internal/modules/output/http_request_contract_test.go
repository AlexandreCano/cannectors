package output

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cannectors/runtime/pkg/connector"
)

func makeOutputConfig(t *testing.T, m map[string]any) *connector.ModuleConfig {
	t.Helper()
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return &connector.ModuleConfig{Type: "httpRequest", Raw: raw}
}

// TestHTTPRequest_DefaultMethodIsPOST locks Story 24.12 AC1.
func TestHTTPRequest_DefaultMethodIsPOST(t *testing.T) {
	module, err := NewHTTPRequestFromConfig(makeOutputConfig(t, map[string]any{
		"endpoint": "https://e/x",
	}))
	if err != nil {
		t.Fatalf("NewHTTPRequestFromConfig: %v", err)
	}
	if module.method != "POST" {
		t.Fatalf("expected POST, got %q", module.method)
	}
}

// TestHTTPRequest_BodySentForGET locks Story 24.12 AC3 (body sent regardless of method).
func TestHTTPRequest_BodySentForGET(t *testing.T) {
	var receivedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		receivedBody = string(b)
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	module, err := NewHTTPRequestFromConfig(makeOutputConfig(t, map[string]any{
		"endpoint": server.URL,
		"method":   "GET",
		"body":     `{"hello":"world"}`,
	}))
	if err != nil {
		t.Fatalf("NewHTTPRequestFromConfig: %v", err)
	}
	if _, err := module.Send(context.Background(), []map[string]any{{"id": 1}}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if receivedBody != `{"hello":"world"}` {
		t.Fatalf("expected body sent on GET, got %q", receivedBody)
	}
}

// TestHTTPRequest_NoDefaultBodyForGET locks Story 24.12 AC5.
func TestHTTPRequest_NoDefaultBodyForGET(t *testing.T) {
	var receivedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		receivedBody = string(b)
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	module, err := NewHTTPRequestFromConfig(makeOutputConfig(t, map[string]any{
		"endpoint": server.URL,
		"method":   "GET",
	}))
	if err != nil {
		t.Fatalf("NewHTTPRequestFromConfig: %v", err)
	}
	if _, err := module.Send(context.Background(), []map[string]any{{"id": 1}}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if receivedBody != "" {
		t.Fatalf("expected empty body on GET without body config, got %q", receivedBody)
	}
}

// TestHTTPRequest_DefaultJSONBodyForPOST locks Story 24.12 AC4.
func TestHTTPRequest_DefaultJSONBodyForPOST(t *testing.T) {
	var receivedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		receivedBody = string(b)
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	module, err := NewHTTPRequestFromConfig(makeOutputConfig(t, map[string]any{
		"endpoint":    server.URL,
		"method":      "POST",
		"requestMode": "single",
	}))
	if err != nil {
		t.Fatalf("NewHTTPRequestFromConfig: %v", err)
	}
	if _, err := module.Send(context.Background(), []map[string]any{{"id": 42}}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !strings.Contains(receivedBody, `"id":42`) {
		t.Fatalf("expected default JSON body, got %q", receivedBody)
	}
}

// TestHTTPRequest_DefaultSuccessCodes locks Story 24.12 AC13.
func TestHTTPRequest_DefaultSuccessCodes(t *testing.T) {
	cases := []struct {
		status   int
		wantSent bool
	}{
		{200, false},
		{201, true},
		{202, true},
		{203, true},
		{204, true},
		{205, false},
	}
	for _, tc := range cases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
			}))
			defer server.Close()

			module, err := NewHTTPRequestFromConfig(makeOutputConfig(t, map[string]any{
				"endpoint": server.URL,
				"method":   "POST",
			}))
			if err != nil {
				t.Fatalf("NewHTTPRequestFromConfig: %v", err)
			}
			sent, err := module.Send(context.Background(), []map[string]any{{"x": 1}})
			gotSuccess := err == nil && sent == 1
			if gotSuccess != tc.wantSent {
				t.Fatalf("status %d: success=%v, want=%v (err=%v)", tc.status, gotSuccess, tc.wantSent, err)
			}
		})
	}
}

// TestHTTPRequest_SuccessExpressionAlone locks Story 24.12 AC8.
func TestHTTPRequest_SuccessExpressionAlone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	module, err := NewHTTPRequestFromConfig(makeOutputConfig(t, map[string]any{
		"endpoint": server.URL,
		"method":   "POST",
		"success":  map[string]any{"expression": "body.ok == true"},
	}))
	if err != nil {
		t.Fatalf("NewHTTPRequestFromConfig: %v", err)
	}
	sent, err := module.Send(context.Background(), []map[string]any{{"x": 1}})
	if err != nil || sent != 1 {
		t.Fatalf("expected success via expression alone, got sent=%d err=%v", sent, err)
	}
}

// TestHTTPRequest_SuccessExpressionAndStatusBothMustMatch locks AC10.
func TestHTTPRequest_SuccessExpressionAndStatusBothMustMatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":false}`))
	}))
	defer server.Close()

	module, err := NewHTTPRequestFromConfig(makeOutputConfig(t, map[string]any{
		"endpoint": server.URL,
		"method":   "POST",
		"success": map[string]any{
			"statusCodes": []int{200},
			"expression":  "body.ok == true",
		},
	}))
	if err != nil {
		t.Fatalf("NewHTTPRequestFromConfig: %v", err)
	}
	if _, err := module.Send(context.Background(), []map[string]any{{"x": 1}}); err == nil {
		t.Fatal("expected failure when expression returns false even though status matches")
	}
}

// TestHTTPRequest_SuccessExpressionInvalid locks Story 24.12 AC12.
func TestHTTPRequest_SuccessExpressionInvalid(t *testing.T) {
	_, err := NewHTTPRequestFromConfig(makeOutputConfig(t, map[string]any{
		"endpoint": "https://e/x",
		"method":   "POST",
		"success":  map[string]any{"expression": "this is not valid expr ((("},
	}))
	if err == nil || !strings.Contains(err.Error(), "success.expression") {
		t.Fatalf("expected invalid success.expression error, got %v", err)
	}
}

// TestHTTPRequest_SuccessStatusCodesValidatedAtRuntime locks AC14.
func TestHTTPRequest_SuccessStatusCodesValidatedAtRuntime(t *testing.T) {
	cases := []struct {
		name  string
		codes []int
	}{
		{"empty", []int{}},
		{"duplicate", []int{200, 200}},
		{"out of range", []int{99}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewHTTPRequestFromConfig(makeOutputConfig(t, map[string]any{
				"endpoint": "https://e/x",
				"method":   "POST",
				"success":  map[string]any{"statusCodes": tc.codes},
			}))
			if err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
		})
	}
}

// TestHTTPRequest_RequestModeUnknownRejected locks Story 24.12 AC15.
func TestHTTPRequest_RequestModeUnknownRejected(t *testing.T) {
	_, err := NewHTTPRequestFromConfig(makeOutputConfig(t, map[string]any{
		"endpoint":    "https://e/x",
		"method":      "POST",
		"requestMode": "stream",
	}))
	if err == nil || !strings.Contains(err.Error(), "requestMode") {
		t.Fatalf("expected requestMode error, got %v", err)
	}
}

// TestHTTPRequest_KeysValidatedAtConstruction locks Story 24.12 AC16.
func TestHTTPRequest_KeysValidatedAtConstruction(t *testing.T) {
	cases := []struct {
		name string
		key  map[string]any
	}{
		{"empty field", map[string]any{"field": "", "paramType": "query", "paramName": "id"}},
		{"empty paramName", map[string]any{"field": "id", "paramType": "query", "paramName": ""}},
		{"invalid paramType", map[string]any{"field": "id", "paramType": "body", "paramName": "id"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewHTTPRequestFromConfig(makeOutputConfig(t, map[string]any{
				"endpoint": "https://e/x",
				"method":   "POST",
				"keys":     []any{tc.key},
			}))
			if err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
		})
	}
}

// TestHTTPRequest_SuccessStatusCodes_Allow4xx locks Story 24.12 AC8/AC9 review fix:
// successCodes containing >=400 must override the default httpclient classifier
// instead of being short-circuited to a server error.
func TestHTTPRequest_SuccessStatusCodes_Allow4xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	module, err := NewHTTPRequestFromConfig(makeOutputConfig(t, map[string]any{
		"endpoint": srv.URL,
		"method":   "POST",
		"success": map[string]any{
			"statusCodes": []any{409},
		},
	}))
	if err != nil {
		t.Fatalf("NewHTTPRequestFromConfig: %v", err)
	}
	if _, err := module.Send(context.Background(), []map[string]any{{"id": 1}}); err != nil {
		t.Fatalf("expected 409 to be considered success, got error: %v", err)
	}
}

// TestHTTPRequest_MissingPathKey_Errors locks Story 24.12 AC16: missing/null/empty
// key fields must surface as errors.
func TestHTTPRequest_MissingPathKey_Errors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	module, err := NewHTTPRequestFromConfig(makeOutputConfig(t, map[string]any{
		"endpoint":    srv.URL + "/users/{userId}",
		"method":      "POST",
		"requestMode": "single",
		"keys": []any{
			map[string]any{"field": "id", "paramType": "path", "paramName": "userId"},
		},
	}))
	if err != nil {
		t.Fatalf("NewHTTPRequestFromConfig: %v", err)
	}

	cases := []struct {
		name   string
		record map[string]any
	}{
		{"absent", map[string]any{"name": "x"}},
		{"null", map[string]any{"id": nil}},
		{"empty", map[string]any{"id": ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := module.Send(context.Background(), []map[string]any{tc.record}); err == nil {
				t.Fatalf("expected error for %s key, got nil", tc.name)
			}
		})
	}
}

// TestHTTPRequest_SuccessExpression_BodyNotJSON locks Story 24.12 AC11 review fix:
// when expression references body and body is not valid JSON, evaluation must
// surface an error instead of silently running with body=nil.
func TestHTTPRequest_SuccessExpression_BodyNotJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`not valid json`))
	}))
	defer srv.Close()

	module, err := NewHTTPRequestFromConfig(makeOutputConfig(t, map[string]any{
		"endpoint": srv.URL,
		"method":   "POST",
		"success": map[string]any{
			"statusCodes": []any{201},
			"expression":  "body.ok == true",
		},
	}))
	if err != nil {
		t.Fatalf("NewHTTPRequestFromConfig: %v", err)
	}
	if _, err := module.Send(context.Background(), []map[string]any{{"id": 1}}); err == nil {
		t.Fatalf("expected error for body-not-JSON expression, got nil")
	}
}

// TestHTTPRequest_SuccessExpression_BodyNotJSON_NoBodyRef verifies the same
// invalid-JSON response is fine when expression doesn't reference body.
func TestHTTPRequest_SuccessExpression_BodyNotJSON_NoBodyRef(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`not valid json`))
	}))
	defer srv.Close()

	module, err := NewHTTPRequestFromConfig(makeOutputConfig(t, map[string]any{
		"endpoint": srv.URL,
		"method":   "POST",
		"success": map[string]any{
			"expression": "statusCode == 201",
		},
	}))
	if err != nil {
		t.Fatalf("NewHTTPRequestFromConfig: %v", err)
	}
	if _, err := module.Send(context.Background(), []map[string]any{{"id": 1}}); err != nil {
		t.Fatalf("expression without body ref should succeed, got error: %v", err)
	}
}
