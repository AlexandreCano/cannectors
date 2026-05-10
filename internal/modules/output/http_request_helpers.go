package output

import (
	"encoding/json"
	"fmt"
	"mime"
	"strings"

	"github.com/cannectors/runtime/internal/recordpath"
	"github.com/cannectors/runtime/internal/template"
)

// validateJSON returns an error when data is empty or not valid JSON.
func validateJSON(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("empty JSON")
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

// isJSONContentType checks whether the module's Content-Type header
// indicates a JSON payload. It uses mime.ParseMediaType so charset and other
// parameters (e.g. "application/json; charset=utf-8") are handled correctly.
func (h *HTTPRequestModule) isJSONContentType() bool {
	contentType := h.headers[headerContentType]
	if contentType == "" {
		contentType = defaultContentType
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}

// truncateString truncates s to maxLen characters, appending "..." when the
// string is shortened. Intended for log output, not user-visible messages.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// getRecordFieldString extracts a string value from a record using dot-notation
// path traversal with array indexing support (e.g. "user.profile.id" or
// "items[0].name"). Returns an empty string when the value is missing.
func getRecordFieldString(record map[string]any, path string) string {
	value, ok := recordpath.Get(record, path)
	if !ok {
		return ""
	}
	return template.ValueToString(value)
}

// requireRecordFieldString extracts a string value the same way as
// getRecordFieldString but treats absent, null, or empty values as errors.
// Used by key resolution (Story 24.12 AC16) where a missing key must surface
// instead of silently dropping a path/query/header parameter.
func requireRecordFieldString(record map[string]any, path string) (string, error) {
	value, ok := recordpath.Get(record, path)
	if !ok {
		return "", fmt.Errorf("field %q is missing in record", path)
	}
	if value == nil {
		return "", fmt.Errorf("field %q is null in record", path)
	}
	str := template.ValueToString(value)
	if str == "" {
		return "", fmt.Errorf("field %q is empty in record", path)
	}
	return str, nil
}
