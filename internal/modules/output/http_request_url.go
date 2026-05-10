package output

import (
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/cannectors/runtime/internal/httpclient"
	"github.com/cannectors/runtime/internal/logger"
	"github.com/cannectors/runtime/internal/template"
)

// resolveEndpointWithStaticQuery appends the module's static query params to
// an endpoint URL. Templates are left untouched (callers apply them first):
// when there are no static query params to append, the endpoint is returned
// verbatim so that template markers like `{{record.id}}` are not
// percent-encoded by url.Parse / url.String round-tripping.
func (h *HTTPRequestModule) resolveEndpointWithStaticQuery(endpoint string) string {
	if len(h.request.QueryParams) == 0 {
		return endpoint
	}
	parsedURL, err := url.Parse(endpoint)
	if err != nil {
		logger.Warn("failed to parse endpoint URL for query params",
			slog.String("endpoint", httpclient.SanitizeURL(endpoint)),
			slog.String("error", err.Error()),
		)
		return endpoint
	}
	q := parsedURL.Query()
	for param, value := range h.request.QueryParams {
		q.Set(param, value)
	}
	parsedURL.RawQuery = q.Encode()
	return parsedURL.String()
}

// resolveEndpointForBatch resolves template variables and keys in the
// endpoint for batch mode. Uses the first record for template evaluation
// and key extraction.
func (h *HTTPRequestModule) resolveEndpointForBatch(endpoint string, records []map[string]any) (string, error) {
	if len(records) == 0 {
		return h.resolveEndpointWithStaticQuery(endpoint), nil
	}
	return h.resolveEndpointForRecord(records[0])
}

// resolveEndpointForRecord resolves path parameters, template variables, and
// query params for a single record. Templates ({{record.field}}) are
// evaluated first, then path parameters ({param}) are substituted.
// Returns an error when a configured key references a missing/null/empty
// record field (Story 24.12 AC16).
func (h *HTTPRequestModule) resolveEndpointForRecord(record map[string]any) (string, error) {
	endpoint := h.endpoint
	if template.HasVariables(endpoint) {
		endpoint = h.templateEvaluator.EvaluateForURL(endpoint, record)
	}

	for _, k := range h.request.Keys {
		if k.paramType == "path" {
			value, err := requireRecordFieldString(record, k.field)
			if err != nil {
				return "", fmt.Errorf("path key %q: %w", k.paramName, err)
			}
			placeholder := "{" + k.paramName + "}"
			endpoint = strings.ReplaceAll(endpoint, placeholder, url.PathEscape(value))
		}
	}

	parsedURL, err := url.Parse(endpoint)
	if err != nil {
		logger.Warn("failed to parse endpoint URL for record",
			slog.String("endpoint", httpclient.SanitizeURL(endpoint)),
			slog.String("error", err.Error()),
		)
		return endpoint, nil
	}

	q := parsedURL.Query()
	for param, value := range h.request.QueryParams {
		q.Set(param, value)
	}
	for _, k := range h.request.Keys {
		if k.paramType == "query" {
			value, err := requireRecordFieldString(record, k.field)
			if err != nil {
				return "", fmt.Errorf("query key %q: %w", k.paramName, err)
			}
			q.Set(k.paramName, value)
		}
	}
	parsedURL.RawQuery = q.Encode()
	finalURL := parsedURL.String()

	if err := validateURL(finalURL); err != nil {
		logger.Warn("invalid URL after template evaluation",
			slog.String("url", httpclient.SanitizeURL(finalURL)),
			slog.String("error", err.Error()),
		)
	}
	return finalURL, nil
}

// validateURL validates that a URL string is well-formed.
func validateURL(urlStr string) error {
	if urlStr == "" {
		return fmt.Errorf("empty URL")
	}
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}
	if parsed.Scheme == "" {
		return fmt.Errorf("URL missing scheme")
	}
	if parsed.Scheme == "http" || parsed.Scheme == "https" {
		if parsed.Host == "" {
			return fmt.Errorf("URL missing host")
		}
	}
	return nil
}
