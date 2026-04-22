package output

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/cannectors/runtime/internal/logger"
)

// Maximum size for body preview (1MB) to prevent memory issues with very
// large payloads.
const maxBodyPreviewSize = 1 * 1024 * 1024

// PreviewRequest prepares request previews without actually sending HTTP
// requests. Used in dry-run mode.
//
// Returns one preview per request that would be made:
//   - Batch mode (requestMode="batch"): returns 1 preview for all records.
//   - Single record mode (requestMode="single"): returns N previews.
//
// By default, authentication headers are masked. Set opts.ShowCredentials
// to true to display actual credential values (debugging only).
func (h *HTTPRequestModule) PreviewRequest(records []map[string]interface{}, opts PreviewOptions) ([]RequestPreview, error) {
	if len(records) == 0 {
		return []RequestPreview{}, nil
	}
	if h.request.RequestMode == "single" {
		return h.previewSingleRecordMode(records, opts)
	}
	return h.previewBatchMode(records, opts)
}

func (h *HTTPRequestModule) previewBatchMode(records []map[string]interface{}, opts PreviewOptions) ([]RequestPreview, error) {
	endpoint := h.resolveEndpointForBatch(h.endpoint, records)
	bodyPreview, err := formatJSONPreview(records)
	if err != nil {
		return nil, fmt.Errorf("formatting body preview: %w", err)
	}
	var batchHeaders map[string]string
	if len(records) > 0 {
		batchHeaders = h.extractHeadersFromRecord(records[0])
	}
	headers := h.buildPreviewHeaders(batchHeaders, opts)
	return []RequestPreview{{
		Endpoint:    endpoint,
		Method:      h.method,
		Headers:     headers,
		BodyPreview: bodyPreview,
		RecordCount: len(records),
	}}, nil
}

func (h *HTTPRequestModule) previewSingleRecordMode(records []map[string]interface{}, opts PreviewOptions) ([]RequestPreview, error) {
	previews := make([]RequestPreview, 0, len(records))
	for _, record := range records {
		endpoint := h.resolveEndpointForRecord(record)
		bodyPreview, err := formatJSONPreview(record)
		if err != nil {
			return nil, fmt.Errorf("formatting body preview: %w", err)
		}
		recordHeaders := h.extractHeadersFromRecord(record)
		headers := h.buildPreviewHeaders(recordHeaders, opts)
		previews = append(previews, RequestPreview{
			Endpoint:    endpoint,
			Method:      h.method,
			Headers:     headers,
			BodyPreview: bodyPreview,
			RecordCount: 1,
		})
	}
	return previews, nil
}

// buildPreviewHeaders applies auth masking (or unmasking) on top of the base
// headers map.
func (h *HTTPRequestModule) buildPreviewHeaders(recordHeaders map[string]string, opts PreviewOptions) map[string]string {
	headers := h.buildBaseHeadersMap(recordHeaders)
	if opts.ShowCredentials {
		h.addUnmaskedAuthHeaders(headers)
	} else {
		h.addMaskedAuthHeaders(headers)
	}
	return headers
}

func (h *HTTPRequestModule) addMaskedAuthHeaders(headers map[string]string) {
	if h.authHandler == nil {
		return
	}
	switch h.authHandler.Type() {
	case authTypeAPIKey:
		headers[defaultAPIKeyHeader] = maskValue(authTypeAPIKey)
	case "bearer":
		headers["Authorization"] = "Bearer " + maskValue("token")
	case "basic":
		headers["Authorization"] = "Basic " + maskValue("credentials")
	case "oauth2":
		headers["Authorization"] = "Bearer " + maskValue("oauth2-token")
	default:
		if _, hasAuth := headers["Authorization"]; !hasAuth {
			headers["Authorization"] = maskValue("auth")
		}
	}
}

// addUnmaskedAuthHeaders adds authentication headers with real values by
// running a dry apply through the auth handler. WARNING: exposes credentials.
// If the dry apply fails (mock request construction or auth handler error),
// the function falls back to addMaskedAuthHeaders so the preview still shows
// the auth intent rather than silently omitting the header.
func (h *HTTPRequestModule) addUnmaskedAuthHeaders(headers map[string]string) {
	if h.authHandler == nil {
		return
	}
	ctx := context.Background()
	mockReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.com", nil)
	if err != nil {
		logger.Warn("failed to build mock request for credential preview, falling back to masked headers",
			slog.String("error", err.Error()),
		)
		h.addMaskedAuthHeaders(headers)
		return
	}
	if err := h.authHandler.ApplyAuth(ctx, mockReq); err != nil {
		logger.Warn("failed to apply auth for credential preview, falling back to masked headers",
			slog.String("error", err.Error()),
		)
		h.addMaskedAuthHeaders(headers)
		return
	}
	for key, values := range mockReq.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}
}

func maskValue(valueType string) string {
	return "[MASKED-" + strings.ToUpper(valueType) + "]"
}

// formatJSONPreview formats data as indented JSON. If the result exceeds
// maxBodyPreviewSize, it truncates at the nearest line boundary and appends
// a "(truncated, X bytes total)" marker.
func formatJSONPreview(data interface{}) (string, error) {
	formatted, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", err
	}
	s := string(formatted)
	if len(s) <= maxBodyPreviewSize {
		return s, nil
	}
	truncated := s[:maxBodyPreviewSize]
	if lastNewline := strings.LastIndex(truncated, "\n"); lastNewline > maxBodyPreviewSize-100 {
		truncated = truncated[:lastNewline]
	}
	return truncated + fmt.Sprintf("\n... (truncated, %d bytes total, %d bytes shown)", len(formatted), len(truncated)), nil
}
