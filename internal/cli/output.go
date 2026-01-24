// Package cli provides CLI output formatting and display functions.
package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/canectors/runtime/pkg/connector"
)

// OutputOptions configures CLI output behavior.
type OutputOptions struct {
	Verbose bool
	Quiet   bool
	DryRun  bool
}

// PrintExecutionResult displays the pipeline execution result.
func PrintExecutionResult(result *connector.ExecutionResult, err error, opts OutputOptions) {
	if result == nil {
		fmt.Fprintln(os.Stderr, "✗ No execution result available")
		return
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "✗ Pipeline execution failed")
		if result.Error != nil {
			fmt.Fprintf(os.Stderr, "  Module: %s\n", result.Error.Module)
			fmt.Fprintf(os.Stderr, "  Error: %s\n", result.Error.Message)
		}
		return
	}

	if !opts.Quiet {
		fmt.Println("✓ Pipeline executed successfully")
		fmt.Printf("  Status: %s\n", result.Status)
		fmt.Printf("  Records processed: %d\n", result.RecordsProcessed)
		if result.RecordsFailed > 0 {
			fmt.Printf("  Records failed: %d\n", result.RecordsFailed)
		}
		if opts.Verbose {
			fmt.Printf("  Duration: %v\n", result.CompletedAt.Sub(result.StartedAt))
		}

		if opts.DryRun && len(result.DryRunPreview) > 0 {
			PrintDryRunPreview(result.DryRunPreview, opts.Verbose)
		}
	}
}

// PrintDryRunPreview displays the request preview for dry-run mode.
func PrintDryRunPreview(previews []connector.RequestPreview, verbose bool) {
	fmt.Println()
	fmt.Println("📋 Dry-Run Preview (what would have been sent):")
	fmt.Println()

	for i, preview := range previews {
		if len(previews) > 1 {
			fmt.Printf("─── Request %d of %d ───\n", i+1, len(previews))
		}

		fmt.Printf("  Endpoint: %s %s\n", preview.Method, preview.Endpoint)
		fmt.Printf("  Records: %d\n", preview.RecordCount)

		if len(preview.Headers) > 0 {
			printHeaders(preview.Headers)
		}

		if preview.BodyPreview != "" {
			printBodyPreview(preview.BodyPreview, verbose)
		}

		if i < len(previews)-1 {
			fmt.Println()
		}
	}

	fmt.Println()
	fmt.Println("ℹ️  No data was sent to the target system (dry-run mode)")
}

// printHeaders prints sorted headers.
func printHeaders(headers map[string]string) {
	fmt.Println("  Headers:")
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Printf("    %s: %s\n", name, headers[name])
	}
}

// printBodyPreview displays the formatted JSON body preview.
func printBodyPreview(bodyPreview string, verbose bool) {
	const maxLinesCompact = 10
	lineCount := countLines(bodyPreview)

	if verbose || lineCount <= maxLinesCompact {
		fmt.Println("  Body:")
		printIndentedBody(bodyPreview, "    ")
		return
	}

	fmt.Println("  Body (truncated, use --verbose for full):")
	printTruncatedBody(bodyPreview, "    ", maxLinesCompact)
}

// printIndentedBody prints the body with indentation.
func printIndentedBody(body string, indent string) {
	lines := splitLines(body)
	for _, line := range lines {
		fmt.Printf("%s%s\n", indent, line)
	}
}

// printTruncatedBody prints the first N lines of the body.
func printTruncatedBody(body string, indent string, maxLines int) {
	lines := splitLines(body)
	for i := 0; i < maxLines && i < len(lines); i++ {
		fmt.Printf("%s%s\n", indent, lines[i])
	}
	if len(lines) > maxLines {
		fmt.Printf("%s... (%d more lines)\n", indent, len(lines)-maxLines)
	}
}

// countLines counts the number of lines in a string.
func countLines(s string) int {
	return strings.Count(s, "\n") + 1
}

// splitLines splits a string into lines.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// PrintConfigSummary prints connector name and version if available.
func PrintConfigSummary(data map[string]interface{}) {
	if data == nil {
		return
	}

	conn, ok := data["connector"].(map[string]interface{})
	if !ok {
		return
	}

	if name, ok := conn["name"].(string); ok {
		fmt.Printf("  Connector: %s\n", name)
	}
	if version, ok := conn["version"].(string); ok {
		fmt.Printf("  Version: %s\n", version)
	}
}
