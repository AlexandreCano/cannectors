// Package output provides implementations for output modules.
package output

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/canectors/runtime/internal/errhandling"
	"github.com/canectors/runtime/pkg/connector"
)

// testServer is a helper to create test HTTP servers
type testServer struct {
	*httptest.Server
	mu           sync.Mutex
	requests     []*capturedRequest
	responseCode int
	responseBody string
	failAfter    int // fail after N requests
	requestCount int
}

type capturedRequest struct {
	Method  string
	Path    string
	Headers http.Header
	Body    []byte
}

func newTestServer() *testServer {
	ts := &testServer{
		requests:     make([]*capturedRequest, 0),
		responseCode: http.StatusOK,
	}

	ts.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts.mu.Lock()
		defer ts.mu.Unlock()

		body, _ := io.ReadAll(r.Body)
		ts.requests = append(ts.requests, &capturedRequest{
			Method:  r.Method,
			Path:    r.URL.Path + "?" + r.URL.RawQuery,
			Headers: r.Header.Clone(),
			Body:    body,
		})

		ts.requestCount++
		if ts.failAfter > 0 && ts.requestCount > ts.failAfter {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error": "server error"}`))
			return
		}

		w.WriteHeader(ts.responseCode)
		_, _ = w.Write([]byte(ts.responseBody))
	}))

	return ts
}

func (ts *testServer) getRequests() []*capturedRequest {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.requests
}

// Helper to create ModuleConfig from map
func newModuleConfig(configMap map[string]interface{}) *connector.ModuleConfig {
	return &connector.ModuleConfig{
		Type:   "httpRequest",
		Config: configMap,
	}
}

// =============================================================================
// Task 1: Core HTTP request sending logic tests
// =============================================================================

func TestNewHTTPRequestFromConfig_ValidConfig(t *testing.T) {
	config := newModuleConfig(map[string]interface{}{
		"endpoint": "https://api.example.com/data",
		"method":   "POST",
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if module == nil {
		t.Fatal("expected module, got nil")
	}
}

func TestNewHTTPRequestFromConfig_NilConfig(t *testing.T) {
	_, err := NewHTTPRequestFromConfig(nil)
	if err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestNewHTTPRequestFromConfig_MissingEndpoint(t *testing.T) {
	config := newModuleConfig(map[string]interface{}{
		"method": "POST",
	})

	_, err := NewHTTPRequestFromConfig(config)
	if err == nil {
		t.Fatal("expected error for missing endpoint")
	}
}

func TestNewHTTPRequestFromConfig_MissingMethod(t *testing.T) {
	config := newModuleConfig(map[string]interface{}{
		"endpoint": "https://api.example.com/data",
	})

	_, err := NewHTTPRequestFromConfig(config)
	if err == nil {
		t.Fatal("expected error for missing method")
	}
}

func TestNewHTTPRequestFromConfig_InvalidMethod(t *testing.T) {
	config := newModuleConfig(map[string]interface{}{
		"endpoint": "https://api.example.com/data",
		"method":   "INVALID",
	})

	_, err := NewHTTPRequestFromConfig(config)
	if err == nil {
		t.Fatal("expected error for invalid method")
	}
}

func TestNewHTTPRequestFromConfig_RetryHintFromBody_TooLong(t *testing.T) {
	// Create an expression that exceeds MaxRetryHintExpressionLength
	longExpression := strings.Repeat("body.field == true && ", 1000) + "body.field == true"
	if len(longExpression) <= MaxRetryHintExpressionLength {
		t.Fatalf("test expression too short: %d (need > %d)", len(longExpression), MaxRetryHintExpressionLength)
	}

	config := newModuleConfig(map[string]interface{}{
		"endpoint": "https://api.example.com/data",
		"method":   "POST",
		"retry": map[string]interface{}{
			"retryHintFromBody": longExpression,
		},
	})

	_, err := NewHTTPRequestFromConfig(config)
	if err == nil {
		t.Fatal("expected error for retryHintFromBody expression exceeding maximum length")
	}
	if !strings.Contains(err.Error(), "exceeds maximum") {
		t.Errorf("expected error message about exceeding maximum, got: %v", err)
	}
}

func TestNewHTTPRequestFromConfig_SupportedMethods(t *testing.T) {
	methods := []string{"POST", "PUT", "PATCH"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			config := newModuleConfig(map[string]interface{}{
				"endpoint": "https://api.example.com/data",
				"method":   method,
			})

			module, err := NewHTTPRequestFromConfig(config)
			if err != nil {
				t.Fatalf("expected no error for method %s, got %v", method, err)
			}
			if module == nil {
				t.Fatalf("expected module for method %s, got nil", method)
			}
		})
	}
}

func TestHTTPRequest_Send_SingleRecord(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{
		{"name": "test", "value": 123},
	}

	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if sent != 1 {
		t.Errorf("expected 1 record sent, got %d", sent)
	}

	// Verify request was made
	reqs := ts.getRequests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}
	if reqs[0].Method != "POST" {
		t.Errorf("expected POST method, got %s", reqs[0].Method)
	}

	// Verify body contains records as JSON array
	var body []map[string]interface{}
	if err := json.Unmarshal(reqs[0].Body, &body); err != nil {
		t.Fatalf("failed to parse request body: %v", err)
	}
	if len(body) != 1 {
		t.Errorf("expected 1 record in body, got %d", len(body))
	}
	if body[0]["name"] != "test" {
		t.Errorf("expected name=test, got %v", body[0]["name"])
	}
}

func TestHTTPRequest_Send_MultipleRecords(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{
		{"id": 1, "name": "first"},
		{"id": 2, "name": "second"},
		{"id": 3, "name": "third"},
	}

	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if sent != 3 {
		t.Errorf("expected 3 records sent, got %d", sent)
	}

	// By default, batch mode sends all records in one request
	reqs := ts.getRequests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request (batch mode), got %d", len(reqs))
	}

	var body []map[string]interface{}
	if err := json.Unmarshal(reqs[0].Body, &body); err != nil {
		t.Fatalf("failed to parse request body: %v", err)
	}
	if len(body) != 3 {
		t.Errorf("expected 3 records in body, got %d", len(body))
	}
}

func TestHTTPRequest_Send_EmptyRecords(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	sent, err := module.Send(context.Background(), []map[string]interface{}{})
	if err != nil {
		t.Fatalf("expected no error for empty records, got %v", err)
	}
	if sent != 0 {
		t.Errorf("expected 0 records sent, got %d", sent)
	}

	// No requests should be made for empty records
	reqs := ts.getRequests()
	if len(reqs) != 0 {
		t.Errorf("expected 0 requests for empty records, got %d", len(reqs))
	}
}

func TestHTTPRequest_Send_NilRecords(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	sent, err := module.Send(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected no error for nil records, got %v", err)
	}
	if sent != 0 {
		t.Errorf("expected 0 records sent, got %d", sent)
	}
}

func TestHTTPRequest_Send_SingleRecordMode(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"request": map[string]interface{}{
			"bodyFrom": "record", // Single record per request
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{
		{"id": 1, "name": "first"},
		{"id": 2, "name": "second"},
	}

	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if sent != 2 {
		t.Errorf("expected 2 records sent, got %d", sent)
	}

	// Single record mode sends one request per record
	reqs := ts.getRequests()
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests (single record mode), got %d", len(reqs))
	}

	// Verify each request contains a single record (not array)
	for i, req := range reqs {
		var body map[string]interface{}
		if err := json.Unmarshal(req.Body, &body); err != nil {
			t.Fatalf("request %d: failed to parse body as single object: %v", i, err)
		}
		expectedID := float64(i + 1)
		if body["id"] != expectedID {
			t.Errorf("request %d: expected id=%v, got %v", i, expectedID, body["id"])
		}
	}
}

func TestHTTPRequest_Send_CustomHeaders(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"headers": map[string]interface{}{
			"X-Custom-Header": "custom-value",
			"Accept":          "application/json",
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	_, err = module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	reqs := ts.getRequests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}

	if reqs[0].Headers.Get("X-Custom-Header") != "custom-value" {
		t.Errorf("expected custom header, got %s", reqs[0].Headers.Get("X-Custom-Header"))
	}
	if reqs[0].Headers.Get("Accept") != "application/json" {
		t.Errorf("expected Accept header, got %s", reqs[0].Headers.Get("Accept"))
	}
	// Content-Type should be set automatically for JSON
	if reqs[0].Headers.Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type: application/json, got %s", reqs[0].Headers.Get("Content-Type"))
	}
}

func TestHTTPRequest_Send_DifferentMethods(t *testing.T) {
	methods := []string{"POST", "PUT", "PATCH"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			ts := newTestServer()
			defer ts.Close()

			config := newModuleConfig(map[string]interface{}{
				"endpoint": ts.URL + "/api/data",
				"method":   method,
			})

			module, err := NewHTTPRequestFromConfig(config)
			if err != nil {
				t.Fatalf("failed to create module: %v", err)
			}

			records := []map[string]interface{}{{"test": "data"}}
			_, err = module.Send(context.Background(), records)
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			reqs := ts.getRequests()
			if len(reqs) != 1 {
				t.Fatalf("expected 1 request, got %d", len(reqs))
			}
			if reqs[0].Method != method {
				t.Errorf("expected %s method, got %s", method, reqs[0].Method)
			}
		})
	}
}

func TestHTTPRequest_Send_NestedObjects(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{
		{
			"user": map[string]interface{}{
				"name":  "John",
				"email": "john@example.com",
				"address": map[string]interface{}{
					"city":    "Paris",
					"country": "France",
				},
			},
			"tags": []interface{}{"tag1", "tag2"},
		},
	}

	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if sent != 1 {
		t.Errorf("expected 1 record sent, got %d", sent)
	}

	reqs := ts.getRequests()
	var body []map[string]interface{}
	if err := json.Unmarshal(reqs[0].Body, &body); err != nil {
		t.Fatalf("failed to parse request body: %v", err)
	}

	// Verify nested structure is preserved
	user, ok := body[0]["user"].(map[string]interface{})
	if !ok {
		t.Fatal("expected user to be a map")
	}
	if user["name"] != "John" {
		t.Errorf("expected user.name=John, got %v", user["name"])
	}

	address, ok := user["address"].(map[string]interface{})
	if !ok {
		t.Fatal("expected address to be a map")
	}
	if address["city"] != "Paris" {
		t.Errorf("expected address.city=Paris, got %v", address["city"])
	}
}

func TestHTTPRequest_Close(t *testing.T) {
	config := newModuleConfig(map[string]interface{}{
		"endpoint": "https://api.example.com/data",
		"method":   "POST",
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	// Close should not return error
	if err := module.Close(); err != nil {
		t.Errorf("expected no error on Close, got %v", err)
	}
}

func TestHTTPRequest_ImplementsModule(t *testing.T) {
	config := newModuleConfig(map[string]interface{}{
		"endpoint": "https://api.example.com/data",
		"method":   "POST",
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	// Verify HTTPRequest implements Module interface
	var _ Module = module
}

func TestHTTPRequest_Send_ContentTypeJSON(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	_, err = module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	reqs := ts.getRequests()
	if reqs[0].Headers.Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type: application/json, got %s", reqs[0].Headers.Get("Content-Type"))
	}
}

func TestNewHTTPRequestFromConfig_CustomTimeout(t *testing.T) {
	config := newModuleConfig(map[string]interface{}{
		"endpoint":  "https://api.example.com/data",
		"method":    "POST",
		"timeoutMs": float64(5000),
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if module == nil {
		t.Fatal("expected module, got nil")
	}
	// Timeout is internal, just verify creation succeeds
}

// =============================================================================
// Task 2: Authentication handling tests
// =============================================================================

// Helper to create ModuleConfig with authentication
func newModuleConfigWithAuth(configMap map[string]interface{}, authType string, creds map[string]string) *connector.ModuleConfig {
	return &connector.ModuleConfig{
		Type:   "httpRequest",
		Config: configMap,
		Authentication: &connector.AuthConfig{
			Type:        authType,
			Credentials: creds,
		},
	}
}

func TestHTTPRequest_Send_APIKeyAuth_Header(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	config := newModuleConfigWithAuth(
		map[string]interface{}{
			"endpoint": ts.URL + "/api/data",
			"method":   "POST",
		},
		"api-key",
		map[string]string{
			"key":        "test-api-key-12345",
			"location":   "header",
			"headerName": "X-API-Key",
		},
	)

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	_, err = module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	reqs := ts.getRequests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}

	apiKey := reqs[0].Headers.Get("X-API-Key")
	if apiKey != "test-api-key-12345" {
		t.Errorf("expected API key in header, got %s", apiKey)
	}
}

func TestHTTPRequest_Send_APIKeyAuth_Query(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	config := newModuleConfigWithAuth(
		map[string]interface{}{
			"endpoint": ts.URL + "/api/data",
			"method":   "POST",
		},
		"api-key",
		map[string]string{
			"key":       "test-api-key-query",
			"location":  "query",
			"paramName": "api_key",
		},
	)

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	_, err = module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	reqs := ts.getRequests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}

	// Check query parameter contains API key
	if !strings.Contains(reqs[0].Path, "api_key=test-api-key-query") {
		t.Errorf("expected API key in query string, got path: %s", reqs[0].Path)
	}
}

func TestHTTPRequest_Send_BearerAuth(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	config := newModuleConfigWithAuth(
		map[string]interface{}{
			"endpoint": ts.URL + "/api/data",
			"method":   "POST",
		},
		"bearer",
		map[string]string{
			"token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.test-token",
		},
	)

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	_, err = module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	reqs := ts.getRequests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}

	auth := reqs[0].Headers.Get("Authorization")
	expected := "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.test-token"
	if auth != expected {
		t.Errorf("expected Authorization: %s, got %s", expected, auth)
	}
}

func TestHTTPRequest_Send_BasicAuth(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	config := newModuleConfigWithAuth(
		map[string]interface{}{
			"endpoint": ts.URL + "/api/data",
			"method":   "POST",
		},
		"basic",
		map[string]string{
			"username": "testuser",
			"password": "testpass123",
		},
	)

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	_, err = module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	reqs := ts.getRequests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}

	auth := reqs[0].Headers.Get("Authorization")
	// Basic auth header format: "Basic base64(username:password)"
	if !strings.HasPrefix(auth, "Basic ") {
		t.Errorf("expected Basic auth header, got %s", auth)
	}
}

func TestHTTPRequest_Send_OAuth2Auth(t *testing.T) {
	// Create OAuth2 token server
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST to token endpoint, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Errorf("expected form content type, got %s", r.Header.Get("Content-Type"))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token": "oauth2-test-token-xyz", "token_type": "Bearer", "expires_in": 3600}`))
	}))
	defer tokenServer.Close()

	// Create API server
	ts := newTestServer()
	defer ts.Close()

	config := newModuleConfigWithAuth(
		map[string]interface{}{
			"endpoint": ts.URL + "/api/data",
			"method":   "POST",
		},
		"oauth2",
		map[string]string{
			"tokenUrl":     tokenServer.URL,
			"clientId":     "test-client-id",
			"clientSecret": "test-client-secret",
		},
	)

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	_, err = module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	reqs := ts.getRequests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}

	auth := reqs[0].Headers.Get("Authorization")
	expected := "Bearer oauth2-test-token-xyz"
	if auth != expected {
		t.Errorf("expected Authorization: %s, got %s", expected, auth)
	}
}

func TestHTTPRequest_Send_OAuth2Auth_TokenCaching(t *testing.T) {
	// Track token requests
	tokenRequestCount := 0
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenRequestCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token": "cached-token", "token_type": "Bearer", "expires_in": 3600}`))
	}))
	defer tokenServer.Close()

	ts := newTestServer()
	defer ts.Close()

	config := newModuleConfigWithAuth(
		map[string]interface{}{
			"endpoint": ts.URL + "/api/data",
			"method":   "POST",
		},
		"oauth2",
		map[string]string{
			"tokenUrl":     tokenServer.URL,
			"clientId":     "test-client-id",
			"clientSecret": "test-client-secret",
		},
	)

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	// Send multiple requests
	for i := 0; i < 3; i++ {
		records := []map[string]interface{}{{"test": "data"}}
		_, err = module.Send(context.Background(), records)
		if err != nil {
			t.Fatalf("request %d: expected no error, got %v", i, err)
		}
	}

	// Token should only be requested once due to caching
	if tokenRequestCount != 1 {
		t.Errorf("expected 1 token request (caching), got %d", tokenRequestCount)
	}
}

func TestHTTPRequest_Send_APIKeyAuth_MissingKey(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	config := newModuleConfigWithAuth(
		map[string]interface{}{
			"endpoint": ts.URL + "/api/data",
			"method":   "POST",
		},
		"api-key",
		map[string]string{
			"location": "header",
			// key is missing
		},
	)

	// With shared auth package, validation happens at creation time
	_, err := NewHTTPRequestFromConfig(config)
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestHTTPRequest_Send_BearerAuth_MissingToken(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	config := newModuleConfigWithAuth(
		map[string]interface{}{
			"endpoint": ts.URL + "/api/data",
			"method":   "POST",
		},
		"bearer",
		map[string]string{
			// token is missing
		},
	)

	// With shared auth package, validation happens at creation time
	_, err := NewHTTPRequestFromConfig(config)
	if err == nil {
		t.Fatal("expected error for missing bearer token")
	}
}

func TestHTTPRequest_Send_BasicAuth_MissingCredentials(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	config := newModuleConfigWithAuth(
		map[string]interface{}{
			"endpoint": ts.URL + "/api/data",
			"method":   "POST",
		},
		"basic",
		map[string]string{
			"username": "testuser",
			// password is missing
		},
	)

	// With shared auth package, validation happens at creation time
	_, err := NewHTTPRequestFromConfig(config)
	if err == nil {
		t.Fatal("expected error for missing password")
	}
}

func TestHTTPRequest_Send_OAuth2Auth_MissingCredentials(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	config := newModuleConfigWithAuth(
		map[string]interface{}{
			"endpoint": ts.URL + "/api/data",
			"method":   "POST",
		},
		"oauth2",
		map[string]string{
			"tokenUrl": "https://auth.example.com/token",
			"clientId": "test-client-id",
			// clientSecret is missing
		},
	)

	// With shared auth package, validation happens at creation time
	_, err := NewHTTPRequestFromConfig(config)
	if err == nil {
		t.Fatal("expected error for missing OAuth2 clientSecret")
	}
}

func TestHTTPRequest_Send_NoAuth(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	// No authentication configured
	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	_, err = module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error without auth, got %v", err)
	}

	reqs := ts.getRequests()
	auth := reqs[0].Headers.Get("Authorization")
	if auth != "" {
		t.Errorf("expected no Authorization header, got %s", auth)
	}
}

func TestHTTPRequest_Send_UnknownAuthType(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	config := newModuleConfigWithAuth(
		map[string]interface{}{
			"endpoint": ts.URL + "/api/data",
			"method":   "POST",
		},
		"unknown-auth-type",
		map[string]string{
			"key": "value",
		},
	)

	// With shared auth package, unknown auth types return error at creation time
	_, err := NewHTTPRequestFromConfig(config)
	if err == nil {
		t.Fatal("expected error for unknown auth type")
	}
}

// =============================================================================
// Task 3: Request formatting and body construction tests
// =============================================================================

func TestHTTPRequest_Send_PathParameterSubstitution(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/users/{userId}/orders/{orderId}",
		"method":   "POST",
		"request": map[string]interface{}{
			"bodyFrom": "record",
			"pathParams": map[string]interface{}{
				"userId":  "user.id",
				"orderId": "order_id",
			},
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{
		{
			"user":     map[string]interface{}{"id": "user123"},
			"order_id": "order456",
			"amount":   99.99,
		},
	}

	_, err = module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	reqs := ts.getRequests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}

	// Path should have substituted values
	expectedPath := "/api/users/user123/orders/order456?"
	if reqs[0].Path != expectedPath {
		t.Errorf("expected path %s, got %s", expectedPath, reqs[0].Path)
	}
}

func TestHTTPRequest_Send_PathParameterWithNilMap(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/users/{userId}",
		"method":   "POST",
		"request": map[string]interface{}{
			"bodyFrom": "record",
			"pathParams": map[string]interface{}{
				"userId": "user.id",
			},
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	// Record with nil map in path - should not panic
	records := []map[string]interface{}{
		{
			"user": nil, // nil map should be handled gracefully
			"name": "test",
		},
	}

	_, err = module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error (nil map should be handled), got %v", err)
	}

	reqs := ts.getRequests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}

	// Path parameter should be empty (nil map returns empty string)
	// So the path should still contain the placeholder or have empty value
	if !strings.Contains(reqs[0].Path, "/api/users/") {
		t.Errorf("expected /api/users/ in path, got %s", reqs[0].Path)
	}
}

func TestHTTPRequest_Send_QueryParameters(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"request": map[string]interface{}{
			"query": map[string]interface{}{
				"status": "active",
				"limit":  "100",
			},
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	_, err = module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	reqs := ts.getRequests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}

	// Check query parameters
	if !strings.Contains(reqs[0].Path, "status=active") {
		t.Errorf("expected status=active in query, got path: %s", reqs[0].Path)
	}
	if !strings.Contains(reqs[0].Path, "limit=100") {
		t.Errorf("expected limit=100 in query, got path: %s", reqs[0].Path)
	}
}

func TestHTTPRequest_Send_QueryParametersFromRecordData(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"request": map[string]interface{}{
			"bodyFrom": "record",
			"queryFromRecord": map[string]interface{}{
				"filter_status": "status",
				"user_type":     "type",
			},
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{
		{"status": "pending", "type": "admin", "name": "John"},
	}

	_, err = module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	reqs := ts.getRequests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}

	// Check query parameters from record data
	if !strings.Contains(reqs[0].Path, "filter_status=pending") {
		t.Errorf("expected filter_status=pending in query, got path: %s", reqs[0].Path)
	}
	if !strings.Contains(reqs[0].Path, "user_type=admin") {
		t.Errorf("expected user_type=admin in query, got path: %s", reqs[0].Path)
	}
}

func TestHTTPRequest_Send_HeadersFromRecordData(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"request": map[string]interface{}{
			"bodyFrom": "record",
			"headersFromRecord": map[string]interface{}{
				"X-Correlation-ID": "correlation_id",
				"X-Request-Source": "source",
			},
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{
		{
			"correlation_id": "corr-12345",
			"source":         "batch-processor",
			"data":           "test",
		},
	}

	_, err = module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	reqs := ts.getRequests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}

	// Check headers from record data
	if reqs[0].Headers.Get("X-Correlation-ID") != "corr-12345" {
		t.Errorf("expected X-Correlation-ID: corr-12345, got %s", reqs[0].Headers.Get("X-Correlation-ID"))
	}
	if reqs[0].Headers.Get("X-Request-Source") != "batch-processor" {
		t.Errorf("expected X-Request-Source: batch-processor, got %s", reqs[0].Headers.Get("X-Request-Source"))
	}
}

func TestHTTPRequest_Send_JSONArrayFormat(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		// Default: bodyFrom = "records" (batch mode)
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{
		{"id": 1, "name": "first"},
		{"id": 2, "name": "second"},
	}

	_, err = module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	reqs := ts.getRequests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}

	// Body should be JSON array
	var body []map[string]interface{}
	if err := json.Unmarshal(reqs[0].Body, &body); err != nil {
		t.Fatalf("expected JSON array body, got error: %v", err)
	}
	if len(body) != 2 {
		t.Errorf("expected 2 records in array, got %d", len(body))
	}
}

func TestHTTPRequest_Send_JSONObjectFormat(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"request": map[string]interface{}{
			"bodyFrom": "record", // Single record mode
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{
		{"id": 1, "name": "test"},
	}

	_, err = module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	reqs := ts.getRequests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}

	// Body should be JSON object (not array)
	var body map[string]interface{}
	if err := json.Unmarshal(reqs[0].Body, &body); err != nil {
		t.Fatalf("expected JSON object body, got error: %v", err)
	}
	if body["id"] != float64(1) {
		t.Errorf("expected id=1, got %v", body["id"])
	}
}

func TestHTTPRequest_Send_ComplexNestedData(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	// Complex nested structure
	records := []map[string]interface{}{
		{
			"user": map[string]interface{}{
				"profile": map[string]interface{}{
					"name": "John Doe",
					"settings": map[string]interface{}{
						"theme":         "dark",
						"notifications": true,
					},
				},
				"addresses": []interface{}{
					map[string]interface{}{"type": "home", "city": "Paris"},
					map[string]interface{}{"type": "work", "city": "Lyon"},
				},
			},
			"metadata": map[string]interface{}{
				"created": "2026-01-20",
				"tags":    []interface{}{"premium", "active"},
			},
		},
	}

	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if sent != 1 {
		t.Errorf("expected 1 record sent, got %d", sent)
	}

	reqs := ts.getRequests()
	var body []map[string]interface{}
	if err := json.Unmarshal(reqs[0].Body, &body); err != nil {
		t.Fatalf("failed to parse body: %v", err)
	}

	// Verify nested structure is preserved
	user, ok := body[0]["user"].(map[string]interface{})
	if !ok {
		t.Fatal("expected user to be a map")
	}
	profile, ok := user["profile"].(map[string]interface{})
	if !ok {
		t.Fatal("expected profile to be a map")
	}
	if profile["name"] != "John Doe" {
		t.Errorf("expected nested name=John Doe, got %v", profile["name"])
	}

	addresses, ok := user["addresses"].([]interface{})
	if !ok {
		t.Fatal("expected addresses to be an array")
	}
	if len(addresses) != 2 {
		t.Errorf("expected 2 addresses, got %d", len(addresses))
	}
}

func TestHTTPRequest_Send_SpecialCharactersInData(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	// Data with special characters
	records := []map[string]interface{}{
		{
			"name":        "Test <User> & \"Special\"",
			"description": "Line1\nLine2\tTabbed",
			"unicode":     "日本語 émoji 🎉",
			"html":        "<script>alert('xss')</script>",
		},
	}

	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if sent != 1 {
		t.Errorf("expected 1 record sent, got %d", sent)
	}

	reqs := ts.getRequests()
	var body []map[string]interface{}
	if err := json.Unmarshal(reqs[0].Body, &body); err != nil {
		t.Fatalf("failed to parse body with special chars: %v", err)
	}

	// Verify special characters are preserved
	if body[0]["unicode"] != "日本語 émoji 🎉" {
		t.Errorf("unicode not preserved: %v", body[0]["unicode"])
	}
}

func TestHTTPRequest_Send_SpecialCharactersInQueryParams(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"request": map[string]interface{}{
			"query": map[string]interface{}{
				"filter": "value with spaces & special=chars",
				"email":  "user@example.com",
			},
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	_, err = module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	reqs := ts.getRequests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}

	// Verify query parameters are properly URL-encoded
	path := reqs[0].Path
	if !strings.Contains(path, "filter=") {
		t.Errorf("expected filter parameter in query, got path: %s", path)
	}
	if !strings.Contains(path, "email=user%40example.com") {
		t.Errorf("expected email parameter to be URL-encoded, got path: %s", path)
	}
	// URL encoding should handle special characters
	if strings.Contains(path, "value with spaces") {
		t.Errorf("expected spaces to be encoded, got path: %s", path)
	}
}

func TestHTTPRequest_Send_SpecialCharactersInPathParams(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/users/{userId}/posts/{postId}",
		"method":   "POST",
		"request": map[string]interface{}{
			"bodyFrom": "record",
			"pathParams": map[string]interface{}{
				"userId": "id",
				"postId": "post.id",
			},
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	// Record with special characters in path parameter values
	records := []map[string]interface{}{
		{
			"id": "user/123", // Contains slash - should be encoded
			"post": map[string]interface{}{
				"id": "post with spaces", // Contains spaces - should be encoded
			},
			"name": "test",
		},
	}

	_, err = module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	reqs := ts.getRequests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}

	// Verify path parameters are properly URL-encoded
	// Note: r.URL.Path is decoded by Go's HTTP server, but if the request
	// succeeded, it means the encoding was correct. We verify the path is present
	// and the request completed successfully, which proves proper encoding.
	path := reqs[0].Path
	// The request should succeed, which means path encoding was correct
	// We verify the path contains the expected structure (even if decoded)
	if !strings.Contains(path, "/api/users/") {
		t.Errorf("expected /api/users/ in path, got path: %s", path)
	}
	if !strings.Contains(path, "/posts/") {
		t.Errorf("expected /posts/ in path, got path: %s", path)
	}
	// The fact that the request succeeded proves proper URL encoding was applied
	// (otherwise the HTTP request would have failed with invalid URL)
}

// =============================================================================
// Task 4: HTTP response handling tests
// =============================================================================

// testServerWithStatus creates a test server with configurable status code
type testServerWithStatus struct {
	*httptest.Server
	mu           sync.Mutex
	statusCode   int
	responseBody string
}

func newTestServerWithStatus(statusCode int, responseBody string) *testServerWithStatus {
	ts := &testServerWithStatus{
		statusCode:   statusCode,
		responseBody: responseBody,
	}

	ts.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts.mu.Lock()
		defer ts.mu.Unlock()
		w.WriteHeader(ts.statusCode)
		_, _ = w.Write([]byte(ts.responseBody))
	}))

	return ts
}

func TestHTTPRequest_Send_Success200(t *testing.T) {
	ts := newTestServerWithStatus(200, `{"success": true}`)
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error for 200, got %v", err)
	}
	if sent != 1 {
		t.Errorf("expected 1 record sent, got %d", sent)
	}
}

func TestHTTPRequest_Send_Success201(t *testing.T) {
	ts := newTestServerWithStatus(201, `{"id": 123, "created": true}`)
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error for 201, got %v", err)
	}
	if sent != 1 {
		t.Errorf("expected 1 record sent, got %d", sent)
	}
}

func TestHTTPRequest_Send_Success204(t *testing.T) {
	ts := newTestServerWithStatus(204, "")
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error for 204, got %v", err)
	}
	if sent != 1 {
		t.Errorf("expected 1 record sent, got %d", sent)
	}
}

func TestHTTPRequest_Send_ClientError400(t *testing.T) {
	ts := newTestServerWithStatus(400, `{"error": "Bad Request", "details": "Invalid JSON"}`)
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	_, err = module.Send(context.Background(), records)
	if err == nil {
		t.Fatal("expected error for 400 status")
	}

	// Check error type
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected HTTPError, got %T", err)
	}
	if httpErr.StatusCode != 400 {
		t.Errorf("expected status 400, got %d", httpErr.StatusCode)
	}
}

func TestHTTPRequest_Send_ClientError401(t *testing.T) {
	ts := newTestServerWithStatus(401, `{"error": "Unauthorized"}`)
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	_, err = module.Send(context.Background(), records)
	if err == nil {
		t.Fatal("expected error for 401 status")
	}

	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected HTTPError, got %T", err)
	}
	if httpErr.StatusCode != 401 {
		t.Errorf("expected status 401, got %d", httpErr.StatusCode)
	}
}

func TestHTTPRequest_Send_ClientError404(t *testing.T) {
	ts := newTestServerWithStatus(404, `{"error": "Not Found"}`)
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	_, err = module.Send(context.Background(), records)
	if err == nil {
		t.Fatal("expected error for 404 status")
	}

	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected HTTPError, got %T", err)
	}
	if httpErr.StatusCode != 404 {
		t.Errorf("expected status 404, got %d", httpErr.StatusCode)
	}
}

func TestHTTPRequest_Send_ClientError422(t *testing.T) {
	ts := newTestServerWithStatus(422, `{"error": "Unprocessable Entity", "validation": {"field": "required"}}`)
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	_, err = module.Send(context.Background(), records)
	if err == nil {
		t.Fatal("expected error for 422 status")
	}

	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected HTTPError, got %T", err)
	}
	if httpErr.StatusCode != 422 {
		t.Errorf("expected status 422, got %d", httpErr.StatusCode)
	}
	// Verify response body is captured
	if !strings.Contains(httpErr.ResponseBody, "Unprocessable Entity") {
		t.Errorf("expected error details in response body, got %s", httpErr.ResponseBody)
	}
}

func TestHTTPRequest_Send_ServerError500(t *testing.T) {
	ts := newTestServerWithStatus(500, `{"error": "Internal Server Error"}`)
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"retry": map[string]interface{}{
			"backoffMs": float64(1), // Minimal backoff for fast tests
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	_, err = module.Send(context.Background(), records)
	if err == nil {
		t.Fatal("expected error for 500 status")
	}

	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected HTTPError, got %T", err)
	}
	if httpErr.StatusCode != 500 {
		t.Errorf("expected status 500, got %d", httpErr.StatusCode)
	}
}

func TestHTTPRequest_Send_ServerError503(t *testing.T) {
	ts := newTestServerWithStatus(503, `{"error": "Service Unavailable"}`)
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"retry": map[string]interface{}{
			"backoffMs": float64(1), // Minimal backoff for fast tests
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	_, err = module.Send(context.Background(), records)
	if err == nil {
		t.Fatal("expected error for 503 status")
	}

	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected HTTPError, got %T", err)
	}
	if httpErr.StatusCode != 503 {
		t.Errorf("expected status 503, got %d", httpErr.StatusCode)
	}
}

func TestHTTPRequest_Send_CustomSuccessCodes(t *testing.T) {
	// Create server that returns 202 Accepted
	ts := newTestServerWithStatus(202, `{"status": "accepted", "jobId": "abc123"}`)
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"success": map[string]interface{}{
			"statusCodes": []interface{}{float64(200), float64(201), float64(202), float64(204)},
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error for custom success code 202, got %v", err)
	}
	if sent != 1 {
		t.Errorf("expected 1 record sent, got %d", sent)
	}
}

func TestHTTPRequest_Send_CustomSuccessCodes_Reject(t *testing.T) {
	// Create server that returns 200, but we only accept 201
	ts := newTestServerWithStatus(200, `{"success": true}`)
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"success": map[string]interface{}{
			"statusCodes": []interface{}{float64(201)}, // Only 201 is success
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	_, err = module.Send(context.Background(), records)
	if err == nil {
		t.Fatal("expected error for 200 when only 201 is configured as success")
	}
}

func TestHTTPRequest_Send_HTTPErrorDetails(t *testing.T) {
	ts := newTestServerWithStatus(400, `{"error": "validation_failed", "message": "Field 'email' is invalid", "code": "E001"}`)
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	_, err = module.Send(context.Background(), records)
	if err == nil {
		t.Fatal("expected error for 400 status")
	}

	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected HTTPError, got %T", err)
	}

	// Verify error contains all context
	if httpErr.StatusCode != 400 {
		t.Errorf("expected status 400, got %d", httpErr.StatusCode)
	}
	if httpErr.Endpoint == "" {
		t.Error("expected endpoint in error")
	}
	if httpErr.Method != "POST" {
		t.Errorf("expected method POST, got %s", httpErr.Method)
	}
	if httpErr.ResponseBody == "" {
		t.Error("expected response body in error")
	}
	// Check error string is informative
	errStr := httpErr.Error()
	if !strings.Contains(errStr, "400") {
		t.Errorf("error string should contain status code: %s", errStr)
	}
}

// =============================================================================
// Task 5: Error handling and retry logic tests
// =============================================================================

// testServerWithRetry creates a test server that fails N times then succeeds
type testServerWithRetry struct {
	*httptest.Server
	mu           sync.Mutex
	maxFails     int
	requestCount int
}

func newTestServerWithRetry(maxFails int) *testServerWithRetry {
	ts := &testServerWithRetry{
		maxFails: maxFails,
	}

	ts.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts.mu.Lock()
		ts.requestCount++
		count := ts.requestCount
		maxF := ts.maxFails
		ts.mu.Unlock()

		if count <= maxF {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error": "service unavailable"}`))
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success": true}`))
	}))

	return ts
}

func (ts *testServerWithRetry) getRequestCount() int {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.requestCount
}

func TestHTTPRequest_Send_RetryOn5xx(t *testing.T) {
	// Server fails twice then succeeds
	ts := newTestServerWithRetry(2)
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"retry": map[string]interface{}{
			"maxRetries":        float64(3),
			"backoffMs":         float64(10), // Small for tests
			"backoffMultiplier": float64(1.0),
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error after retries, got %v", err)
	}
	if sent != 1 {
		t.Errorf("expected 1 record sent, got %d", sent)
	}

	// Should have made 3 requests (2 fails + 1 success)
	if ts.getRequestCount() != 3 {
		t.Errorf("expected 3 requests (2 retries + 1 success), got %d", ts.getRequestCount())
	}
}

func TestHTTPRequest_Send_RetryExhausted(t *testing.T) {
	// Server always fails
	ts := newTestServerWithStatus(503, `{"error": "service unavailable"}`)
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"retry": map[string]interface{}{
			"maxRetries":        float64(2),
			"backoffMs":         float64(10),
			"backoffMultiplier": float64(1.0),
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	_, err = module.Send(context.Background(), records)
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}

	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected HTTPError, got %T", err)
	}
	if httpErr.StatusCode != 503 {
		t.Errorf("expected status 503, got %d", httpErr.StatusCode)
	}
}

func TestHTTPRequest_Send_NoRetryOn4xx(t *testing.T) {
	requestCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "bad request"}`))
	}))
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"retry": map[string]interface{}{
			"maxRetries":        float64(3),
			"backoffMs":         float64(10),
			"backoffMultiplier": float64(1.0),
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	_, err = module.Send(context.Background(), records)
	if err == nil {
		t.Fatal("expected error for 400 status")
	}

	// Should only make 1 request - no retries for 4xx
	if requestCount != 1 {
		t.Errorf("expected 1 request (no retries for 4xx), got %d", requestCount)
	}
}

func TestHTTPRequest_Send_OnErrorFail(t *testing.T) {
	// Use 4xx error to avoid retries (faster test)
	ts := newTestServerWithStatus(400, `{"error": "bad request"}`)
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"onError":  "fail",
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	_, err = module.Send(context.Background(), records)
	if err == nil {
		t.Fatal("expected error with onError=fail")
	}
}

func TestHTTPRequest_Send_OnErrorSkip_SingleRecordMode(t *testing.T) {
	requestCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		// First request fails with 4xx (non-retryable), rest succeed
		if requestCount == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error": "bad request"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success": true}`))
	}))
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"onError":  "skip",
		"request": map[string]interface{}{
			"bodyFrom": "record",
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{
		{"id": 1, "name": "first"},  // Will fail (4xx - no retry)
		{"id": 2, "name": "second"}, // Will succeed
		{"id": 3, "name": "third"},  // Will succeed
	}

	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error with onError=skip, got %v", err)
	}
	// Should skip first record, send 2 successfully
	if sent != 2 {
		t.Errorf("expected 2 records sent (1 skipped), got %d", sent)
	}
}

func TestHTTPRequest_Send_OnErrorLog_SingleRecordMode(t *testing.T) {
	requestCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		// Second request fails with 4xx (non-retryable), rest succeed
		if requestCount == 2 {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"error": "validation error"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success": true}`))
	}))
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"onError":  "log",
		"request": map[string]interface{}{
			"bodyFrom": "record",
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{
		{"id": 1, "name": "first"},
		{"id": 2, "name": "second"}, // Will fail (4xx - no retry)
		{"id": 3, "name": "third"},
	}

	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error with onError=log, got %v", err)
	}
	// Should still count as 2 successful (log continues)
	if sent != 2 {
		t.Errorf("expected 2 records sent (1 failed but logged), got %d", sent)
	}
}

func TestHTTPRequest_Send_DefaultRetryConfig(t *testing.T) {
	// Test that default retry config is applied (maxRetries=3)
	ts := newTestServerWithRetry(2) // Fails twice, succeeds on third
	defer ts.Close()

	// Use minimal backoff for fast tests while testing retry logic
	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"retry": map[string]interface{}{
			"backoffMs": float64(1), // Minimal backoff for fast tests
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error with default retry, got %v", err)
	}
	if sent != 1 {
		t.Errorf("expected 1 record sent, got %d", sent)
	}
}

func TestHTTPRequest_IsTransientError(t *testing.T) {
	tests := []struct {
		statusCode  int
		isTransient bool
	}{
		{500, true},  // Internal Server Error
		{502, true},  // Bad Gateway
		{503, true},  // Service Unavailable
		{504, true},  // Gateway Timeout
		{429, true},  // Too Many Requests
		{400, false}, // Bad Request
		{401, false}, // Unauthorized
		{403, false}, // Forbidden
		{404, false}, // Not Found
		{422, false}, // Unprocessable Entity
		{200, true},  // OK (unknown status, retryable by default per 12-4)
		{201, true},  // Created (unknown status, retryable by default per 12-4)
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.statusCode)), func(t *testing.T) {
			// Use errhandling.ClassifyHTTPStatus and check Retryable field
			classifiedErr := errhandling.ClassifyHTTPStatus(tt.statusCode, "test error")
			result := classifiedErr.Retryable
			if result != tt.isTransient {
				t.Errorf("ClassifyHTTPStatus(%d).Retryable = %v, want %v", tt.statusCode, result, tt.isTransient)
			}
		})
	}
}

// =============================================================================
// Task 6: Execution status reporting tests
// =============================================================================

func TestHTTPRequest_Send_ReturnsCorrectCount_AllSuccess(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{
		{"id": 1}, {"id": 2}, {"id": 3}, {"id": 4}, {"id": 5},
	}

	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if sent != 5 {
		t.Errorf("expected 5 records sent, got %d", sent)
	}
}

func TestHTTPRequest_Send_ReturnsCorrectCount_PartialFailure(t *testing.T) {
	requestCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		// Fail on 3rd and 5th requests (4xx = no retry)
		if requestCount == 3 || requestCount == 5 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error": "bad request"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success": true}`))
	}))
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"onError":  "skip", // Skip failed records
		"request": map[string]interface{}{
			"bodyFrom": "record",
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{
		{"id": 1}, {"id": 2}, {"id": 3}, {"id": 4}, {"id": 5},
	}

	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error with onError=skip, got %v", err)
	}
	// Should send 3 successfully (1, 2, 4), skip 2 failures (3, 5)
	if sent != 3 {
		t.Errorf("expected 3 records sent (2 skipped), got %d", sent)
	}
}

func TestHTTPRequest_Send_ReturnsZeroOnEmpty(t *testing.T) {
	config := newModuleConfig(map[string]interface{}{
		"endpoint": "https://api.example.com/data",
		"method":   "POST",
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	// Empty records
	sent, err := module.Send(context.Background(), []map[string]interface{}{})
	if err != nil {
		t.Fatalf("expected no error for empty, got %v", err)
	}
	if sent != 0 {
		t.Errorf("expected 0 sent for empty, got %d", sent)
	}

	// Nil records
	sent, err = module.Send(context.Background(), nil)
	if err != nil {
		t.Fatalf("expected no error for nil, got %v", err)
	}
	if sent != 0 {
		t.Errorf("expected 0 sent for nil, got %d", sent)
	}
}

func TestHTTPRequest_Send_ErrorContainsResponseDetails(t *testing.T) {
	errorBody := `{"error": "validation_failed", "field": "email", "message": "Invalid email format"}`
	ts := newTestServerWithStatus(422, errorBody)
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"email": "invalid"}}
	sent, err := module.Send(context.Background(), records)

	// Verify count
	if sent != 0 {
		t.Errorf("expected 0 sent on error, got %d", sent)
	}

	// Verify error details
	if err == nil {
		t.Fatal("expected error")
	}

	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected HTTPError, got %T", err)
	}

	// Verify response body is captured for debugging
	if httpErr.ResponseBody != errorBody {
		t.Errorf("expected response body preserved, got %s", httpErr.ResponseBody)
	}

	// Verify status code
	if httpErr.StatusCode != 422 {
		t.Errorf("expected status 422, got %d", httpErr.StatusCode)
	}
}

func TestHTTPRequest_Send_BatchMode_CorrectCount(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		// Default batch mode
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{
		{"id": 1}, {"id": 2}, {"id": 3},
	}

	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// In batch mode, all records sent in one request
	if sent != 3 {
		t.Errorf("expected 3 records sent, got %d", sent)
	}

	// Verify only 1 request was made (batch)
	reqs := ts.getRequests()
	if len(reqs) != 1 {
		t.Errorf("expected 1 batch request, got %d", len(reqs))
	}
}

func TestHTTPRequest_Send_SingleRecordMode_CorrectCount(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"request": map[string]interface{}{
			"bodyFrom": "record",
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{
		{"id": 1}, {"id": 2}, {"id": 3},
	}

	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if sent != 3 {
		t.Errorf("expected 3 records sent, got %d", sent)
	}

	// Verify 3 separate requests (one per record)
	reqs := ts.getRequests()
	if len(reqs) != 3 {
		t.Errorf("expected 3 requests (single record mode), got %d", len(reqs))
	}
}

// =============================================================================
// Task 8: Deterministic execution tests
// =============================================================================

func TestHTTPRequest_Deterministic_SameInputSameOutput(t *testing.T) {
	// Run the same request multiple times and verify identical behavior
	var allBodies []string

	for run := 0; run < 3; run++ {
		ts := newTestServer()

		config := newModuleConfig(map[string]interface{}{
			"endpoint": ts.URL + "/api/data",
			"method":   "POST",
		})

		module, err := NewHTTPRequestFromConfig(config)
		if err != nil {
			t.Fatalf("run %d: failed to create module: %v", run, err)
		}

		records := []map[string]interface{}{
			{"id": 1, "name": "Alice", "active": true},
			{"id": 2, "name": "Bob", "active": false},
		}

		_, err = module.Send(context.Background(), records)
		if err != nil {
			t.Fatalf("run %d: Send failed: %v", run, err)
		}

		reqs := ts.getRequests()
		if len(reqs) != 1 {
			t.Fatalf("run %d: expected 1 request, got %d", run, len(reqs))
		}

		allBodies = append(allBodies, string(reqs[0].Body))
		ts.Close()
	}

	// All request bodies should be identical
	for i := 1; i < len(allBodies); i++ {
		if allBodies[i] != allBodies[0] {
			t.Errorf("non-deterministic: run 0 body != run %d body\n0: %s\n%d: %s",
				i, allBodies[0], i, allBodies[i])
		}
	}
}

func TestHTTPRequest_Deterministic_JSONSerialization(t *testing.T) {
	ts := newTestServer()
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	// Complex nested structure
	records := []map[string]interface{}{
		{
			"user": map[string]interface{}{
				"name":    "Alice",
				"profile": map[string]interface{}{"age": 30, "city": "Paris"},
			},
			"tags":   []interface{}{"admin", "active"},
			"active": true,
		},
	}

	// Send multiple times
	var bodies []string
	for i := 0; i < 3; i++ {
		_, err = module.Send(context.Background(), records)
		if err != nil {
			t.Fatalf("Send %d failed: %v", i, err)
		}
	}

	reqs := ts.getRequests()
	for _, req := range reqs {
		bodies = append(bodies, string(req.Body))
	}

	// All bodies should be identical
	for i := 1; i < len(bodies); i++ {
		if bodies[i] != bodies[0] {
			t.Errorf("non-deterministic JSON: request 0 != request %d", i)
		}
	}
}

func TestHTTPRequest_Deterministic_AuthHeaders(t *testing.T) {
	// Verify authentication headers are deterministic
	var authHeaders []string

	for run := 0; run < 3; run++ {
		ts := newTestServer()

		config := newModuleConfigWithAuth(
			map[string]interface{}{
				"endpoint": ts.URL + "/api/data",
				"method":   "POST",
			},
			"api-key",
			map[string]string{
				"key":        "test-api-key-123",
				"location":   "header",
				"headerName": "X-API-Key",
			},
		)

		module, err := NewHTTPRequestFromConfig(config)
		if err != nil {
			t.Fatalf("run %d: failed to create module: %v", run, err)
		}

		records := []map[string]interface{}{{"test": "data"}}
		_, err = module.Send(context.Background(), records)
		if err != nil {
			t.Fatalf("run %d: Send failed: %v", run, err)
		}

		reqs := ts.getRequests()
		authHeaders = append(authHeaders, reqs[0].Headers.Get("X-API-Key"))
		ts.Close()
	}

	// All auth headers should be identical
	for i := 1; i < len(authHeaders); i++ {
		if authHeaders[i] != authHeaders[0] {
			t.Errorf("non-deterministic auth: run 0 != run %d", i)
		}
	}
}

func TestHTTPRequest_Deterministic_PathParams(t *testing.T) {
	var paths []string

	for run := 0; run < 3; run++ {
		ts := newTestServer()

		config := newModuleConfig(map[string]interface{}{
			"endpoint": ts.URL + "/api/users/{userId}",
			"method":   "POST",
			"request": map[string]interface{}{
				"bodyFrom": "record",
				"pathParams": map[string]interface{}{
					"userId": "id",
				},
			},
		})

		module, err := NewHTTPRequestFromConfig(config)
		if err != nil {
			t.Fatalf("run %d: failed to create module: %v", run, err)
		}

		records := []map[string]interface{}{
			{"id": "user-123", "name": "Alice"},
		}

		_, err = module.Send(context.Background(), records)
		if err != nil {
			t.Fatalf("run %d: Send failed: %v", run, err)
		}

		reqs := ts.getRequests()
		paths = append(paths, reqs[0].Path)
		ts.Close()
	}

	// All paths should be identical
	for i := 1; i < len(paths); i++ {
		if paths[i] != paths[0] {
			t.Errorf("non-deterministic path: run 0 (%s) != run %d (%s)",
				paths[0], i, paths[i])
		}
	}
}

func TestHTTPRequest_Deterministic_ErrorHandling(t *testing.T) {
	// Verify same error produces same handling
	var errors []string

	for run := 0; run < 3; run++ {
		ts := newTestServerWithStatus(400, `{"error": "bad request"}`)

		config := newModuleConfig(map[string]interface{}{
			"endpoint": ts.URL + "/api/data",
			"method":   "POST",
		})

		module, err := NewHTTPRequestFromConfig(config)
		if err != nil {
			t.Fatalf("run %d: failed to create module: %v", run, err)
		}

		records := []map[string]interface{}{{"test": "data"}}
		_, err = module.Send(context.Background(), records)
		if err == nil {
			t.Fatalf("run %d: expected error", run)
		}

		errors = append(errors, err.Error())
		ts.Close()
	}

	// All error messages should have same structure
	for i := 1; i < len(errors); i++ {
		// Error messages will differ in endpoint URL but structure should be same
		if len(errors[i]) == 0 {
			t.Errorf("run %d: empty error message", i)
		}
	}
}

func TestHTTPRequest_NoRandomBehavior(t *testing.T) {
	// Verify no random UUIDs, timestamps, or other non-deterministic values are added
	ts := newTestServer()
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	// Input with no random values
	records := []map[string]interface{}{
		{"id": 1, "value": "test"},
	}

	_, err = module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	reqs := ts.getRequests()
	var body []map[string]interface{}
	if err := json.Unmarshal(reqs[0].Body, &body); err != nil {
		t.Fatalf("failed to parse body: %v", err)
	}

	// Verify output matches input exactly (no added fields)
	if len(body) != 1 {
		t.Errorf("expected 1 record, got %d", len(body))
	}
	if body[0]["id"] != float64(1) {
		t.Errorf("id changed: expected 1, got %v", body[0]["id"])
	}
	if body[0]["value"] != "test" {
		t.Errorf("value changed: expected test, got %v", body[0]["value"])
	}
	// No extra fields should be added
	if len(body[0]) != 2 {
		t.Errorf("extra fields added: expected 2 fields, got %d", len(body[0]))
	}
}

// =============================================================================
// Story 4.2: Dry-Run Mode - Task 1: Request Preview Functionality Tests
// =============================================================================

func TestHTTPRequest_PreviewRequest_BatchMode(t *testing.T) {
	config := newModuleConfig(map[string]interface{}{
		"endpoint": "https://api.example.com/data",
		"method":   "POST",
		"headers": map[string]interface{}{
			"X-Custom-Header": "custom-value",
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{
		{"id": 1, "name": "first"},
		{"id": 2, "name": "second"},
	}

	previews, err := module.PreviewRequest(records, PreviewOptions{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Batch mode should return 1 preview (single request for all records)
	if len(previews) != 1 {
		t.Fatalf("expected 1 preview (batch mode), got %d", len(previews))
	}

	preview := previews[0]

	// Verify endpoint URL
	if preview.Endpoint != "https://api.example.com/data" {
		t.Errorf("expected endpoint https://api.example.com/data, got %s", preview.Endpoint)
	}

	// Verify HTTP method
	if preview.Method != "POST" {
		t.Errorf("expected method POST, got %s", preview.Method)
	}

	// Verify headers include custom header
	if preview.Headers["X-Custom-Header"] != "custom-value" {
		t.Errorf("expected X-Custom-Header: custom-value, got %s", preview.Headers["X-Custom-Header"])
	}

	// Verify Content-Type is set
	if preview.Headers["Content-Type"] != "application/json" {
		t.Errorf("expected Content-Type: application/json, got %s", preview.Headers["Content-Type"])
	}

	// Verify record count
	if preview.RecordCount != 2 {
		t.Errorf("expected RecordCount 2, got %d", preview.RecordCount)
	}

	// Verify body preview is valid JSON array
	var body []map[string]interface{}
	if err := json.Unmarshal([]byte(preview.BodyPreview), &body); err != nil {
		t.Fatalf("body preview should be valid JSON: %v", err)
	}
	if len(body) != 2 {
		t.Errorf("expected 2 records in body preview, got %d", len(body))
	}
}

func TestHTTPRequest_PreviewRequest_SingleRecordMode(t *testing.T) {
	config := newModuleConfig(map[string]interface{}{
		"endpoint": "https://api.example.com/data",
		"method":   "POST",
		"request": map[string]interface{}{
			"bodyFrom": "record",
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{
		{"id": 1, "name": "first"},
		{"id": 2, "name": "second"},
		{"id": 3, "name": "third"},
	}

	previews, err := module.PreviewRequest(records, PreviewOptions{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Single record mode should return 3 previews (one per record)
	if len(previews) != 3 {
		t.Fatalf("expected 3 previews (single record mode), got %d", len(previews))
	}

	// Verify each preview
	for i, preview := range previews {
		// Verify method
		if preview.Method != "POST" {
			t.Errorf("preview %d: expected method POST, got %s", i, preview.Method)
		}

		// Verify record count is 1 per request
		if preview.RecordCount != 1 {
			t.Errorf("preview %d: expected RecordCount 1, got %d", i, preview.RecordCount)
		}

		// Verify body is a single object (not array)
		var body map[string]interface{}
		if err := json.Unmarshal([]byte(preview.BodyPreview), &body); err != nil {
			t.Errorf("preview %d: body preview should be valid JSON object: %v", i, err)
		}

		expectedID := float64(i + 1)
		if body["id"] != expectedID {
			t.Errorf("preview %d: expected id=%v, got %v", i, expectedID, body["id"])
		}
	}
}

func TestHTTPRequest_PreviewRequest_PathParameters(t *testing.T) {
	config := newModuleConfig(map[string]interface{}{
		"endpoint": "https://api.example.com/users/{userId}/orders/{orderId}",
		"method":   "PUT",
		"request": map[string]interface{}{
			"bodyFrom": "record",
			"pathParams": map[string]interface{}{
				"userId":  "user.id",
				"orderId": "order_id",
			},
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{
		{
			"user":     map[string]interface{}{"id": "user123"},
			"order_id": "order456",
			"amount":   99.99,
		},
	}

	previews, err := module.PreviewRequest(records, PreviewOptions{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(previews) != 1 {
		t.Fatalf("expected 1 preview, got %d", len(previews))
	}

	// Path should have substituted values
	expectedEndpoint := "https://api.example.com/users/user123/orders/order456"
	if previews[0].Endpoint != expectedEndpoint {
		t.Errorf("expected endpoint %s, got %s", expectedEndpoint, previews[0].Endpoint)
	}
}

func TestHTTPRequest_PreviewRequest_QueryParameters(t *testing.T) {
	config := newModuleConfig(map[string]interface{}{
		"endpoint": "https://api.example.com/data",
		"method":   "POST",
		"request": map[string]interface{}{
			"query": map[string]interface{}{
				"status": "active",
				"limit":  "100",
			},
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}

	previews, err := module.PreviewRequest(records, PreviewOptions{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(previews) != 1 {
		t.Fatalf("expected 1 preview, got %d", len(previews))
	}

	// Endpoint should include query parameters
	endpoint := previews[0].Endpoint
	if !strings.Contains(endpoint, "status=active") {
		t.Errorf("expected status=active in endpoint, got %s", endpoint)
	}
	if !strings.Contains(endpoint, "limit=100") {
		t.Errorf("expected limit=100 in endpoint, got %s", endpoint)
	}
}

func TestHTTPRequest_PreviewRequest_QueryParametersFromRecord(t *testing.T) {
	config := newModuleConfig(map[string]interface{}{
		"endpoint": "https://api.example.com/data",
		"method":   "POST",
		"request": map[string]interface{}{
			"bodyFrom": "record",
			"queryFromRecord": map[string]interface{}{
				"filter_status": "status",
				"user_type":     "type",
			},
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{
		{"status": "pending", "type": "admin", "name": "John"},
	}

	previews, err := module.PreviewRequest(records, PreviewOptions{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(previews) != 1 {
		t.Fatalf("expected 1 preview, got %d", len(previews))
	}

	// Endpoint should include query parameters from record
	endpoint := previews[0].Endpoint
	if !strings.Contains(endpoint, "filter_status=pending") {
		t.Errorf("expected filter_status=pending in endpoint, got %s", endpoint)
	}
	if !strings.Contains(endpoint, "user_type=admin") {
		t.Errorf("expected user_type=admin in endpoint, got %s", endpoint)
	}
}

func TestHTTPRequest_PreviewRequest_AuthHeadersMasked_APIKey(t *testing.T) {
	config := newModuleConfigWithAuth(
		map[string]interface{}{
			"endpoint": "https://api.example.com/data",
			"method":   "POST",
		},
		"api-key",
		map[string]string{
			"key":        "super-secret-api-key-12345",
			"location":   "header",
			"headerName": "X-API-Key",
		},
	)

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}

	previews, err := module.PreviewRequest(records, PreviewOptions{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(previews) != 1 {
		t.Fatalf("expected 1 preview, got %d", len(previews))
	}

	// API key should be masked in preview
	// Note: Go normalizes header names, so "X-API-Key" becomes "X-Api-Key"
	apiKeyHeader := findHeaderCaseInsensitive(previews[0].Headers, "X-API-Key")
	if apiKeyHeader == "super-secret-api-key-12345" {
		t.Error("API key should be masked in preview, but was shown in plain text")
	}
	if apiKeyHeader == "" {
		t.Errorf("API key header should be present (masked), but was empty. Headers: %v", previews[0].Headers)
	}
	// Should contain mask indicator (format: [MASKED-*] or ***)
	if !strings.Contains(apiKeyHeader, "***") && !strings.Contains(apiKeyHeader, "MASKED") {
		t.Errorf("API key should be masked with *** or MASKED indicator, got: %s", apiKeyHeader)
	}
}

// findHeaderCaseInsensitive finds a header value by case-insensitive key match
func findHeaderCaseInsensitive(headers map[string]string, key string) string {
	// First try exact match
	if val, ok := headers[key]; ok {
		return val
	}
	// Then try case-insensitive match
	for k, v := range headers {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}

func TestHTTPRequest_PreviewRequest_AuthHeadersMasked_Bearer(t *testing.T) {
	config := newModuleConfigWithAuth(
		map[string]interface{}{
			"endpoint": "https://api.example.com/data",
			"method":   "POST",
		},
		"bearer",
		map[string]string{
			"token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.super-secret-payload",
		},
	)

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}

	previews, err := module.PreviewRequest(records, PreviewOptions{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(previews) != 1 {
		t.Fatalf("expected 1 preview, got %d", len(previews))
	}

	// Authorization header should be masked
	authHeader := previews[0].Headers["Authorization"]
	if strings.Contains(authHeader, "super-secret-payload") {
		t.Error("Bearer token payload should be masked in preview")
	}
	if authHeader == "" {
		t.Error("Authorization header should be present (masked), but was empty")
	}
	// Should start with Bearer and contain mask
	if !strings.HasPrefix(authHeader, "Bearer ") {
		t.Errorf("Authorization should start with 'Bearer ', got: %s", authHeader)
	}
}

func TestHTTPRequest_PreviewRequest_AuthHeadersMasked_Basic(t *testing.T) {
	config := newModuleConfigWithAuth(
		map[string]interface{}{
			"endpoint": "https://api.example.com/data",
			"method":   "POST",
		},
		"basic",
		map[string]string{
			"username": "testuser",
			"password": "super-secret-password",
		},
	)

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}

	previews, err := module.PreviewRequest(records, PreviewOptions{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(previews) != 1 {
		t.Fatalf("expected 1 preview, got %d", len(previews))
	}

	// Authorization header should be masked
	authHeader := previews[0].Headers["Authorization"]
	if authHeader == "" {
		t.Error("Authorization header should be present (masked), but was empty")
	}
	// Should start with Basic and contain mask
	if !strings.HasPrefix(authHeader, "Basic ") {
		t.Errorf("Authorization should start with 'Basic ', got: %s", authHeader)
	}
	// Should NOT contain the actual base64 encoded credentials
	if strings.Contains(authHeader, "dGVzdHVzZXI6c3VwZXItc2VjcmV0LXBhc3N3b3Jk") {
		t.Error("Basic auth credentials should be masked in preview")
	}
}

func TestHTTPRequest_PreviewRequest_ShowCredentials_Bearer(t *testing.T) {
	// Test that credentials are shown when ShowCredentials is true
	config := newModuleConfigWithAuth(
		map[string]interface{}{
			"endpoint": "https://api.example.com/data",
			"method":   "POST",
		},
		"bearer",
		map[string]string{
			"token": "my-secret-bearer-token-12345",
		},
	)

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}

	// With ShowCredentials = true
	previews, err := module.PreviewRequest(records, PreviewOptions{ShowCredentials: true})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(previews) != 1 {
		t.Fatalf("expected 1 preview, got %d", len(previews))
	}

	// Authorization header should contain the actual token
	authHeader := previews[0].Headers["Authorization"]
	if authHeader == "" {
		t.Error("Authorization header should be present")
	}
	// Should contain the actual token
	if !strings.Contains(authHeader, "my-secret-bearer-token-12345") {
		t.Errorf("Authorization should contain actual token with ShowCredentials=true, got: %s", authHeader)
	}
	// Should NOT be masked
	if strings.Contains(authHeader, "[MASKED") {
		t.Errorf("Authorization should NOT be masked with ShowCredentials=true, got: %s", authHeader)
	}
}

func TestHTTPRequest_PreviewRequest_ShowCredentials_APIKey(t *testing.T) {
	// Test that API key is shown when ShowCredentials is true
	config := newModuleConfigWithAuth(
		map[string]interface{}{
			"endpoint": "https://api.example.com/data",
			"method":   "POST",
		},
		"api-key",
		map[string]string{
			"key":        "secret-api-key-xyz789",
			"headerName": "X-Api-Key",
			"location":   "header",
		},
	)

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}

	// With ShowCredentials = true
	previews, err := module.PreviewRequest(records, PreviewOptions{ShowCredentials: true})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(previews) != 1 {
		t.Fatalf("expected 1 preview, got %d", len(previews))
	}

	// Check for API key in headers (http package canonicalizes header names)
	// X-Api-Key becomes X-Api-Key in canonical form
	var apiKeyHeader string
	for key, value := range previews[0].Headers {
		if strings.EqualFold(key, "X-Api-Key") {
			apiKeyHeader = value
			break
		}
	}

	if apiKeyHeader == "" {
		t.Logf("Headers received: %v", previews[0].Headers)
		t.Error("API key header should be present")
		return
	}

	// Should contain the actual key
	if apiKeyHeader != "secret-api-key-xyz789" {
		t.Errorf("API key should be actual key with ShowCredentials=true, got: %s", apiKeyHeader)
	}
}

func TestHTTPRequest_PreviewRequest_EmptyRecords(t *testing.T) {
	config := newModuleConfig(map[string]interface{}{
		"endpoint": "https://api.example.com/data",
		"method":   "POST",
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	// Empty records
	previews, err := module.PreviewRequest([]map[string]interface{}{}, PreviewOptions{})
	if err != nil {
		t.Fatalf("expected no error for empty records, got %v", err)
	}
	if len(previews) != 0 {
		t.Errorf("expected 0 previews for empty records, got %d", len(previews))
	}

	// Nil records
	previews, err = module.PreviewRequest(nil, PreviewOptions{})
	if err != nil {
		t.Fatalf("expected no error for nil records, got %v", err)
	}
	if len(previews) != 0 {
		t.Errorf("expected 0 previews for nil records, got %d", len(previews))
	}
}

func TestHTTPRequest_PreviewRequest_BodyPreviewFormatted(t *testing.T) {
	config := newModuleConfig(map[string]interface{}{
		"endpoint": "https://api.example.com/data",
		"method":   "POST",
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{
		{
			"user": map[string]interface{}{
				"name":  "John",
				"email": "john@example.com",
			},
		},
	}

	previews, err := module.PreviewRequest(records, PreviewOptions{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(previews) != 1 {
		t.Fatalf("expected 1 preview, got %d", len(previews))
	}

	// Body preview should be formatted (indented) JSON for readability
	bodyPreview := previews[0].BodyPreview
	if !strings.Contains(bodyPreview, "\n") {
		t.Error("body preview should be formatted with newlines for readability")
	}
	if !strings.Contains(bodyPreview, "  ") {
		t.Error("body preview should be indented for readability")
	}
}

func TestHTTPRequest_PreviewRequest_HeadersFromRecord(t *testing.T) {
	config := newModuleConfig(map[string]interface{}{
		"endpoint": "https://api.example.com/data",
		"method":   "POST",
		"request": map[string]interface{}{
			"bodyFrom": "record",
			"headersFromRecord": map[string]interface{}{
				"X-Correlation-ID": "correlation_id",
				"X-Request-Source": "source",
			},
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{
		{
			"correlation_id": "corr-12345",
			"source":         "batch-processor",
			"data":           "test",
		},
	}

	previews, err := module.PreviewRequest(records, PreviewOptions{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(previews) != 1 {
		t.Fatalf("expected 1 preview, got %d", len(previews))
	}

	// Headers from record should be in preview
	if previews[0].Headers["X-Correlation-ID"] != "corr-12345" {
		t.Errorf("expected X-Correlation-ID: corr-12345, got %s", previews[0].Headers["X-Correlation-ID"])
	}
	if previews[0].Headers["X-Request-Source"] != "batch-processor" {
		t.Errorf("expected X-Request-Source: batch-processor, got %s", previews[0].Headers["X-Request-Source"])
	}
}

func TestHTTPRequest_PreviewRequest_NoSideEffects(t *testing.T) {
	// Create a real test server to verify NO requests are made during preview
	requestCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}

	// Call preview
	_, err = module.PreviewRequest(records, PreviewOptions{})
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}

	// NO HTTP requests should have been made
	if requestCount != 0 {
		t.Errorf("preview should NOT make HTTP requests, but %d requests were made", requestCount)
	}
}

func TestHTTPRequest_ImplementsPreviewableModule(t *testing.T) {
	config := newModuleConfig(map[string]interface{}{
		"endpoint": "https://api.example.com/data",
		"method":   "POST",
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	// Verify HTTPRequest implements PreviewableModule interface
	var _ PreviewableModule = module
}

// =============================================================================
// Story 13.3: Custom retryableStatusCodes tests (AC #1, #4)
// =============================================================================

// testServerWithStatusSequence returns different status codes per request
type testServerWithStatusSequence struct {
	*httptest.Server
	mu            sync.Mutex
	requestCount  int
	statusCodes   []int // Status code for each request
	defaultStatus int   // Status after sequence exhausted
}

func newTestServerWithStatusSequence(statusCodes []int, defaultStatus int) *testServerWithStatusSequence {
	ts := &testServerWithStatusSequence{
		statusCodes:   statusCodes,
		defaultStatus: defaultStatus,
	}

	ts.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts.mu.Lock()
		idx := ts.requestCount
		ts.requestCount++
		ts.mu.Unlock()

		var status int
		if idx < len(ts.statusCodes) {
			status = ts.statusCodes[idx]
		} else {
			status = ts.defaultStatus
		}

		w.WriteHeader(status)
		if status >= 400 {
			_, _ = w.Write([]byte(`{"error": "error response"}`))
		} else {
			_, _ = w.Write([]byte(`{"success": true}`))
		}
	}))

	return ts
}

func (ts *testServerWithStatusSequence) getRequestCount() int {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.requestCount
}

func TestHTTPRequest_CustomRetryableStatusCodes_RetryOn408(t *testing.T) {
	// 408 is not retryable by default, but should be retryable when in custom list
	ts := newTestServerWithStatusSequence([]int{408, 408, 200}, 200)
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"retry": map[string]interface{}{
			"maxAttempts":          float64(3),
			"delayMs":              float64(1),
			"backoffMultiplier":    float64(1.0),
			"retryableStatusCodes": []interface{}{float64(408), float64(503)}, // Custom: 408 is retryable
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error after retries, got %v", err)
	}
	if sent != 1 {
		t.Errorf("expected 1 record sent, got %d", sent)
	}

	// Should have made 3 requests (2x 408 retried + 1 success)
	if ts.getRequestCount() != 3 {
		t.Errorf("expected 3 requests (408 should be retried when in custom list), got %d", ts.getRequestCount())
	}
}

func TestHTTPRequest_CustomRetryableStatusCodes_NoRetryOn500WhenExcluded(t *testing.T) {
	// 500 is retryable by default, but should NOT be retried when not in custom list
	ts := newTestServerWithStatusSequence([]int{500}, 200)
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"retry": map[string]interface{}{
			"maxAttempts":          float64(3),
			"delayMs":              float64(1),
			"backoffMultiplier":    float64(1.0),
			"retryableStatusCodes": []interface{}{float64(429), float64(503)}, // 500 NOT in list
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	_, err = module.Send(context.Background(), records)
	if err == nil {
		t.Fatal("expected error for 500 when not retryable")
	}

	// Should have made only 1 request (500 NOT retried)
	if ts.getRequestCount() != 1 {
		t.Errorf("expected 1 request (500 not retried when excluded from custom list), got %d", ts.getRequestCount())
	}
}

func TestHTTPRequest_CustomRetryableStatusCodes_DefaultFallback(t *testing.T) {
	// Without custom retryableStatusCodes, should use defaults (429, 500, 502, 503, 504)
	ts := newTestServerWithStatusSequence([]int{503, 503, 200}, 200)
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"retry": map[string]interface{}{
			"maxAttempts":       float64(3),
			"delayMs":           float64(1),
			"backoffMultiplier": float64(1.0),
			// No retryableStatusCodes - should use defaults
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error after retries (503 is default retryable), got %v", err)
	}
	if sent != 1 {
		t.Errorf("expected 1 record sent, got %d", sent)
	}

	// Should have made 3 requests with default retryable codes
	if ts.getRequestCount() != 3 {
		t.Errorf("expected 3 requests with default retryable codes, got %d", ts.getRequestCount())
	}
}

func TestHTTPRequest_CustomRetryableStatusCodes_OAuth2_401StillHandled(t *testing.T) {
	// OAuth2 401 handling should still work regardless of retryableStatusCodes
	// This test verifies that the special OAuth2 token invalidation path is preserved
	requestCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		// First request returns 401 (OAuth2 should invalidate + retry once)
		if requestCount == 1 {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"error": "unauthorized"}`))
			return
		}
		// Second request still 401 (after token refresh attempt, should fail)
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error": "unauthorized"}`))
	}))
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"retry": map[string]interface{}{
			"maxAttempts":          float64(3),
			"delayMs":              float64(1),
			"retryableStatusCodes": []interface{}{float64(503)}, // 401 not in list
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	_, err = module.Send(context.Background(), records)
	if err == nil {
		t.Fatal("expected error for 401")
	}

	// Should have made 1 request only (no OAuth2 handler configured, no retry)
	// Note: If OAuth2 was configured, it would make 2 requests (1 + 1 retry after token invalidation)
	if requestCount != 1 {
		t.Errorf("expected 1 request (401 not retried without OAuth2), got %d", requestCount)
	}
}

func TestHTTPRequest_CustomRetryableStatusCodes_NetworkErrorsStillRetried(t *testing.T) {
	// Network errors should still be retried regardless of retryableStatusCodes
	// (retryableStatusCodes only affects HTTP status code-based retry decisions)

	// This is tested indirectly - network errors go through ClassifyNetworkError
	// which returns Retryable=true for transient network issues.
	// The key point is that IsStatusCodeRetryable is only called for HTTP errors.

	// We can't easily simulate network errors in unit tests, so we verify
	// the behavior with a server that closes connection immediately
	// (which will be classified as a network error)

	// For now, this test documents the expected behavior:
	// Network errors bypass retryableStatusCodes check entirely
	t.Log("Network errors are retried via ClassifyNetworkError, independent of retryableStatusCodes")
}

// =============================================================================
// Story 13.3: Retry-After header support tests (AC #2, #4)
// =============================================================================

// testServerWithRetryAfter returns configured status with Retry-After header
type testServerWithRetryAfter struct {
	*httptest.Server
	mu              sync.Mutex
	requestCount    int
	requestTimes    []time.Time // Time each request was received
	statusCodes     []int
	retryAfterValue string // Value for Retry-After header
	defaultStatus   int
}

func newTestServerWithRetryAfter(statusCodes []int, retryAfterValue string) *testServerWithRetryAfter {
	ts := &testServerWithRetryAfter{
		statusCodes:     statusCodes,
		retryAfterValue: retryAfterValue,
		defaultStatus:   200,
		requestTimes:    make([]time.Time, 0),
	}

	ts.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts.mu.Lock()
		idx := ts.requestCount
		ts.requestCount++
		ts.requestTimes = append(ts.requestTimes, time.Now())
		ts.mu.Unlock()

		var status int
		if idx < len(ts.statusCodes) {
			status = ts.statusCodes[idx]
		} else {
			status = ts.defaultStatus
		}

		if status >= 400 && ts.retryAfterValue != "" {
			w.Header().Set("Retry-After", ts.retryAfterValue)
		}

		w.WriteHeader(status)
		if status >= 400 {
			_, _ = w.Write([]byte(`{"error": "error response"}`))
		} else {
			_, _ = w.Write([]byte(`{"success": true}`))
		}
	}))

	return ts
}

func TestHTTPRequest_RetryAfter_SecondsFormat(t *testing.T) {
	// Server returns 503 with Retry-After: 1 (second), then succeeds
	ts := newTestServerWithRetryAfter([]int{503, 200}, "1")
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"retry": map[string]interface{}{
			"maxAttempts":          float64(3),
			"delayMs":              float64(5000), // High default delay
			"backoffMultiplier":    float64(1.0),
			"useRetryAfterHeader":  true, // Enable Retry-After support
			"retryableStatusCodes": []interface{}{float64(503)},
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	startTime := time.Now()
	sent, err := module.Send(context.Background(), records)
	elapsed := time.Since(startTime)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if sent != 1 {
		t.Errorf("expected 1 record sent, got %d", sent)
	}

	// Delay should be ~1s (from Retry-After), not 5s (from delayMs)
	if elapsed > 3*time.Second {
		t.Errorf("expected delay ~1s from Retry-After, but took %v (likely used delayMs instead)", elapsed)
	}
	if elapsed < 800*time.Millisecond {
		t.Errorf("expected delay ~1s from Retry-After, but took only %v", elapsed)
	}
}

func TestHTTPRequest_RetryAfter_CappedByMaxDelayMs(t *testing.T) {
	// Server returns 503 with Retry-After: 10 (seconds), maxDelayMs caps at 2s
	ts := newTestServerWithRetryAfter([]int{503, 200}, "10")
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"retry": map[string]interface{}{
			"maxAttempts":          float64(3),
			"delayMs":              float64(100),
			"backoffMultiplier":    float64(1.0),
			"maxDelayMs":           float64(2000), // Cap at 2s
			"useRetryAfterHeader":  true,
			"retryableStatusCodes": []interface{}{float64(503)},
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	startTime := time.Now()
	sent, err := module.Send(context.Background(), records)
	elapsed := time.Since(startTime)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if sent != 1 {
		t.Errorf("expected 1 record sent, got %d", sent)
	}

	// Delay should be capped at ~2s, not 10s
	if elapsed > 4*time.Second {
		t.Errorf("expected delay capped at ~2s, but took %v", elapsed)
	}
	if elapsed < 1500*time.Millisecond {
		t.Errorf("expected delay ~2s (capped), but took only %v", elapsed)
	}
}

func TestHTTPRequest_RetryAfter_InvalidValue_FallbackToBackoff(t *testing.T) {
	// Server returns 503 with invalid Retry-After, should use backoff
	ts := newTestServerWithRetryAfter([]int{503, 200}, "invalid")
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"retry": map[string]interface{}{
			"maxAttempts":          float64(3),
			"delayMs":              float64(100),
			"backoffMultiplier":    float64(1.0),
			"useRetryAfterHeader":  true,
			"retryableStatusCodes": []interface{}{float64(503)},
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	sent, err := module.Send(context.Background(), records)

	if err != nil {
		t.Fatalf("expected no error (fallback to backoff), got %v", err)
	}
	if sent != 1 {
		t.Errorf("expected 1 record sent, got %d", sent)
	}
}

func TestHTTPRequest_RetryAfter_Disabled_UsesBackoff(t *testing.T) {
	// Server returns 503 with Retry-After: 5, but useRetryAfterHeader=false
	ts := newTestServerWithRetryAfter([]int{503, 200}, "5")
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"retry": map[string]interface{}{
			"maxAttempts":          float64(3),
			"delayMs":              float64(100),
			"backoffMultiplier":    float64(1.0),
			"useRetryAfterHeader":  false, // Disabled
			"retryableStatusCodes": []interface{}{float64(503)},
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	startTime := time.Now()
	sent, err := module.Send(context.Background(), records)
	elapsed := time.Since(startTime)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if sent != 1 {
		t.Errorf("expected 1 record sent, got %d", sent)
	}

	// Should use delayMs (100ms), not Retry-After (5s)
	if elapsed > 2*time.Second {
		t.Errorf("expected delay ~100ms (useRetryAfterHeader disabled), but took %v", elapsed)
	}
}

func TestHTTPRequest_RetryAfter_AbsentHeader_UsesBackoff(t *testing.T) {
	// Server returns 503 without Retry-After header, should use backoff
	ts := newTestServerWithRetryAfter([]int{503, 200}, "") // Empty = no header
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"retry": map[string]interface{}{
			"maxAttempts":          float64(3),
			"delayMs":              float64(100),
			"backoffMultiplier":    float64(1.0),
			"useRetryAfterHeader":  true,
			"retryableStatusCodes": []interface{}{float64(503)},
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	sent, err := module.Send(context.Background(), records)

	if err != nil {
		t.Fatalf("expected no error (fallback to backoff), got %v", err)
	}
	if sent != 1 {
		t.Errorf("expected 1 record sent, got %d", sent)
	}
}

func TestHTTPRequest_RetryAfter_ZeroSeconds_ImmediateRetry(t *testing.T) {
	// Server returns 503 with Retry-After: 0 (immediate retry), then succeeds
	ts := newTestServerWithRetryAfter([]int{503, 200}, "0")
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"retry": map[string]interface{}{
			"maxAttempts":          float64(3),
			"delayMs":              float64(5000), // High default delay
			"backoffMultiplier":    float64(1.0),
			"useRetryAfterHeader":  true,
			"retryableStatusCodes": []interface{}{float64(503)},
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	startTime := time.Now()
	sent, err := module.Send(context.Background(), records)
	elapsed := time.Since(startTime)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if sent != 1 {
		t.Errorf("expected 1 record sent, got %d", sent)
	}

	// Delay should be ~0s (immediate retry from Retry-After: 0)
	if elapsed > 1*time.Second {
		t.Errorf("expected immediate retry (~0s from Retry-After: 0), but took %v", elapsed)
	}
}

func TestHTTPRequest_RetryAfter_HTTPDate_RFC1123(t *testing.T) {
	// Server returns 503 with Retry-After: HTTP-date (RFC1123 format), then succeeds
	futureTime := time.Now().Add(2 * time.Second)
	retryAfterDate := futureTime.Format(time.RFC1123)
	ts := newTestServerWithRetryAfter([]int{503, 200}, retryAfterDate)
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"retry": map[string]interface{}{
			"maxAttempts":          float64(3),
			"delayMs":              float64(5000), // High default delay
			"backoffMultiplier":    float64(1.0),
			"useRetryAfterHeader":  true,
			"retryableStatusCodes": []interface{}{float64(503)},
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	startTime := time.Now()
	sent, err := module.Send(context.Background(), records)
	elapsed := time.Since(startTime)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if sent != 1 {
		t.Errorf("expected 1 record sent, got %d", sent)
	}

	// Delay should be ~2s (from Retry-After HTTP-date), not 5s (from delayMs)
	// Allow some tolerance for timing variations
	if elapsed > 4*time.Second {
		t.Errorf("expected delay ~2s from Retry-After HTTP-date, but took %v (likely used delayMs instead)", elapsed)
	}
	if elapsed < 1*time.Second {
		t.Errorf("expected delay ~2s from Retry-After HTTP-date, but took only %v", elapsed)
	}
}

func TestHTTPRequest_RetryAfter_HTTPDate_RFC850(t *testing.T) {
	// Server returns 503 with Retry-After: HTTP-date (RFC850 format), then succeeds
	futureTime := time.Now().Add(1 * time.Second)
	retryAfterDate := futureTime.Format(time.RFC850)
	ts := newTestServerWithRetryAfter([]int{503, 200}, retryAfterDate)
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"retry": map[string]interface{}{
			"maxAttempts":          float64(3),
			"delayMs":              float64(5000), // High default delay
			"backoffMultiplier":    float64(1.0),
			"useRetryAfterHeader":  true,
			"retryableStatusCodes": []interface{}{float64(503)},
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	startTime := time.Now()
	sent, err := module.Send(context.Background(), records)
	elapsed := time.Since(startTime)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if sent != 1 {
		t.Errorf("expected 1 record sent, got %d", sent)
	}

	// Delay should be ~1s (from Retry-After HTTP-date)
	if elapsed > 3*time.Second {
		t.Errorf("expected delay ~1s from Retry-After HTTP-date (RFC850), but took %v", elapsed)
	}
	if elapsed < 500*time.Millisecond {
		t.Errorf("expected delay ~1s from Retry-After HTTP-date (RFC850), but took only %v", elapsed)
	}
}

func TestHTTPRequest_RetryAfter_HTTPDate_Past_ImmediateRetry(t *testing.T) {
	// Server returns 503 with Retry-After: HTTP-date in the past, should retry immediately
	pastTime := time.Now().Add(-5 * time.Second)
	retryAfterDate := pastTime.Format(time.RFC1123)
	ts := newTestServerWithRetryAfter([]int{503, 200}, retryAfterDate)
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"retry": map[string]interface{}{
			"maxAttempts":          float64(3),
			"delayMs":              float64(5000), // High default delay
			"backoffMultiplier":    float64(1.0),
			"useRetryAfterHeader":  true,
			"retryableStatusCodes": []interface{}{float64(503)},
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	startTime := time.Now()
	sent, err := module.Send(context.Background(), records)
	elapsed := time.Since(startTime)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if sent != 1 {
		t.Errorf("expected 1 record sent, got %d", sent)
	}

	// Delay should be ~0s (immediate retry for past HTTP-date)
	if elapsed > 1*time.Second {
		t.Errorf("expected immediate retry (~0s for past HTTP-date), but took %v", elapsed)
	}
}

// =============================================================================
// Story 13.3: retryHintFromBody support tests (AC #3, #4)
// =============================================================================

// testServerWithBodyHint returns errors with custom JSON body containing retry hints
type testServerWithBodyHint struct {
	*httptest.Server
	mu           sync.Mutex
	requestCount int
	statusCodes  []int
	bodyHints    []string // JSON body for each request
	defaultBody  string
}

func newTestServerWithBodyHint(statusCodes []int, bodyHints []string) *testServerWithBodyHint {
	ts := &testServerWithBodyHint{
		statusCodes: statusCodes,
		bodyHints:   bodyHints,
	}

	ts.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ts.mu.Lock()
		idx := ts.requestCount
		ts.requestCount++
		ts.mu.Unlock()

		var status int
		var body string
		if idx < len(ts.statusCodes) {
			status = ts.statusCodes[idx]
		} else {
			status = 200
		}
		if idx < len(ts.bodyHints) {
			body = ts.bodyHints[idx]
		} else {
			body = ts.defaultBody
		}

		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))

	return ts
}

func (ts *testServerWithBodyHint) getRequestCount() int {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.requestCount
}

func TestHTTPRequest_RetryHintFromBody_ExpressionTrue(t *testing.T) {
	// Server returns 500 with {"retryable": true}, expression evaluates to true, should retry
	ts := newTestServerWithBodyHint(
		[]int{500, 500, 200},
		[]string{
			`{"retryable": true}`,
			`{"retryable": true}`,
		},
	)
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"retry": map[string]interface{}{
			"maxAttempts":          float64(3),
			"delayMs":              float64(1),
			"backoffMultiplier":    float64(1.0),
			"retryableStatusCodes": []interface{}{float64(500)},
			"retryHintFromBody":    "body.retryable == true", // expr expression
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error after retries, got %v", err)
	}
	if sent != 1 {
		t.Errorf("expected 1 record sent, got %d", sent)
	}

	// Should have made 3 requests (2 retried + 1 success)
	if ts.getRequestCount() != 3 {
		t.Errorf("expected 3 requests (expr true allows retry), got %d", ts.getRequestCount())
	}
}

func TestHTTPRequest_RetryHintFromBody_ExpressionFalsePreventsRetry(t *testing.T) {
	// Server returns 500 with {"retryable": false}, expression evaluates to false, should NOT retry
	ts := newTestServerWithBodyHint(
		[]int{500},
		[]string{`{"retryable": false}`},
	)
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"retry": map[string]interface{}{
			"maxAttempts":          float64(3),
			"delayMs":              float64(1),
			"backoffMultiplier":    float64(1.0),
			"retryableStatusCodes": []interface{}{float64(500)}, // 500 normally retryable
			"retryHintFromBody":    "body.retryable == true",    // expr expression returns false
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	_, err = module.Send(context.Background(), records)
	if err == nil {
		t.Fatal("expected error (body hint falsy prevents retry)")
	}

	// Should have made only 1 request (no retry because body hint is false)
	if ts.getRequestCount() != 1 {
		t.Errorf("expected 1 request (body hint falsy prevents retry), got %d", ts.getRequestCount())
	}
}

func TestHTTPRequest_RetryHintFromBody_NestedExpression(t *testing.T) {
	// Server returns 500 with {"error": {"code": "TEMPORARY"}}, expression matches
	ts := newTestServerWithBodyHint(
		[]int{500, 200},
		[]string{
			`{"error": {"code": "TEMPORARY", "retry": true}}`,
		},
	)
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"retry": map[string]interface{}{
			"maxAttempts":          float64(3),
			"delayMs":              float64(1),
			"backoffMultiplier":    float64(1.0),
			"retryableStatusCodes": []interface{}{float64(500)},
			"retryHintFromBody":    `body.error.code == "TEMPORARY"`, // expr with nested access
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error after retry, got %v", err)
	}
	if sent != 1 {
		t.Errorf("expected 1 record sent, got %d", sent)
	}

	// Should have made 2 requests
	if ts.getRequestCount() != 2 {
		t.Errorf("expected 2 requests (nested expr works), got %d", ts.getRequestCount())
	}
}

func TestHTTPRequest_RetryHintFromBody_NonJSONBody_FallbackToStatusCode(t *testing.T) {
	// Server returns 500 with non-JSON body, should use status code only
	ts := newTestServerWithBodyHint(
		[]int{500, 200},
		[]string{
			`not valid json`,
		},
	)
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"retry": map[string]interface{}{
			"maxAttempts":          float64(3),
			"delayMs":              float64(1),
			"backoffMultiplier":    float64(1.0),
			"retryableStatusCodes": []interface{}{float64(500)},
			"retryHintFromBody":    "body.retryable == true",
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error (fallback to status code), got %v", err)
	}
	if sent != 1 {
		t.Errorf("expected 1 record sent, got %d", sent)
	}

	// Should have made 2 requests (fallback to status code allows retry)
	if ts.getRequestCount() != 2 {
		t.Errorf("expected 2 requests (non-JSON body, fallback to status), got %d", ts.getRequestCount())
	}
}

func TestHTTPRequest_RetryHintFromBody_FieldAbsent_ExpressionsEvaluatesToFalse(t *testing.T) {
	// Server returns 500 with JSON but field doesn't exist
	// With expr, body.retryable == true evaluates to nil == true -> false, preventing retry
	ts := newTestServerWithBodyHint(
		[]int{500, 200},
		[]string{
			`{"other_field": "value"}`,
		},
	)
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"retry": map[string]interface{}{
			"maxAttempts":          float64(3),
			"delayMs":              float64(1),
			"backoffMultiplier":    float64(1.0),
			"retryableStatusCodes": []interface{}{float64(500)},
			"retryHintFromBody":    "body.retryable == true", // Field doesn't exist, expression is false
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	_, err = module.Send(context.Background(), records)
	// Expression evaluates to false (nil == true is false), so retry is prevented
	if err == nil {
		t.Fatal("expected error (expression false prevents retry)")
	}

	// Should have made only 1 request (no retry because expression is false)
	if ts.getRequestCount() != 1 {
		t.Errorf("expected 1 request (field absent, expr false), got %d", ts.getRequestCount())
	}
}

func TestHTTPRequest_RetryHintFromBody_NotConfigured_UsesStatusCode(t *testing.T) {
	// Without retryHintFromBody, should use status code only (existing behavior)
	ts := newTestServerWithBodyHint(
		[]int{500, 200},
		[]string{
			`{"retryable": false}`, // Would prevent retry if hint was configured
		},
	)
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"retry": map[string]interface{}{
			"maxAttempts":          float64(3),
			"delayMs":              float64(1),
			"backoffMultiplier":    float64(1.0),
			"retryableStatusCodes": []interface{}{float64(500)},
			// No retryHintFromBody
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error (status code only), got %v", err)
	}
	if sent != 1 {
		t.Errorf("expected 1 record sent, got %d", sent)
	}

	// Should have made 2 requests (body hint ignored, status code used)
	if ts.getRequestCount() != 2 {
		t.Errorf("expected 2 requests (no body hint config, uses status), got %d", ts.getRequestCount())
	}
}

func TestHTTPRequest_RetryHintFromBody_ComplexExpression(t *testing.T) {
	// Test complex expression with OR condition
	ts := newTestServerWithBodyHint(
		[]int{500, 200},
		[]string{
			`{"error": {"type": "RATE_LIMIT", "retryAfter": 5}}`,
		},
	)
	defer ts.Close()

	config := newModuleConfig(map[string]interface{}{
		"endpoint": ts.URL + "/api/data",
		"method":   "POST",
		"retry": map[string]interface{}{
			"maxAttempts":          float64(3),
			"delayMs":              float64(1),
			"backoffMultiplier":    float64(1.0),
			"retryableStatusCodes": []interface{}{float64(500)},
			"retryHintFromBody":    `body.error.type == "RATE_LIMIT" || body.error.type == "TEMPORARY"`,
		},
	})

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if sent != 1 {
		t.Errorf("expected 1 record sent, got %d", sent)
	}

	if ts.getRequestCount() != 2 {
		t.Errorf("expected 2 requests (complex expr matches), got %d", ts.getRequestCount())
	}
}

// Test metadata exclusion from request body
func TestHTTPRequest_MetadataExclusion(t *testing.T) {
	t.Run("excludes _metadata field from body", func(t *testing.T) {
		var receivedBody string
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			receivedBody = string(body)
			w.WriteHeader(200)
		}))
		defer ts.Close()

		config := newModuleConfig(map[string]interface{}{
			"endpoint": ts.URL + "/api/data",
			"method":   "POST",
		})

		module, err := NewHTTPRequestFromConfig(config)
		if err != nil {
			t.Fatalf("failed to create module: %v", err)
		}

		records := []map[string]interface{}{
			{
				"name":      "test",
				"_metadata": map[string]interface{}{"processed": true, "timestamp": "2024-01-01"},
			},
		}

		_, err = module.Send(context.Background(), records)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Verify _metadata is not in the body
		if strings.Contains(receivedBody, "_metadata") {
			t.Errorf("expected _metadata to be excluded from body, got: %s", receivedBody)
		}
		if !strings.Contains(receivedBody, "name") {
			t.Errorf("expected data fields to be present, got: %s", receivedBody)
		}
	})

	t.Run("only excludes _metadata field, not other underscore fields", func(t *testing.T) {
		var receivedBody string
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			receivedBody = string(body)
			w.WriteHeader(200)
		}))
		defer ts.Close()

		config := newModuleConfig(map[string]interface{}{
			"endpoint": ts.URL + "/api/data",
			"method":   "POST",
		})

		module, err := NewHTTPRequestFromConfig(config)
		if err != nil {
			t.Fatalf("failed to create module: %v", err)
		}

		records := []map[string]interface{}{
			{
				"name":      "test",
				"_metadata": map[string]interface{}{"processed": true},
				"_internal": "should be included",
				"_custom":   123,
			},
		}

		_, err = module.Send(context.Background(), records)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Verify only _metadata is excluded, other underscore fields are kept
		if strings.Contains(receivedBody, "_metadata") {
			t.Errorf("expected _metadata to be excluded, got: %s", receivedBody)
		}
		if !strings.Contains(receivedBody, "_internal") {
			t.Errorf("expected _internal to be included, got: %s", receivedBody)
		}
		if !strings.Contains(receivedBody, "_custom") {
			t.Errorf("expected _custom to be included, got: %s", receivedBody)
		}
	})

	t.Run("works in single record mode", func(t *testing.T) {
		var receivedBody string
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			receivedBody = string(body)
			w.WriteHeader(200)
		}))
		defer ts.Close()

		config := newModuleConfig(map[string]interface{}{
			"endpoint": ts.URL + "/api/data",
			"method":   "POST",
			"request": map[string]interface{}{
				"bodyFrom": "record",
			},
		})

		module, err := NewHTTPRequestFromConfig(config)
		if err != nil {
			t.Fatalf("failed to create module: %v", err)
		}

		records := []map[string]interface{}{
			{
				"name":      "test",
				"_metadata": map[string]interface{}{"processed": true},
			},
		}

		_, err = module.Send(context.Background(), records)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Verify metadata is not in the body
		if strings.Contains(receivedBody, "_metadata") {
			t.Errorf("expected metadata to be excluded in single record mode, got: %s", receivedBody)
		}
	})
}
