// Package persistence provides state persistence for pipeline execution.
package persistence

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Errors for ID extraction
var (
	// ErrEmptyFieldPath is returned when the field path is empty.
	ErrEmptyFieldPath = errors.New("field path is required")

	// ErrNilRecord is returned when the record is nil.
	ErrNilRecord = errors.New("record is nil")

	// ErrFieldNotFound is returned when the field is not found in the record.
	ErrFieldNotFound = errors.New("field not found")

	// ErrInvalidFieldType is returned when an intermediate field is not a map.
	ErrInvalidFieldType = errors.New("invalid intermediate field type (expected map)")

	// ErrNoRecords is returned when there are no records to extract IDs from.
	ErrNoRecords = errors.New("no records provided")
)

// ExtractID extracts an ID value from a record using a field path.
// The field path supports dot notation for nested fields (e.g., "data.id").
// Returns the ID as a string, converting numeric types if necessary.
func ExtractID(record map[string]interface{}, fieldPath string) (string, error) {
	if record == nil {
		return "", ErrNilRecord
	}
	if fieldPath == "" {
		return "", ErrEmptyFieldPath
	}

	// Split field path by dots for nested access
	parts := strings.Split(fieldPath, ".")
	current := interface{}(record)

	for i, part := range parts {
		// Check if current is a map
		m, ok := current.(map[string]interface{})
		if !ok {
			return "", fmt.Errorf("%w: at path segment %q", ErrInvalidFieldType, strings.Join(parts[:i], "."))
		}

		// Get the field value
		value, exists := m[part]
		if !exists {
			return "", fmt.Errorf("%w: %q", ErrFieldNotFound, fieldPath)
		}

		current = value
	}

	// Convert the final value to string
	return valueToString(current)
}

// valueToString converts an interface value to a string.
// Handles string, float64, int, int64, and other types.
func valueToString(value interface{}) (string, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case float64:
		// Check if it's a whole number
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10), nil
		}
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case int:
		return strconv.Itoa(v), nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case int32:
		return strconv.FormatInt(int64(v), 10), nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

// ExtractLastID extracts the ID from the last record in a slice.
// Records are expected to be in reception order (as returned by the API).
// The last record's ID (most recently received) is used for state persistence,
// assuming records are returned in chronological order by the API.
// Returns the ID as a string.
func ExtractLastID(records []map[string]interface{}, fieldPath string) (string, error) {
	if len(records) == 0 {
		return "", ErrNoRecords
	}

	// Get the last record (most recently received)
	lastRecord := records[len(records)-1]

	id, err := ExtractID(lastRecord, fieldPath)
	if err != nil {
		return "", fmt.Errorf("extracting ID from last record: %w", err)
	}

	return id, nil
}
