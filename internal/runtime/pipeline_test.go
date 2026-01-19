// Package runtime provides the pipeline execution engine.
package runtime

import (
	"errors"
	"testing"
	"time"

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

func (m *MockInputModule) Fetch() ([]map[string]interface{}, error) {
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

func (m *MockFilterModule) Process(records []map[string]interface{}) ([]map[string]interface{}, error) {
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

func (m *MockOutputModule) Send(records []map[string]interface{}) (int, error) {
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
