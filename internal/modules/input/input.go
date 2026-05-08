// Package input provides implementations for input modules.
// Input modules are responsible for fetching data from source systems.
//
// This package was implemented in Epic 3: Module Execution, Story 3.1.
package input

import (
	"context"
	"errors"
)

// ErrNotImplemented is returned when a feature is not yet implemented.
var ErrNotImplemented = errors.New("not implemented")

// Module represents an input module that fetches data from a source system.
//
// # Responsibilities
//
// Input modules are responsible for:
//   - Fetching data from external source systems (APIs, databases, files, etc.)
//   - Returning data in a standardized format ([]map[string]any)
//   - Managing resources (connections, file handles, etc.) and cleaning them up via Close()
//
// # What Input Modules Should NOT Do
//
// Input modules should NOT:
//   - Transform or filter data (that's the filter module's responsibility)
//   - Send data to destinations (that's the output module's responsibility)
//   - Perform business logic beyond data retrieval
//   - Store state between Fetch() calls (modules should be stateless or manage their own state)
//
// # Context Usage
//
// The context.Context parameter in Fetch() should be used for:
//   - Cancellation: Respect ctx.Done() to cancel long-running operations
//   - Timeouts: Use context timeouts to limit operation duration
//   - Request-scoped values: Access request metadata if needed
//
// Do NOT use context for storing module state or configuration.
//
// # Error Handling
//
// Return errors when:
//   - Network/connection failures occur
//   - Authentication fails
//   - Data cannot be retrieved
//   - Configuration is invalid
//
// The runtime will handle errors according to pipeline error handling configuration.
// Do NOT retry internally unless explicitly required by the module's design.
//
// # Return Format
//
// Fetch() must return []map[string]any where:
//   - Each map represents a single record/entity
//   - Keys are field names (strings)
//   - Values can be any JSON-serializable type (string, number, bool, nested maps/slices)
//
// Example:
//
//	records := []map[string]any{
//	    {"id": 1, "name": "Alice", "email": "alice@example.com"},
//	    {"id": 2, "name": "Bob", "email": "bob@example.com"},
//	}
//
// # Resource Management
//
// Close() must release all resources held by the module:
//   - Close HTTP connections and connection pools
//   - Close file handles
//   - Release database connections
//   - Clean up any temporary resources
//
// Close() may be called multiple times and should be idempotent.
// The runtime will call Close() after input execution completes, even if Fetch() returns an error.
//
// # Stability
//
// This interface is designed to remain stable across versions.
// New methods will not be added to this interface to maintain backward compatibility.
// Optional capabilities (like preview, streaming) are provided via separate interfaces or composition.
//
// # Example Implementation
//
// Here's a complete example of implementing an input module:
//
//	type MyInputModule struct {
//	    client *http.Client
//	    url    string
//	}
//
//	func (m *MyInputModule) Fetch(ctx context.Context) ([]map[string]any, error) {
//	    // Respect context cancellation
//	    select {
//	    case <-ctx.Done():
//	        return nil, ctx.Err()
//	    default:
//	    }
//
//	    // Fetch data from source
//	    req, err := http.NewRequestWithContext(ctx, "GET", m.url, nil)
//	    if err != nil {
//	        return nil, err
//	    }
//
//	    resp, err := m.client.Do(req)
//	    if err != nil {
//	        return nil, err
//	    }
//	    defer resp.Body.Close()
//
//	    // Parse response into []map[string]any
//	    var records []map[string]any
//	    if err := json.NewDecoder(resp.Body).Decode(&records); err != nil {
//	        return nil, err
//	    }
//
//	    return records, nil
//	}
//
//	func (m *MyInputModule) Close() error {
//	    // Close HTTP client connections
//	    m.client.CloseIdleConnections()
//	    return nil
//	}
//
//	// Verify interface compliance at compile time
//	var _ Module = (*MyInputModule)(nil)
type Module interface {
	// Fetch retrieves data from the source system.
	//
	// The context can be used to cancel long-running operations.
	// Implementations should respect ctx.Done() and return promptly when canceled.
	//
	// Returns the fetched data as a slice of records, where each record is a map[string]any.
	// Returns an error if data cannot be retrieved.
	Fetch(ctx context.Context) ([]map[string]any, error)

	// Close releases any resources held by the module.
	//
	// This method should be idempotent (safe to call multiple times).
	// The runtime will call Close() after input execution completes.
	// Implementations must release all resources (connections, file handles, etc.).
	Close() error
}
