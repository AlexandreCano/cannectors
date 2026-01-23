// Package errhandling provides error types and classification for pipeline execution.
package errhandling

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"syscall"
	"testing"
)

// TestErrorCategory tests error category constants and their string values.
func TestErrorCategory(t *testing.T) {
	tests := []struct {
		category ErrorCategory
		expected string
	}{
		{CategoryNetwork, "network"},
		{CategoryAuthentication, "authentication"},
		{CategoryValidation, "validation"},
		{CategoryRateLimit, "rate_limit"},
		{CategoryServer, "server"},
		{CategoryNotFound, "not_found"},
		{CategoryUnknown, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if string(tt.category) != tt.expected {
				t.Errorf("ErrorCategory = %v, want %v", tt.category, tt.expected)
			}
		})
	}
}

// TestClassifiedError tests the ClassifiedError type.
func TestClassifiedError(t *testing.T) {
	t.Run("Error message formatting", func(t *testing.T) {
		err := &ClassifiedError{
			Category:    CategoryNetwork,
			Retryable:   true,
			StatusCode:  0,
			Message:     "connection refused",
			OriginalErr: errors.New("dial tcp: connection refused"),
		}

		errorStr := err.Error()
		if errorStr == "" {
			t.Error("Error() returned empty string")
		}
		// Error message should contain category and message
		if !contains(errorStr, "network") || !contains(errorStr, "connection refused") {
			t.Errorf("Error() = %v, want to contain 'network' and 'connection refused'", errorStr)
		}
	})

	t.Run("Unwrap returns original error", func(t *testing.T) {
		original := errors.New("original error")
		err := &ClassifiedError{
			Category:    CategoryValidation,
			Retryable:   false,
			StatusCode:  400,
			Message:     "bad request",
			OriginalErr: original,
		}

		if err.Unwrap() != original {
			t.Errorf("Unwrap() = %v, want %v", err.Unwrap(), original)
		}
	})

	t.Run("Is checks original error", func(t *testing.T) {
		original := errors.New("original error")
		err := &ClassifiedError{
			Category:    CategoryValidation,
			Retryable:   false,
			StatusCode:  400,
			Message:     "bad request",
			OriginalErr: original,
		}

		if !errors.Is(err, original) {
			t.Error("errors.Is should match original error")
		}
	})
}

// TestClassifyHTTPStatus tests HTTP status code classification.
func TestClassifyHTTPStatus(t *testing.T) {
	tests := []struct {
		name          string
		statusCode    int
		wantCategory  ErrorCategory
		wantRetryable bool
		wantMessage   string
	}{
		// Authentication errors (401, 403)
		{"401 Unauthorized", 401, CategoryAuthentication, false, "unauthorized"},
		{"403 Forbidden", 403, CategoryAuthentication, false, "forbidden"},

		// Validation errors (400, 422)
		{"400 Bad Request", 400, CategoryValidation, false, "bad request"},
		{"422 Unprocessable Entity", 422, CategoryValidation, false, "unprocessable entity"},

		// Not found (404)
		{"404 Not Found", 404, CategoryNotFound, false, "not found"},

		// Rate limiting (429)
		{"429 Too Many Requests", 429, CategoryRateLimit, true, "rate limited"},

		// Server errors (5xx) - all retryable
		{"500 Internal Server Error", 500, CategoryServer, true, "internal server error"},
		{"502 Bad Gateway", 502, CategoryServer, true, "bad gateway"},
		{"503 Service Unavailable", 503, CategoryServer, true, "service unavailable"},
		{"504 Gateway Timeout", 504, CategoryServer, true, "gateway timeout"},
		{"599 Unknown Server Error", 599, CategoryServer, true, "server error"},

		// Other 4xx client errors - not retryable
		{"405 Method Not Allowed", 405, CategoryValidation, false, "client error"},
		{"409 Conflict", 409, CategoryValidation, false, "client error"},
		{"410 Gone", 410, CategoryValidation, false, "client error"},
		{"415 Unsupported Media Type", 415, CategoryValidation, false, "client error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ClassifyHTTPStatus(tt.statusCode, "test error message")

			if err.Category != tt.wantCategory {
				t.Errorf("Category = %v, want %v", err.Category, tt.wantCategory)
			}
			if err.Retryable != tt.wantRetryable {
				t.Errorf("Retryable = %v, want %v", err.Retryable, tt.wantRetryable)
			}
			if err.StatusCode != tt.statusCode {
				t.Errorf("StatusCode = %v, want %v", err.StatusCode, tt.statusCode)
			}
		})
	}
}

// TestClassifyNetworkError tests network error classification.
func TestClassifyNetworkError(t *testing.T) {
	t.Run("Timeout error", func(t *testing.T) {
		// Create a timeout error using context.DeadlineExceeded
		timeoutErr := context.DeadlineExceeded

		err := ClassifyNetworkError(timeoutErr)

		if err.Category != CategoryNetwork {
			t.Errorf("Category = %v, want %v", err.Category, CategoryNetwork)
		}
		if !err.Retryable {
			t.Error("Timeout errors should be retryable")
		}
	})

	t.Run("Connection refused", func(t *testing.T) {
		connRefused := &net.OpError{
			Op:  "dial",
			Net: "tcp",
			Err: syscall.ECONNREFUSED,
		}

		err := ClassifyNetworkError(connRefused)

		if err.Category != CategoryNetwork {
			t.Errorf("Category = %v, want %v", err.Category, CategoryNetwork)
		}
		if !err.Retryable {
			t.Error("Connection refused errors should be retryable")
		}
	})

	t.Run("DNS error", func(t *testing.T) {
		dnsErr := &net.DNSError{
			Name: "unknown.host.example",
			Err:  "no such host",
		}

		err := ClassifyNetworkError(dnsErr)

		if err.Category != CategoryNetwork {
			t.Errorf("Category = %v, want %v", err.Category, CategoryNetwork)
		}
		if !err.Retryable {
			t.Error("DNS errors should be retryable")
		}
	})

	t.Run("URL error", func(t *testing.T) {
		urlErr := &url.Error{
			Op:  "Get",
			URL: "http://example.com",
			Err: errors.New("network error"),
		}

		err := ClassifyNetworkError(urlErr)

		if err.Category != CategoryNetwork {
			t.Errorf("Category = %v, want %v", err.Category, CategoryNetwork)
		}
		if !err.Retryable {
			t.Error("URL errors should be retryable")
		}
	})

	t.Run("Generic error", func(t *testing.T) {
		genericErr := errors.New("some generic error")

		err := ClassifyNetworkError(genericErr)

		if err.Category != CategoryUnknown {
			t.Errorf("Category = %v, want %v", err.Category, CategoryUnknown)
		}
	})
}

// TestClassifyError tests the general error classification function.
func TestClassifyError(t *testing.T) {
	t.Run("Already classified error", func(t *testing.T) {
		classified := &ClassifiedError{
			Category:    CategoryValidation,
			Retryable:   false,
			StatusCode:  400,
			Message:     "already classified",
			OriginalErr: nil,
		}

		result := ClassifyError(classified)

		if result != classified {
			t.Error("Already classified error should be returned as-is")
		}
	})

	t.Run("Wrapped classified error", func(t *testing.T) {
		classified := &ClassifiedError{
			Category:    CategoryServer,
			Retryable:   true,
			StatusCode:  500,
			Message:     "server error",
			OriginalErr: nil,
		}
		wrapped := fmt.Errorf("wrapped: %w", classified)

		result := ClassifyError(wrapped)

		if result.Category != CategoryServer {
			t.Errorf("Category = %v, want %v", result.Category, CategoryServer)
		}
	})

	t.Run("Network error", func(t *testing.T) {
		netErr := &net.OpError{
			Op:  "dial",
			Net: "tcp",
			Err: syscall.ECONNREFUSED,
		}

		result := ClassifyError(netErr)

		if result.Category != CategoryNetwork {
			t.Errorf("Category = %v, want %v", result.Category, CategoryNetwork)
		}
	})

	t.Run("Timeout error", func(t *testing.T) {
		result := ClassifyError(context.DeadlineExceeded)

		if result.Category != CategoryNetwork {
			t.Errorf("Category = %v, want %v", result.Category, CategoryNetwork)
		}
		if !result.Retryable {
			t.Error("Timeout errors should be retryable")
		}
	})

	t.Run("Context canceled - not retryable", func(t *testing.T) {
		result := ClassifyError(context.Canceled)

		// Context canceled is usually user-initiated, not retryable
		if result.Retryable {
			t.Error("Context canceled errors should not be retryable")
		}
	})

	t.Run("Unknown error", func(t *testing.T) {
		result := ClassifyError(errors.New("unknown error"))

		if result.Category != CategoryUnknown {
			t.Errorf("Category = %v, want %v", result.Category, CategoryUnknown)
		}
	})
}

// TestIsRetryable tests the IsRetryable helper function.
func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{"Nil error", nil, false},
		{"Server error", &ClassifiedError{Retryable: true}, true},
		{"Validation error", &ClassifiedError{Retryable: false}, false},
		{"Network timeout", context.DeadlineExceeded, true},
		{"Context canceled", context.Canceled, false},
		{"Wrapped retryable", fmt.Errorf("wrapped: %w", &ClassifiedError{Retryable: true}), true},
		{"Wrapped non-retryable", fmt.Errorf("wrapped: %w", &ClassifiedError{Retryable: false}), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRetryable(tt.err)
			if result != tt.retryable {
				t.Errorf("IsRetryable() = %v, want %v", result, tt.retryable)
			}
		})
	}
}

// TestIsFatal tests the IsFatal helper function.
func TestIsFatal(t *testing.T) {
	tests := []struct {
		name  string
		err   error
		fatal bool
	}{
		{"Nil error", nil, false},
		{"Authentication error", &ClassifiedError{Category: CategoryAuthentication}, true},
		{"Validation error", &ClassifiedError{Category: CategoryValidation}, true},
		{"Not found error", &ClassifiedError{Category: CategoryNotFound}, true},
		{"Server error", &ClassifiedError{Category: CategoryServer}, false},
		{"Network error", &ClassifiedError{Category: CategoryNetwork}, false},
		{"Rate limit error", &ClassifiedError{Category: CategoryRateLimit}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsFatal(tt.err)
			if result != tt.fatal {
				t.Errorf("IsFatal() = %v, want %v", result, tt.fatal)
			}
		})
	}
}

// TestGetErrorCategory tests the GetErrorCategory helper function.
func TestGetErrorCategory(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected ErrorCategory
	}{
		{"Nil error", nil, CategoryUnknown},
		{"Server error", &ClassifiedError{Category: CategoryServer}, CategoryServer},
		{"Network error", &ClassifiedError{Category: CategoryNetwork}, CategoryNetwork},
		{"Wrapped error", fmt.Errorf("wrapped: %w", &ClassifiedError{Category: CategoryValidation}), CategoryValidation},
		{"Unknown error", errors.New("unknown"), CategoryUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetErrorCategory(tt.err)
			if result != tt.expected {
				t.Errorf("GetErrorCategory() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestDefaultRetryableStatusCodes tests the default retryable status codes.
func TestDefaultRetryableStatusCodes(t *testing.T) {
	expected := []int{429, 500, 502, 503, 504}

	codes := DefaultRetryableStatusCodes()

	if len(codes) != len(expected) {
		t.Errorf("DefaultRetryableStatusCodes() length = %v, want %v", len(codes), len(expected))
		return
	}

	for i, code := range expected {
		if codes[i] != code {
			t.Errorf("DefaultRetryableStatusCodes()[%d] = %v, want %v", i, codes[i], code)
		}
	}
}

// TestIsRetryableStatusCode tests the IsRetryableStatusCode function.
func TestIsRetryableStatusCode(t *testing.T) {
	tests := []struct {
		code      int
		retryable bool
	}{
		{200, false},
		{201, false},
		{204, false},
		{400, false},
		{401, false},
		{403, false},
		{404, false},
		{422, false},
		{429, true},  // Rate limit
		{500, true},  // Internal server error
		{502, true},  // Bad gateway
		{503, true},  // Service unavailable
		{504, true},  // Gateway timeout
		{599, false}, // Not in default list
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("status_%d", tt.code), func(t *testing.T) {
			result := IsRetryableStatusCode(tt.code, DefaultRetryableStatusCodes())
			if result != tt.retryable {
				t.Errorf("IsRetryableStatusCode(%d) = %v, want %v", tt.code, result, tt.retryable)
			}
		})
	}
}

// TestIsRetryableStatusCode_CustomCodes tests custom retryable status codes.
func TestIsRetryableStatusCode_CustomCodes(t *testing.T) {
	customCodes := []int{408, 429, 503} // Custom list

	tests := []struct {
		code      int
		retryable bool
	}{
		{408, true},  // In custom list
		{429, true},  // In custom list
		{503, true},  // In custom list
		{500, false}, // Not in custom list
		{502, false}, // Not in custom list
		{504, false}, // Not in custom list
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("status_%d", tt.code), func(t *testing.T) {
			result := IsRetryableStatusCode(tt.code, customCodes)
			if result != tt.retryable {
				t.Errorf("IsRetryableStatusCode(%d, customCodes) = %v, want %v", tt.code, result, tt.retryable)
			}
		})
	}
}

// contains is a helper to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
