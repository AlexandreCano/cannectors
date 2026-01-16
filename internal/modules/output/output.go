// Package output provides implementations for output modules.
// Output modules are responsible for sending data to destination systems.
//
// This package will be implemented in Epic 3: Module Execution.
package output

// Module represents an output module that sends data to a destination.
type Module interface {
	// Send transmits records to the destination system.
	// Returns the number of records successfully sent and any error.
	Send(records []map[string]interface{}) (int, error)

	// Close releases any resources held by the module.
	Close() error
}

// HTTPRequest implements HTTP-based data sending.
// Will be implemented in Story 3.5.
type HTTPRequest struct {
	endpoint string
	method   string
}

// NewHTTPRequest creates a new HTTP request output module.
func NewHTTPRequest(endpoint string, method string) *HTTPRequest {
	return &HTTPRequest{
		endpoint: endpoint,
		method:   method,
	}
}

// Send transmits records via HTTP.
// TODO: Implement in Story 3.5
func (h *HTTPRequest) Send(records []map[string]interface{}) (int, error) {
	return 0, nil
}

// Close releases HTTP resources.
func (h *HTTPRequest) Close() error {
	return nil
}
