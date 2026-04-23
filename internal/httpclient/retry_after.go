package httpclient

import (
	"errors"
	"net/http"
	"strconv"
	"time"
)

// ParseRetryAfter parses a raw Retry-After header value.
//
// Two formats are accepted (RFC 7231 §7.1.3):
//   - delta-seconds: integer >= 0 (e.g. "0", "120")
//   - HTTP-date   : delegated to net/http.ParseTime, which handles the three
//     formats allowed by RFC 7231 §7.1.1 (RFC1123, RFC850, ANSI C asctime)
//     with the same strictness as net/http itself.
//
// For HTTP-date the returned duration is `time.Until(date)` and may be
// negative (date in the past). The caller is responsible for clamping the
// value (for instance, to 0 for an immediate retry).
//
// Returns (duration, true) when the value is valid (including 0), or
// (0, false) when the format is invalid or the value is empty.
func ParseRetryAfter(value string) (time.Duration, bool) {
	if value == "" {
		return 0, false
	}

	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second, true
	}

	if t, err := http.ParseTime(value); err == nil {
		return time.Until(t), true
	}

	return 0, false
}

// RetryAfterFromError extracts and parses the Retry-After header from an
// error when it wraps an *Error.
//
// Returns (duration, true) when the error carries a valid header,
// (0, false) otherwise (header absent, invalid format, or the error does
// not wrap *Error).
func RetryAfterFromError(err error) (time.Duration, bool) {
	if err == nil {
		return 0, false
	}
	var httpErr *Error
	if !errors.As(err, &httpErr) {
		return 0, false
	}
	return ParseRetryAfter(httpErr.GetRetryAfter())
}
