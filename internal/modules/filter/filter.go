// Package filter provides implementations for filter modules.
// Filter modules transform, map, and conditionally process data.
package filter

import (
	"context"
	"errors"
)

// ErrNotImplemented is returned when a feature is not yet implemented.
var ErrNotImplemented = errors.New("not implemented")

// Module represents a filter module that transforms data between input and output stages.
//
// # Responsibilities
//
// Filter modules are responsible for:
//   - Transforming records (field mapping, value conversion, enrichment)
//   - Filtering records (conditional processing, routing)
//   - Data validation and sanitization
//   - Aggregating or splitting records
//
// # What Filter Modules Should NOT Do
//
// Filter modules should NOT:
//   - Send data to destinations (that's the output module's responsibility)
//   - Store state between pipeline executions (filters should be stateless)
//   - Perform side effects outside of data transformation
//
// # Context Usage
//
// The context.Context parameter in Process() should be used for:
//   - Cancellation: Respect ctx.Done() to cancel long-running transformations
//   - Timeouts: Use context timeouts to limit processing duration
//   - Request-scoped values: Access pipeline metadata if needed
//
// Do NOT use context for storing module state or configuration.
//
// # Error Handling
//
// Return errors when:
//   - Data transformation fails (invalid format, type conversion errors)
//   - Required fields are missing
//   - Transformation logic encounters unrecoverable errors
//
// The runtime will handle errors according to pipeline error handling configuration.
// Some filter modules (like condition) may handle errors internally and route records accordingly.
//
// # Record Format
//
// Process() receives and returns []map[string]any where:
//   - Each map represents a single record/entity
//   - Keys are field names (strings)
//   - Values can be any JSON-serializable type
//
// Filter modules can:
//   - Modify existing fields
//   - Add new fields
//   - Remove fields (by omitting them from output)
//   - Split one record into multiple records
//   - Combine multiple records into one
//   - Filter out records entirely (return fewer records than input)
//
// Example transformation:
//
//	Input:  [{"id": 1, "name": "Alice"}]
//	Output: [{"userId": 1, "fullName": "Alice", "status": "active"}]
//
// # Stateless Design
//
// Filter modules should be stateless - each Process() call should be independent.
// If state is needed (e.g., for aggregation), it should be managed internally and reset
// between pipeline executions. The runtime may create new filter instances for each execution.
//
// # Stability
//
// This interface is designed to remain stable across versions.
// New methods will not be added to this interface to maintain backward compatibility.
// Complex filtering capabilities (like conditional routing) are implemented as separate module types.
//
// # Example Implementation
//
// Here's a complete example of implementing a filter module:
//
//	type FieldMappingFilter struct {
//	    mappings map[string]string // source -> target field mappings
//	}
//
//	func (f *FieldMappingFilter) Process(ctx context.Context, records []map[string]any) ([]map[string]any, error) {
//	    // Respect context cancellation
//	    select {
//	    case <-ctx.Done():
//	        return nil, ctx.Err()
//	    default:
//	    }
//
//	    // Transform records
//	    result := make([]map[string]any, len(records))
//	    for i, record := range records {
//	        transformed := make(map[string]any)
//	        for sourceField, targetField := range f.mappings {
//	            if value, exists := record[sourceField]; exists {
//	                transformed[targetField] = value
//	            }
//	        }
//	        result[i] = transformed
//	    }
//
//	    return result, nil
//	}
//
//	// Verify interface compliance at compile time
//	var _ Module = (*FieldMappingFilter)(nil)
type Module interface {
	// Process transforms the input records.
	//
	// The context can be used to cancel long-running operations.
	// Implementations should respect ctx.Done() and return promptly when canceled.
	//
	// The records parameter contains the input data to transform.
	// Returns the transformed records (may be more, fewer, or the same count as input).
	// Returns an error if transformation fails.
	Process(ctx context.Context, records []map[string]any) ([]map[string]any, error)
}

// Note: Condition module is now implemented in condition.go (Story 3.4)
