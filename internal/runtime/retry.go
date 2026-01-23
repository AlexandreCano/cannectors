// Package runtime provides retry configuration and mechanism for pipeline execution.
// This file re-exports retry utilities from the errhandling package.
package runtime

import (
	"github.com/canectors/runtime/internal/errhandling"
)

// OnErrorStrategy defines what action to take when an error occurs (re-exported from errhandling).
type OnErrorStrategy = errhandling.OnErrorStrategy

// RetryConfig holds retry configuration for pipeline execution (re-exported from errhandling).
type RetryConfig = errhandling.RetryConfig

// ErrorHandlingConfig holds error handling configuration for a module (re-exported from errhandling).
type ErrorHandlingConfig = errhandling.ErrorHandlingConfig

// RetryFunc is a function that can be retried (re-exported from errhandling).
type RetryFunc = errhandling.RetryFunc

// RetryInfo contains information about retry attempts (re-exported from errhandling).
type RetryInfo = errhandling.RetryInfo

// RetryExecutor executes functions with retry logic (re-exported from errhandling).
type RetryExecutor = errhandling.RetryExecutor

// Re-export constants
const (
	DefaultMaxAttempts       = errhandling.DefaultMaxAttempts
	DefaultDelayMs           = errhandling.DefaultDelayMs
	DefaultBackoffMultiplier = errhandling.DefaultBackoffMultiplier
	DefaultMaxDelayMs        = errhandling.DefaultMaxDelayMs
	DefaultTimeoutMs         = errhandling.DefaultTimeoutMs
	MaxRetryAttempts         = errhandling.MaxRetryAttempts
	MinBackoffMultiplier     = errhandling.MinBackoffMultiplier
)

// Re-export OnErrorStrategy constants
const (
	OnErrorFail = errhandling.OnErrorFail
	OnErrorSkip = errhandling.OnErrorSkip
	OnErrorLog  = errhandling.OnErrorLog
)

// Re-export functions
var (
	DefaultRetryConfig         = errhandling.DefaultRetryConfig
	DefaultErrorHandlingConfig = errhandling.DefaultErrorHandlingConfig
	ParseRetryConfig           = errhandling.ParseRetryConfig
	ParseErrorHandlingConfig   = errhandling.ParseErrorHandlingConfig
	ResolveRetryConfig         = errhandling.ResolveRetryConfig
	ResolveErrorHandlingConfig = errhandling.ResolveErrorHandlingConfig
	ParseOnErrorStrategy       = errhandling.ParseOnErrorStrategy
	NewRetryExecutor           = errhandling.NewRetryExecutor
)
