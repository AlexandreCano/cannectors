// Package output provides implementations for output modules.
package output

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/canectors/runtime/pkg/connector"
)

// =============================================================================
// Task 7: Integration tests with pipeline executor
// =============================================================================

// These tests verify the HTTPRequestModule integrates correctly with the pipeline.
// They test the full flow: module creation → Send → Close

// captureServer creates a test server that captures all requests
type captureServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []capturedRequestData
}

type capturedRequestData struct {
	Method  string
	Path    string
	Headers http.Header
	Body    []byte
}

func newCaptureServer() *captureServer {
	cs := &captureServer{
		requests: make([]capturedRequestData, 0),
	}

	cs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cs.mu.Lock()
		defer cs.mu.Unlock()

		body, _ := io.ReadAll(r.Body)
		cs.requests = append(cs.requests, capturedRequestData{
			Method:  r.Method,
			Path:    r.URL.Path + "?" + r.URL.RawQuery,
			Headers: r.Header.Clone(),
			Body:    body,
		})

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"success": true}`))
	}))

	return cs
}

func (cs *captureServer) getRequests() []capturedRequestData {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.requests
}

// TestHTTPRequestModule_ImplementsOutputModuleInterface verifies interface compliance
func TestHTTPRequestModule_ImplementsOutputModuleInterface(t *testing.T) {
	config := &connector.ModuleConfig{
		Type: "httpRequest",
		Config: map[string]interface{}{
			"endpoint": "https://api.example.com/data",
			"method":   "POST",
		},
	}

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	// Verify it implements the Module interface
	var _ Module = module
}

// TestHTTPRequestModule_PipelineIntegration_SimpleFlow tests basic pipeline flow
func TestHTTPRequestModule_PipelineIntegration_SimpleFlow(t *testing.T) {
	server := newCaptureServer()
	defer server.Close()

	// Create module (simulating what pipeline executor would do)
	config := &connector.ModuleConfig{
		Type: "httpRequest",
		Config: map[string]interface{}{
			"endpoint": server.URL + "/api/users",
			"method":   "POST",
		},
	}

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	// Simulate pipeline executor calling Send
	records := []map[string]interface{}{
		{"id": 1, "name": "Alice", "email": "alice@example.com"},
		{"id": 2, "name": "Bob", "email": "bob@example.com"},
	}

	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if sent != 2 {
		t.Errorf("expected 2 records sent, got %d", sent)
	}

	// Simulate pipeline executor calling Close
	if err := module.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Verify request was made correctly
	reqs := server.getRequests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}

	var body []map[string]interface{}
	if err := json.Unmarshal(reqs[0].Body, &body); err != nil {
		t.Fatalf("failed to parse request body: %v", err)
	}
	if len(body) != 2 {
		t.Errorf("expected 2 records in body, got %d", len(body))
	}
}

// TestHTTPRequestModule_PipelineIntegration_EmptyRecords tests empty input handling
func TestHTTPRequestModule_PipelineIntegration_EmptyRecords(t *testing.T) {
	server := newCaptureServer()
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "httpRequest",
		Config: map[string]interface{}{
			"endpoint": server.URL + "/api/data",
			"method":   "POST",
		},
	}

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	// Send empty records (e.g., after filtering removed all records)
	sent, err := module.Send(context.Background(), []map[string]interface{}{})
	if err != nil {
		t.Fatalf("Send with empty records should not fail: %v", err)
	}
	if sent != 0 {
		t.Errorf("expected 0 records sent, got %d", sent)
	}

	// Close
	if err := module.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// No requests should have been made
	if len(server.getRequests()) != 0 {
		t.Errorf("expected 0 requests for empty records, got %d", len(server.getRequests()))
	}
}

// TestHTTPRequestModule_PipelineIntegration_WithFiltering simulates pipeline with filtering
func TestHTTPRequestModule_PipelineIntegration_WithFiltering(t *testing.T) {
	server := newCaptureServer()
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "httpRequest",
		Config: map[string]interface{}{
			"endpoint": server.URL + "/api/filtered",
			"method":   "PUT",
		},
	}

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	// Simulated filtered/transformed records from filter module
	filteredRecords := []map[string]interface{}{
		{"target_id": "usr_001", "full_name": "ALICE SMITH", "active": true},
	}

	sent, err := module.Send(context.Background(), filteredRecords)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if sent != 1 {
		t.Errorf("expected 1 record sent, got %d", sent)
	}

	if err := module.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Verify request
	reqs := server.getRequests()
	if reqs[0].Method != "PUT" {
		t.Errorf("expected PUT method, got %s", reqs[0].Method)
	}
}

// TestHTTPRequestModule_PipelineIntegration_MultipleCallsReusesModule tests module reuse
func TestHTTPRequestModule_PipelineIntegration_MultipleCallsReusesModule(t *testing.T) {
	server := newCaptureServer()
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "httpRequest",
		Config: map[string]interface{}{
			"endpoint": server.URL + "/api/data",
			"method":   "POST",
		},
	}

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	// Multiple Send calls (e.g., batched processing)
	batch1 := []map[string]interface{}{{"batch": 1}}
	batch2 := []map[string]interface{}{{"batch": 2}}
	batch3 := []map[string]interface{}{{"batch": 3}}

	for i, batch := range [][]map[string]interface{}{batch1, batch2, batch3} {
		sent, err := module.Send(context.Background(), batch)
		if err != nil {
			t.Fatalf("Send batch %d failed: %v", i+1, err)
		}
		if sent != 1 {
			t.Errorf("batch %d: expected 1 sent, got %d", i+1, sent)
		}
	}

	if err := module.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Verify 3 requests were made
	if len(server.getRequests()) != 3 {
		t.Errorf("expected 3 requests, got %d", len(server.getRequests()))
	}
}

// TestHTTPRequestModule_PipelineIntegration_WithAuthentication tests auth in pipeline context
func TestHTTPRequestModule_PipelineIntegration_WithAuthentication(t *testing.T) {
	server := newCaptureServer()
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "httpRequest",
		Config: map[string]interface{}{
			"endpoint": server.URL + "/api/secure",
			"method":   "POST",
		},
		Authentication: &connector.AuthConfig{
			Type: "bearer",
			Credentials: map[string]string{
				"token": "pipeline-auth-token",
			},
		},
	}

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"secure": "data"}}
	_, err = module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if err := module.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Verify auth header was sent
	reqs := server.getRequests()
	auth := reqs[0].Headers.Get("Authorization")
	if auth != "Bearer pipeline-auth-token" {
		t.Errorf("expected auth header, got %s", auth)
	}
}

// TestHTTPRequestModule_PipelineIntegration_ErrorHandling tests error propagation
func TestHTTPRequestModule_PipelineIntegration_ErrorHandling(t *testing.T) {
	// Server that returns errors
	errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error": "invalid data"}`))
	}))
	defer errorServer.Close()

	config := &connector.ModuleConfig{
		Type: "httpRequest",
		Config: map[string]interface{}{
			"endpoint": errorServer.URL + "/api/data",
			"method":   "POST",
		},
	}

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	records := []map[string]interface{}{{"test": "data"}}
	sent, err := module.Send(context.Background(), records)

	// Error should be returned to pipeline executor
	if err == nil {
		t.Fatal("expected error to be returned to pipeline executor")
	}
	if sent != 0 {
		t.Errorf("expected 0 sent on error, got %d", sent)
	}

	// Close should still work
	if err := module.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

// TestHTTPRequestModule_PipelineIntegration_LargeDataSet tests handling many records
func TestHTTPRequestModule_PipelineIntegration_LargeDataSet(t *testing.T) {
	server := newCaptureServer()
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "httpRequest",
		Config: map[string]interface{}{
			"endpoint": server.URL + "/api/bulk",
			"method":   "POST",
		},
	}

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}

	// Create large dataset
	records := make([]map[string]interface{}, 1000)
	for i := 0; i < 1000; i++ {
		records[i] = map[string]interface{}{
			"id":    i,
			"name":  "User " + string(rune(i)),
			"email": "user@example.com",
		}
	}

	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("Send large dataset failed: %v", err)
	}
	if sent != 1000 {
		t.Errorf("expected 1000 records sent, got %d", sent)
	}

	if err := module.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Verify single batch request
	if len(server.getRequests()) != 1 {
		t.Errorf("expected 1 batch request, got %d", len(server.getRequests()))
	}
}

// =============================================================================
// Story 14.6: Output Templating Integration Tests
// =============================================================================

// TestHTTPRequestModule_Templating_EndpointWithRecordData tests templated endpoints
func TestHTTPRequestModule_Templating_EndpointWithRecordData(t *testing.T) {
	server := newCaptureServer()
	defer server.Close()

	// Configure with templated endpoint
	config := &connector.ModuleConfig{
		Type: "httpRequest",
		Config: map[string]interface{}{
			"endpoint": server.URL + "/api/users/{{record.user_id}}/orders",
			"method":   "POST",
			"request": map[string]interface{}{
				"bodyFrom": "record", // Single record mode to evaluate template per record
			},
		},
	}

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}
	defer func() { _ = module.Close() }()

	// Send records with different user_ids
	records := []map[string]interface{}{
		{"user_id": "123", "order": "A001"},
		{"user_id": "456", "order": "A002"},
	}

	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if sent != 2 {
		t.Errorf("expected 2 records sent, got %d", sent)
	}

	// Verify requests went to different endpoints
	reqs := server.getRequests()
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests (one per record), got %d", len(reqs))
	}

	// Check first request endpoint
	if !contains(reqs[0].Path, "/api/users/123/orders") {
		t.Errorf("expected path /api/users/123/orders, got %s", reqs[0].Path)
	}

	// Check second request endpoint
	if !contains(reqs[1].Path, "/api/users/456/orders") {
		t.Errorf("expected path /api/users/456/orders, got %s", reqs[1].Path)
	}
}

// TestHTTPRequestModule_Templating_NestedFieldAccess tests nested field access in templates
func TestHTTPRequestModule_Templating_NestedFieldAccess(t *testing.T) {
	server := newCaptureServer()
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "httpRequest",
		Config: map[string]interface{}{
			"endpoint": server.URL + "/api/customers/{{record.customer.id}}/profile",
			"method":   "PUT",
			"request": map[string]interface{}{
				"bodyFrom": "record",
			},
		},
	}

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}
	defer func() { _ = module.Close() }()

	records := []map[string]interface{}{
		{
			"customer": map[string]interface{}{
				"id":   "cust_001",
				"name": "Alice",
			},
			"data": "profile update",
		},
	}

	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if sent != 1 {
		t.Errorf("expected 1 record sent, got %d", sent)
	}

	reqs := server.getRequests()
	if !contains(reqs[0].Path, "/api/customers/cust_001/profile") {
		t.Errorf("expected path /api/customers/cust_001/profile, got %s", reqs[0].Path)
	}
}

// TestHTTPRequestModule_Templating_HeadersWithRecordData tests templated headers
func TestHTTPRequestModule_Templating_HeadersWithRecordData(t *testing.T) {
	server := newCaptureServer()
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "httpRequest",
		Config: map[string]interface{}{
			"endpoint": server.URL + "/api/data",
			"method":   "POST",
			"headers": map[string]interface{}{
				"X-User-ID":     "{{record.user_id}}",
				"X-Tenant":      "{{record.tenant}}",
				"X-Static":      "static-value",
				"X-Correlation": "{{record.correlation_id}}",
			},
			"request": map[string]interface{}{
				"bodyFrom": "record",
			},
		},
	}

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}
	defer func() { _ = module.Close() }()

	records := []map[string]interface{}{
		{"user_id": "user_123", "tenant": "acme-corp", "correlation_id": "req-abc"},
	}

	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if sent != 1 {
		t.Errorf("expected 1 record sent, got %d", sent)
	}

	reqs := server.getRequests()
	if reqs[0].Headers.Get("X-User-ID") != "user_123" {
		t.Errorf("expected X-User-ID=user_123, got %s", reqs[0].Headers.Get("X-User-ID"))
	}
	if reqs[0].Headers.Get("X-Tenant") != "acme-corp" {
		t.Errorf("expected X-Tenant=acme-corp, got %s", reqs[0].Headers.Get("X-Tenant"))
	}
	if reqs[0].Headers.Get("X-Static") != "static-value" {
		t.Errorf("expected X-Static=static-value, got %s", reqs[0].Headers.Get("X-Static"))
	}
	if reqs[0].Headers.Get("X-Correlation") != "req-abc" {
		t.Errorf("expected X-Correlation=req-abc, got %s", reqs[0].Headers.Get("X-Correlation"))
	}
}

// TestHTTPRequestModule_Templating_MissingFieldWithDefault tests default values
func TestHTTPRequestModule_Templating_MissingFieldWithDefault(t *testing.T) {
	server := newCaptureServer()
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "httpRequest",
		Config: map[string]interface{}{
			"endpoint": server.URL + `/api/users/{{record.user_id | default: "unknown"}}/data`,
			"method":   "POST",
			"request": map[string]interface{}{
				"bodyFrom": "record",
			},
		},
	}

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}
	defer func() { _ = module.Close() }()

	// Record without user_id field
	records := []map[string]interface{}{
		{"data": "some data"},
	}

	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if sent != 1 {
		t.Errorf("expected 1 record sent, got %d", sent)
	}

	reqs := server.getRequests()
	if !contains(reqs[0].Path, "/api/users/unknown/data") {
		t.Errorf("expected path with default value 'unknown', got %s", reqs[0].Path)
	}
}

// TestHTTPRequestModule_Templating_URLEncoding tests special characters in templates
func TestHTTPRequestModule_Templating_URLEncoding(t *testing.T) {
	server := newCaptureServer()
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "httpRequest",
		Config: map[string]interface{}{
			"endpoint": server.URL + "/api/search/{{record.query}}",
			"method":   "POST",
			"request": map[string]interface{}{
				"bodyFrom": "record",
			},
		},
	}

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}
	defer func() { _ = module.Close() }()

	// Record with special characters
	records := []map[string]interface{}{
		{"query": "hello world"},
	}

	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if sent != 1 {
		t.Errorf("expected 1 record sent, got %d", sent)
	}

	reqs := server.getRequests()
	// URL-encoded space should be present
	if !contains(reqs[0].Path, "hello+world") && !contains(reqs[0].Path, "hello%20world") {
		t.Errorf("expected URL-encoded path, got %s", reqs[0].Path)
	}
}

// TestHTTPRequestModule_Templating_BatchModeUsesFirstRecord tests batch mode template evaluation
func TestHTTPRequestModule_Templating_BatchModeUsesFirstRecord(t *testing.T) {
	server := newCaptureServer()
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "httpRequest",
		Config: map[string]interface{}{
			"endpoint": server.URL + "/api/tenants/{{record.tenant_id}}/bulk",
			"method":   "POST",
			"request": map[string]interface{}{
				"bodyFrom": "records", // Batch mode
			},
		},
	}

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}
	defer func() { _ = module.Close() }()

	// All records have the same tenant_id (batch scenario)
	records := []map[string]interface{}{
		{"tenant_id": "tenant_001", "data": "record1"},
		{"tenant_id": "tenant_001", "data": "record2"},
		{"tenant_id": "tenant_001", "data": "record3"},
	}

	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if sent != 3 {
		t.Errorf("expected 3 records sent, got %d", sent)
	}

	// Should be single request in batch mode
	reqs := server.getRequests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 batch request, got %d", len(reqs))
	}

	// Check endpoint was templated using first record
	if !contains(reqs[0].Path, "/api/tenants/tenant_001/bulk") {
		t.Errorf("expected path /api/tenants/tenant_001/bulk, got %s", reqs[0].Path)
	}
}

// TestHTTPRequestModule_Templating_ArrayIndex tests array index access in templates
func TestHTTPRequestModule_Templating_ArrayIndex(t *testing.T) {
	server := newCaptureServer()
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "httpRequest",
		Config: map[string]interface{}{
			"endpoint": server.URL + "/api/items/{{record.items[0].id}}",
			"method":   "POST",
			"request": map[string]interface{}{
				"bodyFrom": "record",
			},
		},
	}

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}
	defer func() { _ = module.Close() }()

	records := []map[string]interface{}{
		{
			"items": []interface{}{
				map[string]interface{}{"id": "item_001", "name": "First"},
				map[string]interface{}{"id": "item_002", "name": "Second"},
			},
		},
	}

	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if sent != 1 {
		t.Errorf("expected 1 record sent, got %d", sent)
	}

	reqs := server.getRequests()
	if !contains(reqs[0].Path, "/api/items/item_001") {
		t.Errorf("expected path /api/items/item_001, got %s", reqs[0].Path)
	}
}

// TestHTTPRequestModule_Templating_InvalidSyntax tests validation of template syntax
func TestHTTPRequestModule_Templating_InvalidSyntax(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		headers  map[string]interface{}
		wantErr  bool
	}{
		{
			name:     "valid template",
			endpoint: "https://api.example.com/users/{{record.id}}",
			wantErr:  false,
		},
		{
			name:     "unmatched braces in endpoint",
			endpoint: "https://api.example.com/users/{{record.id",
			wantErr:  true,
		},
		{
			name:     "unmatched braces in header",
			endpoint: "https://api.example.com/users",
			headers: map[string]interface{}{
				"X-User": "{{record.id",
			},
			wantErr: true,
		},
		{
			name:     "empty template variable",
			endpoint: "https://api.example.com/users/{{}}",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &connector.ModuleConfig{
				Type: "httpRequest",
				Config: map[string]interface{}{
					"endpoint": tt.endpoint,
					"method":   "POST",
				},
			}
			if tt.headers != nil {
				config.Config["headers"] = tt.headers
			}

			_, err := NewHTTPRequestFromConfig(config)
			if tt.wantErr && err == nil {
				t.Error("expected error for invalid template syntax")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestHTTPRequestModule_Templating_BodyTemplateFile tests templated request body from file
func TestHTTPRequestModule_Templating_BodyTemplateFile(t *testing.T) {
	server := newCaptureServer()
	defer server.Close()

	// Create temporary template file
	templateContent := `{
  "userId": "{{record.user_id}}",
  "userName": "{{record.name}}",
  "email": "{{record.email}}",
  "timestamp": "2024-01-01T00:00:00Z"
}`
	tmpFile, err := os.CreateTemp("", "body-template-*.json")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	if _, writeErr := tmpFile.WriteString(templateContent); writeErr != nil {
		t.Fatalf("failed to write template: %v", writeErr)
	}
	_ = tmpFile.Close()

	config := &connector.ModuleConfig{
		Type: "httpRequest",
		Config: map[string]interface{}{
			"endpoint": server.URL + "/api/users",
			"method":   "POST",
			"request": map[string]interface{}{
				"bodyFrom":         "record",
				"bodyTemplateFile": tmpFile.Name(),
			},
		},
	}

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}
	defer func() { _ = module.Close() }()

	records := []map[string]interface{}{
		{"user_id": "123", "name": "Alice", "email": "alice@example.com"},
	}

	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if sent != 1 {
		t.Errorf("expected 1 record sent, got %d", sent)
	}

	reqs := server.getRequests()
	var body map[string]interface{}
	if err := json.Unmarshal(reqs[0].Body, &body); err != nil {
		t.Fatalf("failed to parse body: %v", err)
	}

	// Check templated values
	if body["userId"] != "123" {
		t.Errorf("expected userId=123, got %v", body["userId"])
	}
	if body["userName"] != "Alice" {
		t.Errorf("expected userName=Alice, got %v", body["userName"])
	}
	if body["email"] != "alice@example.com" {
		t.Errorf("expected email=alice@example.com, got %v", body["email"])
	}
	// Static value should be preserved
	if body["timestamp"] != "2024-01-01T00:00:00Z" {
		t.Errorf("expected timestamp=2024-01-01T00:00:00Z, got %v", body["timestamp"])
	}
}

// TestHTTPRequestModule_Templating_BodyTemplateFileNested tests nested fields in body template file
func TestHTTPRequestModule_Templating_BodyTemplateFileNested(t *testing.T) {
	server := newCaptureServer()
	defer server.Close()

	// Create temporary template file with nested structure
	templateContent := `{
  "data": {
    "id": "{{record.id}}",
    "name": "{{record.profile.name}}"
  },
  "meta": {
    "version": "1.0"
  }
}`
	tmpFile, err := os.CreateTemp("", "body-template-nested-*.json")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	if _, writeErr := tmpFile.WriteString(templateContent); writeErr != nil {
		t.Fatalf("failed to write template: %v", writeErr)
	}
	_ = tmpFile.Close()

	config := &connector.ModuleConfig{
		Type: "httpRequest",
		Config: map[string]interface{}{
			"endpoint": server.URL + "/api/data",
			"method":   "POST",
			"request": map[string]interface{}{
				"bodyFrom":         "record",
				"bodyTemplateFile": tmpFile.Name(),
			},
		},
	}

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}
	defer func() { _ = module.Close() }()

	records := []map[string]interface{}{
		{
			"id": "rec_001",
			"profile": map[string]interface{}{
				"name": "Test User",
			},
		},
	}

	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if sent != 1 {
		t.Errorf("expected 1 record sent, got %d", sent)
	}

	reqs := server.getRequests()
	var body map[string]interface{}
	if err := json.Unmarshal(reqs[0].Body, &body); err != nil {
		t.Fatalf("failed to parse body: %v", err)
	}

	// Check nested templated values
	data, ok := body["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be a map")
	}
	if data["id"] != "rec_001" {
		t.Errorf("expected data.id=rec_001, got %v", data["id"])
	}
	if data["name"] != "Test User" {
		t.Errorf("expected data.name=Test User, got %v", data["name"])
	}
}

// TestHTTPRequestModule_Templating_BodyTemplateFileBatch tests body template file in batch mode
// In batch mode with a template file, the template is evaluated once using the first record's data.
// This is useful for scenarios where all records share common metadata (e.g., tenant ID, batch ID).
func TestHTTPRequestModule_Templating_BodyTemplateFileBatch(t *testing.T) {
	server := newCaptureServer()
	defer server.Close()

	// Create temporary template file - uses first record's data for batch context
	templateContent := `{
  "batchId": "{{record.batch_id}}",
  "tenantId": "{{record.tenant_id}}",
  "processedAt": "2024-01-01T00:00:00Z"
}`
	tmpFile, err := os.CreateTemp("", "body-template-batch-*.json")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	if _, writeErr := tmpFile.WriteString(templateContent); writeErr != nil {
		t.Fatalf("failed to write template: %v", writeErr)
	}
	_ = tmpFile.Close()

	config := &connector.ModuleConfig{
		Type: "httpRequest",
		Config: map[string]interface{}{
			"endpoint": server.URL + "/api/bulk",
			"method":   "POST",
			"request": map[string]interface{}{
				"bodyFrom":         "records", // Batch mode
				"bodyTemplateFile": tmpFile.Name(),
			},
		},
	}

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}
	defer func() { _ = module.Close() }()

	// All records share the same batch_id and tenant_id (batch scenario)
	records := []map[string]interface{}{
		{"batch_id": "batch_001", "tenant_id": "tenant_123", "data": "record1"},
		{"batch_id": "batch_001", "tenant_id": "tenant_123", "data": "record2"},
	}

	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if sent != 2 {
		t.Errorf("expected 2 records sent, got %d", sent)
	}

	reqs := server.getRequests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 batch request, got %d", len(reqs))
	}

	var body map[string]interface{}
	if err := json.Unmarshal(reqs[0].Body, &body); err != nil {
		t.Fatalf("failed to parse body: %v", err)
	}

	// Check templated values from first record
	if body["batchId"] != "batch_001" {
		t.Errorf("expected batchId=batch_001, got %v", body["batchId"])
	}
	if body["tenantId"] != "tenant_123" {
		t.Errorf("expected tenantId=tenant_123, got %v", body["tenantId"])
	}
	if body["processedAt"] != "2024-01-01T00:00:00Z" {
		t.Errorf("expected processedAt=2024-01-01T00:00:00Z, got %v", body["processedAt"])
	}
}

// TestHTTPRequestModule_Templating_XMLTemplate tests XML/SOAP template support
func TestHTTPRequestModule_Templating_XMLTemplate(t *testing.T) {
	server := newCaptureServer()
	defer server.Close()

	// Create temporary XML/SOAP template file
	templateContent := `<?xml version="1.0" encoding="UTF-8"?>
<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/">
  <soap:Body>
    <CreateUser>
      <userId>{{record.user_id}}</userId>
      <userName>{{record.name}}</userName>
      <email>{{record.email}}</email>
    </CreateUser>
  </soap:Body>
</soap:Envelope>`
	tmpFile, err := os.CreateTemp("", "body-template-*.xml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	if _, writeErr := tmpFile.WriteString(templateContent); writeErr != nil {
		t.Fatalf("failed to write template: %v", writeErr)
	}
	_ = tmpFile.Close()

	config := &connector.ModuleConfig{
		Type: "httpRequest",
		Config: map[string]interface{}{
			"endpoint": server.URL + "/soap/api",
			"method":   "POST",
			"headers": map[string]interface{}{
				"Content-Type": "application/soap+xml",
			},
			"request": map[string]interface{}{
				"bodyFrom":         "record",
				"bodyTemplateFile": tmpFile.Name(),
			},
		},
	}

	module, err := NewHTTPRequestFromConfig(config)
	if err != nil {
		t.Fatalf("failed to create module: %v", err)
	}
	defer func() { _ = module.Close() }()

	records := []map[string]interface{}{
		{"user_id": "456", "name": "Bob", "email": "bob@example.com"},
	}

	sent, err := module.Send(context.Background(), records)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if sent != 1 {
		t.Errorf("expected 1 record sent, got %d", sent)
	}

	reqs := server.getRequests()
	bodyStr := string(reqs[0].Body)

	// Check XML content was properly templated
	if !contains(bodyStr, "<userId>456</userId>") {
		t.Errorf("expected userId=456 in XML body, got %s", bodyStr)
	}
	if !contains(bodyStr, "<userName>Bob</userName>") {
		t.Errorf("expected userName=Bob in XML body, got %s", bodyStr)
	}
	if !contains(bodyStr, "<email>bob@example.com</email>") {
		t.Errorf("expected email=bob@example.com in XML body, got %s", bodyStr)
	}
	// Check XML structure is preserved
	if !contains(bodyStr, "<?xml version") {
		t.Errorf("expected XML declaration in body, got %s", bodyStr)
	}
	if !contains(bodyStr, "soap:Envelope") {
		t.Errorf("expected SOAP envelope in body, got %s", bodyStr)
	}
}

// TestNewHTTPRequestFromConfig_InvalidTemplateSyntax tests that invalid template syntax is rejected during configuration
func TestNewHTTPRequestFromConfig_InvalidTemplateSyntax(t *testing.T) {
	tests := []struct {
		name        string
		config      map[string]interface{}
		expectError string
	}{
		{
			name: "invalid endpoint template - unmatched opening brace",
			config: map[string]interface{}{
				"endpoint": "https://api.example.com/{{record.id",
				"method":   "POST",
			},
			expectError: "invalid endpoint template",
		},
		{
			name: "invalid endpoint template - unmatched closing brace",
			config: map[string]interface{}{
				"endpoint": "https://api.example.com/record.id}}",
				"method":   "POST",
			},
			expectError: "invalid endpoint template",
		},
		{
			name: "invalid endpoint template - empty variable",
			config: map[string]interface{}{
				"endpoint": "https://api.example.com/{{}}",
				"method":   "POST",
			},
			expectError: "invalid endpoint template",
		},
		{
			name: "invalid header template - unmatched brace",
			config: map[string]interface{}{
				"endpoint": "https://api.example.com/data",
				"method":   "POST",
				"headers": map[string]interface{}{
					"X-User-ID": "{{record.user_id",
				},
			},
			expectError: "invalid template in header",
		},
		{
			name: "invalid body template file - unmatched brace",
			config: map[string]interface{}{
				"endpoint": "https://api.example.com/data",
				"method":   "POST",
				"request": map[string]interface{}{
					"bodyTemplateFile": createInvalidTemplateFile(t, "{{record.id"),
				},
			},
			expectError: "invalid template syntax",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &connector.ModuleConfig{
				Type:   "httpRequest",
				Config: tt.config,
			}

			module, err := NewHTTPRequestFromConfig(config)
			if err == nil {
				defer func() { _ = module.Close() }()
				t.Fatalf("expected error containing %q, got nil", tt.expectError)
			}

			if !contains(err.Error(), tt.expectError) {
				t.Errorf("expected error containing %q, got %q", tt.expectError, err.Error())
			}
		})
	}
}

// createInvalidTemplateFile creates a temporary file with invalid template content
func createInvalidTemplateFile(t *testing.T, content string) string {
	t.Helper()
	tmpFile, err := os.CreateTemp("", "invalid-template-*.json")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer func() { _ = tmpFile.Close() }()

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	// Don't remove the file immediately - let the test clean it up
	t.Cleanup(func() {
		_ = os.Remove(tmpFile.Name())
	})

	return tmpFile.Name()
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
