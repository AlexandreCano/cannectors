// Package filter provides implementations for filter modules.
// Filter modules transform, map, and conditionally process data.
//
// This package will be implemented in Epic 3: Module Execution.
package filter

import "errors"

// ErrNotImplemented is returned when a feature is not yet implemented.
var ErrNotImplemented = errors.New("not implemented: will be added in Epic 3")

// Module represents a filter module that transforms data.
type Module interface {
	// Process transforms the input records.
	// Returns the transformed data.
	Process(records []map[string]interface{}) ([]map[string]interface{}, error)
}

// Mapping implements field mapping transformations.
// Will be implemented in Story 3.3.
type Mapping struct {
	mappings map[string]string
}

// NewMapping creates a new mapping filter module.
func NewMapping(mappings map[string]string) *Mapping {
	return &Mapping{
		mappings: mappings,
	}
}

// Process applies field mappings to records.
// TODO: Implement in Story 3.3
func (m *Mapping) Process(records []map[string]interface{}) ([]map[string]interface{}, error) {
	return nil, ErrNotImplemented
}

// Condition implements conditional filtering.
// Will be implemented in Story 3.4.
type Condition struct {
	expression string
}

// NewCondition creates a new condition filter module.
func NewCondition(expression string) *Condition {
	return &Condition{
		expression: expression,
	}
}

// Process filters records based on conditions.
// TODO: Implement in Story 3.4
func (c *Condition) Process(records []map[string]interface{}) ([]map[string]interface{}, error) {
	return nil, ErrNotImplemented
}
