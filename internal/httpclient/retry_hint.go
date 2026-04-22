package httpclient

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"

	"github.com/cannectors/runtime/internal/logger"
)

// MaxRetryHintExpressionLength caps the length of a retryHintFromBody
// expression to mitigate DoS attacks with pathological inputs.
const MaxRetryHintExpressionLength = 10000

// CompileRetryHint validates and compiles a retryHintFromBody expression.
//
// The expression must evaluate to a boolean once executed against a "body"
// variable (the parsed JSON response). When expression is empty, the helper
// returns (nil, nil) so that callers can use a nil program as a sentinel for
// "no hint configured". Expressions longer than MaxRetryHintExpressionLength
// are rejected upfront to prevent CPU exhaustion during compilation.
func CompileRetryHint(expression string) (*vm.Program, error) {
	if expression == "" {
		return nil, nil
	}
	if len(expression) > MaxRetryHintExpressionLength {
		return nil, fmt.Errorf("retryHintFromBody expression length %d exceeds maximum %d", len(expression), MaxRetryHintExpressionLength)
	}
	program, err := expr.Compile(expression, expr.AllowUndefinedVariables(), expr.AsBool())
	if err != nil {
		return nil, fmt.Errorf("compiling retryHintFromBody expression: %w", err)
	}
	return program, nil
}

// EvalRetryHint evaluates a compiled retryHintFromBody expression against a
// JSON response body.
//
// Returns (retry, hinted):
//   - hinted=false when the hint cannot be evaluated (nil program, empty
//     body, invalid JSON, evaluation error, or non-boolean result). In that
//     case the caller must fall back to the standard classification.
//   - hinted=true and retry=<bool> when the expression produced a boolean.
//
// Evaluation errors are logged at Warn level but never bubble up, matching
// the previous behavior embedded in output/http_request.go.
func EvalRetryHint(program *vm.Program, body []byte) (retry, hinted bool) {
	if program == nil || len(body) == 0 {
		return false, false
	}

	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		logger.Warn("failed to parse JSON body for retryHintFromBody evaluation, falling back to status code",
			slog.String("error", err.Error()),
		)
		return false, false
	}

	env := map[string]any{"body": data}
	result, err := expr.Run(program, env)
	if err != nil {
		logger.Warn("failed to evaluate retryHintFromBody expression, falling back to status code",
			slog.String("error", err.Error()),
		)
		return false, false
	}

	boolResult, ok := result.(bool)
	if !ok {
		logger.Warn("retryHintFromBody expression returned non-boolean value, falling back to status code",
			slog.String("result_type", fmt.Sprintf("%T", result)),
		)
		return false, false
	}
	return boolResult, true
}
