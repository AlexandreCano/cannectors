// Package httpconfig provides shared HTTP configuration types for input, filter, and output modules.
// This package centralizes common HTTP-related configuration to avoid duplication.
package httpconfig

import (
	"time"

	"github.com/canectors/runtime/pkg/connector"
)

// Default configuration values
const (
	DefaultTimeoutMs = 30000
	DefaultTimeout   = 30 * time.Second
)

// BaseConfig contains common HTTP configuration fields shared across all HTTP modules.
type BaseConfig struct {
	// Endpoint is the HTTP endpoint URL (required).
	// Supports {{record.field}} template variables.
	Endpoint string `json:"endpoint"`

	// Method is the HTTP method (GET, POST, PUT, PATCH).
	// Default varies by module type.
	Method string `json:"method,omitempty"`

	// Headers are custom HTTP headers to include in requests.
	// Supports {{record.field}} template variables.
	Headers map[string]string `json:"headers,omitempty"`

	// TimeoutMs is the request timeout in milliseconds (default 30000).
	TimeoutMs int `json:"timeoutMs,omitempty"`

	// Auth is the optional authentication configuration.
	Auth *connector.AuthConfig `json:"auth,omitempty"`
}

// BodyTemplateConfig contains configuration for request body templating.
type BodyTemplateConfig struct {
	// BodyTemplateFile is the path to an external template file for request body.
	// Supports {{record.field}} placeholders.
	BodyTemplateFile string `json:"bodyTemplateFile,omitempty"`
}

// DynamicParamsConfig contains configuration for dynamic request parameters.
// These parameters can be extracted from record data at runtime.
type DynamicParamsConfig struct {
	// PathParams are path parameter substitutions from record fields.
	// Key is the placeholder name, value is the record field path.
	PathParams map[string]string `json:"pathParams,omitempty"`

	// QueryParams are static query parameters.
	QueryParams map[string]string `json:"queryParams,omitempty"`

	// QueryFromRecord are query parameters extracted from record data.
	// Key is the query param name, value is the record field path.
	QueryFromRecord map[string]string `json:"queryFromRecord,omitempty"`

	// HeadersFromRecord are headers extracted from record data.
	// Key is the header name, value is the record field path.
	HeadersFromRecord map[string]string `json:"headersFromRecord,omitempty"`
}

// ErrorHandlingConfig contains error handling configuration.
type ErrorHandlingConfig struct {
	// OnError specifies error handling mode: "fail" (default), "skip", "log".
	OnError string `json:"onError,omitempty"`
}

// DataExtractionConfig contains configuration for extracting data from responses.
type DataExtractionConfig struct {
	// DataField is the JSON field path containing the data in the response.
	// For array responses wrapped in an object (e.g., {"data": [...]}), specify "data".
	DataField string `json:"dataField,omitempty"`
}

// GetTimeout returns the timeout duration from TimeoutMs, or the default if not set.
func (c *BaseConfig) GetTimeout() time.Duration {
	if c.TimeoutMs > 0 {
		return time.Duration(c.TimeoutMs) * time.Millisecond
	}
	return DefaultTimeout
}
