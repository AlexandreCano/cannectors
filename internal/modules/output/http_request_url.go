package output

import (
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/cannectors/runtime/internal/logger"
)

// resolveEndpointWithStaticQuery appends the module's static query params to
// an endpoint URL. Templates are left untouched (callers apply them first).
func (h *HTTPRequestModule) resolveEndpointWithStaticQuery(endpoint string) string {
	if len(h.request.QueryParams) == 0 && !HasTemplateVariables(endpoint) {
		return endpoint
	}
	parsedURL, err := url.Parse(endpoint)
	if err != nil {
		logger.Warn("failed to parse endpoint URL for query params",
			slog.String("endpoint", endpoint),
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
func (h *HTTPRequestModule) resolveEndpointForBatch(endpoint string, records []map[string]interface{}) string {
	if len(records) == 0 {
		return h.resolveEndpointWithStaticQuery(endpoint)
	}
	return h.resolveEndpointForRecord(records[0])
}

// resolveEndpointForRecord resolves path parameters, template variables, and
// query params for a single record. Templates ({{record.field}}) are
// evaluated first, then path parameters ({param}) are substituted.
func (h *HTTPRequestModule) resolveEndpointForRecord(record map[string]interface{}) string {
	endpoint := h.endpoint
	if HasTemplateVariables(endpoint) {
		endpoint = h.templateEvaluator.EvaluateTemplateForURL(endpoint, record)
	}

	for _, k := range h.request.Keys {
		if k.paramType == "path" {
			value := getFieldValue(record, k.field)
			if value != "" {
				placeholder := "{" + k.paramName + "}"
				endpoint = strings.ReplaceAll(endpoint, placeholder, url.PathEscape(value))
			}
		}
	}

	parsedURL, err := url.Parse(endpoint)
	if err != nil {
		logger.Warn("failed to parse endpoint URL for record",
			slog.String("endpoint", endpoint),
			slog.String("error", err.Error()),
		)
		return endpoint
	}

	q := parsedURL.Query()
	for param, value := range h.request.QueryParams {
		q.Set(param, value)
	}
	for _, k := range h.request.Keys {
		if k.paramType == "query" {
			value := getFieldValue(record, k.field)
			if value != "" {
				q.Set(k.paramName, value)
			}
		}
	}
	parsedURL.RawQuery = q.Encode()
	finalURL := parsedURL.String()

	if err := validateURL(finalURL); err != nil {
		logger.Warn("invalid URL after template evaluation",
			slog.String("url", finalURL),
			slog.String("error", err.Error()),
		)
	}
	return finalURL
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
