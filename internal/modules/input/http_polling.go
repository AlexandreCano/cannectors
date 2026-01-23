// Package input provides implementations for input modules.
// Input modules are responsible for fetching data from source systems.
package input

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/canectors/runtime/internal/auth"
	"github.com/canectors/runtime/internal/logger"
	"github.com/canectors/runtime/pkg/connector"
)

// Default configuration values
const (
	defaultTimeout     = 30 * time.Second
	defaultUserAgent   = "Canectors-Runtime/1.0"
	maxPaginationPages = 1000 // Prevent infinite loops
)

// Error types for HTTP polling module
var (
	ErrNilConfig        = errors.New("module configuration is nil")
	ErrMissingEndpoint  = errors.New("endpoint is required in module configuration")
	ErrHTTPRequest      = errors.New("http request failed")
	ErrJSONParse        = errors.New("failed to parse JSON response")
	ErrInvalidDataField = errors.New("dataField does not contain an array")
)

// HTTPError represents an HTTP error with status code and context
type HTTPError struct {
	StatusCode int
	Status     string
	Endpoint   string
	Message    string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("http error %d (%s) from %s: %s", e.StatusCode, e.Status, e.Endpoint, e.Message)
}

// PaginationConfig holds pagination configuration
type PaginationConfig struct {
	Type            string // "page", "offset", "cursor"
	PageParam       string
	TotalPagesField string
	OffsetParam     string
	LimitParam      string
	Limit           int
	TotalField      string
	CursorParam     string
	NextCursorField string
}

// HTTPPolling implements polling-based HTTP data fetching.
// It supports HTTP GET requests with authentication and pagination.
type HTTPPolling struct {
	endpoint    string
	headers     map[string]string
	timeout     time.Duration
	dataField   string
	pagination  *PaginationConfig
	authHandler auth.Handler
	client      *http.Client
}

// NewHTTPPollingFromConfig creates a new HTTP polling input module from configuration.
// This is the primary constructor that parses ModuleConfig and creates a ready-to-use module.
//
// Required config fields:
//   - endpoint: The HTTP endpoint URL
//
// Optional config fields:
//   - headers: Custom HTTP headers (map[string]string)
//   - timeout: Request timeout in seconds (float64, default 30)
//   - dataField: JSON field containing the array of records (for object responses)
//   - pagination: Pagination configuration (map with type, params, etc.)
func NewHTTPPollingFromConfig(config *connector.ModuleConfig) (*HTTPPolling, error) {
	if config == nil {
		return nil, ErrNilConfig
	}

	// Extract endpoint (required)
	endpoint, ok := config.Config["endpoint"].(string)
	if !ok || endpoint == "" {
		return nil, ErrMissingEndpoint
	}

	// Extract timeout (optional, default 30s)
	timeout := defaultTimeout
	if timeoutVal, ok := config.Config["timeout"].(float64); ok {
		timeout = time.Duration(timeoutVal * float64(time.Second))
	}

	// Extract headers (optional)
	headers := make(map[string]string)
	if headersVal, ok := config.Config["headers"].(map[string]interface{}); ok {
		for k, v := range headersVal {
			if strVal, ok := v.(string); ok {
				headers[k] = strVal
			}
		}
	}

	// Extract dataField (optional)
	dataField, _ := config.Config["dataField"].(string)

	// Extract pagination config (optional)
	var pagination *PaginationConfig
	if paginationVal, ok := config.Config["pagination"].(map[string]interface{}); ok {
		pagination = parsePaginationConfig(paginationVal)
	}

	// Create HTTP client with configured timeout
	client := &http.Client{
		Timeout: timeout,
	}

	// Create authentication handler if configured
	authHandler, err := auth.NewHandler(config.Authentication, client)
	if err != nil {
		return nil, fmt.Errorf("creating auth handler: %w", err)
	}

	h := &HTTPPolling{
		endpoint:    endpoint,
		headers:     headers,
		timeout:     timeout,
		dataField:   dataField,
		pagination:  pagination,
		authHandler: authHandler,
		client:      client,
	}

	logger.Debug("http polling module created",
		"endpoint", endpoint,
		"timeout", timeout.String(),
		"has_auth", authHandler != nil,
		"has_pagination", pagination != nil,
	)

	return h, nil
}

// parsePaginationConfig extracts pagination configuration from map
func parsePaginationConfig(config map[string]interface{}) *PaginationConfig {
	p := &PaginationConfig{}

	if t, ok := config["type"].(string); ok {
		p.Type = t
	}

	// Page-based pagination
	if param, ok := config["pageParam"].(string); ok {
		p.PageParam = param
	}
	if field, ok := config["totalPagesField"].(string); ok {
		p.TotalPagesField = field
	}

	// Offset-based pagination
	if param, ok := config["offsetParam"].(string); ok {
		p.OffsetParam = param
	}
	if param, ok := config["limitParam"].(string); ok {
		p.LimitParam = param
	}
	if limit, ok := config["limit"].(float64); ok {
		p.Limit = int(limit)
	}
	if field, ok := config["totalField"].(string); ok {
		p.TotalField = field
	}

	// Cursor-based pagination
	if param, ok := config["cursorParam"].(string); ok {
		p.CursorParam = param
	}
	if field, ok := config["nextCursorField"].(string); ok {
		p.NextCursorField = field
	}

	return p
}

// Fetch retrieves data via HTTP polling.
// It executes HTTP GET requests to the configured endpoint, handles authentication,
// and aggregates paginated results into a single slice of records.
//
// Returns:
//   - []map[string]interface{}: The fetched records
//   - error: Any error encountered during fetching
func (h *HTTPPolling) Fetch() ([]map[string]interface{}, error) {
	ctx := context.Background()
	startTime := time.Now()

	// Log fetch start with configuration summary
	logger.Info("input fetch started",
		"module_type", "httpPolling",
		"endpoint", h.endpoint,
		"timeout", h.timeout.String(),
		"has_pagination", h.pagination != nil,
		"has_auth", h.authHandler != nil,
	)

	var records []map[string]interface{}
	var err error

	// Handle pagination if configured
	if h.pagination != nil {
		records, err = h.fetchWithPagination(ctx)
	} else {
		// Single request without pagination
		records, err = h.fetchSingle(ctx, h.endpoint)
	}

	duration := time.Since(startTime)

	if err != nil {
		logger.Error("input fetch failed",
			"module_type", "httpPolling",
			"endpoint", h.endpoint,
			"duration", duration,
			"error", err.Error(),
		)
		return nil, err
	}

	// Log successful completion with metrics
	logger.Info("input fetch completed",
		"module_type", "httpPolling",
		"endpoint", h.endpoint,
		"record_count", len(records),
		"duration", duration,
		"has_pagination", h.pagination != nil,
	)

	return records, nil
}

// doRequest executes an HTTP GET request and returns the raw response body
func (h *HTTPPolling) doRequest(ctx context.Context, endpoint string) ([]byte, error) {
	requestStart := time.Now()

	logger.Debug("http request started",
		"module_type", "httpPolling",
		"endpoint", endpoint,
		"method", http.MethodGet,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		logger.Error("http request creation failed",
			"module_type", "httpPolling",
			"endpoint", endpoint,
			"error", err.Error(),
		)
		return nil, fmt.Errorf("creating http request: %w", err)
	}

	req.Header.Set("User-Agent", defaultUserAgent)
	for key, value := range h.headers {
		req.Header.Set(key, value)
	}

	if err = h.applyAuthentication(ctx, req); err != nil {
		logger.Error("authentication failed",
			"module_type", "httpPolling",
			"endpoint", endpoint,
			"error", err.Error(),
		)
		return nil, fmt.Errorf("applying authentication: %w", err)
	}

	resp, err := h.client.Do(req)
	requestDuration := time.Since(requestStart)

	if err != nil {
		logger.Error("http request failed",
			"module_type", "httpPolling",
			"endpoint", endpoint,
			"method", http.MethodGet,
			"duration", requestDuration,
			"error", err.Error(),
		)
		return nil, fmt.Errorf("%w: %w", ErrHTTPRequest, err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			logger.Warn("failed to close response body",
				"endpoint", endpoint,
				"error", closeErr.Error(),
			)
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("response body read failed",
			"module_type", "httpPolling",
			"endpoint", endpoint,
			"status_code", resp.StatusCode,
			"error", err.Error(),
		)
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		// Truncate response body for logging (max 500 chars)
		bodySnippet := string(body)
		if len(bodySnippet) > 500 {
			bodySnippet = bodySnippet[:500] + "..."
		}

		logger.Error("http error response",
			"module_type", "httpPolling",
			"endpoint", endpoint,
			"method", http.MethodGet,
			"status_code", resp.StatusCode,
			"status", resp.Status,
			"duration", requestDuration,
			"response_body", bodySnippet,
		)

		// If we get 401 Unauthorized with OAuth2 auth, invalidate the token
		// to force refresh on next request (token may have been revoked server-side)
		if resp.StatusCode == http.StatusUnauthorized && h.authHandler != nil {
			if invalidator, ok := h.authHandler.(interface{ InvalidateToken() }); ok {
				logger.Debug("401 Unauthorized with OAuth2, invalidating cached token",
					"endpoint", endpoint,
				)
				invalidator.InvalidateToken()
			}
		}

		return nil, &HTTPError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Endpoint:   endpoint,
			Message:    string(body),
		}
	}

	// Log successful response
	logger.Debug("http request completed",
		"module_type", "httpPolling",
		"endpoint", endpoint,
		"method", http.MethodGet,
		"status_code", resp.StatusCode,
		"duration", requestDuration,
		"response_size", len(body),
	)

	return body, nil
}

// fetchSingle executes a single HTTP GET request and returns the records
func (h *HTTPPolling) fetchSingle(ctx context.Context, endpoint string) ([]map[string]interface{}, error) {
	body, err := h.doRequest(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	return h.parseResponse(body)
}

// parseResponse parses JSON response and extracts records
func (h *HTTPPolling) parseResponse(body []byte) ([]map[string]interface{}, error) {
	// Try parsing as array first
	var arrayResult []map[string]interface{}
	if err := json.Unmarshal(body, &arrayResult); err == nil {
		return arrayResult, nil
	}

	// Try parsing as object
	var objectResult map[string]interface{}
	if err := json.Unmarshal(body, &objectResult); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrJSONParse, err)
	}

	// If dataField is specified, extract array from that field
	if h.dataField != "" {
		return h.extractDataFromField(objectResult, h.dataField)
	}

	// Try common data field names
	for _, field := range []string{"data", "items", "results", "records"} {
		if data, ok := objectResult[field]; ok {
			if records, err := h.convertToRecords(data); err == nil {
				return records, nil
			}
		}
	}

	// Return single object as single-element array
	return []map[string]interface{}{objectResult}, nil
}

// extractDataFromField extracts array data from a specific field in the response object
func (h *HTTPPolling) extractDataFromField(obj map[string]interface{}, field string) ([]map[string]interface{}, error) {
	data, ok := obj[field]
	if !ok {
		return nil, fmt.Errorf("%w: field '%s' not found", ErrInvalidDataField, field)
	}

	return h.convertToRecords(data)
}

// convertToRecords converts interface{} to []map[string]interface{}
func (h *HTTPPolling) convertToRecords(data interface{}) ([]map[string]interface{}, error) {
	switch v := data.(type) {
	case []interface{}:
		records := make([]map[string]interface{}, 0, len(v))
		for _, item := range v {
			if record, ok := item.(map[string]interface{}); ok {
				records = append(records, record)
			}
		}
		return records, nil
	case []map[string]interface{}:
		return v, nil
	default:
		return nil, fmt.Errorf("%w: expected array, got %T", ErrInvalidDataField, data)
	}
}

// applyAuthentication applies authentication to the HTTP request using the shared auth package
func (h *HTTPPolling) applyAuthentication(ctx context.Context, req *http.Request) error {
	if h.authHandler == nil {
		return nil
	}
	return h.authHandler.ApplyAuth(ctx, req)
}

// fetchWithPagination handles paginated requests
func (h *HTTPPolling) fetchWithPagination(ctx context.Context) ([]map[string]interface{}, error) {
	switch h.pagination.Type {
	case "page":
		return h.fetchPageBased(ctx)
	case "offset":
		return h.fetchOffsetBased(ctx)
	case "cursor":
		return h.fetchCursorBased(ctx)
	default:
		return h.fetchSingle(ctx, h.endpoint)
	}
}

// fetchPageBased handles page-based pagination
func (h *HTTPPolling) fetchPageBased(ctx context.Context) ([]map[string]interface{}, error) {
	var allRecords []map[string]interface{}
	page := 1

	logger.Debug("pagination started",
		"module_type", "httpPolling",
		"pagination_type", "page",
		"page_param", h.pagination.PageParam,
	)

	for page <= maxPaginationPages {
		// Build URL with page parameter
		pageURL, err := h.buildPaginatedURL(h.pagination.PageParam, strconv.Itoa(page))
		if err != nil {
			return nil, err
		}

		// Fetch page
		records, totalPages, err := h.fetchPageWithMeta(ctx, pageURL, h.pagination.TotalPagesField)
		if err != nil {
			return nil, err
		}

		logger.Debug("pagination page fetched",
			"module_type", "httpPolling",
			"pagination_type", "page",
			"current_page", page,
			"total_pages", totalPages,
			"records_in_page", len(records),
			"total_records_so_far", len(allRecords)+len(records),
		)

		allRecords = append(allRecords, records...)

		// Check if we've reached the last page
		if totalPages > 0 && page >= totalPages {
			break
		}
		if len(records) == 0 {
			break
		}

		page++
	}

	logger.Info("pagination completed",
		"module_type", "httpPolling",
		"pagination_type", "page",
		"pages_fetched", page,
		"total_records", len(allRecords),
	)

	return allRecords, nil
}

// fetchAndParseObject fetches an endpoint and parses response as JSON object.
// Returns the parsed JSON object or an error if parsing fails.
func (h *HTTPPolling) fetchAndParseObject(ctx context.Context, endpoint string) (map[string]interface{}, error) {
	body, err := h.doRequest(ctx, endpoint)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrJSONParse, err)
	}
	return result, nil
}

// extractIntField extracts an integer value from a JSON object field.
// Returns 0 if the field is missing, empty, or not a numeric type.
func extractIntField(obj map[string]interface{}, field string) int {
	if field == "" {
		return 0
	}
	if val, ok := obj[field].(float64); ok {
		return int(val)
	}
	return 0
}

// extractStringField extracts a string value from a JSON object field.
// Returns empty string if the field is missing or not a string type.
func extractStringField(obj map[string]interface{}, field string) string {
	if field == "" {
		return ""
	}
	if val, ok := obj[field].(string); ok {
		return val
	}
	return ""
}

// extractRecordsFromObject extracts records from a JSON object using dataField or common field names
func (h *HTTPPolling) extractRecordsFromObject(obj map[string]interface{}) ([]map[string]interface{}, error) {
	if h.dataField != "" {
		return h.extractDataFromField(obj, h.dataField)
	}

	// Try common field names
	commonFields := []string{"data", "items", "results", "records"}
	for _, field := range commonFields {
		if data, ok := obj[field]; ok {
			if converted, err := h.convertToRecords(data); err == nil {
				return converted, nil
			}
		}
	}

	// No field found - pagination requires dataField when response is object
	// Include attempted field names in error message for better debugging
	return nil, fmt.Errorf("%w: pagination requires dataField when response is object (tried common fields: %v)", ErrInvalidDataField, commonFields)
}

// fetchPageWithMeta fetches a page and extracts metadata
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

// fetchOffsetBased handles offset-based pagination
func (h *HTTPPolling) fetchOffsetBased(ctx context.Context) ([]map[string]interface{}, error) {
	var allRecords []map[string]interface{}
	offset := 0
	limit := h.pagination.Limit
	if limit == 0 {
		limit = 100 // Default limit
	}
	pageNum := 0

	logger.Debug("pagination started",
		"module_type", "httpPolling",
		"pagination_type", "offset",
		"offset_param", h.pagination.OffsetParam,
		"limit_param", h.pagination.LimitParam,
		"limit", limit,
	)

	for offset < maxPaginationPages*limit {
		pageNum++
		// Build URL with offset and limit parameters
		offsetURL, err := h.buildPaginatedURLMulti(map[string]string{
			h.pagination.OffsetParam: strconv.Itoa(offset),
			h.pagination.LimitParam:  strconv.Itoa(limit),
		})
		if err != nil {
			return nil, err
		}

		// Fetch page
		records, total, err := h.fetchOffsetWithMeta(ctx, offsetURL, h.pagination.TotalField)
		if err != nil {
			return nil, err
		}

		logger.Debug("pagination page fetched",
			"module_type", "httpPolling",
			"pagination_type", "offset",
			"current_offset", offset,
			"limit", limit,
			"total_available", total,
			"records_in_page", len(records),
			"total_records_so_far", len(allRecords)+len(records),
		)

		allRecords = append(allRecords, records...)

		// Check if we've fetched all records
		if total > 0 && len(allRecords) >= total {
			break
		}
		if len(records) == 0 || len(records) < limit {
			break
		}

		offset += limit
	}

	logger.Info("pagination completed",
		"module_type", "httpPolling",
		"pagination_type", "offset",
		"pages_fetched", pageNum,
		"total_records", len(allRecords),
	)

	return allRecords, nil
}

// fetchOffsetWithMeta fetches with offset pagination and extracts metadata
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

// fetchCursorBased handles cursor-based pagination
func (h *HTTPPolling) fetchCursorBased(ctx context.Context) ([]map[string]interface{}, error) {
	var allRecords []map[string]interface{}
	cursor := ""
	iterations := 0

	logger.Debug("pagination started",
		"module_type", "httpPolling",
		"pagination_type", "cursor",
		"cursor_param", h.pagination.CursorParam,
	)

	for iterations < maxPaginationPages {
		// Build URL with cursor parameter (only if we have a cursor)
		var fetchURL string
		var err error
		if cursor == "" {
			fetchURL = h.endpoint
		} else {
			fetchURL, err = h.buildPaginatedURL(h.pagination.CursorParam, cursor)
			if err != nil {
				return nil, err
			}
		}

		// Fetch page
		records, nextCursor, err := h.fetchCursorWithMeta(ctx, fetchURL, h.pagination.NextCursorField)
		if err != nil {
			return nil, err
		}

		logger.Debug("pagination page fetched",
			"module_type", "httpPolling",
			"pagination_type", "cursor",
			"iteration", iterations+1,
			"has_next_cursor", nextCursor != "",
			"records_in_page", len(records),
			"total_records_so_far", len(allRecords)+len(records),
		)

		allRecords = append(allRecords, records...)

		// Check if we've reached the end
		if nextCursor == "" {
			break
		}

		cursor = nextCursor
		iterations++
	}

	logger.Info("pagination completed",
		"module_type", "httpPolling",
		"pagination_type", "cursor",
		"iterations", iterations+1,
		"total_records", len(allRecords),
	)

	return allRecords, nil
}

// fetchCursorWithMeta fetches with cursor pagination and extracts next cursor
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

// buildPaginatedURL adds a query parameter to the endpoint URL.
// Returns the modified URL or an error if the endpoint URL cannot be parsed.
func (h *HTTPPolling) buildPaginatedURL(param, value string) (string, error) {
	parsedURL, err := url.Parse(h.endpoint)
	if err != nil {
		return "", fmt.Errorf("parsing endpoint URL: %w", err)
	}

	q := parsedURL.Query()
	q.Set(param, value)
	parsedURL.RawQuery = q.Encode()

	return parsedURL.String(), nil
}

// buildPaginatedURLMulti adds multiple query parameters to the endpoint URL.
// Returns the modified URL or an error if the endpoint URL cannot be parsed.
func (h *HTTPPolling) buildPaginatedURLMulti(params map[string]string) (string, error) {
	parsedURL, err := url.Parse(h.endpoint)
	if err != nil {
		return "", fmt.Errorf("parsing endpoint URL: %w", err)
	}

	q := parsedURL.Query()
	for param, value := range params {
		q.Set(param, value)
	}
	parsedURL.RawQuery = q.Encode()

	return parsedURL.String(), nil
}

// Close releases any resources held by the HTTP polling module.
// For HTTP polling, this is a no-op since connections are not persistent.
func (h *HTTPPolling) Close() error {
	return nil
}
