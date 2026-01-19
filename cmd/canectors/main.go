// Package main provides the CLI entry point for the Canectors runtime.
package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/canectors/runtime/internal/config"
	"github.com/canectors/runtime/internal/logger"
	"github.com/canectors/runtime/internal/modules/filter"
	"github.com/canectors/runtime/internal/modules/input"
	"github.com/canectors/runtime/internal/modules/output"
	"github.com/canectors/runtime/internal/runtime"
	"github.com/canectors/runtime/pkg/connector"
)

// Exit codes
const (
	ExitSuccess         = 0
	ExitValidationError = 1
	ExitParseError      = 2
	ExitRuntimeError    = 3
)

var (
	// Global flags
	verbose bool
	quiet   bool

	// Run command flags
	dryRun bool

	// Build information (set via ldflags during build)
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(ExitRuntimeError)
	}
}

var rootCmd = &cobra.Command{
	Use:   "canectors",
	Short: "Canectors - Declarative data pipeline runtime",
	Long: `Canectors is a CLI tool for running declarative data pipelines.

It parses and validates pipeline configurations (JSON/YAML format),
then executes them according to the defined Input → Filter → Output pattern.

Examples:
  # Validate a configuration file
  canectors validate config.json

  # Run a pipeline
  canectors run config.yaml

  # Validate with verbose output
  canectors validate --verbose config.json`,
	PersistentPreRun: func(_ *cobra.Command, _ []string) {
		// Configure logger level based on flags
		if verbose {
			logger.SetLevel(slog.LevelDebug)
		} else if quiet {
			logger.SetLevel(slog.LevelError)
		}
	},
}

var validateCmd = &cobra.Command{
	Use:   "validate <config-file>",
	Short: "Validate a pipeline configuration file",
	Long: `Validate a pipeline configuration file against the schema.

Supports both JSON and YAML formats. The format is auto-detected
based on file extension (.json, .yaml, .yml) or content.

Exit codes:
  0 - Configuration is valid
  1 - Validation errors (schema violations)
  2 - Parse errors (invalid JSON/YAML syntax)

Examples:
  canectors validate config.json
  canectors validate pipeline.yaml
  canectors validate --verbose config.json`,
	Args: cobra.ExactArgs(1),
	Run:  runValidate,
}

var runCmd = &cobra.Command{
	Use:   "run <config-file>",
	Short: "Run a pipeline from configuration file",
	Long: `Run a pipeline defined in the configuration file.

The configuration file is first validated against the schema.
If validation fails, the pipeline will not be executed.

Flags:
  --dry-run   Validate and prepare the pipeline without executing output module

Exit codes:
  0 - Pipeline executed successfully
  1 - Validation errors
  2 - Parse errors
  3 - Runtime errors

Examples:
  canectors run config.json
  canectors run --verbose pipeline.yaml
  canectors run --dry-run config.json`,
	Args: cobra.ExactArgs(1),
	Run:  runPipeline,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  "Print version, commit hash, and build date information.",
	Run:   runVersion,
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "Suppress non-error output")

	// Run command flags
	runCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate and prepare without executing output module")

	// Add commands
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(versionCmd)
}

func runValidate(_ *cobra.Command, args []string) {
	configPath := args[0]

	if !quiet {
		fmt.Printf("Validating configuration: %s\n", configPath)
	}

	// Parse and validate configuration
	result := config.ParseConfig(configPath)

	// Handle parse errors
	if len(result.ParseErrors) > 0 {
		printParseErrors(result.ParseErrors)
		os.Exit(ExitParseError)
	}

	// Handle validation errors
	if len(result.ValidationErrors) > 0 {
		printValidationErrors(result.ValidationErrors)
		os.Exit(ExitValidationError)
	}

	// Success
	if !quiet {
		fmt.Printf("✓ Configuration is valid (format: %s)\n", result.Format)

		if verbose {
			// Print configuration summary
			if result.Data != nil {
				if connector, ok := result.Data["connector"].(map[string]interface{}); ok {
					if name, ok := connector["name"].(string); ok {
						fmt.Printf("  Connector: %s\n", name)
					}
					if connectorVersion, ok := connector["version"].(string); ok {
						fmt.Printf("  Version: %s\n", connectorVersion)
					}
				}
			}
		}
	}

	os.Exit(ExitSuccess)
}

func runPipeline(_ *cobra.Command, args []string) {
	configPath := args[0]

	if !quiet {
		fmt.Printf("Loading pipeline configuration: %s\n", configPath)
	}

	// Parse and validate configuration
	result := config.ParseConfig(configPath)

	// Handle parse errors
	if len(result.ParseErrors) > 0 {
		printParseErrors(result.ParseErrors)
		os.Exit(ExitParseError)
	}

	// Handle validation errors
	if len(result.ValidationErrors) > 0 {
		printValidationErrors(result.ValidationErrors)
		os.Exit(ExitValidationError)
	}

	if !quiet {
		fmt.Printf("✓ Configuration loaded successfully (format: %s)\n", result.Format)
	}

	// Convert configuration to Pipeline struct
	pipeline, err := config.ConvertToPipeline(result.Data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "✗ Failed to convert configuration: %v\n", err)
		os.Exit(ExitRuntimeError)
	}

	if verbose {
		fmt.Printf("  Pipeline: %s (v%s)\n", pipeline.Name, pipeline.Version)
		if pipeline.Description != "" {
			fmt.Printf("  Description: %s\n", pipeline.Description)
		}
	}

	// Create module instances
	// Note: Mapping filter uses real implementation; other modules are stubbed for now.
	inputModule := createInputModule(pipeline.Input)
	filterModules, err := createFilterModules(pipeline.Filters)
	if err != nil {
		fmt.Fprintf(os.Stderr, "✗ Failed to create filter modules: %v\n", err)
		os.Exit(ExitRuntimeError)
	}
	outputModule := createOutputModule(pipeline.Output)

	// Create executor and run pipeline
	executor := runtime.NewExecutorWithModules(inputModule, filterModules, outputModule, dryRun)

	if !quiet {
		if dryRun {
			fmt.Println("Executing pipeline (dry-run mode - output will not be sent)...")
		} else {
			fmt.Println("Executing pipeline...")
		}
	}

	execResult, err := executor.Execute(pipeline)

	// Display execution result
	printExecutionResult(execResult, err)

	if err != nil {
		os.Exit(ExitRuntimeError)
	}

	os.Exit(ExitSuccess)
}

func runVersion(_ *cobra.Command, _ []string) {
	fmt.Printf("Version: %s\n", version)
	fmt.Printf("Commit: %s\n", commit)
	fmt.Printf("Build Date: %s\n", buildDate)
}

// createInputModule creates an input module instance from configuration.
// Note: This returns a stub module until Epic 3 implements real modules.
func createInputModule(cfg *connector.ModuleConfig) input.Module {
	if cfg == nil {
		return nil
	}
	// For now, return a stub module based on type
	// Real implementations will be added in Epic 3
	switch cfg.Type {
	case "httpPolling":
		endpoint, _ := cfg.Config["endpoint"].(string)
		return &StubInputModule{
			moduleType: cfg.Type,
			endpoint:   endpoint,
		}
	default:
		return &StubInputModule{
			moduleType: cfg.Type,
		}
	}
}

// createFilterModules creates filter module instances from configuration.
// Note: Non-mapping filters are stubbed until Epic 3 implements real modules.
func createFilterModules(cfgs []connector.ModuleConfig) ([]filter.Module, error) {
	if len(cfgs) == 0 {
		return nil, nil
	}
	modules := make([]filter.Module, 0, len(cfgs))
	for i, cfg := range cfgs {
		switch cfg.Type {
		case "mapping":
			mappings, err := filter.ParseFieldMappings(cfg.Config["mappings"])
			if err != nil {
				return nil, fmt.Errorf("invalid mapping config at index %d: %w", i, err)
			}
			onError, _ := cfg.Config["onError"].(string)
			module, err := filter.NewMappingFromConfig(mappings, onError)
			if err != nil {
				return nil, fmt.Errorf("invalid mapping config at index %d: %w", i, err)
			}
			modules = append(modules, module)
		default:
			modules = append(modules, &StubFilterModule{
				moduleType: cfg.Type,
				index:      i,
			})
		}
	}
	return modules, nil
}

// createOutputModule creates an output module instance from configuration.
// Note: This returns a stub module until Epic 3 implements real modules.
func createOutputModule(cfg *connector.ModuleConfig) output.Module {
	if cfg == nil {
		return nil
	}
	switch cfg.Type {
	case "httpRequest":
		endpoint, _ := cfg.Config["endpoint"].(string)
		method, _ := cfg.Config["method"].(string)
		return &StubOutputModule{
			moduleType: cfg.Type,
			endpoint:   endpoint,
			method:     method,
		}
	default:
		return &StubOutputModule{
			moduleType: cfg.Type,
		}
	}
}

// printExecutionResult displays the pipeline execution result.
func printExecutionResult(result *connector.ExecutionResult, err error) {
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

	if !quiet {
		fmt.Println("✓ Pipeline executed successfully")
		fmt.Printf("  Status: %s\n", result.Status)
		fmt.Printf("  Records processed: %d\n", result.RecordsProcessed)
		if result.RecordsFailed > 0 {
			fmt.Printf("  Records failed: %d\n", result.RecordsFailed)
		}
		if verbose {
			fmt.Printf("  Duration: %v\n", result.CompletedAt.Sub(result.StartedAt))
		}
	}
}

// =============================================================================
// Stub Module Implementations (until Epic 3)
// =============================================================================

// StubInputModule is a placeholder input module for testing the pipeline flow.
// Real implementations will be added in Epic 3.
type StubInputModule struct {
	moduleType string
	endpoint   string
}

func (m *StubInputModule) Fetch() ([]map[string]interface{}, error) {
	// Return sample data to demonstrate pipeline flow
	logger.Info("Input module fetching data",
		slog.String("type", m.moduleType),
		slog.String("endpoint", m.endpoint))

	// Simulate fetching data (stub data for testing)
	return []map[string]interface{}{
		{"id": "1", "name": "Sample Record 1", "value": 100},
		{"id": "2", "name": "Sample Record 2", "value": 200},
	}, nil
}

func (m *StubInputModule) Close() error {
	return nil
}

// Verify StubInputModule implements input.Module
var _ input.Module = (*StubInputModule)(nil)

// StubFilterModule is a placeholder filter module for testing the pipeline flow.
// Real implementations will be added in Epic 3.
type StubFilterModule struct {
	moduleType string
	index      int
}

func (m *StubFilterModule) Process(records []map[string]interface{}) ([]map[string]interface{}, error) {
	logger.Info("Filter module processing data",
		slog.String("type", m.moduleType),
		slog.Int("index", m.index),
		slog.Int("records", len(records)))

	// Pass through records unchanged (stub behavior)
	return records, nil
}

// Verify StubFilterModule implements filter.Module
var _ filter.Module = (*StubFilterModule)(nil)

// StubOutputModule is a placeholder output module for testing the pipeline flow.
// Real implementations will be added in Epic 3.
type StubOutputModule struct {
	moduleType string
	endpoint   string
	method     string
}

func (m *StubOutputModule) Send(records []map[string]interface{}) (int, error) {
	logger.Info("Output module sending data",
		slog.String("type", m.moduleType),
		slog.String("endpoint", m.endpoint),
		slog.String("method", m.method),
		slog.Int("records", len(records)))

	// Simulate successful send (stub behavior)
	return len(records), nil
}

func (m *StubOutputModule) Close() error {
	return nil
}

// Verify StubOutputModule implements output.Module
var _ output.Module = (*StubOutputModule)(nil)

func printParseErrors(errors []config.ParseError) {
	fmt.Fprintln(os.Stderr, "✗ Parse errors:")
	for _, err := range errors {
		var location string
		if err.Path != "" {
			location = err.Path
			if err.Line > 0 {
				location += fmt.Sprintf(":%d", err.Line)
				if err.Column > 0 {
					location += fmt.Sprintf(":%d", err.Column)
				}
			}
		}

		if location != "" {
			fmt.Fprintf(os.Stderr, "  %s: %s\n", location, err.Message)
		} else {
			fmt.Fprintf(os.Stderr, "  %s\n", err.Message)
		}

		if verbose && err.Type != "" {
			fmt.Fprintf(os.Stderr, "    Type: %s\n", err.Type)
		}
	}
}

func printValidationErrors(errors []config.ValidationError) {
	fmt.Fprintln(os.Stderr, "✗ Validation errors:")
	for _, err := range errors {
		path := err.Path
		if path == "" {
			path = "/"
		}

		// Format message for readability
		msg := err.Message
		if verbose {
			fmt.Fprintf(os.Stderr, "  %s:\n", path)
			fmt.Fprintf(os.Stderr, "    Message: %s\n", msg)
			if err.Type != "" {
				fmt.Fprintf(os.Stderr, "    Type: %s\n", err.Type)
			}
			if err.Expected != "" {
				fmt.Fprintf(os.Stderr, "    Expected: %s\n", err.Expected)
			}
		} else {
			// Compact format
			shortMsg := msg
			if len(shortMsg) > 80 {
				shortMsg = shortMsg[:77] + "..."
			}
			fmt.Fprintf(os.Stderr, "  %s: %s\n", path, shortMsg)
		}
	}

	// Suggestion
	if !quiet {
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Hint: Use --verbose for detailed error information")
	}
}
