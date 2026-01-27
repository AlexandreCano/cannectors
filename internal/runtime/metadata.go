// Package runtime provides the pipeline execution engine.
// It includes metadata storage utilities for records.
package runtime

import (
	"strings"
)

// DefaultMetadataFieldName is the default field name for storing metadata in records.
const DefaultMetadataFieldName = "_metadata"

// MetadataAccessor provides utilities for accessing and manipulating metadata in records.
// Metadata is stored as a nested map within records, separate from record data.
// By convention, metadata field names start with underscore (e.g., "_metadata").
type MetadataAccessor struct {
	fieldName string
}

// NewMetadataAccessor creates a new MetadataAccessor with the specified field name.
// If fieldName is empty, DefaultMetadataFieldName ("_metadata") is used.
func NewMetadataAccessor(fieldName string) *MetadataAccessor {
	if fieldName == "" {
		fieldName = DefaultMetadataFieldName
	}
	return &MetadataAccessor{fieldName: fieldName}
}

// FieldName returns the metadata field name used by this accessor.
func (m *MetadataAccessor) FieldName() string {
	return m.fieldName
}

// Get retrieves a metadata value by key from the record.
// Returns the value and a boolean indicating if the key was found.
// Supports nested keys using dot notation (e.g., "timing.start").
func (m *MetadataAccessor) Get(record map[string]interface{}, key string) (interface{}, bool) {
	if record == nil {
		return nil, false
	}

	metadata := m.getMetadataMap(record)
	if metadata == nil {
		return nil, false
	}

	return getNestedValue(metadata, key)
}

// Set sets a metadata value by key in the record.
// Creates the metadata field if it doesn't exist.
// Supports nested keys using dot notation (e.g., "timing.start").
func (m *MetadataAccessor) Set(record map[string]interface{}, key string, value interface{}) {
	if record == nil || key == "" {
		return
	}

	metadata := m.ensureMetadataMap(record)
	setNestedValue(metadata, key, value)
}

// Delete removes a metadata key from the record.
// Supports nested keys using dot notation.
func (m *MetadataAccessor) Delete(record map[string]interface{}, key string) {
	if record == nil || key == "" {
		return
	}

	metadata := m.getMetadataMap(record)
	if metadata == nil {
		return
	}

	deleteNestedKey(metadata, key)
}

// GetAll returns the entire metadata map from the record.
// Returns nil if no metadata exists.
func (m *MetadataAccessor) GetAll(record map[string]interface{}) map[string]interface{} {
	if record == nil {
		return nil
	}
	return m.getMetadataMap(record)
}

// SetAll replaces the entire metadata map in the record.
func (m *MetadataAccessor) SetAll(record map[string]interface{}, metadata map[string]interface{}) {
	if record == nil {
		return
	}
	if metadata == nil {
		delete(record, m.fieldName)
		return
	}
	record[m.fieldName] = metadata
}

// Merge merges new metadata values into existing metadata.
// Existing keys are overwritten by new values.
func (m *MetadataAccessor) Merge(record map[string]interface{}, newMetadata map[string]interface{}) {
	if record == nil || newMetadata == nil {
		return
	}

	metadata := m.ensureMetadataMap(record)
	for key, value := range newMetadata {
		metadata[key] = value
	}
}

// Copy copies metadata from source record to destination record.
// Creates a deep copy of the metadata to prevent shared references.
func (m *MetadataAccessor) Copy(src, dst map[string]interface{}) {
	if src == nil || dst == nil {
		return
	}

	srcMetadata := m.getMetadataMap(src)
	if srcMetadata == nil {
		return
	}

	dst[m.fieldName] = deepCopyMap(srcMetadata)
}

// HasMetadata returns true if the record has a metadata field.
func (m *MetadataAccessor) HasMetadata(record map[string]interface{}) bool {
	if record == nil {
		return false
	}
	_, exists := record[m.fieldName]
	return exists
}

// Strip removes the metadata field from the record and returns its value.
// Used when preparing records for output (excluding metadata from request body).
func (m *MetadataAccessor) Strip(record map[string]interface{}) map[string]interface{} {
	if record == nil {
		return nil
	}

	metadata := m.getMetadataMap(record)
	delete(record, m.fieldName)
	return metadata
}

// StripCopy returns a copy of the record without the metadata field.
// The original record is not modified.
func (m *MetadataAccessor) StripCopy(record map[string]interface{}) map[string]interface{} {
	if record == nil {
		return nil
	}

	result := make(map[string]interface{}, len(record))
	for key, value := range record {
		if key != m.fieldName {
			result[key] = value
		}
	}
	return result
}

// getMetadataMap retrieves the metadata map from a record.
func (m *MetadataAccessor) getMetadataMap(record map[string]interface{}) map[string]interface{} {
	if raw, exists := record[m.fieldName]; exists {
		if metadata, ok := raw.(map[string]interface{}); ok {
			return metadata
		}
	}
	return nil
}

// ensureMetadataMap ensures a metadata map exists in the record and returns it.
func (m *MetadataAccessor) ensureMetadataMap(record map[string]interface{}) map[string]interface{} {
	if raw, exists := record[m.fieldName]; exists {
		if metadata, ok := raw.(map[string]interface{}); ok {
			return metadata
		}
	}

	metadata := make(map[string]interface{})
	record[m.fieldName] = metadata
	return metadata
}

// getNestedValue retrieves a value from a nested map using dot notation.
func getNestedValue(m map[string]interface{}, path string) (interface{}, bool) {
	if path == "" {
		return nil, false
	}

	parts := strings.Split(path, ".")
	current := interface{}(m)

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]interface{}:
			if v == nil {
				return nil, false
			}
			val, ok := v[part]
			if !ok {
				return nil, false
			}
			current = val
		default:
			return nil, false
		}
	}

	return current, true
}

// setNestedValue sets a value in a nested map using dot notation.
// Creates intermediate maps as needed.
func setNestedValue(m map[string]interface{}, path string, value interface{}) {
	if path == "" || m == nil {
		return
	}

	parts := strings.Split(path, ".")
	current := m

	for i := 0; i < len(parts)-1; i++ {
		part := parts[i]
		if raw, exists := current[part]; exists {
			if nested, ok := raw.(map[string]interface{}); ok {
				current = nested
				continue
			}
		}
		// Create intermediate map
		nested := make(map[string]interface{})
		current[part] = nested
		current = nested
	}

	current[parts[len(parts)-1]] = value
}

// deleteNestedKey deletes a key from a nested map using dot notation.
func deleteNestedKey(m map[string]interface{}, path string) {
	if path == "" || m == nil {
		return
	}

	parts := strings.Split(path, ".")
	current := m

	for i := 0; i < len(parts)-1; i++ {
		part := parts[i]
		if raw, exists := current[part]; exists {
			if nested, ok := raw.(map[string]interface{}); ok {
				current = nested
				continue
			}
		}
		// Path doesn't exist
		return
	}

	delete(current, parts[len(parts)-1])
}

// deepCopyMap creates a deep copy of a map.
func deepCopyMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}

	dst := make(map[string]interface{}, len(src))
	for key, value := range src {
		switch v := value.(type) {
		case map[string]interface{}:
			dst[key] = deepCopyMap(v)
		case []interface{}:
			dst[key] = deepCopySlice(v)
		default:
			dst[key] = value
		}
	}
	return dst
}

// deepCopySlice creates a deep copy of a slice.
func deepCopySlice(src []interface{}) []interface{} {
	if src == nil {
		return nil
	}

	dst := make([]interface{}, len(src))
	for i, value := range src {
		switch v := value.(type) {
		case map[string]interface{}:
			dst[i] = deepCopyMap(v)
		case []interface{}:
			dst[i] = deepCopySlice(v)
		default:
			dst[i] = value
		}
	}
	return dst
}

// (Unused MetadataFieldNameError type and its Error method removed as metadata field name is now fixed.)
