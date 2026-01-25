// Package errhandling provides retry configuration and mechanism for pipeline execution.
// This file defines retry configuration parsing, validation, and delay calculation.
package errhandling

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

// Default retry configuration values
const (
	DefaultMaxAttempts       = 3
	DefaultDelayMs           = 1000
	DefaultBackoffMultiplier = 2.0
	DefaultMaxDelayMs        = 30000
	DefaultTimeoutMs         = 30000
	MaxRetryAttempts         = 10
	MinBackoffMultiplier     = 1.0
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

// RetryConfig holds retry configuration for pipeline execution.
// This configuration determines how transient errors are handled with automatic retries.
type RetryConfig struct {
	// MaxAttempts is the maximum number of retry attempts (0 = no retry).
	// Default: 3, Max: 10
	MaxAttempts int

	// DelayMs is the initial delay between retries in milliseconds.
	// Default: 1000
	DelayMs int

	// BackoffMultiplier is the multiplier for exponential backoff.
	// Default: 2.0, Min: 1.0, Max: 10.0
	BackoffMultiplier float64

	// MaxDelayMs is the maximum delay between retries in milliseconds.
	// Default: 30000
	MaxDelayMs int

	// RetryableStatusCodes are HTTP status codes that trigger retry.
	// Default: [429, 500, 502, 503, 504]
	RetryableStatusCodes []int

	// UseRetryAfterHeader enables using the Retry-After response header to determine
	// the delay before retrying. When enabled and the server returns a valid Retry-After
	// header (seconds or HTTP-date format), that value is used instead of the backoff.
	// The delay is still capped by MaxDelayMs.
	// Default: false
	//
	// Example usage:
	//   config := RetryConfig{
	//     UseRetryAfterHeader: true,
	//     MaxDelayMs: 60000, // Cap at 60 seconds even if server says more
	//   }
	//   // Server returns: Retry-After: 120 → waits 60s (capped)
	//   // Server returns: Retry-After: 30 → waits 30s
	UseRetryAfterHeader bool

	// RetryHintFromBody is an expr expression (e.g., "body.retryable == true",
	// "body.error.code == \"TEMPORARY\"") to evaluate against the JSON response body.
	// The parsed JSON body is available as the "body" variable.
	// If the expression returns true, the error is retryable; if false, not retryable.
	// If the body is not valid JSON, falls back to status code only.
	// Default: "" (disabled)
	//
	// Example expressions:
	//   "body.retryable == true"                    // Check boolean field
	//   "body.error.code == \"TEMPORARY\""          // Check string field
	//   "body.error.code != \"PERMANENT\""          // Negation
	//   "body.error.code == \"RATE_LIMIT\" || body.error.code == \"TEMPORARY\""  // Complex condition
	//
	// Example usage:
	//   config := RetryConfig{
	//     RetryableStatusCodes: []int{500, 503},
	//     RetryHintFromBody: "body.error.code != \"PERMANENT\"",
	//   }
	//   // 500 with {"error": {"code": "TEMPORARY"}} → retried (expression true)
	//   // 500 with {"error": {"code": "PERMANENT"}} → NOT retried (expression false)
	//   // 500 with non-JSON body → retried (fallback to status code)
	RetryHintFromBody string
}

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
	return RetryConfig{
		MaxAttempts:          DefaultMaxAttempts,
		DelayMs:              DefaultDelayMs,
		BackoffMultiplier:    DefaultBackoffMultiplier,
		MaxDelayMs:           DefaultMaxDelayMs,
		RetryableStatusCodes: []int{429, 500, 502, 503, 504},
	}
}

// DefaultErrorHandlingConfig returns the default error handling configuration.
func DefaultErrorHandlingConfig() ErrorHandlingConfig {
	return ErrorHandlingConfig{
		OnError:   string(OnErrorFail),
		TimeoutMs: DefaultTimeoutMs,
		Retry:     DefaultRetryConfig(),
	}
}

// Validate validates the retry configuration.
// Returns an error if any value is out of valid range.
func (c RetryConfig) Validate() error {
	if c.MaxAttempts < 0 {
		return errors.New("maxAttempts must be >= 0")
	}
	if c.MaxAttempts > MaxRetryAttempts {
		return fmt.Errorf("maxAttempts must be <= %d", MaxRetryAttempts)
	}
	if c.DelayMs < 0 {
		return errors.New("delayMs must be >= 0")
	}
	if c.BackoffMultiplier < MinBackoffMultiplier {
		return fmt.Errorf("backoffMultiplier must be >= %v", MinBackoffMultiplier)
	}
	if c.MaxDelayMs < 0 {
		return errors.New("maxDelayMs must be >= 0")
	}
	return nil
}

// CalculateDelay calculates the retry delay for a given attempt using exponential backoff.
// The formula is: min(delayMs * (backoffMultiplier ^ attempt), maxDelayMs)
func (c RetryConfig) CalculateDelay(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}

	// Calculate delay with exponential backoff
	delayMs := float64(c.DelayMs) * math.Pow(c.BackoffMultiplier, float64(attempt))

	// Cap at max delay
	if delayMs > float64(c.MaxDelayMs) {
		delayMs = float64(c.MaxDelayMs)
	}

	return time.Duration(delayMs) * time.Millisecond
}

// ShouldRetry determines if a retry should be attempted based on the attempt number and error.
// Returns false if:
//   - Error is nil
//   - MaxAttempts is 0 (retries disabled)
//   - Current attempt >= MaxAttempts
//   - Error is not retryable (fatal errors like authentication, validation)
func (c RetryConfig) ShouldRetry(attempt int, err error) bool {
	// No error, no retry needed
	if err == nil {
		return false
	}

	// Retries disabled
	if c.MaxAttempts == 0 {
		return false
	}

	// Max attempts reached
	if attempt >= c.MaxAttempts {
		return false
	}

	// Check if error is retryable
	return IsRetryable(err)
}

// IsStatusCodeRetryable checks if the given status code is in the retryable list.
func (c RetryConfig) IsStatusCodeRetryable(statusCode int) bool {
	for _, code := range c.RetryableStatusCodes {
		if statusCode == code {
			return true
		}
	}
	return false
}

// isZeroRetryConfig checks if a RetryConfig should be considered unset.
// A RetryConfig is considered unset if MaxAttempts=0 AND DelayMs=0,
// regardless of other fields (BackoffMultiplier, MaxDelayMs, RetryableStatusCodes).
// This matches the test expectation that partial configs with MaxAttempts=0 and DelayMs=0
// should fall back to defaults.
func isZeroRetryConfig(c RetryConfig) bool {
	return c.MaxAttempts == 0 && c.DelayMs == 0
}

// ParseRetryConfig parses retry configuration from a map.
// Missing values are filled with defaults.
func ParseRetryConfig(m map[string]interface{}) RetryConfig {
	config := DefaultRetryConfig()

	if m == nil {
		return config
	}

	if maxAttempts, ok := getInt(m, "maxAttempts"); ok {
		config.MaxAttempts = maxAttempts
	}

	if delayMs, ok := getInt(m, "delayMs"); ok {
		config.DelayMs = delayMs
	}

	if backoffMultiplier, ok := getFloat(m, "backoffMultiplier"); ok {
		config.BackoffMultiplier = backoffMultiplier
	}

	if maxDelayMs, ok := getInt(m, "maxDelayMs"); ok {
		config.MaxDelayMs = maxDelayMs
	}

	if codes, ok := getIntSlice(m, "retryableStatusCodes"); ok {
		config.RetryableStatusCodes = codes
	}

	if useRetryAfter, ok := m["useRetryAfterHeader"].(bool); ok {
		config.UseRetryAfterHeader = useRetryAfter
	}

	if retryHintPath, ok := m["retryHintFromBody"].(string); ok {
		config.RetryHintFromBody = retryHintPath
	}

	return config
}

// ParseErrorHandlingConfig parses error handling configuration from a map.
// Missing values are filled with defaults.
func ParseErrorHandlingConfig(m map[string]interface{}) ErrorHandlingConfig {
	config := DefaultErrorHandlingConfig()

	if m == nil {
		return config
	}

	if onError, ok := m["onError"].(string); ok {
		config.OnError = onError
	}

	if timeoutMs, ok := getInt(m, "timeoutMs"); ok {
		config.TimeoutMs = timeoutMs
	}

	if retryMap, ok := m["retry"].(map[string]interface{}); ok {
		config.Retry = ParseRetryConfig(retryMap)
	}

	return config
}

// ResolveRetryConfig resolves the effective retry configuration.
// Precedence: module config > defaults config > default values.
func ResolveRetryConfig(moduleRetry, defaultsRetry *RetryConfig) RetryConfig {
	// Module config takes precedence
	if moduleRetry != nil {
		return *moduleRetry
	}

	// Defaults config is next
	if defaultsRetry != nil {
		return *defaultsRetry
	}

	// Return default values
	return DefaultRetryConfig()
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

	// Apply module config (overrides defaults)
	if moduleConfig != nil {
		if moduleConfig.OnError != "" {
			result.OnError = moduleConfig.OnError
		}
		if moduleConfig.TimeoutMs > 0 {
			result.TimeoutMs = moduleConfig.TimeoutMs
		}
		// Module retry completely overrides if set (non-zero RetryConfig)
		if !isZeroRetryConfig(moduleConfig.Retry) {
			result.Retry = moduleConfig.Retry
		}
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

// getInt extracts an int value from a map, handling float64 (JSON) and int types.
func getInt(m map[string]interface{}, key string) (int, bool) {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case float64:
			return int(val), true
		case int:
			return val, true
		case int64:
			return int(val), true
		}
	}
	return 0, false
}

// getFloat extracts a float64 value from a map.
func getFloat(m map[string]interface{}, key string) (float64, bool) {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case float64:
			return val, true
		case int:
			return float64(val), true
		case int64:
			return float64(val), true
		}
	}
	return 0, false
}

// getIntSlice extracts a slice of ints from a map.
func getIntSlice(m map[string]interface{}, key string) ([]int, bool) {
	v, ok := m[key]
	if !ok {
		return nil, false
	}

	slice, ok := v.([]interface{})
	if !ok {
		return nil, false
	}

	result := make([]int, 0, len(slice))
	for _, item := range slice {
		switch val := item.(type) {
		case float64:
			result = append(result, int(val))
		case int:
			result = append(result, val)
		case int64:
			result = append(result, int(val))
		}
	}

	return result, len(result) > 0
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
	startTime := time.Now()
	e.retryInfo = RetryInfo{
		Delays: make([]time.Duration, 0),
		Errors: make([]error, 0),
	}

	var lastErr error
	maxAttempts := e.config.MaxAttempts + 1

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
			if callback != nil {
				callback(attempt, nil, 0)
			}
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

		// Calculate delay (even if we won't use it, for callback)
		var delay time.Duration
		if attempt < e.config.MaxAttempts && classified.Retryable {
			delay = e.config.CalculateDelay(attempt)
			e.retryInfo.Delays = append(e.retryInfo.Delays, delay)
		}

		// Call callback
		if callback != nil {
			callback(attempt, err, delay)
		}

		// Don't retry fatal errors
		if !classified.Retryable {
			e.retryInfo.TotalDuration = time.Since(startTime)
			return nil, err
		}

		// Don't retry if we've exhausted attempts
		if attempt >= e.config.MaxAttempts {
			break
		}

		// Wait before retry
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
