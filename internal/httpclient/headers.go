package httpclient

import (
	"fmt"
	"log/slog"

	"github.com/cannectors/runtime/internal/logger"
)

// Log messages for header validation (extracted for testability).
const (
	msgInvalidHeaderNameSkipping  = "invalid header name, skipping"
	msgInvalidHeaderValueSkipping = "invalid header value, skipping"
)

// ValidateHeaderName validates an HTTP header name per RFC 7230.
//
// Header names are tokens: visible ASCII characters (0x21..0x7E) excluding
// control characters, whitespace, and ':'. Returns a descriptive error when
// the name is invalid.
func ValidateHeaderName(name string) error {
	if name == "" {
		return fmt.Errorf("header name cannot be empty")
	}
	for _, r := range name {
		if r < 0x21 || r > 0x7E || r == ':' {
			return fmt.Errorf("header name contains invalid character: %q", r)
		}
	}
	return nil
}

// ValidateHeaderValue validates an HTTP header value per RFC 7230.
//
// Values may contain VCHAR, obs-text, and HTAB. Control characters (other
// than HTAB) and CR/LF (header injection risk) are forbidden.
func ValidateHeaderValue(value string) error {
	for _, r := range value {
		if r < 0x20 && r != '\t' {
			return fmt.Errorf("header value contains invalid control character: %q", r)
		}
		if r == '\r' || r == '\n' {
			return fmt.Errorf("header value contains invalid character: %q", r)
		}
	}
	return nil
}

// TryAddValidHeader validates name/value via ValidateHeaderName and
// ValidateHeaderValue, then inserts the entry into headers. On validation
// failure the entry is silently skipped and a warn log is emitted.
//
// This function replaces the tryAddValidHeader helper previously duplicated
// across HTTP modules.
func TryAddValidHeader(headers map[string]string, name, value string) {
	if err := ValidateHeaderName(name); err != nil {
		logger.Warn(msgInvalidHeaderNameSkipping,
			slog.String("header", name),
			slog.String("error", err.Error()),
		)
		return
	}
	if err := ValidateHeaderValue(value); err != nil {
		logger.Warn(msgInvalidHeaderValueSkipping,
			slog.String("header", name),
			slog.String("error", err.Error()),
		)
		return
	}
	headers[name] = value
}
