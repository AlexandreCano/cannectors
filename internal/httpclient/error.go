package httpclient

import (
	"fmt"
	"net/http"
)

// Error represents an HTTP error (status >= 400) emitted by a module.
//
// It unifies the two HTTPError types previously duplicated in
// input/http_polling.go and output/http_request.go: this unified form takes
// the superset of fields (ResponseBody and ResponseHeaders, absent from the
// input module, are simply left at their zero value when unavailable).
//
// The type is compatible with errors.As: callers that need the original
// error use `var httpErr *httpclient.Error; errors.As(err, &httpErr)`.
type Error struct {
	// StatusCode is the HTTP status code returned by the server (e.g. 500).
	StatusCode int

	// Status is the full status line (e.g. "500 Internal Server Error").
	Status string

	// Endpoint is the target URL of the request.
	Endpoint string

	// Method is the HTTP method used (GET, POST, ...). May be empty when
	// the caller does not carry the information.
	Method string

	// Message is a short description or truncated response body.
	Message string

	// ResponseBody is the full response body (may be empty).
	ResponseBody string

	// ResponseHeaders holds the response headers. Used notably for
	// Retry-After extraction. May be nil.
	ResponseHeaders http.Header
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Method != "" {
		return fmt.Sprintf("http error %d (%s) %s %s: %s", e.StatusCode, e.Status, e.Method, e.Endpoint, e.Message)
	}
	return fmt.Sprintf("http error %d (%s) from %s: %s", e.StatusCode, e.Status, e.Endpoint, e.Message)
}

// GetRetryAfter returns the raw Retry-After header value when present, or an
// empty string otherwise. Use ParseRetryAfter to obtain a time.Duration.
func (e *Error) GetRetryAfter() string {
	if e == nil || e.ResponseHeaders == nil {
		return ""
	}
	return e.ResponseHeaders.Get("Retry-After")
}
