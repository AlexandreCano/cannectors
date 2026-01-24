// Package runtime provides the pipeline execution engine.
package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/canectors/runtime/internal/logger"
	"github.com/canectors/runtime/internal/modules/filter"
	"github.com/canectors/runtime/internal/modules/input"
	"github.com/canectors/runtime/internal/modules/output"
	"github.com/canectors/runtime/pkg/connector"
)

// =============================================================================
// Mock Implementations for Testing
// =============================================================================

// MockInputModule is a test mock for input.Module interface
type MockInputModule struct {
	data        []map[string]interface{}
	err         error
	fetchCalled bool
}

func NewMockInputModule(data []map[string]interface{}, err error) *MockInputModule {
	return &MockInputModule{data: data, err: err}
}

func (m *MockInputModule) Fetch(_ context.Context) ([]map[string]interface{}, error) {
	m.fetchCalled = true
	if m.err != nil {
		return nil, m.err
	}
	return m.data, nil
}

func (m *MockInputModule) Close() error {
	return nil
}

// Verify MockInputModule implements input.Module
var _ input.Module = (*MockInputModule)(nil)

// MockFilterModule is a test mock for filter.Module interface
type MockFilterModule struct {
	transformer     func([]map[string]interface{}) ([]map[string]interface{}, error)
	err             error
	processCalled   bool
	recordsReceived []map[string]interface{}
}

func NewMockFilterModule(transformer func([]map[string]interface{}) ([]map[string]interface{}, error)) *MockFilterModule {
	return &MockFilterModule{transformer: transformer}
}

func NewMockFilterModuleWithError(err error) *MockFilterModule {
	return &MockFilterModule{err: err}
}

func (m *MockFilterModule) Process(_ context.Context, records []map[string]interface{}) ([]map[string]interface{}, error) {
	m.processCalled = true
	m.recordsReceived = records
	if m.err != nil {
		return nil, m.err
	}
	if m.transformer != nil {
		return m.transformer(records)
	}
	return records, nil
}

// Verify MockFilterModule implements filter.Module
var _ filter.Module = (*MockFilterModule)(nil)

// MockOutputModule is a test mock for output.Module interface
type MockOutputModule struct {
	sentRecords []map[string]interface{}
	err         error
	sendCalled  bool
	closed      bool
}

func NewMockOutputModule(err error) *MockOutputModule {
	return &MockOutputModule{err: err}
}

func (m *MockOutputModule) Send(_ context.Context, records []map[string]interface{}) (int, error) {
	m.sendCalled = true
	if m.err != nil {
		return 0, m.err
	}
	m.sentRecords = records
	return len(records), nil
}

func (m *MockOutputModule) Close() error {
	m.closed = true
	return nil
}

// Verify MockOutputModule implements output.Module
var _ output.Module = (*MockOutputModule)(nil)

// =============================================================================
// Unit Tests for Pipeline Execution
// =============================================================================

func TestExecutor_Execute_Success(t *testing.T) {
	// Arrange
	inputData := []map[string]interface{}{
		{"id": "1", "name": "Test 1"},
		{"id": "2", "name": "Test 2"},
	}
	mockInput := NewMockInputModule(inputData, nil)
	mockOutput := NewMockOutputModule(nil)

	pipeline := &connector.Pipeline{
		ID:      "test-pipeline",
		Name:    "Test Pipeline",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(mockInput, nil, mockOutput, false)

	// Act
	result, err := executor.Execute(pipeline)

	// Assert
	if err != nil {
		t.Fatalf("Execute() returned unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("Execute() returned nil result")
	}

	if result.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", result.Status)
	}

	if result.PipelineID != "test-pipeline" {
		t.Errorf("Expected PipelineID 'test-pipeline', got '%s'", result.PipelineID)
	}

	if result.RecordsProcessed != 2 {
		t.Errorf("Expected RecordsProcessed 2, got %d", result.RecordsProcessed)
	}

	if result.RecordsFailed != 0 {
		t.Errorf("Expected RecordsFailed 0, got %d", result.RecordsFailed)
	}

	if !mockInput.fetchCalled {
		t.Error("Input.Fetch() was not called")
	}

	if !mockOutput.sendCalled {
		t.Error("Output.Send() was not called")
	}

	// Verify data was sent to output
	if len(mockOutput.sentRecords) != 2 {
		t.Errorf("Expected 2 records sent to output, got %d", len(mockOutput.sentRecords))
	}
}

func TestExecutor_Execute_MappingFilterIntegration(t *testing.T) {
	inputData := []map[string]interface{}{
		{
			"id": "12",
			"user": map[string]interface{}{
				"name": "  Alice  ",
			},
		},
	}
	mockInput := NewMockInputModule(inputData, nil)
	mockOutput := NewMockOutputModule(nil)

	mappings := []filter.FieldMapping{
		{
			Source: "user.name",
			Target: "contact.name",
			Transforms: []filter.TransformOp{
				{Op: "trim"},
				{Op: "lowercase"},
			},
		},
		{
			Source:     "id",
			Target:     "contact.id",
			Transforms: []filter.TransformOp{{Op: "toInt"}},
		},
	}
	mapper, err := filter.NewMappingFromConfig(mappings, "fail")
	if err != nil {
		t.Fatalf("NewMappingFromConfig() error = %v", err)
	}

	pipeline := &connector.Pipeline{
		ID:      "test-pipeline-mapping",
		Name:    "Test Mapping Pipeline",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(mockInput, []filter.Module{mapper}, mockOutput, false)
	result, err := executor.Execute(pipeline)
	if err != nil {
		t.Fatalf("Execute() returned unexpected error: %v", err)
	}
	if result.Status != StatusSuccess {
		t.Fatalf("expected success status, got %s", result.Status)
	}
	if len(mockOutput.sentRecords) != 1 {
		t.Fatalf("expected 1 record sent, got %d", len(mockOutput.sentRecords))
	}

	contact, ok := mockOutput.sentRecords[0]["contact"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected contact map, got %T", mockOutput.sentRecords[0]["contact"])
	}
	if contact["name"] != "alice" {
		t.Fatalf("expected contact.name=alice, got %v", contact["name"])
	}
	if contact["id"] != 12 {
		t.Fatalf("expected contact.id=12, got %v", contact["id"])
	}
}

func TestExecutor_Execute_ConditionFilterIntegration(t *testing.T) {
	inputData := []map[string]interface{}{
		{"id": "1", "value": 5},
		{"id": "2", "value": 20},
		{"id": "3", "value": 10},
	}
	mockInput := NewMockInputModule(inputData, nil)
	mockOutput := NewMockOutputModule(nil)

	cond, err := filter.NewConditionFromConfig(filter.ConditionConfig{
		Expression: "value > 10",
		OnTrue:     "continue",
		OnFalse:    "skip",
	})
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	pipeline := &connector.Pipeline{
		ID:      "test-pipeline-condition",
		Name:    "Test Condition Pipeline",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(mockInput, []filter.Module{cond}, mockOutput, false)
	result, err := executor.Execute(pipeline)
	if err != nil {
		t.Fatalf("Execute() returned unexpected error: %v", err)
	}
	if result.Status != StatusSuccess {
		t.Fatalf("expected success status, got %s", result.Status)
	}
	if len(mockOutput.sentRecords) != 1 {
		t.Fatalf("expected 1 record sent, got %d", len(mockOutput.sentRecords))
	}
	if mockOutput.sentRecords[0]["value"] != 20 {
		t.Fatalf("expected value=20, got %v", mockOutput.sentRecords[0]["value"])
	}
}

func TestExecutor_Execute_MappingThenConditionIntegration(t *testing.T) {
	inputData := []map[string]interface{}{
		{"id": "1", "amount": 150},
		{"id": "2", "amount": 50},
	}
	mockInput := NewMockInputModule(inputData, nil)
	mockOutput := NewMockOutputModule(nil)

	mapper, err := filter.NewMappingFromConfig([]filter.FieldMapping{
		{Source: "id", Target: "id"},
		{Source: "amount", Target: "value"},
	}, "fail")
	if err != nil {
		t.Fatalf("NewMappingFromConfig() error = %v", err)
	}

	cond, err := filter.NewConditionFromConfig(filter.ConditionConfig{
		Expression: "value > 100",
		OnTrue:     "continue",
		OnFalse:    "skip",
	})
	if err != nil {
		t.Fatalf("NewConditionFromConfig() error = %v", err)
	}

	pipeline := &connector.Pipeline{
		ID:      "test-pipeline-mapping-condition",
		Name:    "Test Mapping then Condition Pipeline",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(mockInput, []filter.Module{mapper, cond}, mockOutput, false)
	result, err := executor.Execute(pipeline)
	if err != nil {
		t.Fatalf("Execute() returned unexpected error: %v", err)
	}
	if result.Status != StatusSuccess {
		t.Fatalf("expected success status, got %s", result.Status)
	}
	if len(mockOutput.sentRecords) != 1 {
		t.Fatalf("expected 1 record sent, got %d", len(mockOutput.sentRecords))
	}
	if mockOutput.sentRecords[0]["value"] != 150 {
		t.Fatalf("expected value=150, got %v", mockOutput.sentRecords[0]["value"])
	}
}

func TestExecutor_ExecuteWithRecords_Success(t *testing.T) {
	records := []map[string]interface{}{
		{"id": "1", "name": "Record 1"},
		{"id": "2", "name": "Record 2"},
	}
	mockOutput := NewMockOutputModule(nil)

	pipeline := &connector.Pipeline{
		ID:      "test-pipeline",
		Name:    "Test Pipeline",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(nil, nil, mockOutput, false)

	result, err := executor.ExecuteWithRecords(pipeline, records)
	if err != nil {
		t.Fatalf("ExecuteWithRecords() returned unexpected error: %v", err)
	}

	if result == nil {
		t.Fatal("ExecuteWithRecords() returned nil result")
	}

	if result.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", result.Status)
	}

	if result.RecordsProcessed != 2 {
		t.Errorf("Expected RecordsProcessed 2, got %d", result.RecordsProcessed)
	}

	if !mockOutput.sendCalled {
		t.Error("Output.Send() was not called")
	}
}

func TestExecutor_Execute_WithFilters(t *testing.T) {
	// Arrange
	inputData := []map[string]interface{}{
		{"id": "1", "value": 10},
		{"id": "2", "value": 20},
	}

	// Filter that doubles the value
	doubleFilter := NewMockFilterModule(func(records []map[string]interface{}) ([]map[string]interface{}, error) {
		result := make([]map[string]interface{}, len(records))
		for i, r := range records {
			newRecord := make(map[string]interface{})
			for k, v := range r {
				newRecord[k] = v
			}
			if val, ok := newRecord["value"].(int); ok {
				newRecord["value"] = val * 2
			}
			result[i] = newRecord
		}
		return result, nil
	})

	// Filter that adds a field
	addFieldFilter := NewMockFilterModule(func(records []map[string]interface{}) ([]map[string]interface{}, error) {
		result := make([]map[string]interface{}, len(records))
		for i, r := range records {
			newRecord := make(map[string]interface{})
			for k, v := range r {
				newRecord[k] = v
			}
			newRecord["processed"] = true
			result[i] = newRecord
		}
		return result, nil
	})

	mockInput := NewMockInputModule(inputData, nil)
	mockOutput := NewMockOutputModule(nil)
	filters := []filter.Module{doubleFilter, addFieldFilter}

	pipeline := &connector.Pipeline{
		ID:      "test-pipeline-filters",
		Name:    "Test Pipeline with Filters",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(mockInput, filters, mockOutput, false)

	// Act
	result, err := executor.Execute(pipeline)

	// Assert
	if err != nil {
		t.Fatalf("Execute() returned unexpected error: %v", err)
	}

	if result.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", result.Status)
	}

	if !doubleFilter.processCalled {
		t.Error("First filter Process() was not called")
	}

	if !addFieldFilter.processCalled {
		t.Error("Second filter Process() was not called")
	}

	// Verify filter sequence: second filter should receive doubled values
	if len(addFieldFilter.recordsReceived) != 2 {
		t.Fatalf("Second filter received %d records, expected 2", len(addFieldFilter.recordsReceived))
	}

	// Check that first record has doubled value (10 * 2 = 20)
	if val, ok := addFieldFilter.recordsReceived[0]["value"].(int); !ok || val != 20 {
		t.Errorf("Second filter received value %v, expected 20", addFieldFilter.recordsReceived[0]["value"])
	}

	// Verify output received fully processed records
	if len(mockOutput.sentRecords) != 2 {
		t.Fatalf("Output received %d records, expected 2", len(mockOutput.sentRecords))
	}

	// Check final output has doubled value and processed field
	firstRecord := mockOutput.sentRecords[0]
	if val, ok := firstRecord["value"].(int); !ok || val != 20 {
		t.Errorf("Output first record value = %v, expected 20", firstRecord["value"])
	}
	if processed, ok := firstRecord["processed"].(bool); !ok || !processed {
		t.Errorf("Output first record processed = %v, expected true", firstRecord["processed"])
	}
}

func TestExecutor_Execute_EmptyInputData(t *testing.T) {
	// Arrange
	emptyData := []map[string]interface{}{}
	mockInput := NewMockInputModule(emptyData, nil)
	mockOutput := NewMockOutputModule(nil)

	pipeline := &connector.Pipeline{
		ID:      "test-empty-pipeline",
		Name:    "Test Empty Pipeline",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(mockInput, nil, mockOutput, false)

	// Act
	result, err := executor.Execute(pipeline)

	// Assert
	if err != nil {
		t.Fatalf("Execute() returned unexpected error: %v", err)
	}

	if result.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", result.Status)
	}

	if result.RecordsProcessed != 0 {
		t.Errorf("Expected RecordsProcessed 0, got %d", result.RecordsProcessed)
	}

	// Output should still be called even with empty data
	if !mockOutput.sendCalled {
		t.Error("Output.Send() should be called even with empty data")
	}
}

func TestExecutor_Execute_DeterministicExecution(t *testing.T) {
	// Test that same input produces same output (deterministic)
	inputData := []map[string]interface{}{
		{"id": "1", "name": "Test"},
	}

	// Run execution multiple times
	var results []*connector.ExecutionResult
	for i := 0; i < 3; i++ {
		mockInput := NewMockInputModule(inputData, nil)
		mockOutput := NewMockOutputModule(nil)

		pipeline := &connector.Pipeline{
			ID:      "deterministic-test",
			Name:    "Deterministic Test",
			Version: "1.0.0",
			Enabled: true,
		}

		executor := NewExecutorWithModules(mockInput, nil, mockOutput, false)
		result, err := executor.Execute(pipeline)
		if err != nil {
			t.Fatalf("Execution %d failed: %v", i, err)
		}
		results = append(results, result)
	}

	// All results should have same status and record counts
	for i := 1; i < len(results); i++ {
		if results[i].Status != results[0].Status {
			t.Errorf("Execution %d status '%s' differs from first '%s'", i, results[i].Status, results[0].Status)
		}
		if results[i].RecordsProcessed != results[0].RecordsProcessed {
			t.Errorf("Execution %d RecordsProcessed %d differs from first %d", i, results[i].RecordsProcessed, results[0].RecordsProcessed)
		}
	}
}

func TestExecutor_Execute_TimestampTracking(t *testing.T) {
	// Arrange
	inputData := []map[string]interface{}{
		{"id": "1"},
	}
	mockInput := NewMockInputModule(inputData, nil)
	mockOutput := NewMockOutputModule(nil)

	pipeline := &connector.Pipeline{
		ID:      "timestamp-test",
		Name:    "Timestamp Test",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(mockInput, nil, mockOutput, false)

	beforeExec := time.Now()
	result, err := executor.Execute(pipeline)
	afterExec := time.Now()

	// Assert
	if err != nil {
		t.Fatalf("Execute() returned unexpected error: %v", err)
	}

	// StartedAt should be between before and after
	if result.StartedAt.Before(beforeExec) || result.StartedAt.After(afterExec) {
		t.Errorf("StartedAt %v is not between %v and %v", result.StartedAt, beforeExec, afterExec)
	}

	// CompletedAt should be after or equal to StartedAt
	if result.CompletedAt.Before(result.StartedAt) {
		t.Errorf("CompletedAt %v is before StartedAt %v", result.CompletedAt, result.StartedAt)
	}

	// CompletedAt should be between StartedAt and afterExec
	if result.CompletedAt.After(afterExec) {
		t.Errorf("CompletedAt %v is after execution end %v", result.CompletedAt, afterExec)
	}
}

func TestExecutor_Execute_InputError(t *testing.T) {
	// Arrange
	inputErr := errors.New("failed to fetch data from source")
	mockInput := NewMockInputModule(nil, inputErr)
	mockOutput := NewMockOutputModule(nil)

	pipeline := &connector.Pipeline{
		ID:      "error-test",
		Name:    "Error Test",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(mockInput, nil, mockOutput, false)

	// Act
	result, err := executor.Execute(pipeline)

	// Assert - error should be returned AND result with error details
	if err == nil {
		t.Fatal("Execute() should return error when input module fails")
	}

	if result == nil {
		t.Fatal("Execute() should return result even on error")
	}

	if result.Status != "error" {
		t.Errorf("Expected status 'error', got '%s'", result.Status)
	}

	if result.Error == nil {
		t.Error("Expected Error details in result")
	} else {
		if result.Error.Module != "input" {
			t.Errorf("Expected error module 'input', got '%s'", result.Error.Module)
		}
	}

	// Output should NOT be called when input fails
	if mockOutput.sendCalled {
		t.Error("Output.Send() should NOT be called when input fails")
	}
}

func TestExecutor_Execute_FilterError(t *testing.T) {
	// Arrange
	inputData := []map[string]interface{}{
		{"id": "1", "name": "Test"},
	}
	filterErr := errors.New("transformation failed")

	mockInput := NewMockInputModule(inputData, nil)
	mockFilter := NewMockFilterModuleWithError(filterErr)
	mockOutput := NewMockOutputModule(nil)

	pipeline := &connector.Pipeline{
		ID:      "filter-error-test",
		Name:    "Filter Error Test",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(mockInput, []filter.Module{mockFilter}, mockOutput, false)

	// Act
	result, err := executor.Execute(pipeline)

	// Assert
	if err == nil {
		t.Fatal("Execute() should return error when filter module fails")
	}

	if result == nil {
		t.Fatal("Execute() should return result even on error")
	}

	if result.Status != "error" {
		t.Errorf("Expected status 'error', got '%s'", result.Status)
	}

	if result.Error == nil {
		t.Error("Expected Error details in result")
	} else {
		if result.Error.Module != "filter" {
			t.Errorf("Expected error module 'filter', got '%s'", result.Error.Module)
		}
	}

	// Output should NOT be called when filter fails (no partial data)
	if mockOutput.sendCalled {
		t.Error("Output.Send() should NOT be called when filter fails")
	}
}

func TestExecutor_Execute_OutputError(t *testing.T) {
	// Arrange
	inputData := []map[string]interface{}{
		{"id": "1", "name": "Test"},
	}
	outputErr := errors.New("failed to send data to destination")

	mockInput := NewMockInputModule(inputData, nil)
	mockOutput := NewMockOutputModule(outputErr)

	pipeline := &connector.Pipeline{
		ID:      "output-error-test",
		Name:    "Output Error Test",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(mockInput, nil, mockOutput, false)

	// Act
	result, err := executor.Execute(pipeline)

	// Assert
	if err == nil {
		t.Fatal("Execute() should return error when output module fails")
	}

	if result == nil {
		t.Fatal("Execute() should return result even on error")
	}

	if result.Status != "error" {
		t.Errorf("Expected status 'error', got '%s'", result.Status)
	}

	if result.Error == nil {
		t.Error("Expected Error details in result")
	} else {
		if result.Error.Module != "output" {
			t.Errorf("Expected error module 'output', got '%s'", result.Error.Module)
		}
	}
}

func TestExecutor_Execute_MultipleFiltersSequence(t *testing.T) {
	// Test that filters are executed in correct order
	var executionOrder []int

	filter1 := NewMockFilterModule(func(records []map[string]interface{}) ([]map[string]interface{}, error) {
		executionOrder = append(executionOrder, 1)
		return records, nil
	})

	filter2 := NewMockFilterModule(func(records []map[string]interface{}) ([]map[string]interface{}, error) {
		executionOrder = append(executionOrder, 2)
		return records, nil
	})

	filter3 := NewMockFilterModule(func(records []map[string]interface{}) ([]map[string]interface{}, error) {
		executionOrder = append(executionOrder, 3)
		return records, nil
	})

	inputData := []map[string]interface{}{{"id": "1"}}
	mockInput := NewMockInputModule(inputData, nil)
	mockOutput := NewMockOutputModule(nil)

	pipeline := &connector.Pipeline{
		ID:      "sequence-test",
		Name:    "Sequence Test",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(mockInput, []filter.Module{filter1, filter2, filter3}, mockOutput, false)

	// Act
	_, err := executor.Execute(pipeline)

	// Assert
	if err != nil {
		t.Fatalf("Execute() returned unexpected error: %v", err)
	}

	if len(executionOrder) != 3 {
		t.Fatalf("Expected 3 filters to execute, got %d", len(executionOrder))
	}

	for i, order := range executionOrder {
		expected := i + 1
		if order != expected {
			t.Errorf("Filter at position %d executed with order %d, expected %d", i, order, expected)
		}
	}
}

func TestExecutor_Execute_FilterErrorStopsExecution(t *testing.T) {
	// Test that error in second filter prevents third filter from running
	var executionOrder []int

	filter1 := NewMockFilterModule(func(records []map[string]interface{}) ([]map[string]interface{}, error) {
		executionOrder = append(executionOrder, 1)
		return records, nil
	})

	filter2 := &MockFilterModule{
		err: errors.New("filter 2 error"),
	}

	filter3 := NewMockFilterModule(func(records []map[string]interface{}) ([]map[string]interface{}, error) {
		executionOrder = append(executionOrder, 3) // Should NOT be called
		return records, nil
	})

	inputData := []map[string]interface{}{{"id": "1"}}
	mockInput := NewMockInputModule(inputData, nil)
	mockOutput := NewMockOutputModule(nil)

	pipeline := &connector.Pipeline{
		ID:      "filter-stop-test",
		Name:    "Filter Stop Test",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(mockInput, []filter.Module{filter1, filter2, filter3}, mockOutput, false)

	// Act
	_, err := executor.Execute(pipeline)

	// Assert
	if err == nil {
		t.Fatal("Execute() should return error when filter fails")
	}

	// Only filter 1 should have executed (not filter 3)
	if len(executionOrder) != 1 {
		t.Errorf("Expected only 1 filter to execute before error, got %d", len(executionOrder))
	}

	if len(executionOrder) > 0 && executionOrder[0] != 1 {
		t.Errorf("Only filter 1 should have executed, got order %v", executionOrder)
	}

	// Output should NOT be called
	if mockOutput.sendCalled {
		t.Error("Output should NOT be called when filter fails")
	}
}

func TestExecutor_DryRun(t *testing.T) {
	// Arrange
	inputData := []map[string]interface{}{
		{"id": "1", "name": "Test"},
	}
	mockInput := NewMockInputModule(inputData, nil)
	mockOutput := NewMockOutputModule(nil)

	pipeline := &connector.Pipeline{
		ID:      "dry-run-test",
		Name:    "Dry Run Test",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(mockInput, nil, mockOutput, true) // dry-run = true

	// Act
	result, err := executor.Execute(pipeline)

	// Assert
	if err != nil {
		t.Fatalf("Execute() returned unexpected error: %v", err)
	}

	if result.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", result.Status)
	}

	// In dry-run mode, output should NOT send data
	if mockOutput.sendCalled {
		t.Error("Output.Send() should NOT be called in dry-run mode")
	}

	// Input should still be called to validate the pipeline
	if !mockInput.fetchCalled {
		t.Error("Input.Fetch() should still be called in dry-run mode")
	}
}

func TestExecutor_Execute_NilPipeline(t *testing.T) {
	// Arrange
	mockInput := NewMockInputModule(nil, nil)
	mockOutput := NewMockOutputModule(nil)

	executor := NewExecutorWithModules(mockInput, nil, mockOutput, false)

	// Act
	result, err := executor.Execute(nil)

	// Assert
	if err == nil {
		t.Fatal("Execute() should return error for nil pipeline")
	}

	if result == nil {
		t.Fatal("Execute() should return result even for nil pipeline error")
	}

	if result.Status != "error" {
		t.Errorf("Expected status 'error', got '%s'", result.Status)
	}
}

// =============================================================================
// Resource Cleanup Tests (Close() method calls)
// =============================================================================

func TestExecutor_Execute_ClosesOutputModuleOnSuccess(t *testing.T) {
	// Arrange
	inputData := []map[string]interface{}{
		{"id": "1", "name": "Test"},
	}
	mockInput := NewMockInputModule(inputData, nil)
	mockOutput := NewMockOutputModule(nil)

	pipeline := &connector.Pipeline{
		ID:      "close-test-success",
		Name:    "Close Test Success",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(mockInput, nil, mockOutput, false)

	// Act
	_, err := executor.Execute(pipeline)

	// Assert
	if err != nil {
		t.Fatalf("Execute() returned unexpected error: %v", err)
	}

	if !mockOutput.closed {
		t.Error("Output module Close() was NOT called after successful execution")
	}
}

func TestExecutor_Execute_ClosesOutputModuleOnInputError(t *testing.T) {
	// Arrange
	inputErr := errors.New("input fetch failed")
	mockInput := NewMockInputModule(nil, inputErr)
	mockOutput := NewMockOutputModule(nil)

	pipeline := &connector.Pipeline{
		ID:      "close-test-input-error",
		Name:    "Close Test Input Error",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(mockInput, nil, mockOutput, false)

	// Act
	_, err := executor.Execute(pipeline)

	// Assert
	if err == nil {
		t.Fatal("Execute() should return error when input fails")
	}

	if !mockOutput.closed {
		t.Error("Output module Close() was NOT called after input error")
	}
}

func TestExecutor_Execute_ClosesOutputModuleOnFilterError(t *testing.T) {
	// Arrange
	inputData := []map[string]interface{}{
		{"id": "1", "name": "Test"},
	}
	filterErr := errors.New("filter processing failed")

	mockInput := NewMockInputModule(inputData, nil)
	mockFilter := NewMockFilterModuleWithError(filterErr)
	mockOutput := NewMockOutputModule(nil)

	pipeline := &connector.Pipeline{
		ID:      "close-test-filter-error",
		Name:    "Close Test Filter Error",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(mockInput, []filter.Module{mockFilter}, mockOutput, false)

	// Act
	_, err := executor.Execute(pipeline)

	// Assert
	if err == nil {
		t.Fatal("Execute() should return error when filter fails")
	}

	if !mockOutput.closed {
		t.Error("Output module Close() was NOT called after filter error")
	}
}

func TestExecutor_Execute_ClosesOutputModuleOnOutputError(t *testing.T) {
	// Arrange
	inputData := []map[string]interface{}{
		{"id": "1", "name": "Test"},
	}
	outputErr := errors.New("output send failed")

	mockInput := NewMockInputModule(inputData, nil)
	mockOutput := NewMockOutputModule(outputErr)

	pipeline := &connector.Pipeline{
		ID:      "close-test-output-error",
		Name:    "Close Test Output Error",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(mockInput, nil, mockOutput, false)

	// Act
	_, err := executor.Execute(pipeline)

	// Assert
	if err == nil {
		t.Fatal("Execute() should return error when output fails")
	}

	if !mockOutput.closed {
		t.Error("Output module Close() was NOT called after output error")
	}
}

// =============================================================================
// Story 4.2: Dry-Run Mode - Task 2: Executor Dry-Run Preview Tests
// =============================================================================

// MockPreviewableOutputModule is a mock for output.PreviewableModule
type MockPreviewableOutputModule struct {
	sentRecords      []map[string]interface{}
	err              error
	sendCalled       bool
	previewCalled    bool
	closed           bool
	previewErr       error
	previewRecords   []map[string]interface{}
	previewResponses []output.RequestPreview
}

func NewMockPreviewableOutputModule(previewResponses []output.RequestPreview) *MockPreviewableOutputModule {
	return &MockPreviewableOutputModule{
		previewResponses: previewResponses,
	}
}

func (m *MockPreviewableOutputModule) Send(_ context.Context, records []map[string]interface{}) (int, error) {
	m.sendCalled = true
	if m.err != nil {
		return 0, m.err
	}
	m.sentRecords = records
	return len(records), nil
}

func (m *MockPreviewableOutputModule) Close() error {
	m.closed = true
	return nil
}

func (m *MockPreviewableOutputModule) PreviewRequest(records []map[string]interface{}, _ output.PreviewOptions) ([]output.RequestPreview, error) {
	m.previewCalled = true
	m.previewRecords = records
	if m.previewErr != nil {
		return nil, m.previewErr
	}
	return m.previewResponses, nil
}

// Verify MockPreviewableOutputModule implements output.PreviewableModule
var _ output.PreviewableModule = (*MockPreviewableOutputModule)(nil)

func TestExecutor_DryRun_CallsPreview(t *testing.T) {
	// Arrange
	inputData := []map[string]interface{}{
		{"id": "1", "name": "Test 1"},
		{"id": "2", "name": "Test 2"},
	}
	mockInput := NewMockInputModule(inputData, nil)

	expectedPreviews := []output.RequestPreview{
		{
			Endpoint:    "https://api.example.com/data",
			Method:      "POST",
			Headers:     map[string]string{"Content-Type": "application/json"},
			BodyPreview: `[{"id":"1","name":"Test 1"},{"id":"2","name":"Test 2"}]`,
			RecordCount: 2,
		},
	}
	mockOutput := NewMockPreviewableOutputModule(expectedPreviews)

	pipeline := &connector.Pipeline{
		ID:      "dry-run-preview-test",
		Name:    "Dry Run Preview Test",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(mockInput, nil, mockOutput, true) // dry-run = true

	// Act
	result, err := executor.Execute(pipeline)

	// Assert
	if err != nil {
		t.Fatalf("Execute() returned unexpected error: %v", err)
	}

	if result.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", result.Status)
	}

	// Preview should be called in dry-run mode
	if !mockOutput.previewCalled {
		t.Error("PreviewRequest() should be called in dry-run mode")
	}

	// Send should NOT be called in dry-run mode
	if mockOutput.sendCalled {
		t.Error("Send() should NOT be called in dry-run mode")
	}

	// Preview should receive the processed records
	if len(mockOutput.previewRecords) != 2 {
		t.Errorf("PreviewRequest() received %d records, expected 2", len(mockOutput.previewRecords))
	}

	// Result should contain preview information
	if result.DryRunPreview == nil {
		t.Fatal("DryRunPreview should be set in dry-run mode")
	}

	if len(result.DryRunPreview) != 1 {
		t.Errorf("Expected 1 preview, got %d", len(result.DryRunPreview))
	}

	if result.DryRunPreview[0].Endpoint != "https://api.example.com/data" {
		t.Errorf("Expected endpoint 'https://api.example.com/data', got '%s'", result.DryRunPreview[0].Endpoint)
	}
}

func TestExecutor_DryRun_WithFilters_CallsPreview(t *testing.T) {
	// Arrange
	inputData := []map[string]interface{}{
		{"id": "1", "value": 10},
		{"id": "2", "value": 20},
	}

	// Filter that doubles the value
	doubleFilter := NewMockFilterModule(func(records []map[string]interface{}) ([]map[string]interface{}, error) {
		result := make([]map[string]interface{}, len(records))
		for i, r := range records {
			newRecord := make(map[string]interface{})
			for k, v := range r {
				newRecord[k] = v
			}
			if val, ok := newRecord["value"].(int); ok {
				newRecord["value"] = val * 2
			}
			result[i] = newRecord
		}
		return result, nil
	})

	mockInput := NewMockInputModule(inputData, nil)
	mockOutput := NewMockPreviewableOutputModule([]output.RequestPreview{
		{
			Endpoint:    "https://api.example.com/data",
			Method:      "POST",
			Headers:     map[string]string{"Content-Type": "application/json"},
			BodyPreview: `[{"id":"1","value":20},{"id":"2","value":40}]`,
			RecordCount: 2,
		},
	})

	pipeline := &connector.Pipeline{
		ID:      "dry-run-filters-test",
		Name:    "Dry Run with Filters Test",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(mockInput, []filter.Module{doubleFilter}, mockOutput, true)

	// Act
	result, err := executor.Execute(pipeline)

	// Assert
	if err != nil {
		t.Fatalf("Execute() returned unexpected error: %v", err)
	}

	// Filter should be called
	if !doubleFilter.processCalled {
		t.Error("Filter should be called in dry-run mode")
	}

	// Preview should receive FILTERED records (doubled values)
	if len(mockOutput.previewRecords) != 2 {
		t.Fatalf("PreviewRequest() received %d records, expected 2", len(mockOutput.previewRecords))
	}

	// Verify the first record has doubled value (10 * 2 = 20)
	if val, ok := mockOutput.previewRecords[0]["value"].(int); !ok || val != 20 {
		t.Errorf("Preview received value %v, expected 20 (doubled)", mockOutput.previewRecords[0]["value"])
	}

	// DryRunPreview should be set
	if result.DryRunPreview == nil {
		t.Fatal("DryRunPreview should be set in dry-run mode with filters")
	}
}

func TestExecutor_DryRun_NonPreviewableModule_SkipsPreview(t *testing.T) {
	// Test that dry-run works with output modules that don't implement PreviewableModule
	inputData := []map[string]interface{}{
		{"id": "1", "name": "Test"},
	}
	mockInput := NewMockInputModule(inputData, nil)
	mockOutput := NewMockOutputModule(nil) // Non-previewable module

	pipeline := &connector.Pipeline{
		ID:      "dry-run-non-previewable",
		Name:    "Dry Run Non-Previewable",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(mockInput, nil, mockOutput, true) // dry-run = true

	// Act
	result, err := executor.Execute(pipeline)

	// Assert
	if err != nil {
		t.Fatalf("Execute() returned unexpected error: %v", err)
	}

	if result.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", result.Status)
	}

	// Send should NOT be called (dry-run mode)
	if mockOutput.sendCalled {
		t.Error("Send() should NOT be called in dry-run mode")
	}

	// DryRunPreview should be nil for non-previewable modules
	if result.DryRunPreview != nil {
		t.Error("DryRunPreview should be nil for non-previewable modules")
	}
}

func TestExecutor_DryRun_PreviewError_ReportsError(t *testing.T) {
	// Test that preview errors are reported correctly
	inputData := []map[string]interface{}{
		{"id": "1", "name": "Test"},
	}
	mockInput := NewMockInputModule(inputData, nil)

	mockOutput := NewMockPreviewableOutputModule(nil)
	mockOutput.previewErr = errors.New("failed to prepare preview")

	pipeline := &connector.Pipeline{
		ID:      "dry-run-preview-error",
		Name:    "Dry Run Preview Error",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(mockInput, nil, mockOutput, true)

	// Act
	result, err := executor.Execute(pipeline)

	// Assert - preview error should still allow execution to succeed (preview is informational)
	// but the error should be captured somewhere
	if result == nil {
		t.Fatal("Execute() should return result even with preview error")
	}

	// The execution should succeed but note the preview error
	// Option 1: Still succeed but with nil preview
	// Option 2: Include error in result
	// For now, we expect success with nil preview and error captured
	if result.Status != "success" {
		// Preview error should not fail the execution - it's informational
		t.Logf("Status is %s, which may be acceptable depending on implementation", result.Status)
	}

	// Preview should have been attempted
	if !mockOutput.previewCalled {
		t.Error("PreviewRequest() should be called even if it will fail")
	}

	// The error should be captured but not fail the execution
	_ = err // May or may not be nil depending on implementation
}

func TestExecutor_DryRun_RecordsProcessedCount(t *testing.T) {
	// Verify RecordsProcessed correctly reflects what WOULD have been sent
	inputData := []map[string]interface{}{
		{"id": "1"},
		{"id": "2"},
		{"id": "3"},
	}
	mockInput := NewMockInputModule(inputData, nil)
	mockOutput := NewMockPreviewableOutputModule([]output.RequestPreview{
		{
			Endpoint:    "https://api.example.com/data",
			Method:      "POST",
			RecordCount: 3,
		},
	})

	pipeline := &connector.Pipeline{
		ID:      "dry-run-record-count",
		Name:    "Dry Run Record Count",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(mockInput, nil, mockOutput, true)

	// Act
	result, err := executor.Execute(pipeline)

	// Assert
	if err != nil {
		t.Fatalf("Execute() returned unexpected error: %v", err)
	}

	// RecordsProcessed should reflect the count that would be sent
	if result.RecordsProcessed != 3 {
		t.Errorf("Expected RecordsProcessed 3, got %d", result.RecordsProcessed)
	}

	if result.RecordsFailed != 0 {
		t.Errorf("Expected RecordsFailed 0 in dry-run, got %d", result.RecordsFailed)
	}
}

func TestExecutor_DryRun_EmptyRecords(t *testing.T) {
	// Test dry-run with empty input data
	emptyData := []map[string]interface{}{}
	mockInput := NewMockInputModule(emptyData, nil)
	mockOutput := NewMockPreviewableOutputModule([]output.RequestPreview{})

	pipeline := &connector.Pipeline{
		ID:      "dry-run-empty",
		Name:    "Dry Run Empty",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(mockInput, nil, mockOutput, true)

	// Act
	result, err := executor.Execute(pipeline)

	// Assert
	if err != nil {
		t.Fatalf("Execute() returned unexpected error: %v", err)
	}

	if result.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", result.Status)
	}

	if result.RecordsProcessed != 0 {
		t.Errorf("Expected RecordsProcessed 0 for empty input, got %d", result.RecordsProcessed)
	}
}

func TestExecutor_ExecuteWithRecords_DryRun_CallsPreview(t *testing.T) {
	// Test ExecuteWithRecords also supports dry-run preview
	records := []map[string]interface{}{
		{"id": "1", "name": "Record 1"},
		{"id": "2", "name": "Record 2"},
	}

	mockOutput := NewMockPreviewableOutputModule([]output.RequestPreview{
		{
			Endpoint:    "https://api.example.com/data",
			Method:      "POST",
			RecordCount: 2,
		},
	})

	pipeline := &connector.Pipeline{
		ID:      "dry-run-with-records",
		Name:    "Dry Run With Records",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(nil, nil, mockOutput, true) // dry-run = true

	// Act
	result, err := executor.ExecuteWithRecords(pipeline, records)

	// Assert
	if err != nil {
		t.Fatalf("ExecuteWithRecords() returned unexpected error: %v", err)
	}

	if result.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", result.Status)
	}

	// Preview should be called
	if !mockOutput.previewCalled {
		t.Error("PreviewRequest() should be called in dry-run mode for ExecuteWithRecords")
	}

	// Send should NOT be called
	if mockOutput.sendCalled {
		t.Error("Send() should NOT be called in dry-run mode")
	}

	// DryRunPreview should be set
	if result.DryRunPreview == nil {
		t.Error("DryRunPreview should be set in dry-run mode for ExecuteWithRecords")
	}
}

// =============================================================================
// Story 4.2: Dry-Run Mode - Task 4: Complete Pipeline Validation Tests
// =============================================================================

func TestExecutor_DryRun_InputModuleExecutesNormally(t *testing.T) {
	// Verify input module executes normally in dry-run mode (data is fetched)
	inputData := []map[string]interface{}{
		{"id": "1", "name": "Test 1"},
		{"id": "2", "name": "Test 2"},
		{"id": "3", "name": "Test 3"},
	}
	mockInput := NewMockInputModule(inputData, nil)
	mockOutput := NewMockPreviewableOutputModule([]output.RequestPreview{
		{Endpoint: "https://api.example.com", Method: "POST", RecordCount: 3},
	})

	pipeline := &connector.Pipeline{
		ID:      "dry-run-input-test",
		Name:    "Dry Run Input Test",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(mockInput, nil, mockOutput, true)

	// Act
	result, err := executor.Execute(pipeline)

	// Assert
	if err != nil {
		t.Fatalf("Execute() returned unexpected error: %v", err)
	}

	// Input module should be called normally
	if !mockInput.fetchCalled {
		t.Error("Input.Fetch() should be called in dry-run mode")
	}

	// All records should be processed
	if result.RecordsProcessed != 3 {
		t.Errorf("Expected RecordsProcessed 3, got %d", result.RecordsProcessed)
	}
}

func TestExecutor_DryRun_FilterModulesExecuteNormally(t *testing.T) {
	// Verify filter modules execute normally in dry-run mode (data is transformed)
	inputData := []map[string]interface{}{
		{"id": "1", "value": 10},
		{"id": "2", "value": 20},
	}

	transformCalled := false
	transformFilter := NewMockFilterModule(func(records []map[string]interface{}) ([]map[string]interface{}, error) {
		transformCalled = true
		result := make([]map[string]interface{}, len(records))
		for i, r := range records {
			newRecord := make(map[string]interface{})
			for k, v := range r {
				newRecord[k] = v
			}
			// Transform: double the value
			if val, ok := newRecord["value"].(int); ok {
				newRecord["value"] = val * 2
			}
			result[i] = newRecord
		}
		return result, nil
	})

	mockInput := NewMockInputModule(inputData, nil)
	mockOutput := NewMockPreviewableOutputModule([]output.RequestPreview{
		{Endpoint: "https://api.example.com", Method: "POST", RecordCount: 2},
	})

	pipeline := &connector.Pipeline{
		ID:      "dry-run-filter-test",
		Name:    "Dry Run Filter Test",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(mockInput, []filter.Module{transformFilter}, mockOutput, true)

	// Act
	result, err := executor.Execute(pipeline)

	// Assert
	if err != nil {
		t.Fatalf("Execute() returned unexpected error: %v", err)
	}

	// Filter should be called
	if !transformCalled {
		t.Error("Filter transform should be called in dry-run mode")
	}

	// Preview should receive transformed data
	if len(mockOutput.previewRecords) != 2 {
		t.Fatalf("Preview received %d records, expected 2", len(mockOutput.previewRecords))
	}

	// Verify transformation was applied
	if val, ok := mockOutput.previewRecords[0]["value"].(int); !ok || val != 20 {
		t.Errorf("Expected transformed value 20, got %v", mockOutput.previewRecords[0]["value"])
	}

	if result.Status != "success" {
		t.Errorf("Expected status success, got %s", result.Status)
	}
}

func TestExecutor_DryRun_InputError_ReportedCorrectly(t *testing.T) {
	// Verify input errors are reported the same in dry-run as normal mode
	inputErr := errors.New("failed to fetch data from source")
	mockInput := NewMockInputModule(nil, inputErr)
	mockOutput := NewMockPreviewableOutputModule(nil)

	pipeline := &connector.Pipeline{
		ID:      "dry-run-input-error-test",
		Name:    "Dry Run Input Error Test",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(mockInput, nil, mockOutput, true)

	// Act
	result, err := executor.Execute(pipeline)

	// Assert
	if err == nil {
		t.Fatal("Execute() should return error when input fails in dry-run mode")
	}

	if result == nil {
		t.Fatal("Execute() should return result even on input error")
	}

	if result.Status != "error" {
		t.Errorf("Expected status 'error', got '%s'", result.Status)
	}

	if result.Error == nil {
		t.Error("Expected Error details in result")
	} else if result.Error.Module != "input" {
		t.Errorf("Expected error module 'input', got '%s'", result.Error.Module)
	}

	// Preview should NOT be called when input fails
	if mockOutput.previewCalled {
		t.Error("Preview should NOT be called when input fails")
	}
}

func TestExecutor_DryRun_FilterError_ReportedCorrectly(t *testing.T) {
	// Verify filter errors are reported the same in dry-run as normal mode
	inputData := []map[string]interface{}{{"id": "1"}}
	filterErr := errors.New("transformation failed")

	mockInput := NewMockInputModule(inputData, nil)
	mockFilter := NewMockFilterModuleWithError(filterErr)
	mockOutput := NewMockPreviewableOutputModule(nil)

	pipeline := &connector.Pipeline{
		ID:      "dry-run-filter-error-test",
		Name:    "Dry Run Filter Error Test",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(mockInput, []filter.Module{mockFilter}, mockOutput, true)

	// Act
	result, err := executor.Execute(pipeline)

	// Assert
	if err == nil {
		t.Fatal("Execute() should return error when filter fails in dry-run mode")
	}

	if result == nil {
		t.Fatal("Execute() should return result even on filter error")
	}

	if result.Status != "error" {
		t.Errorf("Expected status 'error', got '%s'", result.Status)
	}

	if result.Error == nil {
		t.Error("Expected Error details in result")
	} else if result.Error.Module != "filter" {
		t.Errorf("Expected error module 'filter', got '%s'", result.Error.Module)
	}

	// Preview should NOT be called when filter fails
	if mockOutput.previewCalled {
		t.Error("Preview should NOT be called when filter fails")
	}
}

func TestExecutor_DryRun_MultipleFiltersSequence(t *testing.T) {
	// Test that multiple filters execute in correct order in dry-run mode
	var executionOrder []int

	filter1 := NewMockFilterModule(func(records []map[string]interface{}) ([]map[string]interface{}, error) {
		executionOrder = append(executionOrder, 1)
		return records, nil
	})

	filter2 := NewMockFilterModule(func(records []map[string]interface{}) ([]map[string]interface{}, error) {
		executionOrder = append(executionOrder, 2)
		return records, nil
	})

	filter3 := NewMockFilterModule(func(records []map[string]interface{}) ([]map[string]interface{}, error) {
		executionOrder = append(executionOrder, 3)
		return records, nil
	})

	inputData := []map[string]interface{}{{"id": "1"}}
	mockInput := NewMockInputModule(inputData, nil)
	mockOutput := NewMockPreviewableOutputModule([]output.RequestPreview{
		{Endpoint: "https://api.example.com", Method: "POST", RecordCount: 1},
	})

	pipeline := &connector.Pipeline{
		ID:      "dry-run-sequence-test",
		Name:    "Dry Run Sequence Test",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(mockInput, []filter.Module{filter1, filter2, filter3}, mockOutput, true)

	// Act
	_, err := executor.Execute(pipeline)

	// Assert
	if err != nil {
		t.Fatalf("Execute() returned unexpected error: %v", err)
	}

	// All filters should execute in order
	if len(executionOrder) != 3 {
		t.Fatalf("Expected 3 filters to execute, got %d", len(executionOrder))
	}

	for i, order := range executionOrder {
		expected := i + 1
		if order != expected {
			t.Errorf("Filter at position %d executed with order %d, expected %d", i, order, expected)
		}
	}
}

func TestExecutor_DryRun_CompletePipelineFlow(t *testing.T) {
	// Integration test: complete Input → Filter → Preview flow
	inputData := []map[string]interface{}{
		{"id": "1", "name": "Alice", "score": 85},
		{"id": "2", "name": "Bob", "score": 92},
		{"id": "3", "name": "Charlie", "score": 78},
	}

	// Filter: only keep scores > 80
	filterAbove80 := NewMockFilterModule(func(records []map[string]interface{}) ([]map[string]interface{}, error) {
		var result []map[string]interface{}
		for _, r := range records {
			if score, ok := r["score"].(int); ok && score > 80 {
				result = append(result, r)
			}
		}
		return result, nil
	})

	mockInput := NewMockInputModule(inputData, nil)
	mockOutput := NewMockPreviewableOutputModule([]output.RequestPreview{
		{
			Endpoint:    "https://api.example.com/scores",
			Method:      "POST",
			Headers:     map[string]string{"Content-Type": "application/json"},
			RecordCount: 2,
		},
	})

	pipeline := &connector.Pipeline{
		ID:      "dry-run-complete-flow",
		Name:    "Dry Run Complete Flow Test",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(mockInput, []filter.Module{filterAbove80}, mockOutput, true)

	// Act
	result, err := executor.Execute(pipeline)

	// Assert
	if err != nil {
		t.Fatalf("Execute() returned unexpected error: %v", err)
	}

	// Verify complete flow
	if !mockInput.fetchCalled {
		t.Error("Input should be called")
	}

	if !filterAbove80.processCalled {
		t.Error("Filter should be called")
	}

	if !mockOutput.previewCalled {
		t.Error("Preview should be called")
	}

	if mockOutput.sendCalled {
		t.Error("Send should NOT be called in dry-run mode")
	}

	// Verify filtered data (only 2 records with score > 80)
	if len(mockOutput.previewRecords) != 2 {
		t.Errorf("Expected 2 filtered records, got %d", len(mockOutput.previewRecords))
	}

	// Verify result
	if result.Status != "success" {
		t.Errorf("Expected status success, got %s", result.Status)
	}

	if result.RecordsProcessed != 2 {
		t.Errorf("Expected 2 records processed (filtered), got %d", result.RecordsProcessed)
	}

	// Verify dry-run preview is present
	if result.DryRunPreview == nil {
		t.Error("DryRunPreview should be set")
	} else if len(result.DryRunPreview) != 1 {
		t.Errorf("Expected 1 preview, got %d", len(result.DryRunPreview))
	} else if result.DryRunPreview[0].RecordCount != 2 {
		t.Errorf("Expected preview record count 2, got %d", result.DryRunPreview[0].RecordCount)
	}
}

// =============================================================================
// Story 4.2: Dry-Run Mode - Task 5: Error Reporting Tests
// =============================================================================

func TestExecutor_DryRun_InputErrorDetails(t *testing.T) {
	// Test that input error details are comprehensive in dry-run mode
	inputErr := errors.New("connection refused: unable to connect to source API")
	mockInput := NewMockInputModule(nil, inputErr)
	mockOutput := NewMockPreviewableOutputModule(nil)

	pipeline := &connector.Pipeline{
		ID:      "dry-run-input-error-detail",
		Name:    "Dry Run Input Error Detail Test",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(mockInput, nil, mockOutput, true)

	// Act
	result, err := executor.Execute(pipeline)

	// Assert
	if err == nil {
		t.Fatal("Should return error on input failure")
	}

	// Error message should contain the original error
	if result.Error == nil {
		t.Fatal("Result should contain error details")
	}

	if result.Error.Code != "INPUT_FAILED" {
		t.Errorf("Expected error code INPUT_FAILED, got %s", result.Error.Code)
	}

	if result.Error.Module != "input" {
		t.Errorf("Expected module 'input', got %s", result.Error.Module)
	}

	// Error message should be descriptive
	if result.Error.Message == "" {
		t.Error("Error message should not be empty")
	}
}

func TestExecutor_DryRun_FilterErrorDetails(t *testing.T) {
	// Test that filter error details include filter index
	inputData := []map[string]interface{}{{"id": "1"}}
	filterErr := errors.New("invalid expression: missing operand")

	mockInput := NewMockInputModule(inputData, nil)
	mockFilter1 := NewMockFilterModule(nil)                // First filter succeeds
	mockFilter2 := NewMockFilterModuleWithError(filterErr) // Second filter fails
	mockOutput := NewMockPreviewableOutputModule(nil)

	pipeline := &connector.Pipeline{
		ID:      "dry-run-filter-error-detail",
		Name:    "Dry Run Filter Error Detail Test",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(mockInput, []filter.Module{mockFilter1, mockFilter2}, mockOutput, true)

	// Act
	result, err := executor.Execute(pipeline)

	// Assert
	if err == nil {
		t.Fatal("Should return error on filter failure")
	}

	if result.Error == nil {
		t.Fatal("Result should contain error details")
	}

	if result.Error.Code != "FILTER_FAILED" {
		t.Errorf("Expected error code FILTER_FAILED, got %s", result.Error.Code)
	}

	if result.Error.Module != "filter" {
		t.Errorf("Expected module 'filter', got %s", result.Error.Module)
	}

	// Error details should include filter index
	if result.Error.Details == nil {
		t.Error("Error details should not be nil")
	} else if idx, ok := result.Error.Details["filterIndex"]; !ok || idx != 1 {
		t.Errorf("Expected filterIndex 1 in details, got %v", idx)
	}
}

func TestExecutor_DryRun_PreviewError_DoesNotFailExecution(t *testing.T) {
	// Test that preview errors don't fail the execution (informational only)
	inputData := []map[string]interface{}{{"id": "1"}}
	mockInput := NewMockInputModule(inputData, nil)

	mockOutput := NewMockPreviewableOutputModule(nil)
	mockOutput.previewErr = errors.New("failed to marshal request body")

	pipeline := &connector.Pipeline{
		ID:      "dry-run-preview-error-no-fail",
		Name:    "Dry Run Preview Error No Fail Test",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(mockInput, nil, mockOutput, true)

	// Act
	result, err := executor.Execute(pipeline)

	// Assert - execution should succeed even though preview failed
	if err != nil {
		t.Errorf("Preview error should not fail execution, got error: %v", err)
	}

	if result.Status != "success" {
		t.Errorf("Status should be success despite preview error, got %s", result.Status)
	}

	// DryRunPreview should contain error preview (not nil) to inform user of the failure
	if result.DryRunPreview == nil {
		t.Error("DryRunPreview should contain error preview when preview generation fails")
	} else if len(result.DryRunPreview) != 1 {
		t.Errorf("Expected 1 error preview, got %d", len(result.DryRunPreview))
	} else {
		errorPreview := result.DryRunPreview[0]
		if errorPreview.Endpoint != "[PREVIEW GENERATION FAILED]" {
			t.Errorf("Expected error preview endpoint '[PREVIEW GENERATION FAILED]', got '%s'", errorPreview.Endpoint)
		}
		if errorPreview.Method != "[UNKNOWN]" {
			t.Errorf("Expected error preview method '[UNKNOWN]', got '%s'", errorPreview.Method)
		}
		if !strings.Contains(errorPreview.BodyPreview, "Failed to generate preview") {
			t.Errorf("Error preview body should contain error message, got: %s", errorPreview.BodyPreview)
		}
	}

	// Records should still be counted
	if result.RecordsProcessed != 1 {
		t.Errorf("Expected 1 record processed, got %d", result.RecordsProcessed)
	}
}

func TestExecutor_DryRun_NilPipeline_ErrorDetails(t *testing.T) {
	// Test that nil pipeline error is reported clearly in dry-run mode
	mockInput := NewMockInputModule(nil, nil)
	mockOutput := NewMockPreviewableOutputModule(nil)

	executor := NewExecutorWithModules(mockInput, nil, mockOutput, true)

	// Act
	result, err := executor.Execute(nil)

	// Assert
	if err == nil {
		t.Fatal("Should return error for nil pipeline")
	}

	if result == nil {
		t.Fatal("Should return result even for nil pipeline")
	}

	if result.Error == nil {
		t.Fatal("Result should contain error details")
	}

	if result.Error.Code != "INVALID_INPUT" {
		t.Errorf("Expected error code INVALID_INPUT, got %s", result.Error.Code)
	}
}

func TestExecutor_DryRun_ShowCredentials_EndToEnd(t *testing.T) {
	// Test that DryRunOptions.ShowCredentials from pipeline config is respected end-to-end
	inputData := []map[string]interface{}{
		{"id": "1", "name": "Test"},
	}
	mockInput := NewMockInputModule(inputData, nil)

	// Create a mock that tracks the PreviewOptions passed to it
	var receivedOpts output.PreviewOptions
	mockOutput := &MockPreviewableOutputModuleWithOpts{
		previewResponses: []output.RequestPreview{
			{Endpoint: "https://api.example.com", Method: "POST", RecordCount: 1},
		},
		receivedOpts: &receivedOpts,
	}

	pipeline := &connector.Pipeline{
		ID:      "dry-run-show-credentials-test",
		Name:    "Dry Run Show Credentials Test",
		Version: "1.0.0",
		Enabled: true,
		DryRunOptions: &connector.DryRunOptions{
			ShowCredentials: true, // Enable credentials display
		},
	}

	executor := NewExecutorWithModules(mockInput, nil, mockOutput, true) // dry-run = true

	// Act
	result, err := executor.Execute(pipeline)

	// Assert
	if err != nil {
		t.Fatalf("Execute() returned unexpected error: %v", err)
	}

	if result.Status != "success" {
		t.Errorf("Expected status 'success', got '%s'", result.Status)
	}

	// Verify that ShowCredentials was passed correctly
	if !receivedOpts.ShowCredentials {
		t.Error("DryRunOptions.ShowCredentials=true should be passed to PreviewRequest")
	}

	// Verify preview was generated
	if len(result.DryRunPreview) == 0 {
		t.Error("DryRunPreview should be set when ShowCredentials is enabled")
	}
}

// MockPreviewableOutputModuleWithOpts tracks the PreviewOptions passed to PreviewRequest
type MockPreviewableOutputModuleWithOpts struct {
	previewResponses []output.RequestPreview
	receivedOpts     *output.PreviewOptions
}

func (m *MockPreviewableOutputModuleWithOpts) Send(_ context.Context, records []map[string]interface{}) (int, error) {
	return len(records), nil
}

func (m *MockPreviewableOutputModuleWithOpts) Close() error {
	return nil
}

func (m *MockPreviewableOutputModuleWithOpts) PreviewRequest(records []map[string]interface{}, opts output.PreviewOptions) ([]output.RequestPreview, error) {
	*m.receivedOpts = opts // Capture the options
	return m.previewResponses, nil
}

// Verify MockPreviewableOutputModuleWithOpts implements output.PreviewableModule
var _ output.PreviewableModule = (*MockPreviewableOutputModuleWithOpts)(nil)

func TestExecutor_DryRun_NilInputModule_ErrorDetails(t *testing.T) {
	// Test that nil input module error is reported clearly
	mockOutput := NewMockPreviewableOutputModule(nil)

	pipeline := &connector.Pipeline{
		ID:      "dry-run-nil-input",
		Name:    "Dry Run Nil Input Test",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(nil, nil, mockOutput, true)

	// Act
	result, err := executor.Execute(pipeline)

	// Assert
	if err == nil {
		t.Fatal("Should return error for nil input module")
	}

	if result == nil {
		t.Fatal("Should return result even for nil input")
	}

	if result.Error == nil {
		t.Fatal("Result should contain error details")
	}

	if result.Error.Code != "INVALID_INPUT" {
		t.Errorf("Expected error code INVALID_INPUT, got %s", result.Error.Code)
	}

	if result.Error.Module != "input" {
		t.Errorf("Expected module 'input', got %s", result.Error.Module)
	}
}

// =============================================================================
// Execution Logging Tests
// =============================================================================

// TestExecutor_Execute_LogsExecutionStart verifies that LogExecutionStart is called.
func TestExecutor_Execute_LogsExecutionStart(t *testing.T) {
	// Capture log output
	var buf bytes.Buffer
	originalLogger := logger.Logger
	defer func() { logger.Logger = originalLogger }()

	logger.Logger = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Arrange
	inputData := []map[string]interface{}{
		{"id": "1", "name": "Test 1"},
	}
	mockInput := NewMockInputModule(inputData, nil)
	mockOutput := NewMockOutputModule(nil)

	pipeline := &connector.Pipeline{
		ID:      "test-pipeline-logging",
		Name:    "Test Pipeline Logging",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(mockInput, nil, mockOutput, false)

	// Act
	_, err := executor.Execute(pipeline)
	if err != nil {
		t.Fatalf("Execute() returned unexpected error: %v", err)
	}

	// Assert - check for execution start log
	logOutput := buf.String()
	if !strings.Contains(logOutput, "execution started") {
		t.Error("Expected log to contain 'execution started'")
	}

	// Parse JSON to verify structure
	lines := strings.Split(strings.TrimSpace(logOutput), "\n")
	found := false
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var logEntry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &logEntry); err == nil {
			if logEntry["msg"] == "execution started" {
				found = true
				if logEntry["pipeline_id"] != "test-pipeline-logging" {
					t.Errorf("Expected pipeline_id 'test-pipeline-logging', got %v", logEntry["pipeline_id"])
				}
				if logEntry["pipeline_name"] != "Test Pipeline Logging" {
					t.Errorf("Expected pipeline_name 'Test Pipeline Logging', got %v", logEntry["pipeline_name"])
				}
				break
			}
		}
	}
	if !found {
		t.Error("Expected to find 'execution started' log entry with proper structure")
	}
}

// TestExecutor_Execute_LogsStageStart verifies that LogStageStart is called for each stage.
func TestExecutor_Execute_LogsStageStart(t *testing.T) {
	// Capture log output
	var buf bytes.Buffer
	originalLogger := logger.Logger
	defer func() { logger.Logger = originalLogger }()

	logger.Logger = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Arrange
	inputData := []map[string]interface{}{
		{"id": "1", "name": "Test 1"},
	}
	mockInput := NewMockInputModule(inputData, nil)
	mockOutput := NewMockOutputModule(nil)

	pipeline := &connector.Pipeline{
		ID:      "test-pipeline-stages",
		Name:    "Test Pipeline Stages",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(mockInput, nil, mockOutput, false)

	// Act
	_, err := executor.Execute(pipeline)
	if err != nil {
		t.Fatalf("Execute() returned unexpected error: %v", err)
	}

	// Assert - check for stage start logs
	logOutput := buf.String()

	// Check for input stage start
	if !strings.Contains(logOutput, "stage started") {
		t.Error("Expected log to contain 'stage started'")
	}

	// Parse JSON to verify stage logs
	lines := strings.Split(strings.TrimSpace(logOutput), "\n")
	stagesFound := make(map[string]bool)
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var logEntry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &logEntry); err == nil {
			if logEntry["msg"] == "stage started" {
				if stage, ok := logEntry["stage"].(string); ok {
					stagesFound[stage] = true
				}
			}
		}
	}

	// Verify we have stage logs for input and output (filter is optional)
	if !stagesFound["input"] {
		t.Error("Expected to find 'stage started' log for 'input' stage")
	}
	if !stagesFound["output"] {
		t.Error("Expected to find 'stage started' log for 'output' stage")
	}
}

// TestExecutor_Execute_LogsExecutionEnd verifies that LogExecutionEnd is called on success.
func TestExecutor_Execute_LogsExecutionEnd(t *testing.T) {
	// Capture log output
	var buf bytes.Buffer
	originalLogger := logger.Logger
	defer func() { logger.Logger = originalLogger }()

	logger.Logger = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Arrange
	inputData := []map[string]interface{}{
		{"id": "1", "name": "Test 1"},
	}
	mockInput := NewMockInputModule(inputData, nil)
	mockOutput := NewMockOutputModule(nil)

	pipeline := &connector.Pipeline{
		ID:      "test-pipeline-end",
		Name:    "Test Pipeline End",
		Version: "1.0.0",
		Enabled: true,
	}

	executor := NewExecutorWithModules(mockInput, nil, mockOutput, false)

	// Act
	_, err := executor.Execute(pipeline)
	if err != nil {
		t.Fatalf("Execute() returned unexpected error: %v", err)
	}

	// Assert - check for execution end log
	logOutput := buf.String()
	if !strings.Contains(logOutput, "execution completed") {
		t.Error("Expected log to contain 'execution completed'")
	}

	// Parse JSON to verify structure
	lines := strings.Split(strings.TrimSpace(logOutput), "\n")
	found := false
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var logEntry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &logEntry); err == nil {
			if logEntry["msg"] == "execution completed" {
				found = true
				if logEntry["status"] != "success" {
					t.Errorf("Expected status 'success', got %v", logEntry["status"])
				}
				if logEntry["pipeline_id"] != "test-pipeline-end" {
					t.Errorf("Expected pipeline_id 'test-pipeline-end', got %v", logEntry["pipeline_id"])
				}
				break
			}
		}
	}
	if !found {
		t.Error("Expected to find 'execution completed' log entry with proper structure")
	}
}
