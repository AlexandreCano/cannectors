// Package output provides implementations for output modules.
package output

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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

	sent, err := module.Send(records)
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
	sent, err := module.Send([]map[string]interface{}{})
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

	sent, err := module.Send(filteredRecords)
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
		sent, err := module.Send(batch)
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
	_, err = module.Send(records)
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
	sent, err := module.Send(records)

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

	sent, err := module.Send(records)
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
