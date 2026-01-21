// Package output provides implementations for output modules.
// Output modules are responsible for sending data to destination systems.
//
// This package implements Epic 3: Module Execution - Output modules.
package output

// Module represents an output module that sends data to a destination.
type Module interface {
	// Send transmits records to the destination system.
	// Returns the number of records successfully sent and any error.
	Send(records []map[string]interface{}) (int, error)

	// Close releases any resources held by the module.
	Close() error
}
