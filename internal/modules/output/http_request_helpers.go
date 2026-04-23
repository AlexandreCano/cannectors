package output

import (
	"encoding/json"
	"fmt"
	"mime"
	"strings"
)

// MetadataFieldName is the reserved field for record metadata. It is stripped
// from request bodies to avoid leaking internal data downstream.
// This mirrors runtime.DefaultMetadataFieldName; it is re-declared here to
// avoid an import cycle.
const MetadataFieldName = "_metadata"

// stripMetadataFromRecords returns a copy of records with the _metadata field
// removed from each entry.
func stripMetadataFromRecords(records []map[string]interface{}) []map[string]interface{} {
	result := make([]map[string]interface{}, len(records))
	for i, record := range records {
		result[i] = stripMetadataFromRecord(record)
	}
	return result
}

// stripMetadataFromRecord returns a copy of record without the _metadata
// field. Returns the original record untouched when _metadata is absent.
func stripMetadataFromRecord(record map[string]interface{}) map[string]interface{} {
	if record == nil {
		return record
	}
	if _, exists := record[MetadataFieldName]; !exists {
		return record
	}
	result := make(map[string]interface{}, len(record)-1)
	for key, value := range record {
		if key != MetadataFieldName {
			result[key] = value
		}
	}
	return result
}

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

// getFieldValue extracts a string value from a record using dot-notation path
// traversal (e.g. "user.profile.id"). Returns an empty string when any
// intermediate value is missing or not traversable.
func getFieldValue(record map[string]interface{}, path string) string {
	parts := strings.Split(path, ".")
	current := interface{}(record)

	for _, part := range parts {
		m, ok := current.(map[string]interface{})
		if !ok || m == nil {
			return ""
		}
		val, exists := m[part]
		if !exists {
			return ""
		}
		current = val
	}

	switch v := current.(type) {
	case nil:
		return ""
	case string:
		return v
	case float64:
		return fmt.Sprintf("%v", v)
	case int:
		return fmt.Sprintf("%d", v)
	case bool:
		return fmt.Sprintf("%t", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}
