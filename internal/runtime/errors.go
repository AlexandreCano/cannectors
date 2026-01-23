// Package runtime provides error types and classification for pipeline execution.
// This file re-exports error handling utilities from the errhandling package.
package runtime

import (
	"github.com/canectors/runtime/internal/errhandling"
)

// ErrorCategory represents the category of an error (re-exported from errhandling).
type ErrorCategory = errhandling.ErrorCategory

// ClassifiedError represents a classified error with category and retry info (re-exported from errhandling).
type ClassifiedError = errhandling.ClassifiedError

// Re-export error category constants
const (
	CategoryNetwork        = errhandling.CategoryNetwork
	CategoryAuthentication = errhandling.CategoryAuthentication
	CategoryValidation     = errhandling.CategoryValidation
	CategoryRateLimit      = errhandling.CategoryRateLimit
	CategoryServer         = errhandling.CategoryServer
	CategoryNotFound       = errhandling.CategoryNotFound
	CategoryUnknown        = errhandling.CategoryUnknown
)

// Re-export functions
var (
	ClassifyHTTPStatus          = errhandling.ClassifyHTTPStatus
	ClassifyNetworkError        = errhandling.ClassifyNetworkError
	ClassifyError               = errhandling.ClassifyError
	IsRetryable                 = errhandling.IsRetryable
	IsFatal                     = errhandling.IsFatal
	GetErrorCategory            = errhandling.GetErrorCategory
	DefaultRetryableStatusCodes = errhandling.DefaultRetryableStatusCodes
	IsRetryableStatusCode       = errhandling.IsRetryableStatusCode
	NewNetworkError             = errhandling.NewNetworkError
	NewAuthenticationError      = errhandling.NewAuthenticationError
	NewValidationError          = errhandling.NewValidationError
	NewServerError              = errhandling.NewServerError
	NewRateLimitError           = errhandling.NewRateLimitError
	NewNotFoundError            = errhandling.NewNotFoundError
)
