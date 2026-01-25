// Package filter provides implementations for filter modules.
// Condition module filters and routes data based on conditional expressions.
package filter

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"

	"github.com/canectors/runtime/internal/logger"
)

// Error codes for condition module
const (
	ErrCodeInvalidExpression = "INVALID_EXPRESSION"
	ErrCodeEvaluationFailed  = "EVALUATION_FAILED"
	ErrCodeUnsupportedLang   = "UNSUPPORTED_LANG"
	ErrCodeExpressionTooLong = "EXPRESSION_TOO_LONG"
	ErrCodeNestingTooDeep    = "NESTING_TOO_DEEP"
)

// Security limits for expression validation
const (
	// MaxExpressionLength is the maximum allowed length for condition expressions
	// to prevent DoS attacks through extremely long expressions.
	MaxExpressionLength = 10000
	// MaxNestingDepth is the maximum allowed nesting depth for nested modules
	// to prevent stack overflow from circular or deeply nested configurations.
	MaxNestingDepth = 50
)

// Common errors for condition module
var (
	// ErrEmptyExpression is returned when the expression is empty or whitespace-only
	ErrEmptyExpression = errors.New("expression cannot be empty")
	// ErrInvalidExpression is returned when the expression syntax is invalid
	ErrInvalidExpression = errors.New("invalid expression syntax")
	// ErrUnsupportedLang is returned when the language is not supported
	ErrUnsupportedLang = errors.New("unsupported expression language")
	// ErrExpressionTooLong is returned when the expression exceeds MaxExpressionLength
	ErrExpressionTooLong = errors.New("expression exceeds maximum length")
	// ErrNestingTooDeep is returned when nested module depth exceeds MaxNestingDepth
	ErrNestingTooDeep = errors.New("nested module depth exceeds maximum")
)

// NestedModuleCreator is a function type that creates a filter module from a NestedModuleConfig.
// This is set by the factory package to enable registry-based module creation in nested contexts.
// If nil, nested modules will fall back to hardcoded behavior for built-in types.
var NestedModuleCreator func(config *NestedModuleConfig, index int) (Module, error)

// Supported expression languages
const (
	LangSimple   = "simple"
	LangCEL      = "cel"
	LangJSONata  = "jsonata"
	LangJMESPath = "jmespath"
)

// Routing behavior constants
const (
	OnConditionContinue = "continue"
	OnConditionSkip     = "skip"
)

// ConditionConfig represents the configuration for a condition filter module.
type ConditionConfig struct {
	// Expression is the condition expression string (required)
	Expression string `json:"expression"`
	// Lang is the expression language: "simple" (default), "cel", "jsonata", "jmespath"
	Lang string `json:"lang,omitempty"`
	// OnTrue specifies behavior when condition is true: "continue" (default) or "skip"
	OnTrue string `json:"onTrue,omitempty"`
	// OnFalse specifies behavior when condition is false: "continue" or "skip" (default)
	OnFalse string `json:"onFalse,omitempty"`
	// OnError specifies error handling mode: "fail" (default), "skip", "log"
	OnError string `json:"onError,omitempty"`
	// Then contains a nested filter module configuration (optional)
	Then *NestedModuleConfig `json:"then,omitempty"`
	// Else contains a nested filter module configuration (optional)
	Else *NestedModuleConfig `json:"else,omitempty"`
}

// NestedModuleConfig represents a nested filter module configuration.
type NestedModuleConfig struct {
	Type     string                 `json:"type"`
	Config   map[string]interface{} `json:"config,omitempty"`
	Mappings []FieldMapping         `json:"mappings,omitempty"`
	// For nested conditions
	Expression string              `json:"expression,omitempty"`
	Lang       string              `json:"lang,omitempty"`
	OnTrue     string              `json:"onTrue,omitempty"`
	OnFalse    string              `json:"onFalse,omitempty"`
	OnError    string              `json:"onError,omitempty"`
	Then       *NestedModuleConfig `json:"then,omitempty"`
	Else       *NestedModuleConfig `json:"else,omitempty"`
}

// ConditionModule implements conditional filtering and routing.
// It evaluates expressions against input records and routes/filters accordingly.
type ConditionModule struct {
	expression string
	lang       string
	onTrue     string
	onFalse    string
	onError    string
	program    *vm.Program
	thenModule Module
	elseModule Module
}

// ConditionError carries structured context for condition evaluation failures.
type ConditionError struct {
	Code        string
	Message     string
	Expression  string
	RecordIndex int
	FieldPath   string
	Details     map[string]interface{}
}

func (e *ConditionError) Error() string {
	return e.Message
}

// newConditionError creates a ConditionError with optional debugging details.
// If underlyingErr is provided, it will be included in the Details field.
func newConditionError(code, message, expression string, recordIdx int, fieldPath string, underlyingErr error) *ConditionError {
	details := make(map[string]interface{})
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
// Security considerations:
//   - Expressions are compiled once during module creation using expr.Compile with
//     AllowUndefinedVariables(). This is efficient for pipelines processing many records.
//   - However, expressions from untrusted sources could potentially cause denial-of-service
//     through expensive operations. Consider adding limits on expression complexity
//     (e.g., maximum expression length, maximum nesting depth) if processing user-provided
//     expressions in production environments.
//   - The expr library does not execute arbitrary code, but complex expressions may still
//     consume significant CPU time during evaluation.
func NewConditionFromConfig(config ConditionConfig) (*ConditionModule, error) {
	return newConditionFromConfigWithDepth(config, 0)
}

// newConditionFromConfigWithDepth creates a condition module with depth tracking for nested modules.
func newConditionFromConfigWithDepth(config ConditionConfig, depth int) (*ConditionModule, error) {
	// Validate and normalize expression
	expression, err := validateExpression(config.Expression)
	if err != nil {
		return nil, fmt.Errorf("validating expression: %w", err)
	}

	// Normalize and validate configuration values
	lang, err := normalizeLang(config.Lang)
	if err != nil {
		return nil, fmt.Errorf("normalizing language: %w", err)
	}

	onTrue := normalizeOnCondition(config.OnTrue, OnConditionContinue)
	onFalse := normalizeOnCondition(config.OnFalse, OnConditionSkip)
	onError := normalizeOnError(config.OnError)

	// Compile expression
	program, err := compileExpression(expression)
	if err != nil {
		return nil, fmt.Errorf("compiling expression: %w", err)
	}

	// Create nested modules
	thenModule, elseModule, err := createNestedModules(config.Then, config.Else, depth+1)
	if err != nil {
		return nil, fmt.Errorf("creating nested modules: %w", err)
	}

	logModuleInitialization(config.Expression, lang, onTrue, onFalse, onError, thenModule, elseModule)

	return &ConditionModule{
		expression: expression,
		lang:       lang,
		onTrue:     onTrue,
		onFalse:    onFalse,
		onError:    onError,
		program:    program,
		thenModule: thenModule,
		elseModule: elseModule,
	}, nil
}

// validateExpression validates that the expression is not empty and within length limits.
func validateExpression(expression string) (string, error) {
	if len(expression) == 0 || isWhitespaceOnly(expression) {
		return "", fmt.Errorf("%w", ErrEmptyExpression)
	}
	if len(expression) > MaxExpressionLength {
		return "", fmt.Errorf("%w: expression length %d exceeds maximum %d", ErrExpressionTooLong, len(expression), MaxExpressionLength)
	}
	return expression, nil
}

// normalizeLang normalizes and validates the language setting.
func normalizeLang(lang string) (string, error) {
	if lang == "" {
		return LangSimple, nil
	}
	switch lang {
	case LangSimple:
		return lang, nil
	case LangCEL, LangJSONata, LangJMESPath:
		return "", fmt.Errorf("%w: %s (not yet implemented)", ErrUnsupportedLang, lang)
	default:
		return "", fmt.Errorf("%w: %s", ErrUnsupportedLang, lang)
	}
}

// normalizeOnCondition normalizes and validates onTrue/onFalse values.
func normalizeOnCondition(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	if value != OnConditionContinue && value != OnConditionSkip {
		logger.Warn("invalid onCondition value for condition module; defaulting",
			slog.String("value", value),
			slog.String("default", defaultValue),
		)
		return defaultValue
	}
	return value
}

// normalizeOnError normalizes and validates onError value.
func normalizeOnError(onError string) string {
	if onError == "" {
		return OnErrorFail
	}
	if onError != OnErrorFail && onError != OnErrorSkip && onError != OnErrorLog {
		logger.Warn("invalid onError value for condition module; defaulting to fail",
			slog.String("on_error", onError),
		)
		return OnErrorFail
	}
	return onError
}

// compileExpression compiles the expression using the expr library.
func compileExpression(expression string) (*vm.Program, error) {
	// AllowUndefinedVariables() handles missing fields gracefully by treating
	// undefined variables as nil rather than causing evaluation errors.
	program, err := expr.Compile(expression, expr.AllowUndefinedVariables())
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidExpression, err)
	}
	return program, nil
}

// createNestedModules creates then and else nested modules with depth tracking.
func createNestedModules(thenConfig, elseConfig *NestedModuleConfig, depth int) (Module, Module, error) {
	var thenModule, elseModule Module
	var err error

	if thenConfig != nil {
		thenModule, err = createNestedModuleWithDepth(thenConfig, depth)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create 'then' module: %w", err)
		}
	}

	if elseConfig != nil {
		elseModule, err = createNestedModuleWithDepth(elseConfig, depth)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create 'else' module: %w", err)
		}
	}

	return thenModule, elseModule, nil
}

// logModuleInitialization logs the initialization of a condition module.
func logModuleInitialization(expression, lang, onTrue, onFalse, onError string, thenModule, elseModule Module) {
	logger.Debug("condition module initialized",
		slog.String("expression", expression),
		slog.String("lang", lang),
		slog.String("on_true", onTrue),
		slog.String("on_false", onFalse),
		slog.String("on_error", onError),
		slog.Bool("has_then", thenModule != nil),
		slog.Bool("has_else", elseModule != nil),
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

// createNestedModuleWithDepth creates a filter module with depth tracking.
// Uses the registry to support custom filter types in nested configurations.
func createNestedModuleWithDepth(config *NestedModuleConfig, depth int) (Module, error) {
	if config == nil {
		return nil, nil
	}

	// Check depth limit to prevent stack overflow from circular or deeply nested configs
	if depth >= MaxNestingDepth {
		return nil, fmt.Errorf("%w: depth %d exceeds maximum %d", ErrNestingTooDeep, depth, MaxNestingDepth)
	}

	// Special handling for condition modules due to nested config structure
	if config.Type == "condition" {
		// Validate expression length for nested conditions too
		if config.Expression != "" {
			if len(config.Expression) > MaxExpressionLength {
				return nil, fmt.Errorf("%w: expression length %d exceeds maximum %d", ErrExpressionTooLong, len(config.Expression), MaxExpressionLength)
			}
		}
		// Create the condition module using the internal function with depth tracking
		return newConditionFromConfigWithDepth(ConditionConfig{
			Expression: config.Expression,
			Lang:       config.Lang,
			OnTrue:     config.OnTrue,
			OnFalse:    config.OnFalse,
			OnError:    config.OnError,
			Then:       config.Then,
			Else:       config.Else,
		}, depth+1)
	}

	// Use the factory's NestedModuleCreator if available (enables registry support)
	// Note: For nested modules, we use 0 as the index since nested modules don't have
	// a pipeline position. The index is primarily used for error logging context.
	if NestedModuleCreator != nil {
		return NestedModuleCreator(config, 0)
	}

	// Fallback to hardcoded behavior for built-in types (backward compatibility)
	switch config.Type {
	case "mapping":
		mappings, err := ParseFieldMappings(config.Mappings)
		if err != nil {
			return nil, fmt.Errorf("parsing field mappings: %w", err)
		}
		onError := config.OnError
		if onError == "" {
			onError = OnErrorFail
		}
		module, err := NewMappingFromConfig(mappings, onError)
		if err != nil {
			return nil, fmt.Errorf("creating mapping module: %w", err)
		}
		return module, nil
	default:
		// Unknown type - return stub
		return NewStub(config.Type, depth), nil
	}
}

// handleError processes an error according to the module's onError setting.
// Returns an error if the error should stop processing, nil if it should be skipped/logged.
func (c *ConditionModule) handleError(err error, recordIdx int, errorType string) error {
	switch c.onError {
	case OnErrorFail:
		return err
	case OnErrorSkip:
		logger.Warn(fmt.Sprintf("skipping record due to %s error", errorType),
			slog.Int("record_index", recordIdx),
			slog.String("expression", c.expression),
			slog.String("error", err.Error()),
		)
		return nil
	case OnErrorLog:
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
	case OnErrorFail:
		return err
	case OnErrorSkip:
		logger.Warn("skipping record due to nested module error",
			slog.Int("record_index", recordIdx),
			slog.String("error", err.Error()),
		)
		return nil
	case OnErrorLog:
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
func (c *ConditionModule) Process(ctx context.Context, records []map[string]interface{}) ([]map[string]interface{}, error) {
	if records == nil {
		return []map[string]interface{}{}, nil
	}

	startTime := time.Now()
	inputCount := len(records)

	logger.Debug("filter processing started",
		slog.String("module_type", "condition"),
		slog.String("expression", c.expression),
		slog.Int("input_records", inputCount),
		slog.String("on_error", c.onError),
		slog.Bool("has_then", c.thenModule != nil),
		slog.Bool("has_else", c.elseModule != nil),
	)

	result := make([]map[string]interface{}, 0, len(records))
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
func (c *ConditionModule) processConditionResult(ctx context.Context, conditionTrue bool, record map[string]interface{}) ([]map[string]interface{}, error) {
	if conditionTrue {
		// Condition is true
		if c.thenModule != nil {
			// Execute 'then' module
			return c.thenModule.Process(ctx, []map[string]interface{}{record})
		}
		// Apply onTrue behavior
		if c.onTrue == OnConditionContinue {
			return []map[string]interface{}{record}, nil
		}
		// onTrue == "skip" - filter out the record
		return []map[string]interface{}{}, nil
	}

	// Condition is false
	if c.elseModule != nil {
		// Execute 'else' module
		return c.elseModule.Process(ctx, []map[string]interface{}{record})
	}
	// Apply onFalse behavior
	if c.onFalse == OnConditionContinue {
		return []map[string]interface{}{record}, nil
	}
	// onFalse == "skip" (default) - filter out the record
	return []map[string]interface{}{}, nil
}

// toBool converts a value to boolean.
// Empty collections (slices, maps) are considered falsy.
func toBool(value interface{}) bool {
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
	case []interface{}:
		return len(v) > 0
	case []string:
		return len(v) > 0
	case []int:
		return len(v) > 0
	case map[string]interface{}:
		return len(v) > 0
	case map[interface{}]interface{}:
		return len(v) > 0
	default:
		// For other types, use reflection to check if it's an empty collection
		// For now, return true for unknown types to maintain backward compatibility
		// but document that this may need refinement
		return true
	}
}
