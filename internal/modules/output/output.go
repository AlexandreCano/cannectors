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

// RequestPreview contains the preview of an HTTP request that would be sent.
// Used in dry-run mode to show what would be sent without actually sending.
type RequestPreview struct {
	// Endpoint is the resolved URL including path parameters and query params
	Endpoint string `json:"endpoint"`

	// Method is the HTTP method (POST, PUT, PATCH)
	Method string `json:"method"`

	// Headers contains all request headers (auth headers may be masked)
	Headers map[string]string `json:"headers"`

	// BodyPreview is the formatted JSON body that would be sent
	BodyPreview string `json:"body_preview"`

	// RecordCount is the number of records included in this request
	RecordCount int `json:"record_count"`
}

// PreviewOptions configures preview generation behavior.
//
// Example usage:
//
//	opts := PreviewOptions{
//	    ShowCredentials: false, // Default: mask credentials for security
//	}
//	previews, err := module.PreviewRequest(records, opts)
type PreviewOptions struct {
	// ShowCredentials when true displays actual credentials instead of masked values.
	// When false (default), authentication headers are masked as [MASKED-TOKEN], [MASKED-API-KEY], etc.
	// WARNING: Only set to true for debugging in secure environments.
	// Never enable this in production or when sharing preview output.
	ShowCredentials bool
}

// PreviewableModule extends Module with preview capability for dry-run mode.
// Modules implementing this interface can preview requests without sending them.
type PreviewableModule interface {
	Module

	// PreviewRequest prepares request previews without actually sending them.
	// Returns one preview per request that would be made:
	//   - Batch mode: returns 1 preview for all records
	//   - Single record mode: returns N previews (one per record)
	//
	// The opts parameter controls preview behavior:
	//   - ShowCredentials: when true, displays actual auth values (use with caution)
	PreviewRequest(records []map[string]interface{}, opts PreviewOptions) ([]RequestPreview, error)
}
