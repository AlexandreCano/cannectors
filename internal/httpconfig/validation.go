// Package httpconfig provides shared HTTP configuration validation utilities.
package httpconfig

import (
	"fmt"

	"github.com/canectors/runtime/internal/template"
)

// ValidationError represents a configuration validation error.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error for %s: %s", e.Field, e.Message)
}

// ValidateBaseConfig validates a BaseConfig.
// Returns an error if required fields are missing or invalid.
func ValidateBaseConfig(config BaseConfig, requireEndpoint bool) error {
	if requireEndpoint && config.Endpoint == "" {
		return &ValidationError{Field: "endpoint", Message: "endpoint is required"}
	}

	// Validate template syntax in endpoint
	if config.Endpoint != "" {
		if err := template.ValidateSyntax(config.Endpoint); err != nil {
			return &ValidationError{Field: "endpoint", Message: fmt.Sprintf("invalid template syntax: %v", err)}
		}
	}

	// Validate template syntax in headers
	for name, value := range config.Headers {
		if err := template.ValidateSyntax(value); err != nil {
			return &ValidationError{Field: fmt.Sprintf("headers.%s", name), Message: fmt.Sprintf("invalid template syntax: %v", err)}
		}
	}

	return nil
}

// ValidateMethod validates an HTTP method against allowed methods.
func ValidateMethod(method string, allowedMethods []string) error {
	if method == "" {
		return nil // Let caller handle default
	}

	for _, allowed := range allowedMethods {
		if method == allowed {
			return nil
		}
	}

	return &ValidationError{
		Field:   "method",
		Message: fmt.Sprintf("method must be one of %v, got: %s", allowedMethods, method),
	}
}

// ValidateOnError validates the onError field value.
func ValidateOnError(onError string) error {
	if onError == "" {
		return nil // Use default
	}

	validValues := []string{"fail", "skip", "log"}
	for _, valid := range validValues {
		if onError == valid {
			return nil
		}
	}

	return &ValidationError{
		Field:   "onError",
		Message: fmt.Sprintf("must be one of %v, got: %s", validValues, onError),
	}
}

// ValidateTemplateFile validates that a template file exists and has valid syntax.
func ValidateTemplateFile(path string, content string) error {
	if path == "" {
		return nil
	}

	if err := template.ValidateSyntax(content); err != nil {
		return &ValidationError{
			Field:   "bodyTemplateFile",
			Message: fmt.Sprintf("invalid template syntax in %s: %v", path, err),
		}
	}

	return nil
}
