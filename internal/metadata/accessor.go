// Package metadata provides utilities for accessing and manipulating per-record
// metadata stored as a nested map within records, separate from record data.
// By convention, metadata field names start with an underscore (e.g. "_metadata").
//
// This package is leaf-level (depends only on recordpath for nested-path
// helpers), so any module package — runtime, output, filter — can import it
// without creating a cycle.
package metadata

import (
	"github.com/cannectors/runtime/internal/recordpath"
)

// DefaultFieldName is the default field name for storing metadata in records.
const DefaultFieldName = "_metadata"

// Accessor encapsulates the metadata field name and provides a typed API to
// read, write, and strip metadata from records.
type Accessor struct {
	fieldName string
}

// NewAccessor creates a new Accessor with the specified field name.
// If fieldName is empty, DefaultFieldName ("_metadata") is used.
func NewAccessor(fieldName string) *Accessor {
	if fieldName == "" {
		fieldName = DefaultFieldName
	}
	return &Accessor{fieldName: fieldName}
}

// FieldName returns the metadata field name used by this accessor.
func (a *Accessor) FieldName() string {
	return a.fieldName
}

// Get retrieves a metadata value by key from the record.
// Supports nested keys using dot notation (e.g. "timing.start").
func (a *Accessor) Get(record map[string]interface{}, key string) (interface{}, bool) {
	if record == nil {
		return nil, false
	}
	metadata := a.getMap(record)
	if metadata == nil {
		return nil, false
	}
	return recordpath.Get(metadata, key)
}

// Set sets a metadata value by key in the record, creating the metadata field
// if it doesn't exist. Supports nested keys using dot notation.
func (a *Accessor) Set(record map[string]interface{}, key string, value interface{}) {
	if record == nil || key == "" {
		return
	}
	metadata := a.ensureMap(record)
	_ = recordpath.Set(metadata, key, value)
}

// Delete removes a metadata key from the record. Supports nested keys.
func (a *Accessor) Delete(record map[string]interface{}, key string) {
	if record == nil || key == "" {
		return
	}
	metadata := a.getMap(record)
	if metadata == nil {
		return
	}
	recordpath.Delete(metadata, key)
}

// GetAll returns the entire metadata map. Returns nil if no metadata exists.
func (a *Accessor) GetAll(record map[string]interface{}) map[string]interface{} {
	if record == nil {
		return nil
	}
	return a.getMap(record)
}

// SetAll replaces the entire metadata map in the record. Passing nil clears
// the metadata field.
func (a *Accessor) SetAll(record, meta map[string]interface{}) {
	if record == nil {
		return
	}
	if meta == nil {
		delete(record, a.fieldName)
		return
	}
	record[a.fieldName] = meta
}

// Merge merges new metadata values into existing metadata. Existing keys are
// overwritten by new values.
func (a *Accessor) Merge(record, newMeta map[string]interface{}) {
	if record == nil || newMeta == nil {
		return
	}
	metadata := a.ensureMap(record)
	for k, v := range newMeta {
		metadata[k] = v
	}
}

// Copy copies metadata from src to dst, deep-copying maps and slices to avoid
// shared references.
func (a *Accessor) Copy(src, dst map[string]interface{}) {
	if src == nil || dst == nil {
		return
	}
	srcMeta := a.getMap(src)
	if srcMeta == nil {
		return
	}
	dst[a.fieldName] = deepCopyMap(srcMeta)
}

// HasMetadata returns true if the record has a metadata field.
func (a *Accessor) HasMetadata(record map[string]interface{}) bool {
	if record == nil {
		return false
	}
	_, exists := record[a.fieldName]
	return exists
}

// Strip removes the metadata field from the record (in-place) and returns its
// value. Used when preparing records for output.
func (a *Accessor) Strip(record map[string]interface{}) map[string]interface{} {
	if record == nil {
		return nil
	}
	meta := a.getMap(record)
	delete(record, a.fieldName)
	return meta
}

// StripCopy returns a shallow copy of the record without the metadata field.
// The original record is not modified.
func (a *Accessor) StripCopy(record map[string]interface{}) map[string]interface{} {
	if record == nil {
		return nil
	}
	result := make(map[string]interface{}, len(record))
	for k, v := range record {
		if k != a.fieldName {
			result[k] = v
		}
	}
	return result
}

// StripFromRecord is a stateless helper equivalent to NewAccessor(fieldName).StripCopy(record)
// — useful for one-off strips without instantiating an Accessor.
// fieldName falls back to DefaultFieldName when empty. Returns the original
// record (not a copy) when the metadata field is absent.
func StripFromRecord(record map[string]interface{}, fieldName string) map[string]interface{} {
	if record == nil {
		return nil
	}
	if fieldName == "" {
		fieldName = DefaultFieldName
	}
	if _, exists := record[fieldName]; !exists {
		return record
	}
	result := make(map[string]interface{}, len(record)-1)
	for k, v := range record {
		if k != fieldName {
			result[k] = v
		}
	}
	return result
}

// StripFromRecords applies StripFromRecord to a slice of records.
func StripFromRecords(records []map[string]interface{}, fieldName string) []map[string]interface{} {
	if records == nil {
		return nil
	}
	result := make([]map[string]interface{}, len(records))
	for i, r := range records {
		result[i] = StripFromRecord(r, fieldName)
	}
	return result
}

func (a *Accessor) getMap(record map[string]interface{}) map[string]interface{} {
	if raw, exists := record[a.fieldName]; exists {
		if m, ok := raw.(map[string]interface{}); ok {
			return m
		}
	}
	return nil
}

func (a *Accessor) ensureMap(record map[string]interface{}) map[string]interface{} {
	if raw, exists := record[a.fieldName]; exists {
		if m, ok := raw.(map[string]interface{}); ok {
			return m
		}
	}
	m := make(map[string]interface{})
	record[a.fieldName] = m
	return m
}

func deepCopyMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		switch tv := v.(type) {
		case map[string]interface{}:
			dst[k] = deepCopyMap(tv)
		case []interface{}:
			dst[k] = deepCopySlice(tv)
		default:
			dst[k] = v
		}
	}
	return dst
}

func deepCopySlice(src []interface{}) []interface{} {
	if src == nil {
		return nil
	}
	dst := make([]interface{}, len(src))
	for i, v := range src {
		switch tv := v.(type) {
		case map[string]interface{}:
			dst[i] = deepCopyMap(tv)
		case []interface{}:
			dst[i] = deepCopySlice(tv)
		default:
			dst[i] = v
		}
	}
	return dst
}
