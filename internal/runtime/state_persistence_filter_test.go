// Package runtime provides the pipeline execution engine.
package runtime

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/canectors/runtime/internal/factory"
	"github.com/canectors/runtime/internal/modules/input"
	"github.com/canectors/runtime/internal/persistence"
	"github.com/canectors/runtime/pkg/connector"
)

// TestExecutor_StatePersistence_ID_WithFilterRenaming tests that ID extraction
// works correctly when filters rename or modify the ID field.
// The ID should be extracted from raw records (before filters), not filtered records.
func TestExecutor_StatePersistence_ID_WithFilterRenaming(t *testing.T) {
	tmpDir := t.TempDir()
	stateStore := persistence.NewStateStore(tmpDir)

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		records := []map[string]interface{}{
			{"id": "10", "name": "Record 10"},
			{"id": "20", "name": "Record 20"},
		}
		_ = json.NewEncoder(w).Encode(records)
	}))
	defer server.Close()

	// Create HTTP polling input with ID persistence
	// ID field is "id" in the raw API response
	config := &connector.ModuleConfig{
		Type: "http-polling",
		Config: map[string]interface{}{
			"endpoint": server.URL,
			"statePersistence": map[string]interface{}{
				"id": map[string]interface{}{
					"enabled":    true,
					"field":      "id", // Field path matches API response structure
					"queryParam": "after_id",
				},
			},
		},
	}

	inputModule, err := input.NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig() error = %v", err)
	}

	// Create mapping filter that renames "id" to "transactionId"
	// This would break ID extraction if we used filteredRecords instead of rawRecords
	mappingConfig := &connector.ModuleConfig{
		Type: "mapping",
		Config: map[string]interface{}{
			"mappings": []map[string]interface{}{
				{
					"source": "id",
					"target": "transactionId", // Rename id to transactionId
				},
			},
		},
	}

	filterModules, err := factory.CreateFilterModules([]connector.ModuleConfig{*mappingConfig})
	if err != nil {
		t.Fatalf("CreateFilterModules() error = %v", err)
	}
	if len(filterModules) != 1 {
		t.Fatalf("Expected 1 filter module, got %d", len(filterModules))
	}

	outputModule := NewMockOutputModule(nil)
	executor := NewExecutorWithModules(inputModule, filterModules, outputModule, false)
	executor.SetStateStore(stateStore)

	pipeline := &connector.Pipeline{
		ID:      "test-pipeline-filter-rename",
		Name:    "Test Pipeline",
		Version: "1.0.0",
		Enabled: true,
	}

	// First execution: no state exists
	result1, err := executor.Execute(pipeline)
	if err != nil {
		t.Fatalf("First execution failed: %v", err)
	}
	if result1.Status != "success" {
		t.Errorf("First execution status = %s, want success", result1.Status)
	}

	// Verify state was persisted with last ID from raw records (before filter)
	// The ID should be "20" (from raw records), not from filtered records where "id" was renamed to "transactionId"
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
		t.Errorf("LastID = %q, want %q (should be from raw records, not filtered)", *state.LastID, "20")
	}

	// Verify that filtered records have "transactionId" instead of "id"
	// This confirms the filter renamed the field
	if len(outputModule.sentRecords) != 2 {
		t.Fatalf("Expected 2 records sent to output, got %d", len(outputModule.sentRecords))
	}
	if _, hasID := outputModule.sentRecords[0]["id"]; hasID {
		t.Error("Filtered records should not have 'id' field (was renamed to 'transactionId')")
	}
	if _, hasTransactionID := outputModule.sentRecords[0]["transactionId"]; !hasTransactionID {
		t.Error("Filtered records should have 'transactionId' field (renamed from 'id')")
	}
}

// TestExecutor_StatePersistence_ID_WithFilterRemoving tests that ID extraction
// works correctly when filters remove the ID field entirely.
func TestExecutor_StatePersistence_ID_WithFilterRemoving(t *testing.T) {
	tmpDir := t.TempDir()
	stateStore := persistence.NewStateStore(tmpDir)

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		records := []map[string]interface{}{
			{"id": "100", "name": "Record 100", "status": "active"},
			{"id": "200", "name": "Record 200", "status": "active"},
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

	// Create mapping filter that only keeps "name" and "status" (removes "id")
	mappingConfig := &connector.ModuleConfig{
		Type: "mapping",
		Config: map[string]interface{}{
			"mappings": []map[string]interface{}{
				{
					"source": "name",
					"target": "name",
				},
				{
					"source": "status",
					"target": "status",
				},
				// Note: "id" is NOT mapped, so it will be removed from filtered records
			},
		},
	}

	filterModules, err := factory.CreateFilterModules([]connector.ModuleConfig{*mappingConfig})
	if err != nil {
		t.Fatalf("CreateFilterModules() error = %v", err)
	}
	if len(filterModules) != 1 {
		t.Fatalf("Expected 1 filter module, got %d", len(filterModules))
	}

	outputModule := NewMockOutputModule(nil)
	executor := NewExecutorWithModules(inputModule, filterModules, outputModule, false)
	executor.SetStateStore(stateStore)

	pipeline := &connector.Pipeline{
		ID:      "test-pipeline-filter-remove",
		Name:    "Test Pipeline",
		Version: "1.0.0",
		Enabled: true,
	}

	// First execution: no state exists
	result1, err := executor.Execute(pipeline)
	if err != nil {
		t.Fatalf("First execution failed: %v", err)
	}
	if result1.Status != "success" {
		t.Errorf("First execution status = %s, want success", result1.Status)
	}

	// Verify state was persisted with last ID from raw records (before filter removed it)
	state, err := stateStore.Load(pipeline.ID)
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}
	if state == nil {
		t.Fatal("State should be persisted after first execution")
	}
	if state.LastID == nil {
		t.Error("State should have LastID extracted from raw records")
	}
	if *state.LastID != "200" {
		t.Errorf("LastID = %q, want %q (should be from raw records before filter removed 'id')", *state.LastID, "200")
	}

	// Verify that filtered records don't have "id" field (was removed by filter)
	if len(outputModule.sentRecords) != 2 {
		t.Fatalf("Expected 2 records sent to output, got %d", len(outputModule.sentRecords))
	}
	if _, hasID := outputModule.sentRecords[0]["id"]; hasID {
		t.Error("Filtered records should not have 'id' field (was removed by filter)")
	}
}
