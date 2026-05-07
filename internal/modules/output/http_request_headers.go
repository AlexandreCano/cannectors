package output

import (
	"github.com/cannectors/runtime/internal/httpclient"
	"github.com/cannectors/runtime/internal/template"
)

// extractHeadersFromRecord extracts header values from record data.
// Supports both HeadersFromRecord (field path lookup) and template syntax ({{record.field}}).
// Validates header names and values per RFC 7230.
func (h *HTTPRequestModule) extractHeadersFromRecord(record map[string]interface{}) map[string]string {
	headers := make(map[string]string)

	for headerName, headerValue := range h.headers {
		value := headerValue
		if template.HasVariables(headerValue) {
			value = h.templateEvaluator.Evaluate(headerValue, record)
			if value == "" {
				continue
			}
		}
		httpclient.TryAddValidHeader(headers, headerName, value)
	}

	// Add headers from keys (paramType=header)
	for _, k := range h.request.Keys {
		if k.paramType == "header" {
			value := getRecordFieldString(record, k.field)
			if value != "" {
				httpclient.TryAddValidHeader(headers, k.paramName, value)
			}
		}
	}

	if len(headers) == 0 {
		return nil
	}
	return headers
}

// buildBaseHeadersMap returns defaults + validated static config headers +
// record-derived headers. Static config headers are validated via
// httpclient.TryAddValidHeader; templated ones are skipped here — they come
// from recordHeaders and have already been validated in
// extractHeadersFromRecord.
func (h *HTTPRequestModule) buildBaseHeadersMap(recordHeaders map[string]string) map[string]string {
	headers := make(map[string]string)
	headers[headerUserAgent] = defaultUserAgent
	headers[headerContentType] = defaultContentType

	for key, value := range h.headers {
		if template.HasVariables(value) {
			continue
		}
		httpclient.TryAddValidHeader(headers, key, value)
	}
	for key, value := range recordHeaders {
		headers[key] = value
	}
	return headers
}
