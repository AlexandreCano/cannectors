// Package filter provides implementations for filter modules.
// Condition module filters and routes data based on conditional expressions.
package filter

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"

	"github.com/canectors/runtime/internal/logger"
)

// Error codes for condition module
const (
	ErrCodeInvalidExpression = "INVALID_EXPRESSION"
	ErrCodeEvaluationFailed  = "EVALUATION_FAILED"
	ErrCodeUnsupportedLang   = "UNSUPPORTED_LANG"
)

// Common errors for condition module
var (
	// ErrEmptyExpression is reserved for strict validation of empty expressions.
	// Empty expressions currently default to pass-through behavior.
	ErrEmptyExpression = errors.New("expression cannot be empty")
	// ErrInvalidExpression is returned when the expression syntax is invalid
	ErrInvalidExpression = errors.New("invalid expression syntax")
	// ErrUnsupportedLang is returned when the language is not supported
	ErrUnsupportedLang = errors.New("unsupported expression language")
)

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

func newConditionError(code, message, expression string, recordIdx int, fieldPath string) *ConditionError {
	return &ConditionError{
		Code:        code,
		Message:     message,
		Expression:  expression,
		RecordIndex: recordIdx,
		FieldPath:   fieldPath,
	}
}

// NewConditionFromConfig creates a new condition filter module from configuration.
// It validates the configuration and returns an error if invalid.
func NewConditionFromConfig(config ConditionConfig) (*ConditionModule, error) {
	// Validate expression is provided
	expression := config.Expression
	hasExpression := len(expression) > 0 && !isWhitespaceOnly(expression)

	// Set defaults
	lang := config.Lang
	if lang == "" {
		lang = LangSimple
	}

	onTrue := config.OnTrue
	if onTrue == "" {
		onTrue = OnConditionContinue
	}

	onFalse := config.OnFalse
	if onFalse == "" {
		onFalse = OnConditionSkip
	}

	onError := config.OnError
	if onError == "" {
		onError = OnErrorFail
	}
	if onError != OnErrorFail && onError != OnErrorSkip && onError != OnErrorLog {
		logger.Warn("invalid onError value for condition module; defaulting to fail",
			slog.String("on_error", onError),
		)
		onError = OnErrorFail
	}

	// Check for unsupported languages
	switch lang {
	case LangSimple:
		// Supported - continue
	case LangCEL, LangJSONata, LangJMESPath:
		// Future: integrate external libraries
		return nil, fmt.Errorf("%w: %s (not yet implemented)", ErrUnsupportedLang, lang)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedLang, lang)
	}

	// Compile the expression using expr library
	// AllowUndefinedVariables() handles missing fields gracefully
	var (
		program *vm.Program
		err     error
	)
	if hasExpression {
		program, err = expr.Compile(expression, expr.AllowUndefinedVariables())
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidExpression, err)
		}
	}

	// Create nested modules if configured
	var thenModule, elseModule Module
	if config.Then != nil {
		thenModule, err = createNestedModule(config.Then)
		if err != nil {
			return nil, fmt.Errorf("failed to create 'then' module: %w", err)
		}
	}
	if config.Else != nil {
		elseModule, err = createNestedModule(config.Else)
		if err != nil {
			return nil, fmt.Errorf("failed to create 'else' module: %w", err)
		}
	}

	logger.Debug("condition module initialized",
		slog.String("expression", config.Expression),
		slog.String("lang", lang),
		slog.String("on_true", onTrue),
		slog.String("on_false", onFalse),
		slog.String("on_error", onError),
		slog.Bool("has_then", thenModule != nil),
		slog.Bool("has_else", elseModule != nil),
	)

	return &ConditionModule{
		expression: config.Expression,
		lang:       lang,
		onTrue:     onTrue,
		onFalse:    onFalse,
		onError:    onError,
		program:    program,
		thenModule: thenModule,
		elseModule: elseModule,
	}, nil
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

// createNestedModule creates a filter module from nested configuration.
func createNestedModule(config *NestedModuleConfig) (Module, error) {
	if config == nil {
		return nil, nil
	}

	switch config.Type {
	case "mapping":
		mappings, err := ParseFieldMappings(config.Mappings)
		if err != nil {
			return nil, err
		}
		onError := config.OnError
		if onError == "" {
			onError = OnErrorFail
		}
		return NewMappingFromConfig(mappings, onError)

	case "condition":
		return NewConditionFromConfig(ConditionConfig{
			Expression: config.Expression,
			Lang:       config.Lang,
			OnTrue:     config.OnTrue,
			OnFalse:    config.OnFalse,
			OnError:    config.OnError,
			Then:       config.Then,
			Else:       config.Else,
		})

	default:
		return nil, fmt.Errorf("unsupported nested module type: %s", config.Type)
	}
}

// Process filters records based on the condition expression.
// For each record:
//  1. Evaluates the condition expression using expr library
//  2. If true and 'then' module exists: execute 'then' module
//  3. If true and no 'then' module: apply onTrue behavior
//  4. If false and 'else' module exists: execute 'else' module
//  5. If false and no 'else' module: apply onFalse behavior
//
// Returns the filtered/routed records and any error that occurred.
func (c *ConditionModule) Process(records []map[string]interface{}) ([]map[string]interface{}, error) {
	if records == nil {
		return []map[string]interface{}{}, nil
	}

	result := make([]map[string]interface{}, 0, len(records))

	for recordIdx, record := range records {
		if c.program == nil {
			// Empty condition defaults to "true" to preserve records unless onTrue says otherwise.
			processedRecords, err := c.processConditionResult(true, record)
			if err != nil {
				switch c.onError {
				case OnErrorFail:
					return nil, err
				case OnErrorSkip:
					logger.Warn("skipping record due to nested module error",
						slog.Int("record_index", recordIdx),
						slog.String("error", err.Error()),
					)
					continue
				case OnErrorLog:
					logger.Error("nested module error (continuing)",
						slog.Int("record_index", recordIdx),
						slog.String("error", err.Error()),
					)
					continue
				default:
					return nil, err
				}
			}

			result = append(result, processedRecords...)
			continue
		}

		// Evaluate the condition using expr library
		output, err := expr.Run(c.program, record)
		if err != nil {
			// Handle evaluation error
			condErr := newConditionError(
				ErrCodeEvaluationFailed,
				fmt.Sprintf("condition evaluation failed at record %d: %v", recordIdx, err),
				c.expression,
				recordIdx,
				"",
			)

			switch c.onError {
			case OnErrorFail:
				return nil, condErr
			case OnErrorSkip:
				logger.Warn("skipping record due to condition evaluation error",
					slog.Int("record_index", recordIdx),
					slog.String("expression", c.expression),
					slog.String("error", err.Error()),
				)
				continue
			case OnErrorLog:
				logger.Error("condition evaluation error (continuing)",
					slog.Int("record_index", recordIdx),
					slog.String("expression", c.expression),
					slog.String("error", err.Error()),
				)
				// Continue with next record
				continue
			default:
				return nil, condErr
			}
		}

		// Convert result to boolean
		conditionResult, ok := output.(bool)
		if !ok {
			// If not a boolean, treat as truthy check
			conditionResult = toBool(output)
		}

		// Process based on condition result
		processedRecords, err := c.processConditionResult(conditionResult, record)
		if err != nil {
			switch c.onError {
			case OnErrorFail:
				return nil, err
			case OnErrorSkip:
				logger.Warn("skipping record due to nested module error",
					slog.Int("record_index", recordIdx),
					slog.String("error", err.Error()),
				)
				continue
			case OnErrorLog:
				logger.Error("nested module error (continuing)",
					slog.Int("record_index", recordIdx),
					slog.String("error", err.Error()),
				)
				continue
			default:
				return nil, err
			}
		}

		result = append(result, processedRecords...)
	}

	return result, nil
}

// processConditionResult handles the result of condition evaluation for a single record.
func (c *ConditionModule) processConditionResult(conditionTrue bool, record map[string]interface{}) ([]map[string]interface{}, error) {
	if conditionTrue {
		// Condition is true
		if c.thenModule != nil {
			// Execute 'then' module
			return c.thenModule.Process([]map[string]interface{}{record})
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
		return c.elseModule.Process([]map[string]interface{}{record})
	}
	// Apply onFalse behavior
	if c.onFalse == OnConditionContinue {
		return []map[string]interface{}{record}, nil
	}
	// onFalse == "skip" (default) - filter out the record
	return []map[string]interface{}{}, nil
}

// toBool converts a value to boolean.
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
	default:
		return true
	}
}
