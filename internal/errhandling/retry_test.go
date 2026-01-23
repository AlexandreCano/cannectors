// Package runtime provides retry configuration and mechanism for pipeline execution.
package errhandling

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// TestRetryConfig_Defaults tests default retry configuration values.
func TestRetryConfig_Defaults(t *testing.T) {
	config := DefaultRetryConfig()

	if config.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", config.MaxAttempts)
	}
	if config.DelayMs != 1000 {
		t.Errorf("DelayMs = %d, want 1000", config.DelayMs)
	}
	if config.BackoffMultiplier != 2.0 {
		t.Errorf("BackoffMultiplier = %f, want 2.0", config.BackoffMultiplier)
	}
	if config.MaxDelayMs != 30000 {
		t.Errorf("MaxDelayMs = %d, want 30000", config.MaxDelayMs)
	}
	if len(config.RetryableStatusCodes) != 5 {
		t.Errorf("RetryableStatusCodes length = %d, want 5", len(config.RetryableStatusCodes))
	}
}

// TestRetryConfig_Validate tests retry configuration validation.
func TestRetryConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  RetryConfig
		wantErr bool
	}{
		{
			name:    "Valid default config",
			config:  DefaultRetryConfig(),
			wantErr: false,
		},
		{
			name: "Valid custom config",
			config: RetryConfig{
				MaxAttempts:          5,
				DelayMs:              500,
				BackoffMultiplier:    1.5,
				MaxDelayMs:           60000,
				RetryableStatusCodes: []int{429, 500},
			},
			wantErr: false,
		},
		{
			name: "Zero max attempts is valid",
			config: RetryConfig{
				MaxAttempts:          0,
				DelayMs:              1000,
				BackoffMultiplier:    2.0,
				MaxDelayMs:           30000,
				RetryableStatusCodes: []int{429, 500},
			},
			wantErr: false,
		},
		{
			name: "Negative max attempts is invalid",
			config: RetryConfig{
				MaxAttempts:       -1,
				DelayMs:           1000,
				BackoffMultiplier: 2.0,
				MaxDelayMs:        30000,
			},
			wantErr: true,
		},
		{
			name: "Max attempts > 10 is invalid",
			config: RetryConfig{
				MaxAttempts:       11,
				DelayMs:           1000,
				BackoffMultiplier: 2.0,
				MaxDelayMs:        30000,
			},
			wantErr: true,
		},
		{
			name: "Negative delay is invalid",
			config: RetryConfig{
				MaxAttempts:       3,
				DelayMs:           -1,
				BackoffMultiplier: 2.0,
				MaxDelayMs:        30000,
			},
			wantErr: true,
		},
		{
			name: "Backoff multiplier < 1 is invalid",
			config: RetryConfig{
				MaxAttempts:       3,
				DelayMs:           1000,
				BackoffMultiplier: 0.5,
				MaxDelayMs:        30000,
			},
			wantErr: true,
		},
		{
			name: "Backoff multiplier = 1 is valid",
			config: RetryConfig{
				MaxAttempts:       3,
				DelayMs:           1000,
				BackoffMultiplier: 1.0,
				MaxDelayMs:        30000,
			},
			wantErr: false,
		},
		{
			name: "Negative max delay is invalid",
			config: RetryConfig{
				MaxAttempts:       3,
				DelayMs:           1000,
				BackoffMultiplier: 2.0,
				MaxDelayMs:        -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestRetryConfig_ParseFromMap tests parsing retry config from map.
func TestRetryConfig_ParseFromMap(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected RetryConfig
	}{
		{
			name:     "Nil map returns defaults",
			input:    nil,
			expected: DefaultRetryConfig(),
		},
		{
			name:     "Empty map returns defaults",
			input:    map[string]interface{}{},
			expected: DefaultRetryConfig(),
		},
		{
			name: "Full custom config",
			input: map[string]interface{}{
				"maxAttempts":          float64(5),
				"delayMs":              float64(500),
				"backoffMultiplier":    float64(1.5),
				"maxDelayMs":           float64(60000),
				"retryableStatusCodes": []interface{}{float64(429), float64(503)},
			},
			expected: RetryConfig{
				MaxAttempts:          5,
				DelayMs:              500,
				BackoffMultiplier:    1.5,
				MaxDelayMs:           60000,
				RetryableStatusCodes: []int{429, 503},
			},
		},
		{
			name: "Partial config uses defaults for missing",
			input: map[string]interface{}{
				"maxAttempts": float64(5),
			},
			expected: RetryConfig{
				MaxAttempts:          5,
				DelayMs:              1000,
				BackoffMultiplier:    2.0,
				MaxDelayMs:           30000,
				RetryableStatusCodes: []int{429, 500, 502, 503, 504},
			},
		},
		{
			name: "Integer types work too",
			input: map[string]interface{}{
				"maxAttempts": 5, // int instead of float64
				"delayMs":     500,
			},
			expected: RetryConfig{
				MaxAttempts:          5,
				DelayMs:              500,
				BackoffMultiplier:    2.0,
				MaxDelayMs:           30000,
				RetryableStatusCodes: []int{429, 500, 502, 503, 504},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseRetryConfig(tt.input)

			if result.MaxAttempts != tt.expected.MaxAttempts {
				t.Errorf("MaxAttempts = %d, want %d", result.MaxAttempts, tt.expected.MaxAttempts)
			}
			if result.DelayMs != tt.expected.DelayMs {
				t.Errorf("DelayMs = %d, want %d", result.DelayMs, tt.expected.DelayMs)
			}
			if result.BackoffMultiplier != tt.expected.BackoffMultiplier {
				t.Errorf("BackoffMultiplier = %f, want %f", result.BackoffMultiplier, tt.expected.BackoffMultiplier)
			}
			if result.MaxDelayMs != tt.expected.MaxDelayMs {
				t.Errorf("MaxDelayMs = %d, want %d", result.MaxDelayMs, tt.expected.MaxDelayMs)
			}
			if len(result.RetryableStatusCodes) != len(tt.expected.RetryableStatusCodes) {
				t.Errorf("RetryableStatusCodes length = %d, want %d", len(result.RetryableStatusCodes), len(tt.expected.RetryableStatusCodes))
			}
		})
	}
}

// TestRetryConfig_CalculateDelay tests exponential backoff calculation.
func TestRetryConfig_CalculateDelay(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:       5,
		DelayMs:           1000,
		BackoffMultiplier: 2.0,
		MaxDelayMs:        30000,
	}

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 1000 * time.Millisecond},   // First attempt: delayMs
		{1, 2000 * time.Millisecond},   // Second: 1000 * 2
		{2, 4000 * time.Millisecond},   // Third: 2000 * 2
		{3, 8000 * time.Millisecond},   // Fourth: 4000 * 2
		{4, 16000 * time.Millisecond},  // Fifth: 8000 * 2
		{5, 30000 * time.Millisecond},  // Sixth: capped at maxDelayMs
		{10, 30000 * time.Millisecond}, // Always capped
	}

	for _, tt := range tests {
		t.Run("attempt_"+string(rune('0'+tt.attempt)), func(t *testing.T) {
			result := config.CalculateDelay(tt.attempt)
			if result != tt.expected {
				t.Errorf("CalculateDelay(%d) = %v, want %v", tt.attempt, result, tt.expected)
			}
		})
	}
}

// TestRetryConfig_CalculateDelay_NoBackoff tests delay without backoff.
func TestRetryConfig_CalculateDelay_NoBackoff(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:       5,
		DelayMs:           1000,
		BackoffMultiplier: 1.0, // No backoff
		MaxDelayMs:        30000,
	}

	// All attempts should have same delay
	for attempt := 0; attempt < 5; attempt++ {
		result := config.CalculateDelay(attempt)
		expected := 1000 * time.Millisecond
		if result != expected {
			t.Errorf("CalculateDelay(%d) = %v, want %v", attempt, result, expected)
		}
	}
}

// TestRetryConfig_ShouldRetry tests the ShouldRetry method.
func TestRetryConfig_ShouldRetry(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:          3,
		DelayMs:              1000,
		BackoffMultiplier:    2.0,
		MaxDelayMs:           30000,
		RetryableStatusCodes: []int{429, 500, 502, 503, 504},
	}

	tests := []struct {
		name        string
		attempt     int
		err         error
		shouldRetry bool
	}{
		{
			name:        "Nil error - no retry",
			attempt:     0,
			err:         nil,
			shouldRetry: false,
		},
		{
			name:        "Max attempts reached",
			attempt:     3,
			err:         NewServerError(500, "server error", nil),
			shouldRetry: false,
		},
		{
			name:        "Retryable error within attempts",
			attempt:     1,
			err:         NewServerError(500, "server error", nil),
			shouldRetry: true,
		},
		{
			name:        "Non-retryable error",
			attempt:     0,
			err:         NewValidationError(400, "bad request", nil),
			shouldRetry: false,
		},
		{
			name:        "Rate limit error - retryable",
			attempt:     0,
			err:         NewRateLimitError("rate limited", nil),
			shouldRetry: true,
		},
		{
			name:        "Network error - retryable",
			attempt:     0,
			err:         NewNetworkError("connection refused", nil),
			shouldRetry: true,
		},
		{
			name:        "Authentication error - not retryable",
			attempt:     0,
			err:         NewAuthenticationError(401, "unauthorized", nil),
			shouldRetry: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.ShouldRetry(tt.attempt, tt.err)
			if result != tt.shouldRetry {
				t.Errorf("ShouldRetry(%d, %v) = %v, want %v", tt.attempt, tt.err, result, tt.shouldRetry)
			}
		})
	}
}

// TestRetryConfig_Disabled tests retry configuration with disabled retries.
func TestRetryConfig_Disabled(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:          0, // Disabled
		DelayMs:              1000,
		BackoffMultiplier:    2.0,
		MaxDelayMs:           30000,
		RetryableStatusCodes: []int{429, 500},
	}

	// Should never retry when maxAttempts is 0
	err := NewServerError(500, "server error", nil)

	if config.ShouldRetry(0, err) {
		t.Error("ShouldRetry should return false when maxAttempts is 0")
	}
}

// TestResolveRetryConfig tests precedence resolution.
func TestResolveRetryConfig(t *testing.T) {
	tests := []struct {
		name          string
		moduleRetry   *RetryConfig
		defaultsRetry *RetryConfig
		expected      RetryConfig
	}{
		{
			name:          "Module retry takes precedence",
			moduleRetry:   &RetryConfig{MaxAttempts: 5, DelayMs: 500, BackoffMultiplier: 1.5, MaxDelayMs: 10000, RetryableStatusCodes: []int{500}},
			defaultsRetry: &RetryConfig{MaxAttempts: 3, DelayMs: 1000, BackoffMultiplier: 2.0, MaxDelayMs: 30000, RetryableStatusCodes: []int{429, 500}},
			expected:      RetryConfig{MaxAttempts: 5, DelayMs: 500, BackoffMultiplier: 1.5, MaxDelayMs: 10000, RetryableStatusCodes: []int{500}},
		},
		{
			name:          "Defaults retry used when module retry is nil",
			moduleRetry:   nil,
			defaultsRetry: &RetryConfig{MaxAttempts: 5, DelayMs: 2000, BackoffMultiplier: 1.5, MaxDelayMs: 60000, RetryableStatusCodes: []int{429}},
			expected:      RetryConfig{MaxAttempts: 5, DelayMs: 2000, BackoffMultiplier: 1.5, MaxDelayMs: 60000, RetryableStatusCodes: []int{429}},
		},
		{
			name:          "Default config when both are nil",
			moduleRetry:   nil,
			defaultsRetry: nil,
			expected:      DefaultRetryConfig(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResolveRetryConfig(tt.moduleRetry, tt.defaultsRetry)

			if result.MaxAttempts != tt.expected.MaxAttempts {
				t.Errorf("MaxAttempts = %d, want %d", result.MaxAttempts, tt.expected.MaxAttempts)
			}
			if result.DelayMs != tt.expected.DelayMs {
				t.Errorf("DelayMs = %d, want %d", result.DelayMs, tt.expected.DelayMs)
			}
		})
	}
}

// TestErrorHandlingConfig tests error handling configuration.
func TestErrorHandlingConfig(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected ErrorHandlingConfig
	}{
		{
			name:  "Nil map returns defaults",
			input: nil,
			expected: ErrorHandlingConfig{
				OnError:   "fail",
				TimeoutMs: 30000,
				Retry:     DefaultRetryConfig(),
			},
		},
		{
			name: "Custom onError",
			input: map[string]interface{}{
				"onError": "skip",
			},
			expected: ErrorHandlingConfig{
				OnError:   "skip",
				TimeoutMs: 30000,
				Retry:     DefaultRetryConfig(),
			},
		},
		{
			name: "Custom timeout",
			input: map[string]interface{}{
				"timeoutMs": float64(60000),
			},
			expected: ErrorHandlingConfig{
				OnError:   "fail",
				TimeoutMs: 60000,
				Retry:     DefaultRetryConfig(),
			},
		},
		{
			name: "Full custom config",
			input: map[string]interface{}{
				"onError":   "log",
				"timeoutMs": float64(45000),
				"retry": map[string]interface{}{
					"maxAttempts": float64(5),
					"delayMs":     float64(500),
				},
			},
			expected: ErrorHandlingConfig{
				OnError:   "log",
				TimeoutMs: 45000,
				Retry: RetryConfig{
					MaxAttempts:          5,
					DelayMs:              500,
					BackoffMultiplier:    2.0,
					MaxDelayMs:           30000,
					RetryableStatusCodes: []int{429, 500, 502, 503, 504},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseErrorHandlingConfig(tt.input)

			if result.OnError != tt.expected.OnError {
				t.Errorf("OnError = %s, want %s", result.OnError, tt.expected.OnError)
			}
			if result.TimeoutMs != tt.expected.TimeoutMs {
				t.Errorf("TimeoutMs = %d, want %d", result.TimeoutMs, tt.expected.TimeoutMs)
			}
			if result.Retry.MaxAttempts != tt.expected.Retry.MaxAttempts {
				t.Errorf("Retry.MaxAttempts = %d, want %d", result.Retry.MaxAttempts, tt.expected.Retry.MaxAttempts)
			}
		})
	}
}

// TestOnErrorStrategy tests the OnError strategy type.
func TestOnErrorStrategy(t *testing.T) {
	tests := []struct {
		strategy OnErrorStrategy
		expected string
	}{
		{OnErrorFail, "fail"},
		{OnErrorSkip, "skip"},
		{OnErrorLog, "log"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if string(tt.strategy) != tt.expected {
				t.Errorf("OnErrorStrategy = %s, want %s", tt.strategy, tt.expected)
			}
		})
	}
}

// TestParseOnErrorStrategy tests parsing OnError strategy.
func TestParseOnErrorStrategy(t *testing.T) {
	tests := []struct {
		input    string
		expected OnErrorStrategy
	}{
		{"fail", OnErrorFail},
		{"skip", OnErrorSkip},
		{"log", OnErrorLog},
		{"FAIL", OnErrorFail},    // Case insensitive
		{"Skip", OnErrorSkip},    // Case insensitive
		{"LOG", OnErrorLog},      // Case insensitive
		{"", OnErrorFail},        // Default
		{"invalid", OnErrorFail}, // Invalid defaults to fail
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := ParseOnErrorStrategy(tt.input)
			if result != tt.expected {
				t.Errorf("ParseOnErrorStrategy(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

// ============================
// Retry Executor Tests
// ============================

// TestRetryExecutor_Success tests successful execution without retry.
func TestRetryExecutor_Success(t *testing.T) {
	config := DefaultRetryConfig()
	executor := NewRetryExecutor(config)

	callCount := 0
	result, err := executor.Execute(context.Background(), func(ctx context.Context) (interface{}, error) {
		callCount++
		return "success", nil
	})

	if err != nil {
		t.Errorf("Execute() error = %v, want nil", err)
	}
	if result != "success" {
		t.Errorf("Execute() result = %v, want 'success'", result)
	}
	if callCount != 1 {
		t.Errorf("Function called %d times, want 1", callCount)
	}
}

// TestRetryExecutor_RetryOnTransientError tests retry on transient errors.
func TestRetryExecutor_RetryOnTransientError(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:          3,
		DelayMs:              10, // Short delay for tests
		BackoffMultiplier:    1.0,
		MaxDelayMs:           100,
		RetryableStatusCodes: []int{500},
	}
	executor := NewRetryExecutor(config)

	var callCount int32
	result, err := executor.Execute(context.Background(), func(ctx context.Context) (interface{}, error) {
		count := atomic.AddInt32(&callCount, 1)
		if count < 3 {
			return nil, NewServerError(500, "transient error", nil)
		}
		return "success after retries", nil
	})

	if err != nil {
		t.Errorf("Execute() error = %v, want nil", err)
	}
	if result != "success after retries" {
		t.Errorf("Execute() result = %v, want 'success after retries'", result)
	}
	if atomic.LoadInt32(&callCount) != 3 {
		t.Errorf("Function called %d times, want 3", callCount)
	}
}

// TestRetryExecutor_NoRetryOnFatalError tests no retry on fatal errors.
func TestRetryExecutor_NoRetryOnFatalError(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:          3,
		DelayMs:              10,
		BackoffMultiplier:    1.0,
		MaxDelayMs:           100,
		RetryableStatusCodes: []int{500},
	}
	executor := NewRetryExecutor(config)

	callCount := 0
	_, err := executor.Execute(context.Background(), func(ctx context.Context) (interface{}, error) {
		callCount++
		return nil, NewAuthenticationError(401, "unauthorized", nil)
	})

	if err == nil {
		t.Error("Execute() error = nil, want error")
	}
	if callCount != 1 {
		t.Errorf("Function called %d times, want 1 (no retry on fatal error)", callCount)
	}
}

// TestRetryExecutor_MaxAttemptsExhausted tests max attempts reached.
func TestRetryExecutor_MaxAttemptsExhausted(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:          3,
		DelayMs:              10,
		BackoffMultiplier:    1.0,
		MaxDelayMs:           100,
		RetryableStatusCodes: []int{500},
	}
	executor := NewRetryExecutor(config)

	var callCount int32
	_, err := executor.Execute(context.Background(), func(ctx context.Context) (interface{}, error) {
		atomic.AddInt32(&callCount, 1)
		return nil, NewServerError(500, "persistent error", nil)
	})

	if err == nil {
		t.Error("Execute() error = nil, want error")
	}
	// Should try 1 + 3 retries = 4 total attempts
	if atomic.LoadInt32(&callCount) != 4 {
		t.Errorf("Function called %d times, want 4 (1 + 3 retries)", callCount)
	}
}

// TestRetryExecutor_ContextCanceled tests context cancellation during retry.
func TestRetryExecutor_ContextCanceled(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:          5,
		DelayMs:              100, // Longer delay to allow cancellation
		BackoffMultiplier:    1.0,
		MaxDelayMs:           1000,
		RetryableStatusCodes: []int{500},
	}
	executor := NewRetryExecutor(config)

	ctx, cancel := context.WithCancel(context.Background())
	callCount := 0

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := executor.Execute(ctx, func(ctx context.Context) (interface{}, error) {
		callCount++
		return nil, NewServerError(500, "transient error", nil)
	})

	if err == nil {
		t.Error("Execute() error = nil, want context canceled error")
	}
	if !errors.Is(err, context.Canceled) {
		// The error might be wrapped
		var classifiedErr *ClassifiedError
		if errors.As(err, &classifiedErr) && classifiedErr.OriginalErr != nil {
			if !errors.Is(classifiedErr.OriginalErr, context.Canceled) {
				t.Errorf("Execute() error = %v, want context.Canceled", err)
			}
		}
	}
}

// TestRetryExecutor_DisabledRetry tests retry disabled (maxAttempts = 0).
func TestRetryExecutor_DisabledRetry(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:          0, // Disabled
		DelayMs:              10,
		BackoffMultiplier:    1.0,
		MaxDelayMs:           100,
		RetryableStatusCodes: []int{500},
	}
	executor := NewRetryExecutor(config)

	callCount := 0
	_, err := executor.Execute(context.Background(), func(ctx context.Context) (interface{}, error) {
		callCount++
		return nil, NewServerError(500, "transient error", nil)
	})

	if err == nil {
		t.Error("Execute() error = nil, want error")
	}
	if callCount != 1 {
		t.Errorf("Function called %d times, want 1 (retry disabled)", callCount)
	}
}

// TestRetryExecutor_ExponentialBackoff tests exponential backoff timing.
func TestRetryExecutor_ExponentialBackoff(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:          3,
		DelayMs:              50,
		BackoffMultiplier:    2.0,
		MaxDelayMs:           500,
		RetryableStatusCodes: []int{500},
	}
	executor := NewRetryExecutor(config)

	var timestamps []time.Time
	_, err := executor.Execute(context.Background(), func(ctx context.Context) (interface{}, error) {
		timestamps = append(timestamps, time.Now())
		if len(timestamps) < 4 {
			return nil, NewServerError(500, "transient error", nil)
		}
		return "success", nil
	})

	if err != nil {
		t.Errorf("Execute() error = %v, want nil", err)
	}

	// Verify delays increase exponentially
	// First retry: ~50ms, second: ~100ms, third: ~200ms
	if len(timestamps) >= 3 {
		delay1 := timestamps[1].Sub(timestamps[0])
		delay2 := timestamps[2].Sub(timestamps[1])

		// Allow 20ms tolerance for timing
		if delay1 < 30*time.Millisecond || delay1 > 100*time.Millisecond {
			t.Errorf("First retry delay = %v, want ~50ms", delay1)
		}
		if delay2 < 60*time.Millisecond || delay2 > 200*time.Millisecond {
			t.Errorf("Second retry delay = %v, want ~100ms", delay2)
		}
	}
}

// TestRetryExecutor_GetResult tests result retrieval.
func TestRetryExecutor_GetResult(t *testing.T) {
	config := DefaultRetryConfig()
	executor := NewRetryExecutor(config)

	type testResult struct {
		Value int
		Name  string
	}

	result, err := executor.Execute(context.Background(), func(ctx context.Context) (interface{}, error) {
		return testResult{Value: 42, Name: "test"}, nil
	})

	if err != nil {
		t.Errorf("Execute() error = %v, want nil", err)
	}

	res, ok := result.(testResult)
	if !ok {
		t.Fatalf("Result type = %T, want testResult", result)
	}
	if res.Value != 42 || res.Name != "test" {
		t.Errorf("Result = %+v, want {Value: 42, Name: test}", res)
	}
}

// TestRetryExecutor_RetryInfo tests retry information tracking.
func TestRetryExecutor_RetryInfo(t *testing.T) {
	config := RetryConfig{
		MaxAttempts:          3,
		DelayMs:              10,
		BackoffMultiplier:    1.0,
		MaxDelayMs:           100,
		RetryableStatusCodes: []int{500},
	}
	executor := NewRetryExecutor(config)

	var callCount int32
	_, _ = executor.Execute(context.Background(), func(ctx context.Context) (interface{}, error) {
		count := atomic.AddInt32(&callCount, 1)
		if count < 3 {
			return nil, NewServerError(500, "transient error", nil)
		}
		return "success", nil
	})

	info := executor.GetRetryInfo()

	if info.TotalAttempts != 3 {
		t.Errorf("TotalAttempts = %d, want 3", info.TotalAttempts)
	}
	if info.SuccessfulAttempt != 3 {
		t.Errorf("SuccessfulAttempt = %d, want 3", info.SuccessfulAttempt)
	}
	if info.RetryCount != 2 {
		t.Errorf("RetryCount = %d, want 2", info.RetryCount)
	}
}
