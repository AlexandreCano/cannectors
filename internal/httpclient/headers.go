package httpclient

import (
	"fmt"

	"golang.org/x/net/http/httpguts"
)

// ValidateHeaderName validates an HTTP header name per RFC 7230 §3.2.6.
//
// Delegates to httpguts.ValidHeaderFieldName, the exact validator used by
// net/http when writing headers on the wire. Anything this function accepts
// will be accepted by net/http; anything rejected here would have been
// rejected at send-time, so TryAddValidHeader can never produce a header
// that breaks the outbound request.
func ValidateHeaderName(name string) error {
	if name == "" {
		return fmt.Errorf("header name cannot be empty")
	}
	if !httpguts.ValidHeaderFieldName(name) {
		return fmt.Errorf("header name is not a valid RFC 7230 token: %q", name)
	}
	return nil
}

// ValidateHeaderValue validates an HTTP header value per RFC 7230 §3.2.6.
//
// Delegates to httpguts.ValidHeaderFieldValue (the same check net/http runs
// internally), which permits VCHAR, obs-text, SP, and HTAB, and rejects CTLs
// (including DEL 0x7F) and CR/LF.
func ValidateHeaderValue(value string) error {
	if !httpguts.ValidHeaderFieldValue(value) {
		return fmt.Errorf("header value is not a valid RFC 7230 field-value")
	}
	return nil
}

// AddValidatedHeader validates name/value via ValidateHeaderName and
// ValidateHeaderValue, then inserts the entry into headers. On validation
// failure it returns an error — invalid headers are NEVER silently skipped.
func AddValidatedHeader(headers map[string]string, name, value string) error {
	if err := ValidateHeaderName(name); err != nil {
		return err
	}
	if err := ValidateHeaderValue(value); err != nil {
		return fmt.Errorf("header %q: %w", name, err)
	}
	headers[name] = value
	return nil
}
