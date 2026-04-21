// Package config provides functionality for parsing and validating
// pipeline configuration files (JSON/YAML).
package config

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
)

//go:embed schema/pipeline-schema.json
var embeddedSchema []byte

//go:embed schema/common-schema.json
var embeddedCommonSchema []byte

//go:embed schema/input-schema.json
var embeddedInputSchema []byte

//go:embed schema/filter-schema.json
var embeddedFilterSchema []byte

//go:embed schema/output-schema.json
var embeddedOutputSchema []byte

//go:embed schema/auth-schema.json
var embeddedAuthSchema []byte

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

// schemaFiles maps schema URLs to their embedded content.
var schemaFiles = map[string]*[]byte{
	"https://cannectors.io/schemas/pipeline/v1.1.0/pipeline-schema.json": &embeddedSchema,
	"https://cannectors.io/schemas/pipeline/v1.1.0/common-schema.json":   &embeddedCommonSchema,
	"https://cannectors.io/schemas/pipeline/v1.1.0/input-schema.json":    &embeddedInputSchema,
	"https://cannectors.io/schemas/pipeline/v1.1.0/filter-schema.json":   &embeddedFilterSchema,
	"https://cannectors.io/schemas/pipeline/v1.1.0/output-schema.json":   &embeddedOutputSchema,
	"https://cannectors.io/schemas/pipeline/v1.1.0/auth-schema.json":     &embeddedAuthSchema,
}

// getCompiledSchema returns the compiled JSON schema, compiling it if necessary.
// Thread-safe via sync.Once.
func getCompiledSchema() (*jsonschema.Schema, error) {
	schemaOnce.Do(func() {
		compiler := jsonschema.NewCompiler()

		// Add all schema files to the compiler
		for schemaURL, schemaBytes := range schemaFiles {
			var schemaDoc interface{}
			if err := json.Unmarshal(*schemaBytes, &schemaDoc); err != nil {
				schemaInitErr = fmt.Errorf("failed to parse schema %s: %w", schemaURL, err)
				return
			}
			if err := compiler.AddResource(schemaURL, schemaDoc); err != nil {
				schemaInitErr = fmt.Errorf("failed to add schema resource %s: %w", schemaURL, err)
				return
			}
		}

		// Compile the main pipeline schema
		var err error
		compiledSchema, err = compiler.Compile("https://cannectors.io/schemas/pipeline/v1.1.0/pipeline-schema.json")
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

// Known valid types for each module category.
var (
	validInputTypes  = []string{"webhook", "httpPolling", "database"}
	validFilterTypes = []string{"mapping", "condition", "script", "http_call", "sql_call", "set", "remove"}
	validOutputTypes = []string{"httpRequest", "database"}
)

// convertValidationErrors converts jsonschema validation errors to actionable messages.
func convertValidationErrors(err *jsonschema.ValidationError) []ValidationError {
	var errors []ValidationError
	collectErrors(err, &errors)
	return errors
}

// collectErrors recursively collects leaf validation errors (errors with no causes).
// Intermediate errors (allOf, oneOf wrappers) are skipped to avoid noisy messages.
func collectErrors(err *jsonschema.ValidationError, errors *[]ValidationError) {
	if err.ErrorKind == nil {
		for _, cause := range err.Causes {
			collectErrors(cause, errors)
		}
		return
	}

	// Skip intermediate group/composition errors, drill into causes
	switch err.ErrorKind.(type) {
	case *kind.AllOf, *kind.AnyOf, *kind.Group, *kind.Schema:
		for _, cause := range err.Causes {
			collectErrors(cause, errors)
		}
		return
	case *kind.OneOf:
		// For oneOf: if there are causes, drill down; otherwise report this error
		if len(err.Causes) > 0 {
			for _, cause := range err.Causes {
				collectErrors(cause, errors)
			}
			return
		}
	}

	// Leaf error: build an actionable message
	path := formatInstanceLocation(err.InstanceLocation)
	ve := ValidationError{
		Path:    path,
		Type:    extractErrorType(err),
		Message: buildActionableMessage(err),
	}

	// Populate Expected/Actual when possible
	switch k := err.ErrorKind.(type) {
	case *kind.Type:
		ve.Expected = strings.Join(k.Want, " or ")
		ve.Actual = k.Got
	case *kind.Required:
		ve.Expected = strings.Join(k.Missing, ", ")
	}

	*errors = append(*errors, ve)
}

// buildActionableMessage creates a user-friendly error message with context.
func buildActionableMessage(err *jsonschema.ValidationError) string {
	path := formatInstanceLocation(err.InstanceLocation)

	switch k := err.ErrorKind.(type) {
	case *kind.Required:
		if len(k.Missing) == 1 {
			return fmt.Sprintf("%s: missing required property %q", path, k.Missing[0])
		}
		return fmt.Sprintf("%s: missing required properties: %s", path, strings.Join(k.Missing, ", "))

	case *kind.Type:
		return fmt.Sprintf("%s: expected %s, got %s", path, strings.Join(k.Want, " or "), k.Got)

	case *kind.Enum:
		return fmt.Sprintf("%s: invalid value. Supported values: %s", path, formatEnumValues(k.Want))

	case *kind.AdditionalProperties:
		return fmt.Sprintf("%s: unknown properties %s are not allowed", path, strings.Join(k.Properties, ", "))

	case *kind.OneOf:
		hint := suggestValidTypes(err.InstanceLocation)
		if hint != "" {
			return fmt.Sprintf("%s: %s", path, hint)
		}
		return fmt.Sprintf("%s: value does not match any of the expected schemas", path)

	case *kind.Pattern:
		return fmt.Sprintf("%s: value does not match required pattern %q", path, k.Want)

	case *kind.MinLength:
		return fmt.Sprintf("%s: string must be at least %d characters", path, k.Want)

	case *kind.MaxLength:
		return fmt.Sprintf("%s: string must be at most %d characters", path, k.Want)

	case *kind.Minimum:
		return fmt.Sprintf("%s: value must be >= %v", path, k.Want)

	case *kind.Maximum:
		return fmt.Sprintf("%s: value must be <= %v", path, k.Want)

	case *kind.MinItems:
		return fmt.Sprintf("%s: array must have at least %d items", path, k.Want)

	case *kind.Format:
		return fmt.Sprintf("%s: invalid format, expected %q", path, k.Want)

	default:
		return fmt.Sprintf("%s: %s", path, err.Error())
	}
}

// formatInstanceLocation formats the instance location as a readable path.
// Converts ["filters", "0", "type"] to "filters[0].type" instead of "/filters/0/type".
func formatInstanceLocation(loc []string) string {
	if len(loc) == 0 {
		return "/"
	}

	var sb strings.Builder
	for i, segment := range loc {
		if _, err := strconv.Atoi(segment); err == nil {
			sb.WriteString("[" + segment + "]")
		} else {
			if i > 0 {
				sb.WriteString(".")
			}
			sb.WriteString(segment)
		}
	}
	return sb.String()
}

// extractErrorType extracts a simplified error type from the validation error.
func extractErrorType(err *jsonschema.ValidationError) string {
	switch err.ErrorKind.(type) {
	case *kind.Required:
		return "required"
	case *kind.Type:
		return "type"
	case *kind.Pattern:
		return "pattern"
	case *kind.Enum:
		return "enum"
	case *kind.Minimum, *kind.Maximum, *kind.ExclusiveMinimum, *kind.ExclusiveMaximum:
		return "range"
	case *kind.Format:
		return "format"
	case *kind.AdditionalProperties:
		return "additionalProperties"
	case *kind.MinLength, *kind.MaxLength:
		return "length"
	case *kind.MinItems, *kind.MaxItems:
		return "items"
	case *kind.OneOf, *kind.AnyOf, *kind.AllOf:
		return "schema"
	default:
		return "validation"
	}
}

// suggestValidTypes returns a helpful message about valid types based on the error path.
func suggestValidTypes(location []string) string {
	for i, segment := range location {
		switch segment {
		case "input":
			return fmt.Sprintf("invalid input configuration. Supported input types: %s", strings.Join(validInputTypes, ", "))
		case "filters":
			idx := ""
			if i+1 < len(location) {
				if _, err := strconv.Atoi(location[i+1]); err == nil {
					idx = "[" + location[i+1] + "]"
				}
			}
			return fmt.Sprintf("invalid filter%s configuration. Supported filter types: %s", idx, strings.Join(validFilterTypes, ", "))
		case "output":
			return fmt.Sprintf("invalid output configuration. Supported output types: %s", strings.Join(validOutputTypes, ", "))
		}
	}
	return ""
}

// formatEnumValues formats enum values for display.
func formatEnumValues(values []any) string {
	strs := make([]string, 0, len(values))
	for _, v := range values {
		strs = append(strs, fmt.Sprintf("%q", v))
	}
	return strings.Join(strs, ", ")
}
