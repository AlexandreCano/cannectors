// Package input provides implementations for input modules.
package input

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/canectors/runtime/pkg/connector"
)

// =============================================================================
// Task 1: HTTP GET Request Execution Tests
// =============================================================================

// TestHTTPPolling_Fetch_SuccessfulGET tests basic HTTP GET request execution.
func TestHTTPPolling_Fetch_SuccessfulGET(t *testing.T) {
	// Setup: Create test server returning JSON array
	expected := []map[string]interface{}{
		{"id": float64(1), "name": "item1"},
		{"id": float64(2), "name": "item2"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET method, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(expected)
	}))
	defer server.Close()

	// Create module configuration
	config := &connector.ModuleConfig{
		Type: "http-polling",
		Config: map[string]interface{}{
			"endpoint": server.URL,
		},
	}

	// Execute
	polling, err := NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig failed: %v", err)
	}

	records, err := polling.Fetch()

	// Verify
	if err != nil {
		t.Fatalf("Fetch() returned error: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("expected 2 records, got %d", len(records))
	}
	if records[0]["name"] != "item1" {
		t.Errorf("expected first record name 'item1', got '%v'", records[0]["name"])
	}
}

// TestHTTPPolling_Fetch_JSONArrayResponse tests parsing JSON array response.
func TestHTTPPolling_Fetch_JSONArrayResponse(t *testing.T) {
	expected := []map[string]interface{}{
		{"field": "value1"},
		{"field": "value2"},
		{"field": "value3"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(expected)
	}))
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "http-polling",
		Config: map[string]interface{}{
			"endpoint": server.URL,
		},
	}

	polling, err := NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig failed: %v", err)
	}

	records, err := polling.Fetch()

	if err != nil {
		t.Fatalf("Fetch() returned error: %v", err)
	}
	if len(records) != 3 {
		t.Errorf("expected 3 records, got %d", len(records))
	}
}

// TestHTTPPolling_Fetch_JSONObjectWithArrayField tests parsing JSON object containing array.
func TestHTTPPolling_Fetch_JSONObjectWithArrayField(t *testing.T) {
	responseBody := map[string]interface{}{
		"data": []map[string]interface{}{
			{"id": float64(1)},
			{"id": float64(2)},
		},
		"meta": map[string]interface{}{
			"total": float64(2),
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(responseBody)
	}))
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "http-polling",
		Config: map[string]interface{}{
			"endpoint":  server.URL,
			"dataField": "data",
		},
	}

	polling, err := NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig failed: %v", err)
	}

	records, err := polling.Fetch()

	if err != nil {
		t.Fatalf("Fetch() returned error: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("expected 2 records from 'data' field, got %d", len(records))
	}
}

// TestHTTPPolling_Fetch_EmptyResponse tests handling of empty response.
func TestHTTPPolling_Fetch_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]interface{}{})
	}))
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "http-polling",
		Config: map[string]interface{}{
			"endpoint": server.URL,
		},
	}

	polling, err := NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig failed: %v", err)
	}

	records, err := polling.Fetch()

	if err != nil {
		t.Fatalf("Fetch() returned error: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("expected 0 records for empty response, got %d", len(records))
	}
}

// TestHTTPPolling_Fetch_CustomHeaders tests custom headers are sent.
func TestHTTPPolling_Fetch_CustomHeaders(t *testing.T) {
	receivedHeaders := make(map[string]string)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders["X-Custom-Header"] = r.Header.Get("X-Custom-Header")
		receivedHeaders["Accept"] = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]interface{}{})
	}))
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "http-polling",
		Config: map[string]interface{}{
			"endpoint": server.URL,
			"headers": map[string]interface{}{
				"X-Custom-Header": "custom-value",
				"Accept":          "application/json",
			},
		},
	}

	polling, err := NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig failed: %v", err)
	}

	_, err = polling.Fetch()
	if err != nil {
		t.Fatalf("Fetch() returned error: %v", err)
	}

	if receivedHeaders["X-Custom-Header"] != "custom-value" {
		t.Errorf("expected X-Custom-Header 'custom-value', got '%s'", receivedHeaders["X-Custom-Header"])
	}
}

// TestHTTPPolling_Fetch_ConfigurableTimeout tests timeout configuration.
func TestHTTPPolling_Fetch_ConfigurableTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Delay response longer than timeout
		time.Sleep(200 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]interface{}{})
	}))
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "http-polling",
		Config: map[string]interface{}{
			"endpoint": server.URL,
			"timeout":  float64(0.1), // 100ms timeout
		},
	}

	polling, err := NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig failed: %v", err)
	}

	_, err = polling.Fetch()

	if err == nil {
		t.Error("expected timeout error, got nil")
	}
}

// TestHTTPPolling_NewHTTPPollingFromConfig_MissingEndpoint tests validation of required fields.
func TestHTTPPolling_NewHTTPPollingFromConfig_MissingEndpoint(t *testing.T) {
	config := &connector.ModuleConfig{
		Type:   "http-polling",
		Config: map[string]interface{}{},
	}

	_, err := NewHTTPPollingFromConfig(config)

	if err == nil {
		t.Error("expected error for missing endpoint, got nil")
	}
}

// TestHTTPPolling_NewHTTPPollingFromConfig_NilConfig tests nil config handling.
func TestHTTPPolling_NewHTTPPollingFromConfig_NilConfig(t *testing.T) {
	_, err := NewHTTPPollingFromConfig(nil)

	if err == nil {
		t.Error("expected error for nil config, got nil")
	}
}

// =============================================================================
// Task 2: Authentication Tests
// =============================================================================

// TestHTTPPolling_Fetch_APIKeyHeader tests API key authentication in header.
func TestHTTPPolling_Fetch_APIKeyHeader(t *testing.T) {
	var receivedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]interface{}{{"status": "ok"}})
	}))
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "http-polling",
		Config: map[string]interface{}{
			"endpoint": server.URL,
		},
		Authentication: &connector.AuthConfig{
			Type: "api-key",
			Credentials: map[string]string{
				"key":      "my-secret-key",
				"location": "header",
			},
		},
	}

	polling, err := NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig failed: %v", err)
	}

	_, err = polling.Fetch()
	if err != nil {
		t.Fatalf("Fetch() returned error: %v", err)
	}

	expectedAuth := "Bearer my-secret-key"
	if receivedAuth != expectedAuth {
		t.Errorf("expected Authorization header '%s', got '%s'", expectedAuth, receivedAuth)
	}
}

// TestHTTPPolling_Fetch_APIKeyQuery tests API key authentication in query parameter.
func TestHTTPPolling_Fetch_APIKeyQuery(t *testing.T) {
	var receivedAPIKey string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAPIKey = r.URL.Query().Get("api_key")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]interface{}{{"status": "ok"}})
	}))
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "http-polling",
		Config: map[string]interface{}{
			"endpoint": server.URL,
		},
		Authentication: &connector.AuthConfig{
			Type: "api-key",
			Credentials: map[string]string{
				"key":       "my-secret-key",
				"location":  "query",
				"paramName": "api_key",
			},
		},
	}

	polling, err := NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig failed: %v", err)
	}

	_, err = polling.Fetch()
	if err != nil {
		t.Fatalf("Fetch() returned error: %v", err)
	}

	if receivedAPIKey != "my-secret-key" {
		t.Errorf("expected api_key query param 'my-secret-key', got '%s'", receivedAPIKey)
	}
}

// TestHTTPPolling_Fetch_OAuth2ClientCredentials tests OAuth2 client credentials flow.
func TestHTTPPolling_Fetch_OAuth2ClientCredentials(t *testing.T) {
	// Token server
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("token endpoint: expected POST, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("token endpoint: failed to parse form: %v", err)
		}
		if r.FormValue("grant_type") != "client_credentials" {
			t.Errorf("token endpoint: expected grant_type 'client_credentials', got '%s'", r.FormValue("grant_type"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "test-access-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer tokenServer.Close()

	var receivedAuth string
	// API server
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]interface{}{{"data": "value"}})
	}))
	defer apiServer.Close()

	config := &connector.ModuleConfig{
		Type: "http-polling",
		Config: map[string]interface{}{
			"endpoint": apiServer.URL,
		},
		Authentication: &connector.AuthConfig{
			Type: "oauth2",
			Credentials: map[string]string{
				"clientId":     "test-client-id",
				"clientSecret": "test-client-secret",
				"tokenUrl":     tokenServer.URL,
			},
		},
	}

	polling, err := NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig failed: %v", err)
	}

	_, err = polling.Fetch()
	if err != nil {
		t.Fatalf("Fetch() returned error: %v", err)
	}

	expectedAuth := "Bearer test-access-token"
	if receivedAuth != expectedAuth {
		t.Errorf("expected Authorization header '%s', got '%s'", expectedAuth, receivedAuth)
	}
}

// TestHTTPPolling_Fetch_BearerToken tests bearer token authentication.
func TestHTTPPolling_Fetch_BearerToken(t *testing.T) {
	var receivedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]interface{}{{"status": "ok"}})
	}))
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "http-polling",
		Config: map[string]interface{}{
			"endpoint": server.URL,
		},
		Authentication: &connector.AuthConfig{
			Type: "bearer",
			Credentials: map[string]string{
				"token": "my-bearer-token",
			},
		},
	}

	polling, err := NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig failed: %v", err)
	}

	_, err = polling.Fetch()
	if err != nil {
		t.Fatalf("Fetch() returned error: %v", err)
	}

	expectedAuth := "Bearer my-bearer-token"
	if receivedAuth != expectedAuth {
		t.Errorf("expected Authorization header '%s', got '%s'", expectedAuth, receivedAuth)
	}
}

// TestHTTPPolling_Fetch_BasicAuth tests basic authentication.
func TestHTTPPolling_Fetch_BasicAuth(t *testing.T) {
	var receivedUser, receivedPass string
	var authOK bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUser, receivedPass, authOK = r.BasicAuth()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]interface{}{{"status": "ok"}})
	}))
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "http-polling",
		Config: map[string]interface{}{
			"endpoint": server.URL,
		},
		Authentication: &connector.AuthConfig{
			Type: "basic",
			Credentials: map[string]string{
				"username": "testuser",
				"password": "testpass",
			},
		},
	}

	polling, err := NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig failed: %v", err)
	}

	_, err = polling.Fetch()
	if err != nil {
		t.Fatalf("Fetch() returned error: %v", err)
	}

	if !authOK {
		t.Error("basic auth was not sent")
	}
	if receivedUser != "testuser" {
		t.Errorf("expected username 'testuser', got '%s'", receivedUser)
	}
	if receivedPass != "testpass" {
		t.Errorf("expected password 'testpass', got '%s'", receivedPass)
	}
}

// TestHTTPPolling_Fetch_AuthenticationError tests handling of authentication errors.
func TestHTTPPolling_Fetch_AuthenticationError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "unauthorized",
		})
	}))
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "http-polling",
		Config: map[string]interface{}{
			"endpoint": server.URL,
		},
		Authentication: &connector.AuthConfig{
			Type: "api-key",
			Credentials: map[string]string{
				"key":      "invalid-key",
				"location": "header",
			},
		},
	}

	polling, err := NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig failed: %v", err)
	}

	_, err = polling.Fetch()

	if err == nil {
		t.Error("expected authentication error, got nil")
	}
}

// =============================================================================
// Task 3: Pagination Tests
// =============================================================================

// TestHTTPPolling_Fetch_PageBasedPagination tests page-based pagination.
func TestHTTPPolling_Fetch_PageBasedPagination(t *testing.T) {
	pageRequests := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pageRequests++
		page := r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "application/json")

		switch page {
		case "", "1":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data":        []map[string]interface{}{{"id": float64(1)}, {"id": float64(2)}},
				"total_pages": float64(3),
			})
		case "2":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data":        []map[string]interface{}{{"id": float64(3)}, {"id": float64(4)}},
				"total_pages": float64(3),
			})
		case "3":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data":        []map[string]interface{}{{"id": float64(5)}},
				"total_pages": float64(3),
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data":        []map[string]interface{}{},
				"total_pages": float64(3),
			})
		}
	}))
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "http-polling",
		Config: map[string]interface{}{
			"endpoint":  server.URL,
			"dataField": "data",
			"pagination": map[string]interface{}{
				"type":            "page",
				"pageParam":       "page",
				"totalPagesField": "total_pages",
			},
		},
	}

	polling, err := NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig failed: %v", err)
	}

	records, err := polling.Fetch()

	if err != nil {
		t.Fatalf("Fetch() returned error: %v", err)
	}
	if len(records) != 5 {
		t.Errorf("expected 5 records from pagination, got %d", len(records))
	}
	if pageRequests != 3 {
		t.Errorf("expected 3 page requests, got %d", pageRequests)
	}
}

// TestHTTPPolling_Fetch_OffsetBasedPagination tests offset-based pagination.
func TestHTTPPolling_Fetch_OffsetBasedPagination(t *testing.T) {
	offsetRequests := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		offsetRequests++
		offset := r.URL.Query().Get("offset")
		w.Header().Set("Content-Type", "application/json")

		switch offset {
		case "", "0":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"items": []map[string]interface{}{{"id": float64(1)}, {"id": float64(2)}},
				"total": float64(5),
			})
		case "2":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"items": []map[string]interface{}{{"id": float64(3)}, {"id": float64(4)}},
				"total": float64(5),
			})
		case "4":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"items": []map[string]interface{}{{"id": float64(5)}},
				"total": float64(5),
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"items": []map[string]interface{}{},
				"total": float64(5),
			})
		}
	}))
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "http-polling",
		Config: map[string]interface{}{
			"endpoint":  server.URL,
			"dataField": "items",
			"pagination": map[string]interface{}{
				"type":        "offset",
				"offsetParam": "offset",
				"limitParam":  "limit",
				"limit":       float64(2),
				"totalField":  "total",
			},
		},
	}

	polling, err := NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig failed: %v", err)
	}

	records, err := polling.Fetch()

	if err != nil {
		t.Fatalf("Fetch() returned error: %v", err)
	}
	if len(records) != 5 {
		t.Errorf("expected 5 records from offset pagination, got %d", len(records))
	}
}

// TestHTTPPolling_Fetch_CursorBasedPagination tests cursor-based pagination.
func TestHTTPPolling_Fetch_CursorBasedPagination(t *testing.T) {
	cursorRequests := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cursorRequests++
		cursor := r.URL.Query().Get("cursor")
		w.Header().Set("Content-Type", "application/json")

		switch cursor {
		case "":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"results":     []map[string]interface{}{{"id": float64(1)}, {"id": float64(2)}},
				"next_cursor": "cursor_page2",
			})
		case "cursor_page2":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"results":     []map[string]interface{}{{"id": float64(3)}},
				"next_cursor": "",
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"results":     []map[string]interface{}{},
				"next_cursor": "",
			})
		}
	}))
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "http-polling",
		Config: map[string]interface{}{
			"endpoint":  server.URL,
			"dataField": "results",
			"pagination": map[string]interface{}{
				"type":            "cursor",
				"cursorParam":     "cursor",
				"nextCursorField": "next_cursor",
			},
		},
	}

	polling, err := NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig failed: %v", err)
	}

	records, err := polling.Fetch()

	if err != nil {
		t.Fatalf("Fetch() returned error: %v", err)
	}
	if len(records) != 3 {
		t.Errorf("expected 3 records from cursor pagination, got %d", len(records))
	}
	if cursorRequests != 2 {
		t.Errorf("expected 2 cursor requests, got %d", cursorRequests)
	}
}

// =============================================================================
// Task 4: Error Handling Tests
// =============================================================================

// TestHTTPPolling_Fetch_HTTPError400 tests handling of 400 Bad Request.
func TestHTTPPolling_Fetch_HTTPError400(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "bad request"})
	}))
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "http-polling",
		Config: map[string]interface{}{
			"endpoint": server.URL,
		},
	}

	polling, err := NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig failed: %v", err)
	}

	_, err = polling.Fetch()

	if err == nil {
		t.Error("expected error for HTTP 400, got nil")
	}
}

// TestHTTPPolling_Fetch_HTTPError401 tests handling of 401 Unauthorized.
func TestHTTPPolling_Fetch_HTTPError401(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "http-polling",
		Config: map[string]interface{}{
			"endpoint": server.URL,
		},
	}

	polling, err := NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig failed: %v", err)
	}

	_, err = polling.Fetch()

	if err == nil {
		t.Error("expected error for HTTP 401, got nil")
	}
}

// TestHTTPPolling_Fetch_HTTPError403 tests handling of 403 Forbidden.
func TestHTTPPolling_Fetch_HTTPError403(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "http-polling",
		Config: map[string]interface{}{
			"endpoint": server.URL,
		},
	}

	polling, err := NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig failed: %v", err)
	}

	_, err = polling.Fetch()

	if err == nil {
		t.Error("expected error for HTTP 403, got nil")
	}
}

// TestHTTPPolling_Fetch_HTTPError404 tests handling of 404 Not Found.
func TestHTTPPolling_Fetch_HTTPError404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "http-polling",
		Config: map[string]interface{}{
			"endpoint": server.URL,
		},
	}

	polling, err := NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig failed: %v", err)
	}

	_, err = polling.Fetch()

	if err == nil {
		t.Error("expected error for HTTP 404, got nil")
	}
}

// TestHTTPPolling_Fetch_HTTPError500 tests handling of 500 Internal Server Error.
func TestHTTPPolling_Fetch_HTTPError500(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "http-polling",
		Config: map[string]interface{}{
			"endpoint": server.URL,
		},
	}

	polling, err := NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig failed: %v", err)
	}

	_, err = polling.Fetch()

	if err == nil {
		t.Error("expected error for HTTP 500, got nil")
	}
}

// TestHTTPPolling_Fetch_NetworkError tests handling of network errors.
func TestHTTPPolling_Fetch_NetworkError(t *testing.T) {
	config := &connector.ModuleConfig{
		Type: "http-polling",
		Config: map[string]interface{}{
			"endpoint": "http://localhost:99999/nonexistent",
			"timeout":  float64(1),
		},
	}

	polling, err := NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig failed: %v", err)
	}

	_, err = polling.Fetch()

	if err == nil {
		t.Error("expected network error, got nil")
	}
}

// TestHTTPPolling_Fetch_JSONParseError tests handling of invalid JSON response.
func TestHTTPPolling_Fetch_JSONParseError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("invalid json {{{"))
	}))
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "http-polling",
		Config: map[string]interface{}{
			"endpoint": server.URL,
		},
	}

	polling, err := NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig failed: %v", err)
	}

	_, err = polling.Fetch()

	if err == nil {
		t.Error("expected JSON parse error, got nil")
	}
}

// =============================================================================
// Task 5: Deterministic Execution Tests
// =============================================================================

// TestHTTPPolling_Fetch_DeterministicOutput tests same input = same output.
func TestHTTPPolling_Fetch_DeterministicOutput(t *testing.T) {
	data := []map[string]interface{}{
		{"id": float64(1), "name": "a"},
		{"id": float64(2), "name": "b"},
		{"id": float64(3), "name": "c"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(data)
	}))
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "http-polling",
		Config: map[string]interface{}{
			"endpoint": server.URL,
		},
	}

	// Execute multiple times
	results := make([][]map[string]interface{}, 3)
	for i := 0; i < 3; i++ {
		polling, err := NewHTTPPollingFromConfig(config)
		if err != nil {
			t.Fatalf("NewHTTPPollingFromConfig failed: %v", err)
		}
		results[i], err = polling.Fetch()
		if err != nil {
			t.Fatalf("Fetch() %d returned error: %v", i, err)
		}

	}

	// Verify all results are identical
	for i := 1; i < len(results); i++ {
		if len(results[i]) != len(results[0]) {
			t.Errorf("result %d has different length: %d vs %d", i, len(results[i]), len(results[0]))
		}
		for j := range results[0] {
			if results[i][j]["id"] != results[0][j]["id"] {
				t.Errorf("result %d[%d] differs from result 0[%d]", i, j, j)
			}
		}
	}
}

// TestHTTPPolling_Fetch_DeterministicPaginationOrder tests pagination order is consistent.
func TestHTTPPolling_Fetch_DeterministicPaginationOrder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "application/json")

		switch page {
		case "", "1":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data":        []map[string]interface{}{{"order": float64(1)}, {"order": float64(2)}},
				"total_pages": float64(2),
			})
		case "2":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data":        []map[string]interface{}{{"order": float64(3)}, {"order": float64(4)}},
				"total_pages": float64(2),
			})
		}
	}))
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "http-polling",
		Config: map[string]interface{}{
			"endpoint":  server.URL,
			"dataField": "data",
			"pagination": map[string]interface{}{
				"type":            "page",
				"pageParam":       "page",
				"totalPagesField": "total_pages",
			},
		},
	}

	// Execute multiple times
	for i := 0; i < 3; i++ {
		polling, err := NewHTTPPollingFromConfig(config)
		if err != nil {
			t.Fatalf("NewHTTPPollingFromConfig failed: %v", err)
		}
		records, err := polling.Fetch()
		if err != nil {
			t.Fatalf("Fetch() %d returned error: %v", i, err)
		}

		// Verify order is always 1, 2, 3, 4
		for j, record := range records {
			expectedOrder := float64(j + 1)
			if record["order"] != expectedOrder {
				t.Errorf("iteration %d: record %d has order %v, expected %v", i, j, record["order"], expectedOrder)
			}
		}
	}
}

// =============================================================================
// Task 6: Integration Tests
// =============================================================================

// TestHTTPPolling_ImplementsModule tests that HTTPPolling implements input.Module interface.
func TestHTTPPolling_ImplementsModule(t *testing.T) {
	config := &connector.ModuleConfig{
		Type: "http-polling",
		Config: map[string]interface{}{
			"endpoint": "http://example.com",
		},
	}

	polling, err := NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig failed: %v", err)
	}

	// This should compile - if it doesn't, HTTPPolling doesn't implement Module
	var _ Module = polling
}

// TestHTTPPolling_Fetch_LargeDataset tests performance with large datasets.
func TestHTTPPolling_Fetch_LargeDataset(t *testing.T) {
	// Generate large dataset (1000 records)
	largeData := make([]map[string]interface{}, 1000)
	for i := 0; i < 1000; i++ {
		largeData[i] = map[string]interface{}{
			"id":    float64(i),
			"name":  fmt.Sprintf("item-%d", i),
			"value": float64(i * 100),
		}
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(largeData)
	}))
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "http-polling",
		Config: map[string]interface{}{
			"endpoint": server.URL,
		},
	}

	polling, err := NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig failed: %v", err)
	}

	start := time.Now()
	records, err := polling.Fetch()
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Fetch() returned error: %v", err)
	}
	if len(records) != 1000 {
		t.Errorf("expected 1000 records, got %d", len(records))
	}
	// Performance check: should complete in reasonable time
	if duration > 5*time.Second {
		t.Errorf("Fetch() took too long: %v (expected < 5s)", duration)
	}
}

// =============================================================================
// Integration Tests with Pipeline Executor (Task 6)
// =============================================================================

// TestHTTPPolling_IntegrationWithExecutor tests end-to-end integration with pipeline executor.
func TestHTTPPolling_IntegrationWithExecutor(t *testing.T) {
	// Setup: Create test server returning JSON data
	testData := []map[string]interface{}{
		{"id": float64(1), "name": "Alice", "active": true},
		{"id": float64(2), "name": "Bob", "active": false},
		{"id": float64(3), "name": "Charlie", "active": true},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(testData)
	}))
	defer server.Close()

	// Create HTTPPolling module from config
	config := &connector.ModuleConfig{
		Type: "http-polling",
		Config: map[string]interface{}{
			"endpoint": server.URL,
		},
	}

	polling, err := NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig failed: %v", err)
	}

	// Verify it implements input.Module interface
	var module Module = polling
	_ = module // module cannot be nil if NewHTTPPollingFromConfig succeeded

	// Execute Fetch (simulating what Executor would do)
	records, err := module.Fetch()
	if err != nil {
		t.Fatalf("Fetch() failed: %v", err)
	}

	// Verify data is correct format for pipeline processing
	if len(records) != 3 {
		t.Errorf("expected 3 records, got %d", len(records))
	}

	// Verify record structure matches what Executor expects
	for i, record := range records {
		if _, ok := record["id"]; !ok {
			t.Errorf("record %d missing 'id' field", i)
		}
		if _, ok := record["name"]; !ok {
			t.Errorf("record %d missing 'name' field", i)
		}
	}
}

// TestHTTPPolling_IntegrationWithPaginatedData tests paginated data integration.
func TestHTTPPolling_IntegrationWithPaginatedData(t *testing.T) {
	// Simulate a real API with pagination (total_pages at root level)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		w.Header().Set("Content-Type", "application/json")

		switch page {
		case "", "1":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"users": []map[string]interface{}{
					{"id": float64(1), "email": "user1@example.com"},
					{"id": float64(2), "email": "user2@example.com"},
				},
				"page":        float64(1),
				"total_pages": float64(3),
			})
		case "2":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"users": []map[string]interface{}{
					{"id": float64(3), "email": "user3@example.com"},
					{"id": float64(4), "email": "user4@example.com"},
				},
				"page":        float64(2),
				"total_pages": float64(3),
			})
		case "3":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"users": []map[string]interface{}{
					{"id": float64(5), "email": "user5@example.com"},
				},
				"page":        float64(3),
				"total_pages": float64(3),
			})
		default:
			// Handle unexpected pages
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"users":       []map[string]interface{}{},
				"page":        float64(4),
				"total_pages": float64(3),
			})
		}
	}))
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "http-polling",
		Config: map[string]interface{}{
			"endpoint":  server.URL,
			"dataField": "users",
			"pagination": map[string]interface{}{
				"type":            "page",
				"pageParam":       "page",
				"totalPagesField": "total_pages",
			},
		},
	}

	polling, err := NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig failed: %v", err)
	}

	// Fetch all paginated data
	records, err := polling.Fetch()
	if err != nil {
		t.Fatalf("Fetch() failed: %v", err)
	}

	// Should have aggregated all 5 users from 3 pages
	if len(records) != 5 {
		t.Errorf("expected 5 records from pagination, got %d", len(records))
	}

	// Verify data integrity - records should be in order
	for i, record := range records {
		expectedID := float64(i + 1)
		if record["id"] != expectedID {
			t.Errorf("record %d has id %v, expected %v", i, record["id"], expectedID)
		}
	}
}

// TestHTTPPolling_IntegrationWithAuthentication tests authenticated API integration.
func TestHTTPPolling_IntegrationWithAuthentication(t *testing.T) {
	expectedToken := "secret-api-token-12345"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify authentication
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+expectedToken {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"error": "unauthorized"})
			return
		}

		// Return data only if authenticated
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]interface{}{
			{"secret": "data1"},
			{"secret": "data2"},
		})
	}))
	defer server.Close()

	// Test with valid authentication
	config := &connector.ModuleConfig{
		Type: "http-polling",
		Config: map[string]interface{}{
			"endpoint": server.URL,
		},
		Authentication: &connector.AuthConfig{
			Type: "bearer",
			Credentials: map[string]string{
				"token": expectedToken,
			},
		},
	}

	polling, err := NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig failed: %v", err)
	}

	records, err := polling.Fetch()
	if err != nil {
		t.Fatalf("Fetch() with auth failed: %v", err)
	}

	if len(records) != 2 {
		t.Errorf("expected 2 records with auth, got %d", len(records))
	}

	// Test without authentication (should fail)
	configNoAuth := &connector.ModuleConfig{
		Type: "http-polling",
		Config: map[string]interface{}{
			"endpoint": server.URL,
		},
	}

	pollingNoAuth, _ := NewHTTPPollingFromConfig(configNoAuth)

	_, err = pollingNoAuth.Fetch()
	if err == nil {
		t.Error("Fetch() without auth should fail")
	}
}
