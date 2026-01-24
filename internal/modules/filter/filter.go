// Package filter provides implementations for filter modules.
// Filter modules transform, map, and conditionally process data.
package filter

import (
	"context"
	"errors"
)

// ErrNotImplemented is returned when a feature is not yet implemented.
var ErrNotImplemented = errors.New("not implemented: will be added in Epic 3")

// Module represents a filter module that transforms data.
type Module interface {
	// Process transforms the input records.
	// The context can be used to cancel long-running operations.
	// Returns the transformed data.
	Process(ctx context.Context, records []map[string]interface{}) ([]map[string]interface{}, error)
}

// Note: Condition module is now implemented in condition.go (Story 3.4)
