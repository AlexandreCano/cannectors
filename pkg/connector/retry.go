package connector

import (
	"errors"
	"fmt"
	"math"
	"time"
)

const (
	// MaxRetryAttempts is the maximum number of retry attempts allowed.
	MaxRetryAttempts = 10
	// MinBackoffMultiplier is the minimum backoff multiplier allowed.
	MinBackoffMultiplier = 1.0
)

// DefaultRetryableStatusCodes returns the default HTTP status codes that trigger retries.
func DefaultRetryableStatusCodes() []int {
	return []int{429, 500, 502, 503, 504}
}

// DefaultRetryConfig returns a RetryConfig with sensible defaults.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:          3,
		DelayMs:              1000,
		BackoffMultiplier:    2.0,
		MaxDelayMs:           30000,
		RetryableStatusCodes: DefaultRetryableStatusCodes(),
	}
}

// Validate checks that retry configuration values are within acceptable ranges.
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
	delayMs := float64(c.DelayMs) * math.Pow(c.BackoffMultiplier, float64(attempt))
	if delayMs > float64(c.MaxDelayMs) {
		delayMs = float64(c.MaxDelayMs)
	}
	return time.Duration(delayMs) * time.Millisecond
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
