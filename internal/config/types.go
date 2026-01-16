// Package config provides functionality for parsing and validating
// pipeline configuration files (JSON/YAML).
package config

import (
	"fmt"
	"strings"
)

// ParseResult contains the result of parsing a configuration file.
type ParseResult struct {
	// Data contains the parsed configuration as a map
	Data map[string]interface{}
	// Errors contains any parsing errors encountered
	Errors []ParseError
	// FilePath is the path to the parsed file (empty if parsed from string)
	FilePath string
	// Format indicates the detected format (json, yaml)
	Format string
}

// IsValid returns true if no parsing errors occurred.
func (r *ParseResult) IsValid() bool {
	return len(r.Errors) == 0
}

// ParseError represents a parsing error with location information.
type ParseError struct {
	// Path is the file path where the error occurred
	Path string
	// Line is the line number (1-based, 0 if unknown)
	Line int
	// Column is the column number (1-based, 0 if unknown)
	Column int
	// Offset is the byte offset in the file (0 if unknown)
	Offset int64
	// Message is the error message
	Message string
	// Type categorizes the error (syntax, io, format)
	Type string
}

// Error implements the error interface.
func (e ParseError) Error() string {
	var sb strings.Builder
	if e.Path != "" {
		sb.WriteString(e.Path)
		sb.WriteString(": ")
	}
	if e.Line > 0 {
		sb.WriteString(fmt.Sprintf("line %d", e.Line))
		if e.Column > 0 {
			sb.WriteString(fmt.Sprintf(", column %d", e.Column))
		}
		sb.WriteString(": ")
	}
	sb.WriteString(e.Message)
	return sb.String()
}

// ValidationResult contains the result of validating a configuration.
type ValidationResult struct {
	// Valid indicates whether the configuration is valid
	Valid bool
	// Errors contains validation errors
	Errors []ValidationError
}

// ValidationError represents a schema validation error.
type ValidationError struct {
	// Path is the JSON path where the error occurred (e.g., "/connector/input/endpoint")
	Path string
	// Type is the error type (required, type, format, enum, etc.)
	Type string
	// Expected is what was expected
	Expected string
	// Actual is what was found
	Actual string
	// Message is the error message
	Message string
}

// Error implements the error interface.
func (e ValidationError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("%s: %s", e.Path, e.Message)
	}
	return e.Message
}

// Result contains the combined result of parsing and validation.
type Result struct {
	// Data contains the parsed and validated configuration
	Data map[string]interface{}
	// ParseErrors contains parsing errors
	ParseErrors []ParseError
	// ValidationErrors contains validation errors
	ValidationErrors []ValidationError
	// FilePath is the path to the configuration file
	FilePath string
	// Format is the detected format (json, yaml)
	Format string
}

// IsValid returns true if no errors occurred.
func (r *Result) IsValid() bool {
	return len(r.ParseErrors) == 0 && len(r.ValidationErrors) == 0
}

// AllErrors returns all errors (parsing and validation) as a single slice.
func (r *Result) AllErrors() []error {
	errors := make([]error, 0, len(r.ParseErrors)+len(r.ValidationErrors))
	for _, e := range r.ParseErrors {
		errors = append(errors, e)
	}
	for _, e := range r.ValidationErrors {
		errors = append(errors, e)
	}
	return errors
}

// FormatErrorType constants for categorizing parse errors.
const (
	ErrorTypeIO     = "io"
	ErrorTypeSyntax = "syntax"
	ErrorTypeFormat = "format"
)
