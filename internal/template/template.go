// Package template provides template evaluation functionality for dynamic string construction.
// It supports variable substitution using {{record.field}} syntax with optional default values.
package template

import (
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strings"

	"github.com/canectors/runtime/internal/logger"
)

// Template syntax constants
const (
	// TemplatePrefix is the opening delimiter for template variables
	TemplatePrefix = "{{"
	// TemplateSuffix is the closing delimiter for template variables
	TemplateSuffix = "}}"
	// DefaultValueSeparator separates variable path from default value
	DefaultValueSeparator = "|"
	// DefaultKeyword indicates a default value follows
	DefaultKeyword = "default:"
)

// Error messages for template evaluation
const (
	ErrMsgInvalidTemplateSyntax = "invalid template syntax"
	ErrMsgMissingClosingBrace   = "missing closing }}"
	ErrMsgEmptyVariablePath     = "empty variable path"
)

// templateVarRegex matches template variables like {{record.field}} or {{record.field | default: "value"}}
// Group 1: variable path (e.g., "record.user.id")
// Group 2: optional default value clause including quotes (e.g., " | default: \"fallback\"")
// Group 3: the default value itself (may be empty string)
var templateVarRegex = regexp.MustCompile(`\{\{\s*([^|}]+?)(\s*\|\s*default:\s*"([^"]*)")?\s*\}\}`)

// Variable represents a parsed template variable
type Variable struct {
	FullMatch    string // The full matched string including {{ }}
	Path         string // The variable path (e.g., "record.user.id")
	DefaultValue string // Default value if specified (empty string if not)
	HasDefault   bool   // Whether a default value was specified
}

// Evaluator evaluates template strings using record data.
// It supports:
// - Variable substitution: {{record.field}}
// - Nested field access: {{record.user.profile.id}}
// - Array indexing: {{record.items[0].name}}
// - Default values: {{record.field | default: "fallback"}}
//
// Performance: The evaluator caches parsed template variables to avoid re-parsing
// the same template strings. The cache is unbounded and grows with the number of
// unique template strings evaluated. For typical use cases with a limited number
// of template configurations, this provides good performance without memory concerns.
// The cache is not thread-safe and should not be shared across goroutines.
type Evaluator struct {
	// Cache for parsed template variables per template string.
	// Maps template string to its parsed variables.
	// Cache is unbounded - grows with unique template strings.
	// Not thread-safe - each goroutine should use its own Evaluator instance.
	cache map[string][]Variable
}

// NewEvaluator creates a new template evaluator.
func NewEvaluator() *Evaluator {
	return &Evaluator{
		cache: make(map[string][]Variable),
	}
}

// HasVariables checks if a string contains template variables.
func HasVariables(s string) bool {
	return strings.Contains(s, TemplatePrefix) && strings.Contains(s, TemplateSuffix)
}

// ParseVariables extracts all template variables from a template string.
// Returns a slice of Variable structs.
func (e *Evaluator) ParseVariables(template string) []Variable {
	// Check cache first
	if cached, ok := e.cache[template]; ok {
		return cached
	}

	matches := templateVarRegex.FindAllStringSubmatch(template, -1)
	variables := make([]Variable, 0, len(matches))

	for _, match := range matches {
		if len(match) >= 2 {
			v := Variable{
				FullMatch: match[0],
				Path:      strings.TrimSpace(match[1]),
			}

			// Check if default value clause was captured (group 2 is the full clause, group 3 is the value)
			// Group 2: " | default: \"value\"" - if not empty, default was specified
			// Group 3: "value" - the actual default value (may be empty string)
			if len(match) >= 4 && match[2] != "" {
				v.DefaultValue = match[3] // Group 3 is the value inside quotes
				v.HasDefault = true
			}

			variables = append(variables, v)
		}
	}

	// Cache the result
	e.cache[template] = variables

	return variables
}

// Evaluate evaluates a template string using the provided record data.
// Returns the evaluated string with all template variables replaced.
//
// Template syntax:
//   - {{record.field}} - Access field from record
//   - {{record.user.name}} - Access nested field using dot notation
//   - {{record.items[0].id}} - Access array element with index
//   - {{record.field | default: "value"}} - Use default if field is missing/null
//
// Missing fields return empty string unless a default is specified.
// Null values are converted to empty string.
func (e *Evaluator) Evaluate(template string, record map[string]interface{}) string {
	if !HasVariables(template) {
		return template
	}

	variables := e.ParseVariables(template)
	if len(variables) == 0 {
		return template
	}

	logger.Debug("evaluating template",
		slog.String("template", truncateForLog(template, 100)),
		slog.Int("variable_count", len(variables)),
	)

	result := template

	for _, v := range variables {
		value := e.resolveVariable(v, record)
		result = strings.Replace(result, v.FullMatch, value, 1)
	}

	return result
}

// truncateForLog truncates a string for logging purposes.
func truncateForLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// EvaluateForURL evaluates a template string for use in URLs.
// All template variable values are URL-encoded using QueryEscape, which provides
// comprehensive encoding of special characters including =, &, /, and spaces.
//
// Note: QueryEscape is used for both path segments and query parameters to ensure
// maximum safety. This means path segments containing "/" will be encoded as "%2F",
// which is acceptable for most APIs. If an API requires unencoded "/" in paths,
// the template should be structured to avoid placing dynamic values in path segments.
func (e *Evaluator) EvaluateForURL(template string, record map[string]interface{}) string {
	if !HasVariables(template) {
		return template
	}

	variables := e.ParseVariables(template)
	if len(variables) == 0 {
		return template
	}

	result := template

	for _, v := range variables {
		value := e.resolveVariable(v, record)
		// URL-encode the value using QueryEscape which encodes more characters
		// including = and & which PathEscape leaves as-is
		encodedValue := url.QueryEscape(value)
		result = strings.Replace(result, v.FullMatch, encodedValue, 1)
	}

	return result
}

// resolveVariable resolves a single template variable using record data.
func (e *Evaluator) resolveVariable(v Variable, record map[string]interface{}) string {
	// Extract the field path (remove "record." prefix if present)
	path := strings.TrimPrefix(v.Path, "record.")

	// Get the value from the record
	value, found := GetNestedValue(record, path)

	// Handle missing or null values
	if !found || value == nil {
		if v.HasDefault {
			logger.Debug("template variable using default",
				slog.String("path", v.Path),
				slog.String("default", v.DefaultValue),
			)
			return v.DefaultValue
		}
		// Log at WARN level when template variable is missing without default
		// This helps users identify configuration issues
		logger.Warn("template variable missing, using empty string",
			slog.String("path", v.Path),
			slog.String("field", path),
		)
		return ""
	}

	// Convert value to string
	return ValueToString(value)
}

// GetNestedValue extracts a value from a nested object using dot notation.
// Supports array indexing with [n] syntax.
// Returns the value and a boolean indicating if the field was found.
func GetNestedValue(obj map[string]interface{}, path string) (interface{}, bool) {
	if path == "" {
		return nil, false
	}

	parts := strings.Split(path, ".")
	current := interface{}(obj)

	for _, part := range parts {
		// Handle array indexing (e.g., "items[0]")
		arrayIdx := -1
		key, index, hasIndex := parseArrayNotation(part)
		if hasIndex {
			arrayIdx = index
			part = key
		}

		// Navigate to the field
		switch v := current.(type) {
		case map[string]interface{}:
			if v == nil {
				return nil, false
			}
			val, ok := v[part]
			if !ok {
				return nil, false
			}
			current = val
		default:
			return nil, false
		}

		// Handle array indexing
		if arrayIdx >= 0 {
			switch arr := current.(type) {
			case []interface{}:
				if arrayIdx >= len(arr) {
					return nil, false
				}
				current = arr[arrayIdx]
			default:
				return nil, false
			}
		}
	}

	return current, true
}

// parseArrayNotation parses a path part for array indexing.
// Returns the key, index, and whether an index was found.
// E.g., "items[0]" returns ("items", 0, true)
func parseArrayNotation(part string) (string, int, bool) {
	idx := strings.Index(part, "[")
	if idx == -1 {
		return part, -1, false
	}

	endIdx := strings.Index(part, "]")
	if endIdx == -1 || endIdx < idx+1 || endIdx != len(part)-1 {
		return part, -1, false
	}

	indexStr := part[idx+1 : endIdx]
	var index int
	_, err := fmt.Sscanf(indexStr, "%d", &index)
	if err != nil || index < 0 {
		return part, -1, false
	}

	return part[:idx], index, true
}

// ValueToString converts any value to its string representation.
func ValueToString(value interface{}) string {
	if value == nil {
		return ""
	}

	switch v := value.(type) {
	case string:
		return v
	case float64:
		// Format integers without decimal point
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return fmt.Sprintf("%v", v)
	case int:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	case bool:
		return fmt.Sprintf("%t", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// ValidateSyntax validates that a template string has valid syntax.
// Returns an error if the syntax is invalid (e.g., unmatched braces).
func ValidateSyntax(template string) error {
	if template == "" {
		return nil
	}

	// Count opening and closing braces
	openCount := strings.Count(template, TemplatePrefix)
	closeCount := strings.Count(template, TemplateSuffix)

	if openCount != closeCount {
		return fmt.Errorf("%s: unmatched template delimiters (found %d '{{' and %d '}}')",
			ErrMsgInvalidTemplateSyntax, openCount, closeCount)
	}

	// Check each template variable has content and that all {{/}} form valid expressions
	if openCount > 0 {
		// First check for completely empty braces: {{}} or {{ }}
		emptyBracesRegex := regexp.MustCompile(`\{\{\s*\}\}`)
		if emptyBracesRegex.MatchString(template) {
			return fmt.Errorf("%s: %s", ErrMsgInvalidTemplateSyntax, ErrMsgEmptyVariablePath)
		}

		// Validate properly formed variables
		variables := templateVarRegex.FindAllStringSubmatch(template, -1)
		for _, match := range variables {
			if len(match) >= 2 && strings.TrimSpace(match[1]) == "" {
				return fmt.Errorf("%s: %s", ErrMsgInvalidTemplateSyntax, ErrMsgEmptyVariablePath)
			}
		}

		// Ensure every {{ and }} is part of a valid match (e.g. "}}{{" has balanced count but invalid pairing)
		remainder := templateVarRegex.ReplaceAllString(template, "")
		if strings.Contains(remainder, TemplatePrefix) || strings.Contains(remainder, TemplateSuffix) {
			return fmt.Errorf("%s: template delimiters must form valid {{...}} expressions (stray '{{' or '}}' found)",
				ErrMsgInvalidTemplateSyntax)
		}
	}

	return nil
}

// EvaluateHeaders evaluates template variables in HTTP header values.
// Returns a new map with evaluated header values.
func (e *Evaluator) EvaluateHeaders(headers map[string]string, record map[string]interface{}) map[string]string {
	if len(headers) == 0 {
		return headers
	}

	evaluated := make(map[string]string, len(headers))
	for key, value := range headers {
		evaluated[key] = e.Evaluate(value, record)
	}
	return evaluated
}

// EvaluateMapValues evaluates template variables in map values (used for JSON body construction).
// Recursively processes nested maps and arrays.
func (e *Evaluator) EvaluateMapValues(data interface{}, record map[string]interface{}) interface{} {
	switch v := data.(type) {
	case string:
		if HasVariables(v) {
			return e.Evaluate(v, record)
		}
		return v
	case map[string]interface{}:
		result := make(map[string]interface{}, len(v))
		for key, val := range v {
			result[key] = e.EvaluateMapValues(val, record)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = e.EvaluateMapValues(item, record)
		}
		return result
	default:
		return data
	}
}
