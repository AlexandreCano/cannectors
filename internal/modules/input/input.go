// Package input provides implementations for input modules.
// Input modules are responsible for fetching data from source systems.
//
// This package was implemented in Epic 3: Module Execution, Story 3.1.
package input

import "errors"

// ErrNotImplemented is returned when a feature is not yet implemented.
var ErrNotImplemented = errors.New("not implemented")

// Module represents an input module that fetches data from a source.
type Module interface {
	// Fetch retrieves data from the source system.
	// Returns the fetched data as a slice of records.
	Fetch() ([]map[string]interface{}, error)
	// Close releases any resources held by the module.
	Close() error
}
