// Package main provides the CLI entry point for the Canectors runtime.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/canectors/runtime/internal/config"
	"github.com/canectors/runtime/internal/logger"
	"github.com/canectors/runtime/internal/modules/filter"
	"github.com/canectors/runtime/internal/modules/input"
	"github.com/canectors/runtime/internal/modules/output"
	"github.com/canectors/runtime/internal/runtime"
	"github.com/canectors/runtime/internal/scheduler"
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

If the configuration includes a 'schedule' field with a CRON expression,
the pipeline runs on a recurring schedule until interrupted (Ctrl+C).
Without a schedule, the pipeline executes once and exits.

CRON Schedule (optional):
  If your config includes "schedule": "*/5 * * * *", the pipeline runs every 5 minutes.
  Standard format: minute hour day month weekday
  Extended format: second minute hour day month weekday

Flags:
  --dry-run   Validate and prepare the pipeline without executing output module

Exit codes:
  0 - Pipeline executed successfully
  1 - Validation errors
  2 - Parse errors
  3 - Runtime errors

Examples:
  canectors run config.json                    # Run once
  canectors run --verbose pipeline.yaml        # Run once with verbose output
  canectors run --dry-run config.json          # Dry-run mode
  canectors run scheduled-pipeline.yaml        # Run on CRON schedule (if schedule field present)`,
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

	result := config.ParseConfig(configPath)

	if exitCode := handleConfigErrors(result); exitCode != ExitSuccess {
		os.Exit(exitCode)
	}

	printValidationSuccess(result)
	os.Exit(ExitSuccess)
}

// handleConfigErrors checks for parse and validation errors and returns the appropriate exit code.
func handleConfigErrors(result *config.Result) int {
	if len(result.ParseErrors) > 0 {
		printParseErrors(result.ParseErrors)
		return ExitParseError
	}

	if len(result.ValidationErrors) > 0 {
		printValidationErrors(result.ValidationErrors)
		return ExitValidationError
	}

	return ExitSuccess
}

// printValidationSuccess prints success message and optional configuration summary.
func printValidationSuccess(result *config.Result) {
	if quiet {
		return
	}

	fmt.Printf("✓ Configuration is valid (format: %s)\n", result.Format)

	if verbose {
		printConfigSummary(result.Data)
	}
}

// printConfigSummary prints connector name and version if available.
func printConfigSummary(data map[string]interface{}) {
	if data == nil {
		return
	}

	connector, ok := data["connector"].(map[string]interface{})
	if !ok {
		return
	}

	if name, ok := connector["name"].(string); ok {
		fmt.Printf("  Connector: %s\n", name)
	}
	if connectorVersion, ok := connector["version"].(string); ok {
		fmt.Printf("  Version: %s\n", connectorVersion)
	}
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

	// Check if pipeline has a schedule - if so, run in scheduler mode
	if pipeline.Schedule != "" {
		runScheduledPipeline(pipeline)
		return
	}

	// Run pipeline once (no schedule)
	runPipelineOnce(pipeline)
}

// runPipelineOnce executes a pipeline a single time and exits.
func runPipelineOnce(pipeline *connector.Pipeline) {
	// Create module instances
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

// runScheduledPipeline runs a pipeline on a CRON schedule until interrupted.
func runScheduledPipeline(pipeline *connector.Pipeline) {
	validateScheduledPipeline(pipeline)

	sched := createAndStartScheduler(pipeline)

	printSchedulerStarted(pipeline, sched)

	waitForShutdownSignal(sched)
}

// validateScheduledPipeline validates the pipeline for scheduled execution.
func validateScheduledPipeline(pipeline *connector.Pipeline) {
	if err := scheduler.ValidateCronExpression(pipeline.Schedule); err != nil {
		fmt.Fprintf(os.Stderr, "✗ Invalid CRON expression: %v\n", err)
		os.Exit(ExitValidationError)
	}

	if !pipeline.Enabled {
		fmt.Fprintln(os.Stderr, "✗ Pipeline is disabled")
		fmt.Fprintln(os.Stderr, "  Set 'enabled: true' in the configuration to run the pipeline")
		os.Exit(ExitValidationError)
	}

	if verbose {
		fmt.Printf("  Schedule: %s\n", pipeline.Schedule)
	}
}

// createAndStartScheduler creates the scheduler, registers the pipeline, and starts it.
func createAndStartScheduler(pipeline *connector.Pipeline) *scheduler.Scheduler {
	executorAdapter := &PipelineExecutorAdapter{dryRun: dryRun}
	sched := scheduler.NewWithExecutor(executorAdapter)

	if err := sched.Register(pipeline); err != nil {
		fmt.Fprintf(os.Stderr, "✗ Failed to register pipeline with scheduler: %v\n", err)
		os.Exit(ExitRuntimeError)
	}

	if err := sched.Start(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "✗ Failed to start scheduler: %v\n", err)
		os.Exit(ExitRuntimeError)
	}

	return sched
}

// printSchedulerStarted prints scheduler status information.
func printSchedulerStarted(pipeline *connector.Pipeline, sched *scheduler.Scheduler) {
	if quiet {
		return
	}

	if dryRun {
		fmt.Println("🕐 Scheduler started (dry-run mode - output will not be sent)")
	} else {
		fmt.Println("🕐 Scheduler started")
	}

	fmt.Printf("  Pipeline: %s\n", pipeline.ID)
	fmt.Printf("  Schedule: %s\n", pipeline.Schedule)

	printNextRunTime(pipeline.ID, sched)

	fmt.Println("  Press Ctrl+C to stop...")
}

// printNextRunTime prints the next scheduled run time if available.
func printNextRunTime(pipelineID string, sched *scheduler.Scheduler) {
	nextRun, err := sched.GetNextRun(pipelineID)
	if err == nil && !nextRun.IsZero() {
		fmt.Printf("  Next run: %s\n", nextRun.Format(time.RFC3339))
	} else if verbose {
		fmt.Printf("  Next run: unavailable (%v)\n", err)
	}
}

// waitForShutdownSignal waits for an interrupt signal and gracefully stops the scheduler.
func waitForShutdownSignal(sched *scheduler.Scheduler) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan

	if !quiet {
		fmt.Printf("\n⏹ Received %s signal, stopping scheduler...\n", sig)
	}

	gracefulShutdown(sched)
}

// gracefulShutdown stops the scheduler with a timeout.
func gracefulShutdown(sched *scheduler.Scheduler) {
	stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := sched.Stop(stopCtx); err != nil {
		fmt.Fprintf(os.Stderr, "⚠ Scheduler stop timeout: %v\n", err)
		os.Exit(ExitRuntimeError)
	}

	if !quiet {
		fmt.Println("✓ Scheduler stopped gracefully")
	}

	os.Exit(ExitSuccess)
}

func runVersion(_ *cobra.Command, _ []string) {
	fmt.Printf("Version: %s\n", version)
	fmt.Printf("Commit: %s\n", commit)
	fmt.Printf("Build Date: %s\n", buildDate)
}

// PipelineExecutorAdapter adapts the runtime.Executor for use with the scheduler.
// It implements the scheduler.Executor interface.
type PipelineExecutorAdapter struct {
	dryRun bool
}

// Execute runs a pipeline using the runtime executor.
// It creates fresh module instances for each execution.
func (a *PipelineExecutorAdapter) Execute(pipeline *connector.Pipeline) (*connector.ExecutionResult, error) {
	// Create module instances for this execution
	inputModule := createInputModule(pipeline.Input)
	filterModules, err := createFilterModules(pipeline.Filters)
	if err != nil {
		return &connector.ExecutionResult{
			PipelineID:  pipeline.ID,
			Status:      "error",
			StartedAt:   time.Now(),
			CompletedAt: time.Now(),
			Error: &connector.ExecutionError{
				Code:    "FILTER_CREATION_FAILED",
				Message: err.Error(),
			},
		}, err
	}
	outputModule := createOutputModule(pipeline.Output)

	// Create executor and run pipeline
	executor := runtime.NewExecutorWithModules(inputModule, filterModules, outputModule, a.dryRun)
	return executor.Execute(pipeline)
}

// Verify PipelineExecutorAdapter implements scheduler.Executor
var _ scheduler.Executor = (*PipelineExecutorAdapter)(nil)

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
// Supports mapping and condition filter types. Other filter types use stub implementations.
func createFilterModules(cfgs []connector.ModuleConfig) ([]filter.Module, error) {
	if len(cfgs) == 0 {
		return nil, nil
	}

	modules := make([]filter.Module, 0, len(cfgs))
	for i, cfg := range cfgs {
		module, err := createSingleFilterModule(cfg, i)
		if err != nil {
			return nil, err
		}
		modules = append(modules, module)
	}
	return modules, nil
}

// createSingleFilterModule creates a single filter module based on its type.
func createSingleFilterModule(cfg connector.ModuleConfig, index int) (filter.Module, error) {
	switch cfg.Type {
	case "mapping":
		return createMappingFilterModule(cfg, index)
	case "condition":
		return createConditionFilterModule(cfg, index)
	default:
		return &StubFilterModule{moduleType: cfg.Type, index: index}, nil
	}
}

// createMappingFilterModule creates a mapping filter module from configuration.
func createMappingFilterModule(cfg connector.ModuleConfig, index int) (filter.Module, error) {
	mappings, err := filter.ParseFieldMappings(cfg.Config["mappings"])
	if err != nil {
		return nil, fmt.Errorf("invalid mapping config at index %d: %w", index, err)
	}

	onError, _ := cfg.Config["onError"].(string)
	module, err := filter.NewMappingFromConfig(mappings, onError)
	if err != nil {
		return nil, fmt.Errorf("invalid mapping config at index %d: %w", index, err)
	}

	return module, nil
}

// createConditionFilterModule creates a condition filter module from configuration.
func createConditionFilterModule(cfg connector.ModuleConfig, index int) (filter.Module, error) {
	condConfig, err := parseConditionConfig(cfg.Config)
	if err != nil {
		return nil, fmt.Errorf("invalid condition config at index %d: %w", index, err)
	}

	module, err := filter.NewConditionFromConfig(condConfig)
	if err != nil {
		return nil, fmt.Errorf("invalid condition config at index %d: %w", index, err)
	}

	return module, nil
}

// parseConditionConfig parses a condition filter configuration from raw config.
func parseConditionConfig(cfg map[string]interface{}) (filter.ConditionConfig, error) {
	condConfig := filter.ConditionConfig{}

	// Validate that expression field is present and not empty
	expr, ok := cfg["expression"].(string)
	if !ok || expr == "" {
		return condConfig, fmt.Errorf("required field 'expression' is missing or empty in condition config")
	}
	condConfig.Expression = expr
	if lang, ok := cfg["lang"].(string); ok {
		condConfig.Lang = lang
	}
	if onTrue, ok := cfg["onTrue"].(string); ok {
		condConfig.OnTrue = onTrue
	}
	if onFalse, ok := cfg["onFalse"].(string); ok {
		condConfig.OnFalse = onFalse
	}
	if onError, ok := cfg["onError"].(string); ok {
		condConfig.OnError = onError
	}

	// Parse nested 'then' module
	if thenCfg, ok := cfg["then"].(map[string]interface{}); ok {
		nestedModule, err := parseNestedModuleConfig(thenCfg)
		if err != nil {
			return condConfig, fmt.Errorf("invalid 'then' config: %w", err)
		}
		condConfig.Then = nestedModule
	}

	// Parse nested 'else' module
	if elseCfg, ok := cfg["else"].(map[string]interface{}); ok {
		nestedModule, err := parseNestedModuleConfig(elseCfg)
		if err != nil {
			return condConfig, fmt.Errorf("invalid 'else' config: %w", err)
		}
		condConfig.Else = nestedModule
	}

	return condConfig, nil
}

// parseNestedModuleConfig parses a nested module configuration.
// It handles both direct field access and nested config field access to support
// different configuration formats.
// depth tracks the current nesting depth to prevent infinite recursion.
func parseNestedModuleConfig(cfg map[string]interface{}) (*filter.NestedModuleConfig, error) {
	return parseNestedModuleConfigWithDepth(cfg, 0)
}

// parseNestedModuleConfigWithDepth parses a nested module configuration with depth tracking.
func parseNestedModuleConfigWithDepth(cfg map[string]interface{}, depth int) (*filter.NestedModuleConfig, error) {
	if err := validateNestingDepth(depth); err != nil {
		return nil, err
	}

	nestedConfig := initializeNestedConfig(cfg)
	nestedConfigMap := extractNestedConfigMap(cfg, nestedConfig)

	// Parse module-specific configuration based on type
	switch nestedConfig.Type {
	case "mapping":
		if err := parseMappingConfig(cfg, nestedConfig, nestedConfigMap); err != nil {
			return nil, err
		}
	case "condition":
		if err := parseConditionNestedConfig(cfg, nestedConfig, nestedConfigMap, depth); err != nil {
			return nil, err
		}
	}

	return nestedConfig, nil
}

// validateNestingDepth checks if the nesting depth is within limits.
func validateNestingDepth(depth int) error {
	if depth >= 50 { // Use same limit as MaxNestingDepth
		return fmt.Errorf("nested module depth %d exceeds maximum 50", depth)
	}
	return nil
}

// initializeNestedConfig initializes a NestedModuleConfig from the raw config.
func initializeNestedConfig(cfg map[string]interface{}) *filter.NestedModuleConfig {
	nestedConfig := &filter.NestedModuleConfig{}
	if typ, ok := cfg["type"].(string); ok {
		nestedConfig.Type = typ
	}
	if onError, ok := cfg["onError"].(string); ok {
		nestedConfig.OnError = onError
	}
	return nestedConfig
}

// extractNestedConfigMap extracts the nested config map if present.
func extractNestedConfigMap(cfg map[string]interface{}, nestedConfig *filter.NestedModuleConfig) map[string]interface{} {
	if config, ok := cfg["config"].(map[string]interface{}); ok {
		nestedConfig.Config = config
		return config
	}
	return nil
}

// getNestedString retrieves a string value from either the direct config field or nested config field.
func getNestedString(cfg map[string]interface{}, nestedConfigMap map[string]interface{}, key string) (string, bool) {
	if val, ok := cfg[key].(string); ok {
		return val, true
	}
	if nestedConfigMap != nil {
		if val, ok := nestedConfigMap[key].(string); ok {
			return val, true
		}
	}
	return "", false
}

// getNestedMap retrieves a map value from either the direct config field or nested config field.
func getNestedMap(cfg map[string]interface{}, nestedConfigMap map[string]interface{}, key string) (map[string]interface{}, bool) {
	if val, ok := cfg[key].(map[string]interface{}); ok {
		return val, true
	}
	if nestedConfigMap != nil {
		if val, ok := nestedConfigMap[key].(map[string]interface{}); ok {
			return val, true
		}
	}
	return nil, false
}

// parseMappingConfig parses configuration for a mapping module.
func parseMappingConfig(cfg map[string]interface{}, nestedConfig *filter.NestedModuleConfig, nestedConfigMap map[string]interface{}) error {
	mappingsRaw, ok := getMappingsRaw(cfg, nestedConfigMap)
	if ok {
		mappings, err := filter.ParseFieldMappings(mappingsRaw)
		if err != nil {
			return err
		}
		nestedConfig.Mappings = mappings
	}

	// Get onError from nested config if not already set
	if nestedConfig.OnError == "" && nestedConfigMap != nil {
		if configOnError, ok := nestedConfigMap["onError"].(string); ok {
			nestedConfig.OnError = configOnError
		}
	}
	return nil
}

// getMappingsRaw retrieves mappings from either direct field or nested config.
func getMappingsRaw(cfg map[string]interface{}, nestedConfigMap map[string]interface{}) (interface{}, bool) {
	if mappingsRaw, ok := cfg["mappings"]; ok {
		return mappingsRaw, true
	}
	if nestedConfigMap != nil {
		if mappingsRaw, ok := nestedConfigMap["mappings"]; ok {
			return mappingsRaw, true
		}
	}
	return nil, false
}

// parseConditionNestedConfig parses configuration for a condition module.
func parseConditionNestedConfig(cfg map[string]interface{}, nestedConfig *filter.NestedModuleConfig, nestedConfigMap map[string]interface{}, depth int) error {
	// Parse condition-specific fields
	if expr, ok := getNestedString(cfg, nestedConfigMap, "expression"); ok {
		nestedConfig.Expression = expr
	}
	if lang, ok := getNestedString(cfg, nestedConfigMap, "lang"); ok {
		nestedConfig.Lang = lang
	}
	if onTrue, ok := getNestedString(cfg, nestedConfigMap, "onTrue"); ok {
		nestedConfig.OnTrue = onTrue
	}
	if onFalse, ok := getNestedString(cfg, nestedConfigMap, "onFalse"); ok {
		nestedConfig.OnFalse = onFalse
	}
	if nestedConfig.OnError == "" {
		if onError, ok := getNestedString(cfg, nestedConfigMap, "onError"); ok {
			nestedConfig.OnError = onError
		}
	}

	// Parse recursive nested modules
	if err := parseNestedThenElse(cfg, nestedConfig, nestedConfigMap, depth); err != nil {
		return err
	}

	return nil
}

// parseNestedThenElse parses the then and else nested modules recursively.
func parseNestedThenElse(cfg map[string]interface{}, nestedConfig *filter.NestedModuleConfig, nestedConfigMap map[string]interface{}, depth int) error {
	if thenCfg, ok := getNestedMap(cfg, nestedConfigMap, "then"); ok {
		then, err := parseNestedModuleConfigWithDepth(thenCfg, depth+1)
		if err != nil {
			return err
		}
		nestedConfig.Then = then
	}

	if elseCfg, ok := getNestedMap(cfg, nestedConfigMap, "else"); ok {
		elseModule, err := parseNestedModuleConfigWithDepth(elseCfg, depth+1)
		if err != nil {
			return err
		}
		nestedConfig.Else = elseModule
	}

	return nil
}

// createOutputModule creates an output module instance from configuration.
// Uses real HTTPRequestModule for httpRequest type, stub for others.
func createOutputModule(cfg *connector.ModuleConfig) output.Module {
	if cfg == nil {
		return nil
	}
	switch cfg.Type {
	case "httpRequest":
		// Use real HTTPRequestModule which implements PreviewableModule
		module, err := output.NewHTTPRequestFromConfig(cfg)
		if err != nil {
			logger.Warn("failed to create HTTP request module, using stub",
				slog.String("error", err.Error()))
			endpoint, _ := cfg.Config["endpoint"].(string)
			method, _ := cfg.Config["method"].(string)
			return &StubOutputModule{
				moduleType: cfg.Type,
				endpoint:   endpoint,
				method:     method,
			}
		}
		return module
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

		// Display dry-run preview if available
		if dryRun && len(result.DryRunPreview) > 0 {
			printDryRunPreview(result.DryRunPreview)
		}
	}
}

// printDryRunPreview displays the request preview for dry-run mode.
func printDryRunPreview(previews []connector.RequestPreview) {
	fmt.Println()
	fmt.Println("📋 Dry-Run Preview (what would have been sent):")
	fmt.Println()

	for i, preview := range previews {
		if len(previews) > 1 {
			fmt.Printf("─── Request %d of %d ───\n", i+1, len(previews))
		}

		// Endpoint and method
		fmt.Printf("  Endpoint: %s %s\n", preview.Method, preview.Endpoint)
		fmt.Printf("  Records: %d\n", preview.RecordCount)

		// Headers (always show all headers in dry-run mode)
		if len(preview.Headers) > 0 {
			fmt.Println("  Headers:")
			for name, value := range preview.Headers {
				fmt.Printf("    %s: %s\n", name, value)
			}
		}

		// Body preview
		if preview.BodyPreview != "" {
			printBodyPreview(preview.BodyPreview)
		}

		if i < len(previews)-1 {
			fmt.Println()
		}
	}

	fmt.Println()
	fmt.Println("ℹ️  No data was sent to the target system (dry-run mode)")
}

// printBodyPreview displays the formatted JSON body preview.
// In verbose mode or for small payloads (<=10 lines), shows full body.
// For large payloads in non-verbose mode, truncates to 10 lines.
func printBodyPreview(bodyPreview string) {
	const maxLinesCompact = 10
	lineCount := countLines(bodyPreview)

	// Show full body in verbose mode or for small payloads
	if verbose || lineCount <= maxLinesCompact {
		fmt.Println("  Body:")
		printIndentedBody(bodyPreview, "    ")
		return
	}

	// Truncate large payloads in non-verbose mode
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
// Optimized using strings.Count for better performance.
func countLines(s string) int {
	return strings.Count(s, "\n") + 1
}

// splitLines splits a string into lines.
// Uses strings.Split for simplicity and performance.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	// Remove trailing empty line if string ends with newline
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
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
		printSingleParseError(err)
	}
}

// printSingleParseError prints a single parse error with location information.
func printSingleParseError(err config.ParseError) {
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

func printValidationErrors(errors []config.ValidationError) {
	fmt.Fprintln(os.Stderr, "✗ Validation errors:")
	for _, err := range errors {
		printSingleValidationError(err)
	}
	printValidationHint()
}

// printSingleValidationError prints a single validation error.
func printSingleValidationError(err config.ValidationError) {
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
func printValidationHint() {
	if !quiet {
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Hint: Use --verbose for detailed error information")
	}
}
