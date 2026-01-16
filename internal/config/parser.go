// Package config provides functionality for parsing and validating
// pipeline configuration files (JSON/YAML).
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

// ParseJSONFile parses a JSON configuration file from the given path.
// Returns a ParseResult containing the parsed data or errors.
func ParseJSONFile(filepath string) *ParseResult {
	result := &ParseResult{
		FilePath: filepath,
		Format:   "json",
	}

	// Read file contents
	content, err := os.ReadFile(filepath)
	if err != nil {
		result.Errors = append(result.Errors, ParseError{
			Path:    filepath,
			Message: fmt.Sprintf("failed to read file: %v", err),
			Type:    ErrorTypeIO,
		})
		return result
	}

	// Parse JSON content
	parseResult := ParseJSONString(string(content))
	result.Data = parseResult.Data
	result.Errors = parseResult.Errors

	// Update error paths to include file path
	for i := range result.Errors {
		if result.Errors[i].Path == "" {
			result.Errors[i].Path = filepath
		}
	}

	return result
}

// ParseJSONString parses JSON content from a string.
// Returns a ParseResult containing the parsed data or errors.
func ParseJSONString(content string) *ParseResult {
	result := &ParseResult{
		Format: "json",
	}

	// Handle empty content
	content = strings.TrimSpace(content)
	if content == "" {
		result.Errors = append(result.Errors, ParseError{
			Message: "empty content: expected JSON object",
			Type:    ErrorTypeSyntax,
		})
		return result
	}

	// Parse JSON
	var data interface{}
	err := json.Unmarshal([]byte(content), &data)
	if err != nil {
		parseErr := parseJSONError(err, content)
		result.Errors = append(result.Errors, parseErr)
		return result
	}

	// Check if result is a map (config must be an object)
	if data == nil {
		// null JSON - valid JSON but not a valid config
		return result
	}

	dataMap, ok := data.(map[string]interface{})
	if !ok {
		result.Errors = append(result.Errors, ParseError{
			Message: fmt.Sprintf("invalid configuration: expected JSON object, got %T", data),
			Type:    ErrorTypeFormat,
		})
		return result
	}

	result.Data = dataMap
	return result
}

// parseJSONError extracts detailed error information from a JSON unmarshaling error.
func parseJSONError(err error, content string) ParseError {
	parseErr := ParseError{
		Message: err.Error(),
		Type:    ErrorTypeSyntax,
	}

	// Try to extract line/column from json.SyntaxError
	if syntaxErr, ok := err.(*json.SyntaxError); ok {
		parseErr.Offset = syntaxErr.Offset
		parseErr.Line, parseErr.Column = offsetToLineColumn(content, syntaxErr.Offset)
		parseErr.Message = fmt.Sprintf("JSON syntax error at offset %d: %s", syntaxErr.Offset, syntaxErr.Error())
	}

	// Try to extract information from json.UnmarshalTypeError
	if typeErr, ok := err.(*json.UnmarshalTypeError); ok {
		parseErr.Offset = typeErr.Offset
		parseErr.Line, parseErr.Column = offsetToLineColumn(content, typeErr.Offset)
		parseErr.Message = fmt.Sprintf("type error at field '%s': expected %s, got %s",
			typeErr.Field, typeErr.Type.String(), typeErr.Value)
	}

	return parseErr
}

// offsetToLineColumn converts a byte offset to line and column numbers (1-based).
func offsetToLineColumn(content string, offset int64) (line, column int) {
	if offset <= 0 {
		return 1, 1
	}

	line = 1
	column = 1
	for i := int64(0); i < offset && i < int64(len(content)); i++ {
		if content[i] == '\n' {
			line++
			column = 1
		} else {
			column++
		}
	}
	return line, column
}

// ============================================================================
// Unified Configuration Parser
// ============================================================================

// ParseConfig parses and validates a configuration file.
// It auto-detects the format (JSON/YAML) based on file extension or content.
// Returns a Result with parsed data, validation results, and any errors.
func ParseConfig(filepath string) *Result {
	result := &Result{
		FilePath: filepath,
	}

	// Detect format by extension
	format := DetectFormat(filepath)

	// Parse based on detected format
	var parseResult *ParseResult
	switch format {
	case "json":
		parseResult = ParseJSONFile(filepath)
	case "yaml":
		parseResult = ParseYAMLFile(filepath)
	default:
		// Try to auto-detect from content by trying JSON first, then YAML
		content, err := os.ReadFile(filepath)
		if err != nil {
			result.ParseErrors = append(result.ParseErrors, ParseError{
				Path:    filepath,
				Message: fmt.Sprintf("failed to read file: %v", err),
				Type:    ErrorTypeIO,
			})
			return result
		}

		contentStr := string(content)
		if IsJSON(contentStr) {
			parseResult = ParseJSONString(contentStr)
			parseResult.FilePath = filepath
		} else if IsYAML(contentStr) {
			parseResult = ParseYAMLString(contentStr)
			parseResult.FilePath = filepath
		} else {
			result.ParseErrors = append(result.ParseErrors, ParseError{
				Path:    filepath,
				Message: "unable to detect configuration format: not valid JSON or YAML",
				Type:    ErrorTypeFormat,
			})
			return result
		}
	}

	// Transfer parse results
	result.Data = parseResult.Data
	result.ParseErrors = parseResult.Errors
	result.Format = parseResult.Format

	// If parsing failed, skip validation
	if !parseResult.IsValid() {
		return result
	}

	// Validate against schema
	validationResult := ValidateConfig(parseResult.Data)
	result.ValidationErrors = validationResult.Errors

	return result
}

// ParseConfigString parses and validates configuration content from a string.
// If format is empty, it auto-detects from content.
// Returns a Result with parsed data, validation results, and any errors.
func ParseConfigString(content string, format string) *Result {
	result := &Result{
		Format: format,
	}

	// Auto-detect format if not specified
	if format == "" {
		if IsJSON(content) {
			format = "json"
		} else if IsYAML(content) {
			format = "yaml"
		} else {
			result.ParseErrors = append(result.ParseErrors, ParseError{
				Message: "unable to detect configuration format: not valid JSON or YAML",
				Type:    ErrorTypeFormat,
			})
			return result
		}
		result.Format = format
	}

	// Parse based on format
	var parseResult *ParseResult
	switch format {
	case "json":
		parseResult = ParseJSONString(content)
	case "yaml":
		parseResult = ParseYAMLString(content)
	default:
		result.ParseErrors = append(result.ParseErrors, ParseError{
			Message: fmt.Sprintf("unsupported format: %s", format),
			Type:    ErrorTypeFormat,
		})
		return result
	}

	// Transfer parse results
	result.Data = parseResult.Data
	result.ParseErrors = parseResult.Errors
	result.Format = parseResult.Format

	// If parsing failed, skip validation
	if !parseResult.IsValid() {
		return result
	}

	// Validate against schema
	validationResult := ValidateConfig(parseResult.Data)
	result.ValidationErrors = validationResult.Errors

	return result
}

// DetectFormat detects the configuration format from file extension.
// Returns "json", "yaml", or empty string if format cannot be detected.
func DetectFormat(filepath string) string {
	ext := strings.ToLower(path.Ext(filepath))
	switch ext {
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	default:
		return ""
	}
}

// IsJSON checks if the content appears to be JSON format.
func IsJSON(content string) bool {
	content = strings.TrimSpace(content)
	if content == "" {
		return false
	}
	// JSON must start with { or [
	return strings.HasPrefix(content, "{") || strings.HasPrefix(content, "[")
}

// IsYAML checks if the content appears to be valid YAML.
// Note: JSON is also valid YAML, so this may return true for JSON content.
func IsYAML(content string) bool {
	content = strings.TrimSpace(content)
	if content == "" {
		return false
	}

	// Try to parse as YAML
	var data interface{}
	err := yaml.Unmarshal([]byte(content), &data)
	return err == nil && data != nil
}

// ============================================================================
// YAML Parsing
// ============================================================================

// ParseYAMLFile parses a YAML configuration file from the given path.
// Returns a ParseResult containing the parsed data or errors.
func ParseYAMLFile(filepath string) *ParseResult {
	result := &ParseResult{
		FilePath: filepath,
		Format:   "yaml",
	}

	// Read file contents
	content, err := os.ReadFile(filepath)
	if err != nil {
		result.Errors = append(result.Errors, ParseError{
			Path:    filepath,
			Message: fmt.Sprintf("failed to read file: %v", err),
			Type:    ErrorTypeIO,
		})
		return result
	}

	// Parse YAML content
	parseResult := ParseYAMLString(string(content))
	result.Data = parseResult.Data
	result.Errors = parseResult.Errors

	// Update error paths to include file path
	for i := range result.Errors {
		if result.Errors[i].Path == "" {
			result.Errors[i].Path = filepath
		}
	}

	return result
}

// ParseYAMLString parses YAML content from a string.
// Returns a ParseResult containing the parsed data or errors.
func ParseYAMLString(content string) *ParseResult {
	result := &ParseResult{
		Format: "yaml",
	}

	// Handle empty content
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		result.Errors = append(result.Errors, ParseError{
			Message: "empty content: expected YAML document",
			Type:    ErrorTypeSyntax,
		})
		return result
	}

	// Parse YAML
	var data interface{}
	err := yaml.Unmarshal([]byte(content), &data)
	if err != nil {
		parseErr := parseYAMLError(err)
		result.Errors = append(result.Errors, parseErr)
		return result
	}

	// Check if result is a map (config must be an object)
	if data == nil {
		// null YAML or comments only - valid YAML but not a valid config
		return result
	}

	dataMap, ok := data.(map[string]interface{})
	if !ok {
		result.Errors = append(result.Errors, ParseError{
			Message: fmt.Sprintf("invalid configuration: expected YAML mapping, got %T", data),
			Type:    ErrorTypeFormat,
		})
		return result
	}

	result.Data = dataMap
	return result
}

// parseYAMLError extracts detailed error information from a YAML unmarshaling error.
func parseYAMLError(err error) ParseError {
	parseErr := ParseError{
		Message: err.Error(),
		Type:    ErrorTypeSyntax,
	}

	// Try to extract line/column from yaml.TypeError
	if typeErr, ok := err.(*yaml.TypeError); ok {
		// TypeError contains multiple error strings
		parseErr.Message = fmt.Sprintf("YAML type error: %s", strings.Join(typeErr.Errors, "; "))
	}

	// The yaml.v3 library includes line info in the error message
	// Format: "yaml: line X: ..."
	if strings.Contains(err.Error(), "yaml: line ") {
		var line int
		_, scanErr := fmt.Sscanf(err.Error(), "yaml: line %d:", &line)
		if scanErr == nil {
			parseErr.Line = line
		}
	}

	return parseErr
}
