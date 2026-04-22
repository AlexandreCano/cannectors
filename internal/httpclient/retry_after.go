package httpclient

import (
	"errors"
	"strconv"
	"time"
)

// httpDateFormats lists the date formats accepted for an HTTP-date
// (RFC 7231 §7.1.1). Order follows the RFC: RFC1123 (plus numeric-TZ
// variant), RFC850, ANSI C asctime.
var httpDateFormats = []string{
	time.RFC1123,
	time.RFC1123Z,
	time.RFC850,
	"Mon Jan _2 15:04:05 2006", // ANSI C asctime format
}

// ParseRetryAfter parses a raw Retry-After header value.
//
// Two formats are accepted (RFC 7231 §7.1.3):
//   - delta-seconds: integer >= 0 (e.g. "0", "120")
//   - HTTP-date   : RFC1123 / RFC850 / ANSI C asctime
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

	for _, format := range httpDateFormats {
		if t, err := time.Parse(format, value); err == nil {
			return time.Until(t), true
		}
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
