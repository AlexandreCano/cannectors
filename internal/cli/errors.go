// Package cli provides CLI output formatting and display functions.
package cli

import (
	"fmt"
	"os"

	"github.com/canectors/runtime/internal/config"
)

// PrintParseErrors prints parse errors to stderr.
func PrintParseErrors(errors []config.ParseError, verbose bool) {
	fmt.Fprintln(os.Stderr, "✗ Parse errors:")
	for _, err := range errors {
		printSingleParseError(err, verbose)
	}
}

// printSingleParseError prints a single parse error with location information.
func printSingleParseError(err config.ParseError, verbose bool) {
	location := formatErrorLocation(err.Path, err.Line, err.Column)

	if location != "" {
		fmt.Fprintf(os.Stderr, "  %s: %s\n", location, err.Message)
	} else {
		fmt.Fprintf(os.Stderr, "  %s\n", err.Message)
	}

	if verbose && err.Type != "" {
		fmt.Fprintf(os.Stderr, "    Type: %s\n", err.Type)
	}
}

// formatErrorLocation formats the error location string (path:line:column).
func formatErrorLocation(path string, line, column int) string {
	if path == "" {
		return ""
	}

	location := path
	if line > 0 {
		location += fmt.Sprintf(":%d", line)
		if column > 0 {
			location += fmt.Sprintf(":%d", column)
		}
	}
	return location
}

// PrintValidationErrors prints validation errors to stderr.
func PrintValidationErrors(errors []config.ValidationError, verbose, quiet bool) {
	fmt.Fprintln(os.Stderr, "✗ Validation errors:")
	for _, err := range errors {
		printSingleValidationError(err, verbose)
	}
	printValidationHint(quiet)
}

// printSingleValidationError prints a single validation error.
func printSingleValidationError(err config.ValidationError, verbose bool) {
	path := err.Path
	if path == "" {
		path = "/"
	}

	if verbose {
		printVerboseValidationError(path, err)
	} else {
		printCompactValidationError(path, err.Message)
	}
}

// printVerboseValidationError prints detailed validation error information.
func printVerboseValidationError(path string, err config.ValidationError) {
	fmt.Fprintf(os.Stderr, "  %s:\n", path)
	fmt.Fprintf(os.Stderr, "    Message: %s\n", err.Message)
	if err.Type != "" {
		fmt.Fprintf(os.Stderr, "    Type: %s\n", err.Type)
	}
	if err.Expected != "" {
		fmt.Fprintf(os.Stderr, "    Expected: %s\n", err.Expected)
	}
}

// printCompactValidationError prints a compact validation error message.
func printCompactValidationError(path, message string) {
	shortMsg := message
	if len(shortMsg) > 80 {
		shortMsg = shortMsg[:77] + "..."
	}
	fmt.Fprintf(os.Stderr, "  %s: %s\n", path, shortMsg)
}

// printValidationHint prints a hint about verbose mode.
func printValidationHint(quiet bool) {
	if !quiet {
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Hint: Use --verbose for detailed error information")
	}
}
