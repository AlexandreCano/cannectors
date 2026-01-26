// Package main provides the CLI entry point for the Canectors runtime.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/canectors/runtime/internal/cli"
	"github.com/canectors/runtime/internal/config"
	"github.com/canectors/runtime/internal/factory"
	"github.com/canectors/runtime/internal/logger"
	"github.com/canectors/runtime/internal/persistence"
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
	logFile string

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

  # Validate with verbose output (human-readable logs)
  canectors validate --verbose config.json`,
	PersistentPreRun: func(_ *cobra.Command, _ []string) {
		logLevel := slog.LevelInfo
		consoleFormat := logger.FormatJSON

		if verbose {
			logLevel = slog.LevelDebug
			consoleFormat = logger.FormatHuman
		} else if quiet {
			logLevel = slog.LevelError
		}

		if logFile != "" {
			if err := logger.SetLogFile(logFile, logLevel, consoleFormat); err != nil {
				fmt.Fprintf(os.Stderr, "⚠ Failed to open log file: %v\n", err)
				logger.SetLevelAndFormat(logLevel, consoleFormat)
			}
		} else {
			logger.SetLevelAndFormat(logLevel, consoleFormat)
		}
	},
	PersistentPostRun: func(_ *cobra.Command, _ []string) {
		logger.CloseLogFile()
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

If the input module includes a 'schedule' field with a CRON expression,
the pipeline runs on a recurring schedule until interrupted (Ctrl+C).
Without a schedule in the input module, the pipeline executes once and exits.

CRON Schedule (input module level):
  If the input module includes "schedule": "*/5 * * * *", the pipeline runs every 5 minutes.
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
  canectors run scheduled-pipeline.yaml        # Run on CRON schedule (if input module has schedule)`,
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
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output (human-readable format)")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "Suppress non-error output")
	rootCmd.PersistentFlags().StringVar(&logFile, "log-file", "", "Write logs to file (always JSON format, in addition to console)")

	runCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate and prepare without executing output module")

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

	if len(result.ParseErrors) > 0 {
		cli.PrintParseErrors(result.ParseErrors, verbose)
		os.Exit(ExitParseError)
	}

	if len(result.ValidationErrors) > 0 {
		cli.PrintValidationErrors(result.ValidationErrors, verbose, quiet)
		os.Exit(ExitValidationError)
	}

	if !quiet {
		fmt.Printf("✓ Configuration is valid (format: %s)\n", result.Format)
		if verbose {
			cli.PrintConfigSummary(result.Data)
		}
	}

	os.Exit(ExitSuccess)
}

func runPipeline(_ *cobra.Command, args []string) {
	configPath := args[0]

	if !quiet {
		fmt.Printf("Loading pipeline configuration: %s\n", configPath)
	}

	result := config.ParseConfig(configPath)

	if len(result.ParseErrors) > 0 {
		cli.PrintParseErrors(result.ParseErrors, verbose)
		os.Exit(ExitParseError)
	}

	if len(result.ValidationErrors) > 0 {
		cli.PrintValidationErrors(result.ValidationErrors, verbose, quiet)
		os.Exit(ExitValidationError)
	}

	if !quiet {
		fmt.Printf("✓ Configuration loaded successfully (format: %s)\n", result.Format)
	}

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

	// Check if pipeline has a schedule in input module config
	schedule := scheduler.GetScheduleFromInput(pipeline)
	if schedule != "" {
		runScheduledPipeline(pipeline, schedule)
		return
	}

	runPipelineOnce(pipeline)
}

func runPipelineOnce(pipeline *connector.Pipeline) {
	inputModule, err := factory.CreateInputModule(pipeline.Input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "✗ Failed to create input module: %v\n", err)
		os.Exit(ExitRuntimeError)
	}
	filterModules, err := factory.CreateFilterModules(pipeline.Filters)
	if err != nil {
		fmt.Fprintf(os.Stderr, "✗ Failed to create filter modules: %v\n", err)
		os.Exit(ExitRuntimeError)
	}
	outputModule, err := factory.CreateOutputModule(pipeline.Output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "✗ Failed to create output module: %v\n", err)
		os.Exit(ExitRuntimeError)
	}

	executor := runtime.NewExecutorWithModules(inputModule, filterModules, outputModule, dryRun)

	// Configure state persistence if input module supports it
	// Create stateStore with default path (input module will use its own if it has custom storagePath)
	stateStore := persistence.NewStateStore("")
	executor.SetStateStore(stateStore)

	if !quiet {
		if dryRun {
			fmt.Println("Executing pipeline (dry-run mode - output will not be sent)...")
		} else {
			fmt.Println("Executing pipeline...")
		}
	}

	execResult, execErr := executor.Execute(pipeline)

	opts := cli.OutputOptions{Verbose: verbose, Quiet: quiet, DryRun: dryRun}
	cli.PrintExecutionResult(execResult, execErr, opts)

	if execErr != nil {
		os.Exit(ExitRuntimeError)
	}

	os.Exit(ExitSuccess)
}

func runScheduledPipeline(pipeline *connector.Pipeline, schedule string) {
	if err := scheduler.ValidateCronExpression(schedule); err != nil {
		fmt.Fprintf(os.Stderr, "✗ Invalid CRON expression: %v\n", err)
		os.Exit(ExitValidationError)
	}

	if !pipeline.Enabled {
		fmt.Fprintln(os.Stderr, "✗ Pipeline is disabled")
		fmt.Fprintln(os.Stderr, "  Set 'enabled: true' in the configuration to run the pipeline")
		os.Exit(ExitValidationError)
	}

	if verbose {
		fmt.Printf("  Schedule: %s\n", schedule)
	}

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

	if !quiet {
		if dryRun {
			fmt.Println("🕐 Scheduler started (dry-run mode - output will not be sent)")
		} else {
			fmt.Println("🕐 Scheduler started")
		}
		fmt.Printf("  Pipeline: %s\n", pipeline.ID)
		fmt.Printf("  Schedule: %s\n", schedule)

		if nextRun, err := sched.GetNextRun(pipeline.ID); err == nil && !nextRun.IsZero() {
			fmt.Printf("  Next run: %s\n", nextRun.Format(time.RFC3339))
		} else if verbose {
			fmt.Printf("  Next run: unavailable (%v)\n", err)
		}

		fmt.Println("  Press Ctrl+C to stop...")
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan

	if !quiet {
		fmt.Printf("\n⏹ Received %s signal, stopping scheduler...\n", sig)
	}

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
type PipelineExecutorAdapter struct {
	dryRun bool
}

// Execute runs a pipeline using the runtime executor.
func (a *PipelineExecutorAdapter) Execute(pipeline *connector.Pipeline) (*connector.ExecutionResult, error) {
	inputModule, err := factory.CreateInputModule(pipeline.Input)
	if err != nil {
		return &connector.ExecutionResult{
			PipelineID:  pipeline.ID,
			Status:      "error",
			StartedAt:   time.Now(),
			CompletedAt: time.Now(),
			Error: &connector.ExecutionError{
				Code:    "INPUT_CREATION_FAILED",
				Message: err.Error(),
			},
		}, err
	}
	filterModules, err := factory.CreateFilterModules(pipeline.Filters)
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
	outputModule, err := factory.CreateOutputModule(pipeline.Output)
	if err != nil {
		return &connector.ExecutionResult{
			PipelineID:  pipeline.ID,
			Status:      "error",
			StartedAt:   time.Now(),
			CompletedAt: time.Now(),
			Error: &connector.ExecutionError{
				Code:    "OUTPUT_CREATION_FAILED",
				Message: err.Error(),
			},
		}, err
	}

	executor := runtime.NewExecutorWithModules(inputModule, filterModules, outputModule, a.dryRun)

	// Configure state persistence if input module supports it
	// Create stateStore with default path (input module will use its own if it has custom storagePath)
	stateStore := persistence.NewStateStore("")
	executor.SetStateStore(stateStore)

	return executor.Execute(pipeline)
}

// Verify PipelineExecutorAdapter implements scheduler.Executor
var _ scheduler.Executor = (*PipelineExecutorAdapter)(nil)
