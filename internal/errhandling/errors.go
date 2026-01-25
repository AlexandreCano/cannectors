// Package errhandling provides error types, classification, and retry utilities.
// This file defines error categories, classification functions, and helper utilities
// for robust error handling across the Canectors runtime.
package errhandling

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
)

// ErrorCategory represents the type/category of an error.
// Categories help determine the appropriate error handling strategy.
type ErrorCategory string

// Error categories for classification.
const (
	// CategoryNetwork represents network-related errors (timeout, connection refused, DNS).
	// Network errors are typically transient and retryable.
	CategoryNetwork ErrorCategory = "network"

	// CategoryAuthentication represents authentication errors (401, 403).
	// Authentication errors are fatal and should not be retried.
	CategoryAuthentication ErrorCategory = "authentication"

	// CategoryValidation represents validation errors (400, 422).
	// Validation errors are fatal - the request is malformed.
	CategoryValidation ErrorCategory = "validation"

	// CategoryRateLimit represents rate limiting errors (429).
	// Rate limit errors are transient and should be retried with backoff.
	CategoryRateLimit ErrorCategory = "rate_limit"

	// CategoryServer represents server errors (5xx).
	// Server errors are typically transient and retryable.
	CategoryServer ErrorCategory = "server"

	// CategoryNotFound represents not found errors (404).
	// Not found errors are fatal - the resource doesn't exist.
	CategoryNotFound ErrorCategory = "not_found"

	// CategoryUnknown represents unclassified errors.
	// Unknown errors are retryable by default (transient more likely than permanent).
	CategoryUnknown ErrorCategory = "unknown"
)

// ClassifiedError wraps an error with classification metadata.
// It provides category, retryability status, and contextual information.
type ClassifiedError struct {
	// Category is the error classification category.
	Category ErrorCategory

	// Retryable indicates whether the error is transient and can be retried.
	Retryable bool

	// StatusCode is the HTTP status code (0 if not an HTTP error).
	StatusCode int

	// Message is a human-readable error message.
	Message string

	// OriginalErr is the underlying error that was classified.
	OriginalErr error
}

// Error implements the error interface.
func (e *ClassifiedError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("%s error (status %d): %s", e.Category, e.StatusCode, e.Message)
	}
	return fmt.Sprintf("%s error: %s", e.Category, e.Message)
}

// Unwrap returns the original error for use with errors.Is and errors.As.
func (e *ClassifiedError) Unwrap() error {
	return e.OriginalErr
}

// ClassifyHTTPStatus classifies an HTTP error based on status code.
// It returns a ClassifiedError with appropriate category and retryability.
//
// Classification rules:
//   - 401, 403: Authentication errors (not retryable)
//   - 400, 422: Validation errors (not retryable)
//   - 404: Not found errors (not retryable)
//   - 429: Rate limit errors (retryable)
//   - 5xx: Server errors (retryable)
//   - Other 4xx: Validation errors (not retryable)
//   - Unknown status codes: CategoryUnknown (retryable by default)
func ClassifyHTTPStatus(statusCode int, message string) *ClassifiedError {
	switch {
	case statusCode == 401:
		return &ClassifiedError{
			Category:   CategoryAuthentication,
			Retryable:  false,
			StatusCode: statusCode,
			Message:    "unauthorized",
		}
	case statusCode == 403:
		return &ClassifiedError{
			Category:   CategoryAuthentication,
			Retryable:  false,
			StatusCode: statusCode,
			Message:    "forbidden",
		}
	case statusCode == 400:
		return &ClassifiedError{
			Category:   CategoryValidation,
			Retryable:  false,
			StatusCode: statusCode,
			Message:    "bad request",
		}
	case statusCode == 422:
		return &ClassifiedError{
			Category:   CategoryValidation,
			Retryable:  false,
			StatusCode: statusCode,
			Message:    "unprocessable entity",
		}
	case statusCode == 404:
		return &ClassifiedError{
			Category:   CategoryNotFound,
			Retryable:  false,
			StatusCode: statusCode,
			Message:    "not found",
		}
	case statusCode == 429:
		return &ClassifiedError{
			Category:   CategoryRateLimit,
			Retryable:  true,
			StatusCode: statusCode,
			Message:    "rate limited",
		}
	case statusCode == 500:
		return &ClassifiedError{
			Category:   CategoryServer,
			Retryable:  true,
			StatusCode: statusCode,
			Message:    "internal server error",
		}
	case statusCode == 502:
		return &ClassifiedError{
			Category:   CategoryServer,
			Retryable:  true,
			StatusCode: statusCode,
			Message:    "bad gateway",
		}
	case statusCode == 503:
		return &ClassifiedError{
			Category:   CategoryServer,
			Retryable:  true,
			StatusCode: statusCode,
			Message:    "service unavailable",
		}
	case statusCode == 504:
		return &ClassifiedError{
			Category:   CategoryServer,
			Retryable:  true,
			StatusCode: statusCode,
			Message:    "gateway timeout",
		}
	case statusCode >= 500:
		return &ClassifiedError{
			Category:   CategoryServer,
			Retryable:  true,
			StatusCode: statusCode,
			Message:    "server error",
		}
	case statusCode >= 400:
		return &ClassifiedError{
			Category:   CategoryValidation,
			Retryable:  false,
			StatusCode: statusCode,
			Message:    "client error",
		}
	default:
		// Unknown status codes are retryable by default (transient more likely than permanent).
		return &ClassifiedError{
			Category:   CategoryUnknown,
			Retryable:  true,
			StatusCode: statusCode,
			Message:    message,
		}
	}
}

// ClassifyNetworkError classifies a network-related error.
// Network errors include timeouts, connection refused, DNS errors, etc.
//
// Classification rules:
//   - Timeout errors: Network category (retryable)
//   - Connection refused: Network category (retryable)
//   - DNS errors: Network category (retryable)
//   - URL errors: Network category (retryable)
//   - Unknown: Unknown category (retryable by default)
func ClassifyNetworkError(err error) *ClassifiedError {
	if err == nil {
		return &ClassifiedError{
			Category:  CategoryUnknown,
			Retryable: false,
			Message:   "nil error",
		}
	}

	// Check for timeout errors
	if errors.Is(err, context.DeadlineExceeded) {
		return &ClassifiedError{
			Category:    CategoryNetwork,
			Retryable:   true,
			StatusCode:  0,
			Message:     "request timeout",
			OriginalErr: err,
		}
	}

	// Check for context canceled (not retryable - user initiated)
	if errors.Is(err, context.Canceled) {
		return &ClassifiedError{
			Category:    CategoryNetwork,
			Retryable:   false,
			StatusCode:  0,
			Message:     "context canceled",
			OriginalErr: err,
		}
	}

	// Check for net.OpError (connection refused, network unreachable, etc.)
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return &ClassifiedError{
			Category:    CategoryNetwork,
			Retryable:   true,
			StatusCode:  0,
			Message:     fmt.Sprintf("network error: %s %s", opErr.Op, opErr.Net),
			OriginalErr: err,
		}
	}

	// Check for DNS errors
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return &ClassifiedError{
			Category:    CategoryNetwork,
			Retryable:   true,
			StatusCode:  0,
			Message:     fmt.Sprintf("DNS error: %s", dnsErr.Name),
			OriginalErr: err,
		}
	}

	// Check for URL errors (wraps other network errors)
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return &ClassifiedError{
			Category:    CategoryNetwork,
			Retryable:   true,
			StatusCode:  0,
			Message:     fmt.Sprintf("URL error: %s %s", urlErr.Op, urlErr.URL),
			OriginalErr: err,
		}
	}

	// Check for timeout interface
	type timeoutError interface {
		Timeout() bool
	}
	var timeoutErr timeoutError
	if errors.As(err, &timeoutErr) && timeoutErr.Timeout() {
		return &ClassifiedError{
			Category:    CategoryNetwork,
			Retryable:   true,
			StatusCode:  0,
			Message:     "timeout",
			OriginalErr: err,
		}
	}

	// Unknown network error - retryable by default (transient more likely than permanent).
	return &ClassifiedError{
		Category:    CategoryUnknown,
		Retryable:   true,
		StatusCode:  0,
		Message:     err.Error(),
		OriginalErr: err,
	}
}

// ClassifyError classifies any error into a ClassifiedError.
// It handles already classified errors, HTTP errors, and network errors.
// Unknown (unclassified) errors are retryable by default.
func ClassifyError(err error) *ClassifiedError {
	if err == nil {
		return &ClassifiedError{
			Category:  CategoryUnknown,
			Retryable: false,
			Message:   "nil error",
		}
	}

	// Check if already classified
	var classified *ClassifiedError
	if errors.As(err, &classified) {
		return classified
	}

	// Check for timeout/context errors first
	if errors.Is(err, context.DeadlineExceeded) {
		return &ClassifiedError{
			Category:    CategoryNetwork,
			Retryable:   true,
			StatusCode:  0,
			Message:     "request timeout",
			OriginalErr: err,
		}
	}

	if errors.Is(err, context.Canceled) {
		return &ClassifiedError{
			Category:    CategoryNetwork,
			Retryable:   false,
			StatusCode:  0,
			Message:     "context canceled",
			OriginalErr: err,
		}
	}

	// Check for network errors
	var opErr *net.OpError
	var dnsErr *net.DNSError
	var urlErr *url.Error

	if errors.As(err, &opErr) || errors.As(err, &dnsErr) || errors.As(err, &urlErr) {
		return ClassifyNetworkError(err)
	}

	// Unknown error - retryable by default (transient more likely than permanent).
	return &ClassifiedError{
		Category:    CategoryUnknown,
		Retryable:   true,
		StatusCode:  0,
		Message:     err.Error(),
		OriginalErr: err,
	}
}

// IsRetryable returns true if the error is classified as retryable.
// Nil errors return false.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Check for classified error
	var classified *ClassifiedError
	if errors.As(err, &classified) {
		return classified.Retryable
	}

	// Check for timeout (retryable)
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// Check for context canceled (not retryable)
	if errors.Is(err, context.Canceled) {
		return false
	}

	// Classify and check
	classifiedErr := ClassifyError(err)
	return classifiedErr.Retryable
}

// IsFatal returns true if the error is classified as fatal (should not be retried).
// Fatal categories: Authentication, Validation, NotFound.
func IsFatal(err error) bool {
	if err == nil {
		return false
	}

	category := GetErrorCategory(err)

	switch category {
	case CategoryAuthentication, CategoryValidation, CategoryNotFound:
		return true
	default:
		return false
	}
}

// GetErrorCategory returns the error category for a given error.
// Returns CategoryUnknown for nil or unclassified errors.
func GetErrorCategory(err error) ErrorCategory {
	if err == nil {
		return CategoryUnknown
	}

	var classified *ClassifiedError
	if errors.As(err, &classified) {
		return classified.Category
	}

	return CategoryUnknown
}

// defaultRetryableStatusCodes is the list of HTTP status codes that are retryable by default.
var defaultRetryableStatusCodes = []int{429, 500, 502, 503, 504}

// DefaultRetryableStatusCodes returns the default list of retryable HTTP status codes.
func DefaultRetryableStatusCodes() []int {
	// Return a copy to prevent modification
	result := make([]int, len(defaultRetryableStatusCodes))
	copy(result, defaultRetryableStatusCodes)
	return result
}

// IsRetryableStatusCode checks if a status code is in the list of retryable codes.
func IsRetryableStatusCode(statusCode int, retryableCodes []int) bool {
	for _, code := range retryableCodes {
		if statusCode == code {
			return true
		}
	}
	return false
}

// NewNetworkError creates a ClassifiedError for network errors.
func NewNetworkError(message string, originalErr error) *ClassifiedError {
	return &ClassifiedError{
		Category:    CategoryNetwork,
		Retryable:   true,
		StatusCode:  0,
		Message:     message,
		OriginalErr: originalErr,
	}
}

// NewAuthenticationError creates a ClassifiedError for authentication errors.
func NewAuthenticationError(statusCode int, message string, originalErr error) *ClassifiedError {
	return &ClassifiedError{
		Category:    CategoryAuthentication,
		Retryable:   false,
		StatusCode:  statusCode,
		Message:     message,
		OriginalErr: originalErr,
	}
}

// NewValidationError creates a ClassifiedError for validation errors.
func NewValidationError(statusCode int, message string, originalErr error) *ClassifiedError {
	return &ClassifiedError{
		Category:    CategoryValidation,
		Retryable:   false,
		StatusCode:  statusCode,
		Message:     message,
		OriginalErr: originalErr,
	}
}

// NewServerError creates a ClassifiedError for server errors.
func NewServerError(statusCode int, message string, originalErr error) *ClassifiedError {
	return &ClassifiedError{
		Category:    CategoryServer,
		Retryable:   true,
		StatusCode:  statusCode,
		Message:     message,
		OriginalErr: originalErr,
	}
}

// NewRateLimitError creates a ClassifiedError for rate limit errors.
func NewRateLimitError(message string, originalErr error) *ClassifiedError {
	return &ClassifiedError{
		Category:    CategoryRateLimit,
		Retryable:   true,
		StatusCode:  429,
		Message:     message,
		OriginalErr: originalErr,
	}
}

// NewNotFoundError creates a ClassifiedError for not found errors.
func NewNotFoundError(message string, originalErr error) *ClassifiedError {
	return &ClassifiedError{
		Category:    CategoryNotFound,
		Retryable:   false,
		StatusCode:  404,
		Message:     message,
		OriginalErr: originalErr,
	}
}
