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
