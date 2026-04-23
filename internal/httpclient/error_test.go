package httpclient

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestError_Error_WithMethod(t *testing.T) {
	e := &Error{
		StatusCode: 500,
		Status:     "500 Internal Server Error",
		Endpoint:   "https://api.example.com/x",
		Method:     "POST",
		Message:    "boom",
	}
	got := e.Error()
	for _, want := range []string{"500", "Internal Server Error", "POST", "api.example.com/x", "boom"} {
		if !strings.Contains(got, want) {
			t.Errorf("Error() = %q, missing %q", got, want)
		}
	}
}

func TestError_Error_WithoutMethod(t *testing.T) {
	e := &Error{
		StatusCode: 404,
		Status:     "404 Not Found",
		Endpoint:   "https://api.example.com/y",
		Message:    "nope",
	}
	got := e.Error()
	if !strings.Contains(got, "from https://api.example.com/y") {
		t.Errorf("Error() = %q, expected `from <endpoint>` when method is empty", got)
	}
}

func TestError_GetRetryAfter(t *testing.T) {
	tests := []struct {
		name    string
		headers http.Header
		want    string
	}{
		{"nil headers", nil, ""},
		{"absent", http.Header{}, ""},
		{"present", http.Header{"Retry-After": []string{"120"}}, "120"},
		{"http-date", http.Header{"Retry-After": []string{"Fri, 31 Dec 1999 23:59:59 GMT"}}, "Fri, 31 Dec 1999 23:59:59 GMT"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := &Error{ResponseHeaders: tc.headers}
			if got := e.GetRetryAfter(); got != tc.want {
				t.Errorf("GetRetryAfter() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestError_GetRetryAfter_NilReceiver(t *testing.T) {
	var e *Error
	if got := e.GetRetryAfter(); got != "" {
		t.Errorf("GetRetryAfter() on nil = %q, want empty", got)
	}
}

func TestError_Error_SanitizesEndpointQueryParams(t *testing.T) {
	// Secrets embedded in query params (api_key, access_token, ...) must never
	// reach the formatted error string — it may be logged or surfaced to users.
	e := &Error{
		StatusCode: 500,
		Status:     "500 Internal Server Error",
		Endpoint:   "https://api.example.com/users?api_key=super-secret-xyz&access_token=token-123",
		Method:     "GET",
		Message:    "boom",
	}
	got := e.Error()
	for _, secret := range []string{"super-secret-xyz", "token-123", "api_key=", "access_token="} {
		if strings.Contains(got, secret) {
			t.Errorf("Error() leaked sensitive query param %q: %s", secret, got)
		}
	}
	if !strings.Contains(got, "https://api.example.com/users") {
		t.Errorf("Error() should still contain sanitized endpoint path, got: %s", got)
	}
}

func TestError_Error_SanitizesEndpointFragment(t *testing.T) {
	e := &Error{
		StatusCode: 404,
		Status:     "404 Not Found",
		Endpoint:   "https://api.example.com/resource#access_token=frag-secret",
		Message:    "nope",
	}
	got := e.Error()
	if strings.Contains(got, "frag-secret") {
		t.Errorf("Error() leaked fragment secret: %s", got)
	}
	if strings.Contains(got, "#") {
		t.Errorf("Error() should strip fragment, got: %s", got)
	}
}

func TestSanitizeURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"no query", "https://api.example.com/x", "https://api.example.com/x"},
		{"with query", "https://api.example.com/x?api_key=secret", "https://api.example.com/x"},
		{"with fragment", "https://api.example.com/x#token=abc", "https://api.example.com/x"},
		{"with both", "https://api.example.com/x?k=v#f=1", "https://api.example.com/x"},
		{"invalid", "://not a url", "[invalid URL]"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := SanitizeURL(tc.in); got != tc.want {
				t.Errorf("SanitizeURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestError_ErrorsAs(t *testing.T) {
	orig := &Error{
		StatusCode: 429,
		Status:     "429 Too Many Requests",
		Endpoint:   "https://api.example.com/z",
		Method:     "GET",
	}
	wrapped := fmt.Errorf("request failed: %w", orig)

	var got *Error
	if !errors.As(wrapped, &got) {
		t.Fatal("errors.As failed to extract *Error")
	}
	if got.StatusCode != 429 {
		t.Errorf("StatusCode = %d, want 429", got.StatusCode)
	}
}
