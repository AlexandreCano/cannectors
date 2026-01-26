// Package runtime provides the pipeline execution engine.
package runtime

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/canectors/runtime/internal/modules/input"
	"github.com/canectors/runtime/internal/persistence"
	"github.com/canectors/runtime/pkg/connector"
)

// TestExecutor_StatePersistence_Timestamp tests end-to-end state persistence with timestamp.
// Verifies that state is persisted after first execution and used in subsequent executions.
func TestExecutor_StatePersistence_Timestamp(t *testing.T) {
	tmpDir := t.TempDir()
	stateStore := persistence.NewStateStore(tmpDir)

	// Create test server that tracks query parameters
	var lastQueryParam string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastQueryParam = r.URL.Query().Get("since")
		w.Header().Set("Content-Type", "application/json")
		records := []map[string]interface{}{
			{"id": "1", "name": "Record 1"},
			{"id": "2", "name": "Record 2"},
		}
		_ = json.NewEncoder(w).Encode(records)
	}))
	defer server.Close()

	// Create HTTP polling input with timestamp persistence
	config := &connector.ModuleConfig{
		Type: "http-polling",
		Config: map[string]interface{}{
			"endpoint": server.URL,
			"statePersistence": map[string]interface{}{
				"timestamp": map[string]interface{}{
					"enabled":    true,
					"queryParam": "since",
				},
			},
		},
	}

	inputModule, err := input.NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig() error = %v", err)
	}

	outputModule := NewMockOutputModule(nil)
	executor := NewExecutorWithModules(inputModule, nil, outputModule, false)
	executor.SetStateStore(stateStore)

	pipeline := &connector.Pipeline{
		ID:      "test-pipeline-timestamp",
		Name:    "Test Pipeline",
		Version: "1.0.0",
		Enabled: true,
	}

	// First execution: no state exists, should not have query param
	lastQueryParam = ""
	result1, err := executor.Execute(pipeline)
	if err != nil {
		t.Fatalf("First execution failed: %v", err)
	}
	if result1.Status != "success" {
		t.Errorf("First execution status = %s, want success", result1.Status)
	}
	if lastQueryParam != "" {
		t.Errorf("First execution should not have query param, got %q", lastQueryParam)
	}

	// Verify state was persisted
	state, err := stateStore.Load(pipeline.ID)
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}
	if state == nil {
		t.Fatal("State should be persisted after first execution")
	}
	if state.LastTimestamp == nil {
		t.Error("State should have LastTimestamp")
	}

	// Second execution: state exists, should have query param with timestamp
	// Create new input module (previous one was closed after first execution)
	inputModule2, err := input.NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig() error = %v", err)
	}
	outputModule2 := NewMockOutputModule(nil)
	executor2 := NewExecutorWithModules(inputModule2, nil, outputModule2, false)
	executor2.SetStateStore(stateStore)

	lastQueryParam = ""
	time.Sleep(10 * time.Millisecond) // Ensure different timestamp
	result2, err := executor2.Execute(pipeline)
	if err != nil {
		t.Fatalf("Second execution failed: %v", err)
	}
	if result2.Status != "success" {
		t.Errorf("Second execution status = %s, want success", result2.Status)
	}
	if lastQueryParam == "" {
		t.Error("Second execution should have query param with timestamp")
	}
	if state.LastTimestamp != nil {
		expectedTimestamp := state.LastTimestamp.Format(time.RFC3339)
		if lastQueryParam != expectedTimestamp {
			t.Errorf("Query param = %q, want %q", lastQueryParam, expectedTimestamp)
		}
	}
}

// TestExecutor_StatePersistence_ID tests end-to-end state persistence with ID.
// Verifies that state is persisted after first execution and used in subsequent executions.
func TestExecutor_StatePersistence_ID(t *testing.T) {
	tmpDir := t.TempDir()
	stateStore := persistence.NewStateStore(tmpDir)

	// Create test server that tracks query parameters
	var lastQueryParam string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastQueryParam = r.URL.Query().Get("after_id")
		w.Header().Set("Content-Type", "application/json")
		records := []map[string]interface{}{
			{"id": "10", "name": "Record 10"},
			{"id": "20", "name": "Record 20"},
		}
		_ = json.NewEncoder(w).Encode(records)
	}))
	defer server.Close()

	// Create HTTP polling input with ID persistence
	config := &connector.ModuleConfig{
		Type: "http-polling",
		Config: map[string]interface{}{
			"endpoint": server.URL,
			"statePersistence": map[string]interface{}{
				"id": map[string]interface{}{
					"enabled":    true,
					"field":      "id",
					"queryParam": "after_id",
				},
			},
		},
	}

	inputModule, err := input.NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig() error = %v", err)
	}

	outputModule := NewMockOutputModule(nil)
	executor := NewExecutorWithModules(inputModule, nil, outputModule, false)
	executor.SetStateStore(stateStore)

	pipeline := &connector.Pipeline{
		ID:      "test-pipeline-id",
		Name:    "Test Pipeline",
		Version: "1.0.0",
		Enabled: true,
	}

	// First execution: no state exists, should not have query param
	lastQueryParam = ""
	result1, err := executor.Execute(pipeline)
	if err != nil {
		t.Fatalf("First execution failed: %v", err)
	}
	if result1.Status != "success" {
		t.Errorf("First execution status = %s, want success", result1.Status)
	}
	if lastQueryParam != "" {
		t.Errorf("First execution should not have query param, got %q", lastQueryParam)
	}

	// Verify state was persisted with last ID
	state, err := stateStore.Load(pipeline.ID)
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}
	if state == nil {
		t.Fatal("State should be persisted after first execution")
	}
	if state.LastID == nil {
		t.Error("State should have LastID")
	}
	if *state.LastID != "20" {
		t.Errorf("LastID = %q, want %q", *state.LastID, "20")
	}

	// Second execution: state exists, should have query param with last ID
	// Create new input module (previous one was closed after first execution)
	inputModule2, err := input.NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig() error = %v", err)
	}
	outputModule2 := NewMockOutputModule(nil)
	executor2 := NewExecutorWithModules(inputModule2, nil, outputModule2, false)
	executor2.SetStateStore(stateStore)

	lastQueryParam = ""
	result2, err := executor2.Execute(pipeline)
	if err != nil {
		t.Fatalf("Second execution failed: %v", err)
	}
	if result2.Status != "success" {
		t.Errorf("Second execution status = %s, want success", result2.Status)
	}
	if lastQueryParam == "" {
		t.Error("Second execution should have query param with last ID")
	}
	if lastQueryParam != "20" {
		t.Errorf("Query param = %q, want %q", lastQueryParam, "20")
	}
}

// TestExecutor_StatePersistence_AfterRestart simulates pipeline restart by creating new executor.
// Verifies that state persists across executor instances (simulating runtime restart).
func TestExecutor_StatePersistence_AfterRestart(t *testing.T) {
	tmpDir := t.TempDir()
	stateStore := persistence.NewStateStore(tmpDir)

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		records := []map[string]interface{}{
			{"id": "1", "name": "Record 1"},
		}
		_ = json.NewEncoder(w).Encode(records)
	}))
	defer server.Close()

	// Create HTTP polling input with timestamp persistence
	config := &connector.ModuleConfig{
		Type: "http-polling",
		Config: map[string]interface{}{
			"endpoint": server.URL,
			"statePersistence": map[string]interface{}{
				"timestamp": map[string]interface{}{
					"enabled":    true,
					"queryParam": "since",
				},
			},
		},
	}

	pipeline := &connector.Pipeline{
		ID:      "test-pipeline-restart",
		Name:    "Test Pipeline",
		Version: "1.0.0",
		Enabled: true,
	}

	// First execution with first executor instance
	inputModule1, err := input.NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig() error = %v", err)
	}
	outputModule1 := NewMockOutputModule(nil)
	executor1 := NewExecutorWithModules(inputModule1, nil, outputModule1, false)
	executor1.SetStateStore(stateStore)

	result1, err := executor1.Execute(pipeline)
	if err != nil {
		t.Fatalf("First execution failed: %v", err)
	}
	if result1.Status != "success" {
		t.Errorf("First execution status = %s, want success", result1.Status)
	}

	// Verify state file exists
	stateFile := filepath.Join(tmpDir, pipeline.ID+".json")
	if _, statErr := os.Stat(stateFile); os.IsNotExist(statErr) {
		t.Fatal("State file should exist after first execution")
	}

	// Simulate restart: create new executor instance
	inputModule2, err := input.NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig() error = %v", err)
	}
	outputModule2 := NewMockOutputModule(nil)
	executor2 := NewExecutorWithModules(inputModule2, nil, outputModule2, false)
	executor2.SetStateStore(stateStore)

	// Second execution with new executor instance should load persisted state
	result2, err := executor2.Execute(pipeline)
	if err != nil {
		t.Fatalf("Second execution failed: %v", err)
	}
	if result2.Status != "success" {
		t.Errorf("Second execution status = %s, want success", result2.Status)
	}

	// Verify state was loaded and updated
	state, err := stateStore.Load(pipeline.ID)
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}
	if state == nil {
		t.Fatal("State should exist after second execution")
	}
	if state.LastTimestamp == nil {
		t.Error("State should have LastTimestamp")
	}
}

// TestExecutor_StatePersistence_NoStateStore verifies that execution works without state store.
func TestExecutor_StatePersistence_NoStateStore(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		records := []map[string]interface{}{
			{"id": "1", "name": "Record 1"},
		}
		_ = json.NewEncoder(w).Encode(records)
	}))
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "http-polling",
		Config: map[string]interface{}{
			"endpoint": server.URL,
			"statePersistence": map[string]interface{}{
				"timestamp": map[string]interface{}{
					"enabled":    true,
					"queryParam": "since",
				},
			},
		},
	}

	inputModule, err := input.NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig() error = %v", err)
	}

	outputModule := NewMockOutputModule(nil)
	executor := NewExecutorWithModules(inputModule, nil, outputModule, false)
	// Note: SetStateStore is NOT called

	pipeline := &connector.Pipeline{
		ID:      "test-pipeline-no-store",
		Name:    "Test Pipeline",
		Version: "1.0.0",
		Enabled: true,
	}

	// Execution should work without state store (graceful degradation)
	result, err := executor.Execute(pipeline)
	if err != nil {
		t.Fatalf("Execution failed: %v", err)
	}
	if result.Status != "success" {
		t.Errorf("Execution status = %s, want success", result.Status)
	}
}

// TestExecutor_StatePersistence_FailedExecution verifies that state is NOT persisted on failure.
func TestExecutor_StatePersistence_FailedExecution(t *testing.T) {
	tmpDir := t.TempDir()
	stateStore := persistence.NewStateStore(tmpDir)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		records := []map[string]interface{}{
			{"id": "1", "name": "Record 1"},
		}
		_ = json.NewEncoder(w).Encode(records)
	}))
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "http-polling",
		Config: map[string]interface{}{
			"endpoint": server.URL,
			"statePersistence": map[string]interface{}{
				"timestamp": map[string]interface{}{
					"enabled":    true,
					"queryParam": "since",
				},
			},
		},
	}

	inputModule, err := input.NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig() error = %v", err)
	}

	// Output module that fails
	outputModule := NewMockOutputModule(errors.New("simulated output failure"))
	executor := NewExecutorWithModules(inputModule, nil, outputModule, false)
	executor.SetStateStore(stateStore)

	pipeline := &connector.Pipeline{
		ID:      "test-pipeline-failure",
		Name:    "Test Pipeline",
		Version: "1.0.0",
		Enabled: true,
	}

	// Execute with failing output
	result, err := executor.Execute(pipeline)
	if err == nil {
		t.Fatal("Execution should fail")
	}
	if result.Status != "error" {
		t.Errorf("Execution status = %s, want error", result.Status)
	}

	// Verify state was NOT persisted (execution failed)
	state, err := stateStore.Load(pipeline.ID)
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}
	if state != nil {
		t.Error("State should NOT be persisted when execution fails")
	}
}
