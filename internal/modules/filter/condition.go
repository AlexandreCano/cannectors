// Package filter provides implementations for filter modules.
// Condition module filters and routes data based on conditional expressions.
package filter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"

	"github.com/cannectors/runtime/internal/errhandling"
	"github.com/cannectors/runtime/internal/logger"
	"github.com/cannectors/runtime/pkg/connector"
)

// Error codes for condition module
const (
	ErrCodeEvaluationFailed = "EVALUATION_FAILED"
)

// Security limits for expression validation
const (
	// MaxExpressionLength is the maximum allowed length for condition expressions
	// to prevent DoS attacks through extremely long expressions.
	MaxExpressionLength = 10000
)

// Common errors for condition module
var (
	// ErrEmptyExpression is returned when the expression is empty or whitespace-only
	ErrEmptyExpression = errors.New("expression cannot be empty")
	// ErrInvalidExpression is returned when the expression syntax is invalid
	ErrInvalidExpression = errors.New("invalid expression syntax")
	// ErrExpressionTooLong is returned when the expression exceeds MaxExpressionLength
	ErrExpressionTooLong = errors.New("expression exceeds maximum length")
)

// NestedModuleCreator is a function type that creates a filter module from a
// NestedModuleConfig. It is injected by the caller (typically the registry's
// "condition" entry) so the filter package does not need to import the
// registry, avoiding a circular dependency.
//
// When nil, nested module creation falls back to hardcoded support for the
// built-in types parsed inline in this package.
type NestedModuleCreator func(config *NestedModuleConfig, index int) (Module, error)

// Supported expression languages
// Currently only the default expr-based language is supported. The schema no
// longer exposes a `lang` field.

// ConditionConfig represents the configuration for a condition filter module.
type ConditionConfig struct {
	connector.ModuleBase
	// Expression is the condition expression string (required).
	Expression string `json:"expression"`
	// Then contains nested filter module configurations to execute when condition is true (optional).
	Then []*NestedModuleConfig `json:"then,omitempty"`
	// Else contains nested filter module configurations to execute when condition is false (optional).
	Else []*NestedModuleConfig `json:"else,omitempty"`
}

// NestedModuleConfig represents a nested filter module configuration.
type NestedModuleConfig struct {
	Type     string         `json:"type"`
	Config   map[string]any `json:"config,omitempty"`
	Mappings []FieldMapping `json:"mappings,omitempty"`
	// Enabled mirrors moduleBase.enabled — when explicitly false the nested
	// module is skipped at construction time, just like top-level filters
	// (cf. internal/factory/modules.go).
	Enabled *bool `json:"enabled,omitempty"`
	// For nested conditions
	Expression string                `json:"expression,omitempty"`
	OnError    string                `json:"onError,omitempty"`
	Then       []*NestedModuleConfig `json:"then,omitempty"`
	Else       []*NestedModuleConfig `json:"else,omitempty"`
}

// UnmarshalJSON preserves the top-level filter shape for nested condition
// modules. Known routing fields stay on NestedModuleConfig; every other field
// is treated as module-specific configuration and passed through Config.
func (c *NestedModuleConfig) UnmarshalJSON(data []byte) error {
	type nestedModuleConfigAlias NestedModuleConfig

	var known nestedModuleConfigAlias
	if err := json.Unmarshal(data, &known); err != nil {
		return err
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	*c = NestedModuleConfig(known)
	if c.Config == nil {
		c.Config = make(map[string]any)
	}

	for key, valueRaw := range raw {
		if isNestedModuleStructuralField(key) {
			continue
		}

		var value any
		if err := json.Unmarshal(valueRaw, &value); err != nil {
			return fmt.Errorf("parsing nested module field %q: %w", key, err)
		}
		c.Config[key] = value
	}

	if len(c.Config) == 0 {
		c.Config = nil
	}

	return nil
}

func isNestedModuleStructuralField(key string) bool {
	switch key {
	case "type", "config", "mappings", "expression", "onError", "then", "else":
		return true
	default:
		return false
	}
}

// ConditionModule implements conditional filtering and routing.
// It evaluates expressions against input records and routes accordingly.
type ConditionModule struct {
	expression  string
	onError     errhandling.OnErrorStrategy
	program     *vm.Program
	thenModules []Module
	elseModules []Module
}

// ConditionError carries structured context for condition evaluation failures.
type ConditionError struct {
	Code        string
	Message     string
	Expression  string
	RecordIndex int
	FieldPath   string
	Details     map[string]any
}

func (e *ConditionError) Error() string {
	return e.Message
}

// ErrorCode implements errhandling.ModuleError.
func (e *ConditionError) ErrorCode() string { return e.Code }

// ErrorModule implements errhandling.ModuleError.
func (e *ConditionError) ErrorModule() string { return "condition" }

// ErrorRecordIndex implements errhandling.ModuleError.
func (e *ConditionError) ErrorRecordIndex() int { return e.RecordIndex }

// ErrorDetails implements errhandling.ModuleError. Returns a copy of the
// existing Details map enriched with the expression and field path so
// callers get a flat view suitable for ExecutionError.Details.
func (e *ConditionError) ErrorDetails() map[string]any {
	d := make(map[string]any, len(e.Details)+2)
	for k, v := range e.Details {
		d[k] = v
	}
	if e.Expression != "" {
		d["expression"] = e.Expression
	}
	if e.FieldPath != "" {
		d["field_path"] = e.FieldPath
	}
	return d
}

// newConditionError creates a ConditionError with optional debugging details.
// If underlyingErr is provided, it will be included in the Details field.
func newConditionError(code, message, expression string, recordIdx int, fieldPath string, underlyingErr error) *ConditionError {
	details := make(map[string]any)
	if underlyingErr != nil {
		details["underlying_error"] = underlyingErr.Error()
		details["error_type"] = fmt.Sprintf("%T", underlyingErr)
	}
	if recordIdx >= 0 {
		details["record_index"] = recordIdx
	}
	if fieldPath != "" {
		details["field_path"] = fieldPath
	}

	return &ConditionError{
		Code:        code,
		Message:     message,
		Expression:  expression,
		RecordIndex: recordIdx,
		FieldPath:   fieldPath,
		Details:     details,
	}
}

// NewConditionFromConfig creates a new condition filter module from configuration.
// It validates the configuration and returns an error if invalid.
//
// nestedCreator is invoked to construct each nested then/else module by type;
// pass nil to fall back to the built-in inline parsing for "mapping",
// "condition" and "drop" only. The registry's "condition" entry injects a
// creator that resolves the full set of registered filter types.
//
// Security considerations:
//   - Expressions are compiled once during module creation using expr.Compile with
//     AllowUndefinedVariables(). This is efficient for pipelines processing many records.
//   - However, expressions from untrusted sources could potentially cause denial-of-service
//     through expensive operations.
//   - The expr library does not execute arbitrary code, but complex expressions may still
//     consume significant CPU time during evaluation.
func NewConditionFromConfig(config ConditionConfig, nestedCreator NestedModuleCreator) (*ConditionModule, error) {
	expression, err := validateExpression(config.Expression)
	if err != nil {
		return nil, fmt.Errorf("validating expression: %w", err)
	}

	onError, err := errhandling.ParseOnErrorStrategy(config.OnError)
	if err != nil {
		return nil, err
	}

	program, err := compileExpression(expression)
	if err != nil {
		return nil, fmt.Errorf("compiling expression: %w", err)
	}

	thenModules, elseModules, err := createNestedModules(config.Then, config.Else, nestedCreator)
	if err != nil {
		return nil, fmt.Errorf("creating nested modules: %w", err)
	}

	logModuleInitialization(expression, onError, thenModules, elseModules)

	return &ConditionModule{
		expression:  expression,
		onError:     onError,
		program:     program,
		thenModules: thenModules,
		elseModules: elseModules,
	}, nil
}

// validateExpression validates that the expression is not empty/whitespace and within length limits.
func validateExpression(expression string) (string, error) {
	if len(expression) == 0 || isWhitespaceOnly(expression) {
		return "", fmt.Errorf("%w", ErrEmptyExpression)
	}
	if len(expression) > MaxExpressionLength {
		return "", fmt.Errorf("%w: expression length %d exceeds maximum %d", ErrExpressionTooLong, len(expression), MaxExpressionLength)
	}
	return expression, nil
}

// compileExpression compiles the expression using the expr library.
func compileExpression(expression string) (*vm.Program, error) {
	// AllowUndefinedVariables() handles missing fields gracefully by treating
	// undefined variables as nil rather than causing evaluation errors.
	program, err := expr.Compile(expression, expr.AllowUndefinedVariables())
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidExpression, err)
	}
	return program, nil
}

// createNestedModules creates then and else nested modules.
// Nested modules with `enabled: false` are skipped (not constructed, not run),
// mirroring the top-level filter behavior in internal/factory.
func createNestedModules(thenConfigs, elseConfigs []*NestedModuleConfig, nestedCreator NestedModuleCreator) ([]Module, []Module, error) {
	var thenModules, elseModules []Module

	for i, cfg := range thenConfigs {
		if cfg == nil {
			continue
		}
		if cfg.Enabled != nil && !*cfg.Enabled {
			logger.Debug("skipping disabled nested 'then' module", "index", i, "type", cfg.Type)
			continue
		}
		m, err := createNestedModule(cfg, nestedCreator)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create 'then' module at index %d: %w", i, err)
		}
		thenModules = append(thenModules, m)
	}

	for i, cfg := range elseConfigs {
		if cfg == nil {
			continue
		}
		if cfg.Enabled != nil && !*cfg.Enabled {
			logger.Debug("skipping disabled nested 'else' module", "index", i, "type", cfg.Type)
			continue
		}
		m, err := createNestedModule(cfg, nestedCreator)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create 'else' module at index %d: %w", i, err)
		}
		elseModules = append(elseModules, m)
	}

	return thenModules, elseModules, nil
}

// logModuleInitialization logs the initialization of a condition module.
func logModuleInitialization(expression string, onError errhandling.OnErrorStrategy, thenModules, elseModules []Module) {
	logger.Debug("condition module initialized",
		slog.String("expression", expression),
		slog.String("on_error", string(onError)),
		slog.Int("then_count", len(thenModules)),
		slog.Int("else_count", len(elseModules)),
	)
}

// isWhitespaceOnly checks if a string contains only whitespace characters.
func isWhitespaceOnly(s string) bool {
	for _, r := range s {
		if r != ' ' && r != '\t' && r != '\n' && r != '\r' {
			return false
		}
	}
	return true
}

// createNestedModule creates a nested filter module.
// Delegates type resolution to the injected nestedCreator (which typically
// uses the registry); falls back to hardcoded built-in support when no
// creator is provided (e.g. unit tests of condition module in isolation).
func createNestedModule(config *NestedModuleConfig, nestedCreator NestedModuleCreator) (Module, error) {
	if config == nil {
		return nil, nil
	}

	// Nested condition modules are constructed inline so the captured
	// nestedCreator propagates correctly.
	if config.Type == "condition" {
		if config.Expression != "" && len(config.Expression) > MaxExpressionLength {
			return nil, fmt.Errorf("%w: expression length %d exceeds maximum %d", ErrExpressionTooLong, len(config.Expression), MaxExpressionLength)
		}
		return NewConditionFromConfig(ConditionConfig{
			ModuleBase: connector.ModuleBase{OnError: config.OnError},
			Expression: config.Expression,
			Then:       config.Then,
			Else:       config.Else,
		}, nestedCreator)
	}

	// Index 0 is fine: nested modules have no pipeline position; the index is
	// used only for error logging context.
	if nestedCreator != nil {
		return nestedCreator(config, 0)
	}

	// Fallback for callers that did not inject a creator (legacy / standalone tests).
	switch config.Type {
	case "mapping":
		mappings, err := ParseFieldMappings(config.Mappings)
		if err != nil {
			return nil, fmt.Errorf("parsing field mappings: %w", err)
		}
		module, err := NewMappingFromConfig(mappings, config.OnError)
		if err != nil {
			return nil, fmt.Errorf("creating mapping module: %w", err)
		}
		return module, nil
	case "drop":
		return NewDrop(), nil
	default:
		return nil, fmt.Errorf("unknown filter module type %q in nested config (no nested creator injected)", config.Type)
	}
}

// handleError processes an error according to the module's onError setting.
// Returns an error if the error should stop processing, nil if it should be skipped/logged.
func (c *ConditionModule) handleError(err error, recordIdx int, errorType string) error {
	switch c.onError {
	case errhandling.OnErrorFail:
		return err
	case errhandling.OnErrorSkip:
		logger.Warn(fmt.Sprintf("skipping record due to %s error", errorType),
			slog.Int("record_index", recordIdx),
			slog.String("expression", c.expression),
			slog.String("error", err.Error()),
		)
		return nil
	case errhandling.OnErrorLog:
		logger.Error(fmt.Sprintf("%s error (continuing)", errorType),
			slog.Int("record_index", recordIdx),
			slog.String("expression", c.expression),
			slog.String("error", err.Error()),
		)
		return nil
	default:
		return err
	}
}

// handleNestedModuleError processes an error from a nested module according to the module's onError setting.
// Returns an error if the error should stop processing, nil if it should be skipped/logged.
func (c *ConditionModule) handleNestedModuleError(err error, recordIdx int) error {
	switch c.onError {
	case errhandling.OnErrorFail:
		return err
	case errhandling.OnErrorSkip:
		logger.Warn("skipping record due to nested module error",
			slog.Int("record_index", recordIdx),
			slog.String("error", err.Error()),
		)
		return nil
	case errhandling.OnErrorLog:
		logger.Error("nested module error (continuing)",
			slog.Int("record_index", recordIdx),
			slog.String("error", err.Error()),
		)
		return nil
	default:
		return err
	}
}

// Process filters records based on the condition expression.
//
// The context can be used to cancel long-running operations.
//
// For each record:
//  1. Evaluates the condition expression using expr library
//  2. If true and 'then' module exists: execute 'then' module
//  3. If true and no 'then' module: apply onTrue behavior
//  4. If false and 'else' module exists: execute 'else' module
//  5. If false and no 'else' module: apply onFalse behavior
//
// Returns the filtered/routed records and any error that occurred.
func (c *ConditionModule) Process(ctx context.Context, records []map[string]any) ([]map[string]any, error) {
	if records == nil {
		return []map[string]any{}, nil
	}

	startTime := time.Now()
	inputCount := len(records)

	logger.Debug("filter processing started",
		slog.String("module_type", "condition"),
		slog.String("expression", c.expression),
		slog.Int("input_records", inputCount),
		slog.String("on_error", string(c.onError)),
		slog.Bool("has_then", len(c.thenModules) > 0),
		slog.Bool("has_else", len(c.elseModules) > 0),
	)

	result := make([]map[string]any, 0, len(records))
	trueCount := 0
	falseCount := 0
	errorCount := 0

	for recordIdx, record := range records {
		// Evaluate the condition using expr library
		output, err := expr.Run(c.program, record)
		if err != nil {
			errorCount++
			// Handle evaluation error
			condErr := newConditionError(
				ErrCodeEvaluationFailed,
				fmt.Sprintf("condition evaluation failed at record %d: %v", recordIdx, err),
				c.expression,
				recordIdx,
				"",
				err,
			)

			if handleErr := c.handleError(condErr, recordIdx, "condition evaluation"); handleErr != nil {
				duration := time.Since(startTime)
				logger.Error("filter processing failed",
					slog.String("module_type", "condition"),
					slog.String("expression", c.expression),
					slog.Int("record_index", recordIdx),
					slog.Duration("duration", duration),
					slog.String("error", handleErr.Error()),
				)
				return nil, handleErr
			}
			continue
		}

		// Convert result to boolean
		conditionResult, ok := output.(bool)
		if !ok {
			// If not a boolean, treat as truthy check
			conditionResult = toBool(output)
		}

		if conditionResult {
			trueCount++
		} else {
			falseCount++
		}

		// Process based on condition result
		processedRecords, err := c.processConditionResult(ctx, conditionResult, record)
		if err != nil {
			errorCount++
			if handleErr := c.handleNestedModuleError(err, recordIdx); handleErr != nil {
				duration := time.Since(startTime)
				logger.Error("filter processing failed",
					slog.String("module_type", "condition"),
					slog.String("expression", c.expression),
					slog.Int("record_index", recordIdx),
					slog.Duration("duration", duration),
					slog.String("error", handleErr.Error()),
				)
				return nil, handleErr
			}
			continue
		}

		result = append(result, processedRecords...)
	}

	duration := time.Since(startTime)
	outputCount := len(result)

	logger.Info("filter processing completed",
		slog.String("module_type", "condition"),
		slog.String("expression", c.expression),
		slog.Int("input_records", inputCount),
		slog.Int("output_records", outputCount),
		slog.Int("true_count", trueCount),
		slog.Int("false_count", falseCount),
		slog.Int("error_count", errorCount),
		slog.Duration("duration", duration),
	)

	return result, nil
}

// processConditionResult handles the result of condition evaluation for a single record.
// When a branch has no nested filters the record is kept unchanged. To drop a
// record explicitly, use the `drop` filter inside the branch.
func (c *ConditionModule) processConditionResult(ctx context.Context, conditionTrue bool, record map[string]any) ([]map[string]any, error) {
	if conditionTrue {
		if len(c.thenModules) > 0 {
			return c.runNestedModules(ctx, c.thenModules, record)
		}
		return []map[string]any{record}, nil
	}

	if len(c.elseModules) > 0 {
		return c.runNestedModules(ctx, c.elseModules, record)
	}
	return []map[string]any{record}, nil
}

// runNestedModules executes a chain of nested filter modules in sequence.
// When onError is "skip" or "log", a module error does not stop the chain:
// subsequent modules receive the records from before the failed module.
func (c *ConditionModule) runNestedModules(ctx context.Context, modules []Module, record map[string]any) ([]map[string]any, error) {
	records := []map[string]any{record}
	for _, m := range modules {
		processed, err := m.Process(ctx, records)
		if err != nil {
			switch c.onError {
			case errhandling.OnErrorSkip:
				logger.Warn("skipping nested module error, continuing chain",
					slog.String("error", err.Error()),
				)
				// Keep records from before this module and continue
			case errhandling.OnErrorLog:
				logger.Error("nested module error, continuing chain",
					slog.String("error", err.Error()),
				)
				// Keep records from before this module and continue
			default:
				return nil, err
			}
		} else {
			records = processed
		}
		if len(records) == 0 {
			return records, nil
		}
	}
	return records, nil
}

// toBool converts a value to boolean.
// Empty collections (slices, maps) are considered falsy.
func toBool(value any) bool {
	if value == nil {
		return false
	}
	switch v := value.(type) {
	case bool:
		return v
	case int:
		return v != 0
	case int64:
		return v != 0
	case float64:
		return v != 0
	case string:
		return v != ""
	case []any:
		return len(v) > 0
	case []string:
		return len(v) > 0
	case []int:
		return len(v) > 0
	case map[string]any:
		return len(v) > 0
	case map[any]any:
		return len(v) > 0
	default:
		// For other types, use reflection to check if it's an empty collection
		// For now, return true for unknown types to maintain backward compatibility
		// but document that this may need refinement
		return true
	}
}
