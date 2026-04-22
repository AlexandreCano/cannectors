package input

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/cannectors/runtime/internal/logger"
)

// fetchWithPagination dispatches to the correct pagination strategy based on
// the module configuration. Non-paginated endpoints are handled by
// fetchSingle directly, not through this file.
func (h *HTTPPolling) fetchWithPagination(ctx context.Context) ([]map[string]interface{}, error) {
	baseEndpoint, err := h.buildEndpointWithState(h.endpoint)
	if err != nil {
		return nil, fmt.Errorf("building endpoint with state: %w", err)
	}
	switch h.pagination.Type {
	case "page":
		return h.fetchPageBased(ctx, baseEndpoint)
	case "offset":
		return h.fetchOffsetBased(ctx, baseEndpoint)
	case "cursor":
		return h.fetchCursorBased(ctx, baseEndpoint)
	default:
		return h.fetchSingle(ctx, baseEndpoint)
	}
}

func (h *HTTPPolling) fetchPageBased(ctx context.Context, baseEndpoint string) ([]map[string]interface{}, error) {
	var allRecords []map[string]interface{}
	page := 1

	logger.Debug(logMsgPaginationStarted,
		"module_type", "httpPolling",
		"pagination_type", "page",
		"page_param", h.pagination.PageParam,
	)

	for page <= maxPaginationPages {
		pageURL, err := h.buildPaginatedURLFrom(baseEndpoint, h.pagination.PageParam, strconv.Itoa(page))
		if err != nil {
			return nil, err
		}
		records, totalPages, err := h.fetchPageWithMeta(ctx, pageURL, h.pagination.TotalPagesField)
		if err != nil {
			return nil, err
		}

		logger.Debug(logMsgPaginationPageFetched,
			"module_type", "httpPolling",
			"pagination_type", "page",
			"current_page", page,
			"total_pages", totalPages,
			"records_in_page", len(records),
			"total_records_so_far", len(allRecords)+len(records),
		)

		allRecords = append(allRecords, records...)
		if totalPages > 0 && page >= totalPages {
			break
		}
		if len(records) == 0 {
			break
		}
		page++
	}

	logger.Info(logMsgPaginationCompleted,
		"module_type", "httpPolling",
		"pagination_type", "page",
		"pages_fetched", page,
		"total_records", len(allRecords),
	)
	return allRecords, nil
}

func (h *HTTPPolling) fetchOffsetBased(ctx context.Context, baseEndpoint string) ([]map[string]interface{}, error) {
	var allRecords []map[string]interface{}
	offset := 0
	limit := h.pagination.Limit
	if limit == 0 {
		limit = 100
	}
	pageNum := 0

	logger.Debug(logMsgPaginationStarted,
		"module_type", "httpPolling",
		"pagination_type", "offset",
		"offset_param", h.pagination.OffsetParam,
		"limit_param", h.pagination.LimitParam,
		"limit", limit,
	)

	for offset < maxPaginationPages*limit {
		pageNum++
		offsetURL, err := h.buildPaginatedURLMultiFrom(baseEndpoint, map[string]string{
			h.pagination.OffsetParam: strconv.Itoa(offset),
			h.pagination.LimitParam:  strconv.Itoa(limit),
		})
		if err != nil {
			return nil, err
		}
		records, total, err := h.fetchOffsetWithMeta(ctx, offsetURL, h.pagination.TotalField)
		if err != nil {
			return nil, err
		}

		logger.Debug(logMsgPaginationPageFetched,
			"module_type", "httpPolling",
			"pagination_type", "offset",
			"current_offset", offset,
			"limit", limit,
			"total_available", total,
			"records_in_page", len(records),
			"total_records_so_far", len(allRecords)+len(records),
		)

		allRecords = append(allRecords, records...)
		if total > 0 && len(allRecords) >= total {
			break
		}
		if len(records) == 0 || len(records) < limit {
			break
		}
		offset += limit
	}

	logger.Info(logMsgPaginationCompleted,
		"module_type", "httpPolling",
		"pagination_type", "offset",
		"pages_fetched", pageNum,
		"total_records", len(allRecords),
	)
	return allRecords, nil
}

func (h *HTTPPolling) fetchCursorBased(ctx context.Context, baseEndpoint string) ([]map[string]interface{}, error) {
	var allRecords []map[string]interface{}
	cursor := ""
	iterations := 0

	logger.Debug(logMsgPaginationStarted,
		"module_type", "httpPolling",
		"pagination_type", "cursor",
		"cursor_param", h.pagination.CursorParam,
	)

	for iterations < maxPaginationPages {
		var fetchURL string
		var err error
		if cursor == "" {
			fetchURL = baseEndpoint
		} else {
			fetchURL, err = h.buildPaginatedURLFrom(baseEndpoint, h.pagination.CursorParam, cursor)
			if err != nil {
				return nil, err
			}
		}
		records, nextCursor, err := h.fetchCursorWithMeta(ctx, fetchURL, h.pagination.NextCursorField)
		if err != nil {
			return nil, err
		}

		logger.Debug(logMsgPaginationPageFetched,
			"module_type", "httpPolling",
			"pagination_type", "cursor",
			"iteration", iterations+1,
			"has_next_cursor", nextCursor != "",
			"records_in_page", len(records),
			"total_records_so_far", len(allRecords)+len(records),
		)

		allRecords = append(allRecords, records...)
		if nextCursor == "" {
			break
		}
		cursor = nextCursor
		iterations++
	}

	logger.Info(logMsgPaginationCompleted,
		"module_type", "httpPolling",
		"pagination_type", "cursor",
		"iterations", iterations+1,
		"total_records", len(allRecords),
	)
	return allRecords, nil
}

func (h *HTTPPolling) fetchPageWithMeta(ctx context.Context, endpoint, totalPagesField string) ([]map[string]interface{}, int, error) {
	obj, err := h.fetchAndParseObject(ctx, endpoint)
	if err != nil {
		return nil, 0, err
	}
	records, err := h.extractRecordsFromObject(obj)
	if err != nil {
		return nil, 0, err
	}
	return records, extractIntField(obj, totalPagesField), nil
}

func (h *HTTPPolling) fetchOffsetWithMeta(ctx context.Context, endpoint, totalField string) ([]map[string]interface{}, int, error) {
	obj, err := h.fetchAndParseObject(ctx, endpoint)
	if err != nil {
		return nil, 0, err
	}
	records, err := h.extractRecordsFromObject(obj)
	if err != nil {
		return nil, 0, err
	}
	return records, extractIntField(obj, totalField), nil
}

func (h *HTTPPolling) fetchCursorWithMeta(ctx context.Context, endpoint, nextCursorField string) ([]map[string]interface{}, string, error) {
	obj, err := h.fetchAndParseObject(ctx, endpoint)
	if err != nil {
		return nil, "", err
	}
	records, err := h.extractRecordsFromObject(obj)
	if err != nil {
		return nil, "", err
	}
	return records, extractStringField(obj, nextCursorField), nil
}

func (h *HTTPPolling) fetchAndParseObject(ctx context.Context, endpoint string) (map[string]interface{}, error) {
	body, err := h.doRequestWithRetry(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrJSONParse, err)
	}
	return result, nil
}

func extractIntField(obj map[string]interface{}, field string) int {
	if field == "" {
		return 0
	}
	if val, ok := obj[field].(float64); ok {
		return int(val)
	}
	return 0
}

func extractStringField(obj map[string]interface{}, field string) string {
	if field == "" {
		return ""
	}
	if val, ok := obj[field].(string); ok {
		return val
	}
	return ""
}

func (h *HTTPPolling) extractRecordsFromObject(obj map[string]interface{}) ([]map[string]interface{}, error) {
	if h.dataField != "" {
		return h.extractDataFromField(obj, h.dataField)
	}
	commonFields := []string{"data", "items", "results", "records"}
	for _, field := range commonFields {
		if data, ok := obj[field]; ok {
			if converted, err := h.convertToRecords(data); err == nil {
				return converted, nil
			}
		}
	}
	return nil, fmt.Errorf("%w: pagination requires dataField when response is object (tried common fields: %v)", ErrInvalidDataField, commonFields)
}

// buildPaginatedURLFrom adds a single query parameter to baseURL.
func (h *HTTPPolling) buildPaginatedURLFrom(baseURL, param, value string) (string, error) {
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf(errMsgParsingEndpointURL, err)
	}
	q := parsedURL.Query()
	q.Set(param, value)
	parsedURL.RawQuery = q.Encode()
	return parsedURL.String(), nil
}

// buildPaginatedURLMultiFrom adds multiple query parameters to baseURL.
func (h *HTTPPolling) buildPaginatedURLMultiFrom(baseURL string, params map[string]string) (string, error) {
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf(errMsgParsingEndpointURL, err)
	}
	q := parsedURL.Query()
	for param, value := range params {
		q.Set(param, value)
	}
	parsedURL.RawQuery = q.Encode()
	return parsedURL.String(), nil
}
