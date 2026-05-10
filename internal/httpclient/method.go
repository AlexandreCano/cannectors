package httpclient

import (
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/http/httpguts"
)

// NormalizeAndValidateMethod uppercases the method and validates that it is a
// well-formed RFC 7230 token. Empty input falls back to GET. Any HTTP method
// (GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS, custom verbs) is accepted as
// long as it consists only of token characters per RFC 7230 §3.2.6.
func NormalizeAndValidateMethod(method string) (string, error) {
	m := strings.TrimSpace(method)
	if m == "" {
		return "GET", nil
	}
	m = strings.ToUpper(m)
	if !httpguts.ValidHeaderFieldName(m) {
		return "", fmt.Errorf("invalid HTTP method: %q (must be an RFC 7230 token)", method)
	}
	return m, nil
}

// ValidateAbsoluteURL parses raw and ensures it is an absolute URL with a
// scheme (http/https) and a non-empty host. Use after template resolution to
// catch bad concatenations or unresolved placeholders that produced an
// invalid request URL.
func ValidateAbsoluteURL(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("URL is empty")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid URL %q: %w", SanitizeURL(raw), err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("invalid URL %q: scheme must be http or https", SanitizeURL(raw))
	}
	if u.Host == "" {
		return fmt.Errorf("invalid URL %q: host is empty", SanitizeURL(raw))
	}
	return nil
}
