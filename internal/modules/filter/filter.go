// Package filter provides implementations for filter modules.
// Filter modules transform, map, and conditionally process data.
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
func (c *Condition) Process(_ []map[string]interface{}) ([]map[string]interface{}, error) {
	return nil, ErrNotImplemented
}
