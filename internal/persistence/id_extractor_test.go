// Package persistence provides state persistence for pipeline execution.
package persistence

import (
	"testing"
)

func TestExtractID_SimpleField(t *testing.T) {
	record := map[string]interface{}{
		"id":   "12345",
		"name": "test",
	}

	id, err := ExtractID(record, "id")
	if err != nil {
		t.Fatalf("ExtractID failed: %v", err)
	}
	if id != "12345" {
		t.Errorf("ExtractID = %q, want %q", id, "12345")
	}
}

func TestExtractID_NumericID(t *testing.T) {
	record := map[string]interface{}{
		"id": float64(12345), // JSON numbers are float64
	}

	id, err := ExtractID(record, "id")
	if err != nil {
		t.Fatalf("ExtractID failed: %v", err)
	}
	if id != "12345" {
		t.Errorf("ExtractID = %q, want %q", id, "12345")
	}
}

func TestExtractID_IntegerID(t *testing.T) {
	record := map[string]interface{}{
		"id": int(67890),
	}

	id, err := ExtractID(record, "id")
	if err != nil {
		t.Fatalf("ExtractID failed: %v", err)
	}
	if id != "67890" {
		t.Errorf("ExtractID = %q, want %q", id, "67890")
	}
}

func TestExtractID_NestedField(t *testing.T) {
	record := map[string]interface{}{
		"data": map[string]interface{}{
			"id": "nested-123",
		},
	}

	id, err := ExtractID(record, "data.id")
	if err != nil {
		t.Fatalf("ExtractID failed: %v", err)
	}
	if id != "nested-123" {
		t.Errorf("ExtractID = %q, want %q", id, "nested-123")
	}
}

func TestExtractID_DeeplyNestedField(t *testing.T) {
	record := map[string]interface{}{
		"response": map[string]interface{}{
			"data": map[string]interface{}{
				"record": map[string]interface{}{
					"cursor": "deep-cursor-456",
				},
			},
		},
	}

	id, err := ExtractID(record, "response.data.record.cursor")
	if err != nil {
		t.Fatalf("ExtractID failed: %v", err)
	}
	if id != "deep-cursor-456" {
		t.Errorf("ExtractID = %q, want %q", id, "deep-cursor-456")
	}
}

func TestExtractID_MissingField(t *testing.T) {
	record := map[string]interface{}{
		"name": "test",
	}

	_, err := ExtractID(record, "id")
	if err == nil {
		t.Error("ExtractID should return error for missing field")
	}
}

func TestExtractID_MissingNestedField(t *testing.T) {
	record := map[string]interface{}{
		"data": map[string]interface{}{
			"name": "test",
		},
	}

	_, err := ExtractID(record, "data.id")
	if err == nil {
		t.Error("ExtractID should return error for missing nested field")
	}
}

func TestExtractID_InvalidIntermediateType(t *testing.T) {
	record := map[string]interface{}{
		"data": "not a map",
	}

	_, err := ExtractID(record, "data.id")
	if err == nil {
		t.Error("ExtractID should return error for invalid intermediate type")
	}
}

func TestExtractID_EmptyFieldPath(t *testing.T) {
	record := map[string]interface{}{
		"id": "12345",
	}

	_, err := ExtractID(record, "")
	if err == nil {
		t.Error("ExtractID should return error for empty field path")
	}
}

func TestExtractID_NilRecord(t *testing.T) {
	_, err := ExtractID(nil, "id")
	if err == nil {
		t.Error("ExtractID should return error for nil record")
	}
}

func TestExtractLastID_StringIDs(t *testing.T) {
	// Records in reception order - last record has "bbb"
	records := []map[string]interface{}{
		{"id": "aaa"},
		{"id": "ccc"},
		{"id": "bbb"},
	}

	lastID, err := ExtractLastID(records, "id")
	if err != nil {
		t.Fatalf("ExtractLastID failed: %v", err)
	}
	// Should return the last record's ID, not the max
	if lastID != "bbb" {
		t.Errorf("ExtractLastID = %q, want %q", lastID, "bbb")
	}
}

func TestExtractLastID_NumericIDs(t *testing.T) {
	// Records in reception order - last record has 200
	records := []map[string]interface{}{
		{"id": float64(100)},
		{"id": float64(300)},
		{"id": float64(200)},
	}

	lastID, err := ExtractLastID(records, "id")
	if err != nil {
		t.Fatalf("ExtractLastID failed: %v", err)
	}
	// Should return the last record's ID
	if lastID != "200" {
		t.Errorf("ExtractLastID = %q, want %q", lastID, "200")
	}
}

func TestExtractLastID_SingleRecord(t *testing.T) {
	records := []map[string]interface{}{
		{"id": "only-one"},
	}

	lastID, err := ExtractLastID(records, "id")
	if err != nil {
		t.Fatalf("ExtractLastID failed: %v", err)
	}
	if lastID != "only-one" {
		t.Errorf("ExtractLastID = %q, want %q", lastID, "only-one")
	}
}

func TestExtractLastID_NestedField(t *testing.T) {
	// Records in reception order - last record has "002"
	records := []map[string]interface{}{
		{"data": map[string]interface{}{"id": "001"}},
		{"data": map[string]interface{}{"id": "003"}},
		{"data": map[string]interface{}{"id": "002"}},
	}

	lastID, err := ExtractLastID(records, "data.id")
	if err != nil {
		t.Fatalf("ExtractLastID failed: %v", err)
	}
	// Should return the last record's ID
	if lastID != "002" {
		t.Errorf("ExtractLastID = %q, want %q", lastID, "002")
	}
}

func TestExtractLastID_EmptyRecords(t *testing.T) {
	records := []map[string]interface{}{}

	_, err := ExtractLastID(records, "id")
	if err == nil {
		t.Error("ExtractLastID should return error for empty records")
	}
}

func TestExtractLastID_LastRecordMissingID(t *testing.T) {
	// Last record doesn't have the ID field
	records := []map[string]interface{}{
		{"id": "100"},
		{"id": "200"},
		{"name": "no-id"}, // Last record missing ID
	}

	_, err := ExtractLastID(records, "id")
	if err == nil {
		t.Error("ExtractLastID should return error when last record is missing ID")
	}
}

func TestExtractLastID_PreservesReceptionOrder(t *testing.T) {
	// Simulate API response where records come in chronological order
	// The last record is the most recent one
	records := []map[string]interface{}{
		{"id": "event-001", "timestamp": "2026-01-26T10:00:00Z"},
		{"id": "event-002", "timestamp": "2026-01-26T10:01:00Z"},
		{"id": "event-003", "timestamp": "2026-01-26T10:02:00Z"},
	}

	lastID, err := ExtractLastID(records, "id")
	if err != nil {
		t.Fatalf("ExtractLastID failed: %v", err)
	}
	// Should return the ID of the most recently received record
	if lastID != "event-003" {
		t.Errorf("ExtractLastID = %q, want %q", lastID, "event-003")
	}
}
