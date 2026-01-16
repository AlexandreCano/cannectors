// Package input provides implementations for input modules.
// Input modules are responsible for fetching data from source systems.
//
// This package will be implemented in Epic 3: Module Execution.
package input

import "errors"

// ErrNotImplemented is returned when a feature is not yet implemented.
var ErrNotImplemented = errors.New("not implemented: will be added in Story 3.1")

// Module represents an input module that fetches data from a source.
type Module interface {
	// Fetch retrieves data from the source system.
	// Returns the fetched data as a slice of records.
	Fetch() ([]map[string]interface{}, error)

	// Close releases any resources held by the module.
	Close() error
}

// HTTPPolling implements polling-based HTTP data fetching.
// Will be implemented in Story 3.1.
type HTTPPolling struct {
	endpoint string
	interval int
}

// NewHTTPPolling creates a new HTTP polling input module.
func NewHTTPPolling(endpoint string, interval int) *HTTPPolling {
	return &HTTPPolling{
		endpoint: endpoint,
		interval: interval,
	}
}

// Fetch retrieves data via HTTP polling.
// TODO: Implement in Story 3.1
func (h *HTTPPolling) Fetch() ([]map[string]interface{}, error) {
	return nil, ErrNotImplemented
}

// Close releases HTTP resources.
func (h *HTTPPolling) Close() error {
	return nil
}
