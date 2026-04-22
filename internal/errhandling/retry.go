// Package errhandling provides retry configuration and mechanism for pipeline execution.
// This file defines retry configuration parsing, validation, and delay calculation.
package errhandling

import (
	"context"
	"strings"
	"time"

	"github.com/cannectors/runtime/pkg/connector"
)

// Default retry configuration values
const (
	DefaultMaxAttempts       = 3
	DefaultDelayMs           = 1000
	DefaultBackoffMultiplier = 2.0
	DefaultMaxDelayMs        = 30000
	DefaultTimeoutMs         = 30000
)

// OnErrorStrategy defines what action to take when an error occurs.
type OnErrorStrategy string

// Error handling strategies
const (
	// OnErrorFail stops execution and returns error (default).
	OnErrorFail OnErrorStrategy = "fail"

	// OnErrorSkip skips the failed record/module and continues execution.
	OnErrorSkip OnErrorStrategy = "skip"

	// OnErrorLog logs the error and continues execution.
	OnErrorLog OnErrorStrategy = "log"
)

// RetryConfig is an alias for connector.RetryConfig.
// Methods (Validate, CalculateDelay, IsStatusCodeRetryable) are defined on connector.RetryConfig.
type RetryConfig = connector.RetryConfig

// ErrorHandlingConfig holds error handling configuration for a module.
type ErrorHandlingConfig struct {
	// OnError specifies the action to take on error ("fail", "skip", "log").
	OnError string

	// TimeoutMs is the timeout for operations in milliseconds.
	TimeoutMs int

	// Retry is the retry configuration.
	Retry RetryConfig
}

// DefaultRetryConfig returns the default retry configuration.
func DefaultRetryConfig() RetryConfig {
	return connector.DefaultRetryConfig()
}

// DefaultErrorHandlingConfig returns the default error handling configuration.
func DefaultErrorHandlingConfig() ErrorHandlingConfig {
	return ErrorHandlingConfig{
		OnError:   string(OnErrorFail),
		TimeoutMs: DefaultTimeoutMs,
		Retry:     DefaultRetryConfig(),
	}
}

// ShouldRetry determines if a retry should be attempted based on the config, attempt number and error.
func ShouldRetry(c RetryConfig, attempt int, err error) bool {
	if err == nil {
		return false
	}
	if c.MaxAttempts == 0 {
		return false
	}
	if attempt >= c.MaxAttempts {
		return false
	}
	return IsRetryable(err)
}

// ResolveRetryConfig resolves the effective retry configuration with granular merge.
// Precedence: module config fields > defaults config fields > default values.
func ResolveRetryConfig(moduleRetry, defaultsRetry *RetryConfig) RetryConfig {
	base := DefaultRetryConfig()

	// Apply defaults first
	if defaultsRetry != nil {
		if defaultsRetry.MaxAttempts != 0 {
			base.MaxAttempts = defaultsRetry.MaxAttempts
		}
		if defaultsRetry.DelayMs != 0 {
			base.DelayMs = defaultsRetry.DelayMs
		}
		if defaultsRetry.BackoffMultiplier != 0 {
			base.BackoffMultiplier = defaultsRetry.BackoffMultiplier
		}
		if defaultsRetry.MaxDelayMs != 0 {
			base.MaxDelayMs = defaultsRetry.MaxDelayMs
		}
		if len(defaultsRetry.RetryableStatusCodes) > 0 {
			base.RetryableStatusCodes = defaultsRetry.RetryableStatusCodes
		}
		base.UseRetryAfterHeader = defaultsRetry.UseRetryAfterHeader
		if defaultsRetry.RetryHintFromBody != "" {
			base.RetryHintFromBody = defaultsRetry.RetryHintFromBody
		}
	}

	// Apply module config (overrides defaults per field)
	if moduleRetry != nil {
		if moduleRetry.MaxAttempts != 0 {
			base.MaxAttempts = moduleRetry.MaxAttempts
		}
		if moduleRetry.DelayMs != 0 {
			base.DelayMs = moduleRetry.DelayMs
		}
		if moduleRetry.BackoffMultiplier != 0 {
			base.BackoffMultiplier = moduleRetry.BackoffMultiplier
		}
		if moduleRetry.MaxDelayMs != 0 {
			base.MaxDelayMs = moduleRetry.MaxDelayMs
		}
		if len(moduleRetry.RetryableStatusCodes) > 0 {
			base.RetryableStatusCodes = moduleRetry.RetryableStatusCodes
		}
		if moduleRetry.UseRetryAfterHeader {
			base.UseRetryAfterHeader = true
		}
		if moduleRetry.RetryHintFromBody != "" {
			base.RetryHintFromBody = moduleRetry.RetryHintFromBody
		}
	}

	return base
}

// ResolveErrorHandlingConfig resolves the effective error handling configuration.
// Precedence: module config > defaults config > default values.
func ResolveErrorHandlingConfig(moduleConfig, defaultsConfig *ErrorHandlingConfig) ErrorHandlingConfig {
	result := DefaultErrorHandlingConfig()

	// Apply defaults first
	if defaultsConfig != nil {
		if defaultsConfig.OnError != "" {
			result.OnError = defaultsConfig.OnError
		}
		if defaultsConfig.TimeoutMs > 0 {
			result.TimeoutMs = defaultsConfig.TimeoutMs
		}
		result.Retry = defaultsConfig.Retry
	}

	// Apply module config (overrides defaults per field)
	if moduleConfig != nil {
		if moduleConfig.OnError != "" {
			result.OnError = moduleConfig.OnError
		}
		if moduleConfig.TimeoutMs > 0 {
			result.TimeoutMs = moduleConfig.TimeoutMs
		}
		// Granular merge for retry
		result.Retry = ResolveRetryConfig(&moduleConfig.Retry, &result.Retry)
	}

	return result
}

// ParseOnErrorStrategy parses an error strategy string.
// Returns OnErrorFail for invalid or empty input.
func ParseOnErrorStrategy(s string) OnErrorStrategy {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "fail", "":
		return OnErrorFail
	case "skip":
		return OnErrorSkip
	case "log":
		return OnErrorLog
	default:
		return OnErrorFail
	}
}

// ============================
// Retry Executor
// ============================

// RetryFunc is a function that can be retried.
// It takes a context and returns a result and an error.
type RetryFunc func(ctx context.Context) (interface{}, error)

// RetryInfo contains information about retry attempts.
type RetryInfo struct {
	// TotalAttempts is the total number of attempts made.
	TotalAttempts int

	// SuccessfulAttempt is the attempt number that succeeded (0 if failed).
	SuccessfulAttempt int

	// RetryCount is the number of retries (TotalAttempts - 1).
	RetryCount int

	// TotalDuration is the total time spent including retries.
	TotalDuration time.Duration

	// Delays is the list of delays between retries.
	Delays []time.Duration

	// Errors is the list of errors encountered during retries.
	Errors []error
}

// RetryExecutor executes functions with retry logic.
type RetryExecutor struct {
	config    RetryConfig
	retryInfo RetryInfo
}

// NewRetryExecutor creates a new retry executor with the given configuration.
func NewRetryExecutor(config RetryConfig) *RetryExecutor {
	return &RetryExecutor{
		config: config,
		retryInfo: RetryInfo{
			Delays: make([]time.Duration, 0),
			Errors: make([]error, 0),
		},
	}
}

// Execute runs the given function with retry logic.
// It retries on transient errors up to MaxAttempts times.
// Returns the result and any error encountered.
func (e *RetryExecutor) Execute(ctx context.Context, fn RetryFunc) (interface{}, error) {
	startTime := time.Now()
	e.retryInfo = RetryInfo{
		Delays: make([]time.Duration, 0),
		Errors: make([]error, 0),
	}

	var lastErr error
	maxAttempts := e.config.MaxAttempts + 1 // Initial attempt + retries

	for attempt := 0; attempt < maxAttempts; attempt++ {
		e.retryInfo.TotalAttempts = attempt + 1

		// Check context before attempt
		select {
		case <-ctx.Done():
			e.retryInfo.TotalDuration = time.Since(startTime)
			return nil, ClassifyNetworkError(ctx.Err())
		default:
		}

		// Execute function
		result, err := fn(ctx)

		// Success
		if err == nil {
			e.retryInfo.SuccessfulAttempt = attempt + 1
			e.retryInfo.RetryCount = attempt
			e.retryInfo.TotalDuration = time.Since(startTime)
			return result, nil
		}

		// Record error
		lastErr = err
		e.retryInfo.Errors = append(e.retryInfo.Errors, err)

		// Classify error
		classified := ClassifyError(err)

		// Don't retry fatal errors
		if !classified.Retryable {
			e.retryInfo.TotalDuration = time.Since(startTime)
			return nil, err
		}

		// Don't retry if we've exhausted attempts
		if attempt >= e.config.MaxAttempts {
			break
		}

		// Calculate delay for next attempt
		delay := e.config.CalculateDelay(attempt)
		e.retryInfo.Delays = append(e.retryInfo.Delays, delay)

		// Wait before retry (respect context cancellation)
		select {
		case <-ctx.Done():
			e.retryInfo.TotalDuration = time.Since(startTime)
			return nil, ClassifyNetworkError(ctx.Err())
		case <-time.After(delay):
			// Continue to next attempt
		}
	}

	e.retryInfo.RetryCount = e.retryInfo.TotalAttempts - 1
	e.retryInfo.TotalDuration = time.Since(startTime)
	return nil, lastErr
}

// GetRetryInfo returns information about the retry attempts.
func (e *RetryExecutor) GetRetryInfo() RetryInfo {
	return e.retryInfo
}

// ExecuteWithCallback executes the function with retry and calls the callback after each attempt.
// The callback receives the attempt number (0-indexed), the error (nil on success), and the delay before next retry.
func (e *RetryExecutor) ExecuteWithCallback(
	ctx context.Context,
	fn RetryFunc,
	callback func(attempt int, err error, nextDelay time.Duration),
) (interface{}, error) {
	return e.ExecuteWithHooks(ctx, fn, Hooks{}, callback)
}

// Hooks carry optional overrides that influence the RetryExecutor decisions
// without binding errhandling to HTTP-specific types.
type Hooks struct {
	// ExtractRetryAfter, when non-nil, is consulted on every failed attempt.
	// If it returns (d, true), d replaces the exponential-backoff delay for
	// the next retry (capped by config.MaxDelayMs and clamped to 0 when
	// negative). If it returns (_, false), the delay falls back to the
	// computed backoff.
	ExtractRetryAfter func(err error) (time.Duration, bool)
}

// ExecuteWithHooks behaves like ExecuteWithCallback but also honors the
// provided Hooks. It is the primary entrypoint for callers that need to
// override delay computation (e.g. Retry-After header).
func (e *RetryExecutor) ExecuteWithHooks(
	ctx context.Context,
	fn RetryFunc,
	hooks Hooks,
	callback func(attempt int, err error, nextDelay time.Duration),
) (interface{}, error) {
	startTime := time.Now()
	e.retryInfo = RetryInfo{
		Delays: make([]time.Duration, 0),
		Errors: make([]error, 0),
	}

	var lastErr error
	maxAttempts := e.config.MaxAttempts + 1

	for attempt := 0; attempt < maxAttempts; attempt++ {
		e.retryInfo.TotalAttempts = attempt + 1

		select {
		case <-ctx.Done():
			e.retryInfo.TotalDuration = time.Since(startTime)
			return nil, ClassifyNetworkError(ctx.Err())
		default:
		}

		result, err := fn(ctx)

		if err == nil {
			if callback != nil {
				callback(attempt, nil, 0)
			}
			e.retryInfo.SuccessfulAttempt = attempt + 1
			e.retryInfo.RetryCount = attempt
			e.retryInfo.TotalDuration = time.Since(startTime)
			return result, nil
		}

		lastErr = err
		e.retryInfo.Errors = append(e.retryInfo.Errors, err)

		classified := ClassifyError(err)

		var delay time.Duration
		if attempt < e.config.MaxAttempts && classified.Retryable {
			delay = e.config.CalculateDelay(attempt)
			if hooks.ExtractRetryAfter != nil {
				if override, ok := hooks.ExtractRetryAfter(err); ok {
					delay = clampDelay(override, e.config.MaxDelayMs)
				}
			}
			e.retryInfo.Delays = append(e.retryInfo.Delays, delay)
		}

		if callback != nil {
			callback(attempt, err, delay)
		}

		if !classified.Retryable {
			e.retryInfo.TotalDuration = time.Since(startTime)
			return nil, err
		}

		if attempt >= e.config.MaxAttempts {
			break
		}

		select {
		case <-ctx.Done():
			e.retryInfo.TotalDuration = time.Since(startTime)
			return nil, ClassifyNetworkError(ctx.Err())
		case <-time.After(delay):
		}
	}

	e.retryInfo.RetryCount = e.retryInfo.TotalAttempts - 1
	e.retryInfo.TotalDuration = time.Since(startTime)
	return nil, lastErr
}

// clampDelay constrains a Retry-After-derived delay to [0, maxDelayMs].
func clampDelay(d time.Duration, maxDelayMs int) time.Duration {
	if d < 0 {
		return 0
	}
	if maxDelayMs <= 0 {
		return d
	}
	maxDelay := time.Duration(maxDelayMs) * time.Millisecond
	if d > maxDelay {
		return maxDelay
	}
	return d
}
