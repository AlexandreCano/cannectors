// Package config provides functionality for parsing and validating
// pipeline configuration files (JSON/YAML).
package config

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed schema/pipeline-schema.json
var embeddedSchema []byte

// schemaOnce ensures thread-safe initialization of the compiled schema.
var schemaOnce sync.Once

// compiledSchema is the cached compiled schema.
var compiledSchema *jsonschema.Schema

// schemaInitErr stores any error from schema initialization.
var schemaInitErr error

// GetEmbeddedSchema returns the embedded pipeline schema.
func GetEmbeddedSchema() []byte {
	return embeddedSchema
}

// getCompiledSchema returns the compiled JSON schema, compiling it if necessary.
// Thread-safe via sync.Once.
func getCompiledSchema() (*jsonschema.Schema, error) {
	schemaOnce.Do(func() {
		// Parse the schema JSON
		var schemaDoc interface{}
		if err := json.Unmarshal(embeddedSchema, &schemaDoc); err != nil {
			schemaInitErr = fmt.Errorf("failed to parse embedded schema: %w", err)
			return
		}

		// Create a new compiler
		compiler := jsonschema.NewCompiler()

		// Add the schema to the compiler
		schemaURL := "https://canectors.io/schemas/pipeline/v1.1.0/pipeline-schema.json"
		if err := compiler.AddResource(schemaURL, schemaDoc); err != nil {
			schemaInitErr = fmt.Errorf("failed to add schema resource: %w", err)
			return
		}

		// Compile the schema
		var err error
		compiledSchema, err = compiler.Compile(schemaURL)
		if err != nil {
			schemaInitErr = fmt.Errorf("failed to compile schema: %w", err)
			return
		}
	})

	if schemaInitErr != nil {
		return nil, schemaInitErr
	}
	return compiledSchema, nil
}

// ValidateConfig validates a parsed configuration against the pipeline schema.
// Returns a ValidationResult with validation status and any errors.
func ValidateConfig(data map[string]interface{}) *ValidationResult {
	result := &ValidationResult{
		Valid: true,
	}

	// Handle nil data
	if data == nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Path:    "/",
			Type:    "required",
			Message: "configuration data is nil",
		})
		return result
	}

	// Handle empty data
	if len(data) == 0 {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Path:    "/",
			Type:    "required",
			Message: "configuration data is empty",
		})
		return result
	}

	// Get the compiled schema
	schema, err := getCompiledSchema()
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Path:    "/",
			Type:    "schema",
			Message: fmt.Sprintf("failed to load schema: %v", err),
		})
		return result
	}

	// Validate the data against the schema
	validationErr := schema.Validate(data)
	if validationErr != nil {
		result.Valid = false

		// Convert validation errors to our format
		if detailedErr, ok := validationErr.(*jsonschema.ValidationError); ok {
			result.Errors = convertValidationErrors(detailedErr)
		} else {
			result.Errors = append(result.Errors, ValidationError{
				Path:    "/",
				Type:    "validation",
				Message: validationErr.Error(),
			})
		}
	}

	return result
}

// convertValidationErrors converts jsonschema validation errors to our format.
func convertValidationErrors(err *jsonschema.ValidationError) []ValidationError {
	var errors []ValidationError

	// Get the error message from ErrorKind
	errMsg := err.Error()

	// Process this error if it has an ErrorKind
	if err.ErrorKind != nil {
		path := formatInstanceLocation(err.InstanceLocation)
		errors = append(errors, ValidationError{
			Path:    path,
			Type:    extractErrorType(err),
			Message: errMsg,
		})
	}

	// Process all causes (nested errors)
	for _, cause := range err.Causes {
		causeErrors := convertValidationErrors(cause)
		errors = append(errors, causeErrors...)
	}

	return errors
}

// formatInstanceLocation formats the instance location as a JSON path.
func formatInstanceLocation(loc []string) string {
	if len(loc) == 0 {
		return "/"
	}
	return "/" + strings.Join(loc, "/")
}

// extractErrorType extracts a simplified error type from the validation error.
func extractErrorType(err *jsonschema.ValidationError) string {
	errStr := err.Error()
	msg := strings.ToLower(errStr)

	switch {
	case strings.Contains(msg, "required"):
		return "required"
	case strings.Contains(msg, "type"):
		return "type"
	case strings.Contains(msg, "pattern"):
		return "pattern"
	case strings.Contains(msg, "enum"):
		return "enum"
	case strings.Contains(msg, "minimum") || strings.Contains(msg, "maximum"):
		return "range"
	case strings.Contains(msg, "format"):
		return "format"
	case strings.Contains(msg, "additionalproperties"):
		return "additionalProperties"
	default:
		return "validation"
	}
}
