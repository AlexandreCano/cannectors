package output

import (
	"fmt"

	"github.com/cannectors/runtime/internal/httpclient"
	"github.com/cannectors/runtime/internal/template"
)

// extractHeadersFromRecord extracts header values from record data.
// Supports both HeadersFromRecord (field path lookup) and template syntax ({{record.field}}).
// Validates header names and values per RFC 7230; returns an error on invalid header.
func (h *HTTPRequestModule) extractHeadersFromRecord(record map[string]any) (map[string]string, error) {
	headers := make(map[string]string)

	for headerName, headerValue := range h.headers {
		value := headerValue
		if template.HasVariables(headerValue) {
			value = h.templateEvaluator.Evaluate(headerValue, record)
			if value == "" {
				continue
			}
		}
		if err := httpclient.AddValidatedHeader(headers, headerName, value); err != nil {
			return nil, fmt.Errorf("invalid header %q: %w", headerName, err)
		}
	}

	for _, k := range h.request.Keys {
		if k.paramType == "header" {
			value := getRecordFieldString(record, k.field)
			if value != "" {
				if err := httpclient.AddValidatedHeader(headers, k.paramName, value); err != nil {
					return nil, fmt.Errorf("invalid key header %q: %w", k.paramName, err)
				}
			}
		}
	}

	if len(headers) == 0 {
		return nil, nil
	}
	return headers, nil
}

// buildBaseHeadersMap returns defaults + validated static config headers +
// record-derived headers. Returns error on invalid static header.
func (h *HTTPRequestModule) buildBaseHeadersMap(recordHeaders map[string]string) (map[string]string, error) {
	headers := make(map[string]string)
	headers[headerUserAgent] = defaultUserAgent
	headers[headerContentType] = defaultContentType

	for key, value := range h.headers {
		if template.HasVariables(value) {
			continue
		}
		if err := httpclient.AddValidatedHeader(headers, key, value); err != nil {
			return nil, fmt.Errorf("invalid header %q: %w", key, err)
		}
	}
	for key, value := range recordHeaders {
		headers[key] = value
	}
	return headers, nil
}
