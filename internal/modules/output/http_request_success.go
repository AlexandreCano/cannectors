package output

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"regexp"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"

	"github.com/cannectors/runtime/internal/logger"
	"github.com/cannectors/runtime/internal/moduleconfig"
)

// MaxSuccessExpressionLength caps the length of a success.expression to
// mitigate DoS attacks with pathological inputs.
const MaxSuccessExpressionLength = 10000

// normalizeRequestMode validates and normalizes the requestMode field
// (Story 24.12 AC15). Empty defaults to defaultRequestMode.
func normalizeRequestMode(mode string) (string, error) {
	switch mode {
	case "":
		return defaultRequestMode, nil
	case "batch", "single":
		return mode, nil
	default:
		return "", fmt.Errorf("httpRequest invalid requestMode %q: must be 'batch' or 'single'", mode)
	}
}

// validateRequestKeyEntries validates each key entry independently of
// presence (Story 24.12 AC16): keys are optional but, when present, must be
// well-formed.
func validateRequestKeyEntries(keys []moduleconfig.KeyConfig) error {
	for i, key := range keys {
		if key.Field == "" {
			return fmt.Errorf("httpRequest keys[%d] field is required", i)
		}
		if key.ParamType == "" {
			return fmt.Errorf("httpRequest keys[%d] paramType is required", i)
		}
		if key.ParamType != "query" && key.ParamType != "path" && key.ParamType != "header" {
			return fmt.Errorf("httpRequest keys[%d] paramType must be 'query', 'path', or 'header' (got %q)", i, key.ParamType)
		}
		if key.ParamName == "" {
			return fmt.Errorf("httpRequest keys[%d] paramName is required", i)
		}
	}
	return nil
}

// buildSuccessCondition validates and compiles the success condition
// (Story 24.12 AC6/AC7/AC12/AC13/AC14). Returns:
//   - successCodes: when nil, the runtime relies on the expression alone.
//   - successProgram: nil when no expression is configured.
//   - expression: raw expression string for diagnostics.
func buildSuccessCondition(cfg *SuccessConditionConfig) ([]int, *vm.Program, string, error) {
	if cfg == nil {
		return defaultSuccessCodes, nil, "", nil
	}

	var successCodes []int
	if cfg.StatusCodes != nil {
		if err := validateSuccessStatusCodes(cfg.StatusCodes); err != nil {
			return nil, nil, "", err
		}
		successCodes = make([]int, len(cfg.StatusCodes))
		copy(successCodes, cfg.StatusCodes)
	}

	var program *vm.Program
	expression := cfg.Expression
	if expression != "" {
		if len(expression) > MaxSuccessExpressionLength {
			return nil, nil, "", fmt.Errorf("httpRequest success.expression length %d exceeds maximum %d", len(expression), MaxSuccessExpressionLength)
		}
		var err error
		program, err = expr.Compile(expression, expr.AllowUndefinedVariables(), expr.AsBool())
		if err != nil {
			return nil, nil, "", fmt.Errorf("httpRequest success.expression invalid: %w", err)
		}
	}

	if expression == "" && cfg.StatusCodes == nil {
		return defaultSuccessCodes, nil, "", nil
	}

	return successCodes, program, expression, nil
}

// validateSuccessStatusCodes enforces minItems, uniqueness, and the 100..599
// range at runtime (Story 24.12 AC14).
func validateSuccessStatusCodes(codes []int) error {
	if len(codes) == 0 {
		return errors.New("httpRequest success.statusCodes must contain at least one code")
	}
	seen := make(map[int]struct{}, len(codes))
	for _, code := range codes {
		if code < 100 || code > 599 {
			return fmt.Errorf("httpRequest success.statusCodes contains out-of-range code %d (must be 100..599)", code)
		}
		if _, dup := seen[code]; dup {
			return fmt.Errorf("httpRequest success.statusCodes contains duplicate code %d", code)
		}
		seen[code] = struct{}{}
	}
	return nil
}

// errSuccessExpressionBodyNotJSON signals that an expression referencing
// `body` ran against a non-JSON response (Story 24.12 AC11).
var errSuccessExpressionBodyNotJSON = errors.New("httpRequest success.expression references body but response is not valid JSON")

// evaluateSuccessExpression runs the compiled expression against the response
// metadata. Returns (matched, err). err is non-nil only for cases that should
// surface to the caller (e.g. body referenced but invalid JSON, AC11).
func (h *HTTPRequestModule) evaluateSuccessExpression(statusCode int, headers map[string][]string, body []byte) (bool, error) {
	if h.successProgram == nil {
		return true, nil
	}
	env := map[string]any{
		"statusCode": statusCode,
		"headers":    headers,
	}
	if len(body) > 0 {
		var parsedBody any
		if err := json.Unmarshal(body, &parsedBody); err != nil {
			if expressionReferencesBody(h.successExpression) {
				logger.Warn("success.expression references body but response is not valid JSON",
					slog.String("expression", h.successExpression),
					slog.String("error", err.Error()),
					slog.String("body_preview", truncateString(string(body), 100)),
				)
				return false, errSuccessExpressionBodyNotJSON
			}
			env["body"] = nil
		} else {
			env["body"] = parsedBody
		}
	} else {
		env["body"] = nil
	}
	result, err := expr.Run(h.successProgram, env)
	if err != nil {
		logger.Warn("success.expression evaluation error",
			slog.String("expression", h.successExpression),
			slog.String("error", err.Error()),
		)
		return false, fmt.Errorf("httpRequest success.expression evaluation failed: %w", err)
	}
	matched, ok := result.(bool)
	if !ok {
		return false, fmt.Errorf("httpRequest success.expression must return a boolean (got %T)", result)
	}
	return matched, nil
}

// bodyIdentifierRegexp matches a `body` token used as an identifier in an
// expression (i.e. not part of a longer word). Used as a heuristic to decide
// whether to surface a "body is not JSON" error (Story 24.12 AC11).
var bodyIdentifierRegexp = regexp.MustCompile(`\bbody\b`)

func expressionReferencesBody(expression string) bool {
	return bodyIdentifierRegexp.MatchString(expression)
}
