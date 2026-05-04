package output

import (
	"encoding/json"
	"fmt"
	"mime"
	"strings"

	"github.com/cannectors/runtime/internal/moduleconfig"
	"github.com/cannectors/runtime/internal/template"
)

// validateJSON returns an error when data is empty or not valid JSON.
func validateJSON(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("empty JSON")
	}
	var v interface{}
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
func getRecordFieldString(record map[string]interface{}, path string) string {
	value, ok := moduleconfig.GetNestedValue(record, path)
	if !ok {
		return ""
	}
	return template.ValueToString(value)
}
