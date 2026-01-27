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
	"github.com/canectors/runtime/internal/errhandling"
	"github.com/canectors/runtime/internal/httpconfig"
	"github.com/canectors/runtime/internal/logger"
	"github.com/canectors/runtime/internal/persistence"
	"github.com/canectors/runtime/pkg/connector"
)

// Default configuration values
const (
	defaultTimeout     = 30 * time.Second
	defaultUserAgent   = "Canectors-Runtime/1.0"
	maxPaginationPages = 1000 // Prevent infinite loops
)

// Error messages
const (
	errMsgParsingEndpointURL = "parsing endpoint URL: %w"
)

// Log message constants
const (
	logMsgPaginationStarted     = "pagination started"
	logMsgPaginationPageFetched = "pagination page fetched"
	logMsgPaginationCompleted   = "pagination completed"
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
// It supports HTTP GET requests with authentication, pagination, and retry logic.
// State persistence can be configured to track last timestamp and/or last ID
// for reliable resumption after restarts.
type HTTPPolling struct {
	endpoint      string
	headers       map[string]string
	timeout       time.Duration
	dataField     string
	pagination    *PaginationConfig
	authHandler   auth.Handler
	client        *http.Client
	retryConfig   errhandling.RetryConfig
	lastRetryInfo *connector.RetryInfo

	// State persistence
	persistenceConfig *persistence.StatePersistenceConfig
	stateStore        *persistence.StateStore
	pipelineID        string
	lastState         *persistence.State
}

// NewHTTPPollingFromConfig creates a new HTTP polling input module from configuration.
// This is the primary constructor that parses ModuleConfig and creates a ready-to-use module.
//
// Required config fields:
//   - endpoint: The HTTP endpoint URL
//
// Optional config fields:
//   - headers: Custom HTTP headers (map[string]string)
//   - timeoutMs: Request timeout in milliseconds (default 30000). Also accepts timeout in seconds (float64) for backward compatibility.
//   - dataField: JSON field containing the array of records (for object responses)
//   - pagination: Pagination configuration (map with type, params, etc.)
func NewHTTPPollingFromConfig(config *connector.ModuleConfig) (*HTTPPolling, error) {
	if config == nil {
		return nil, ErrNilConfig
	}

	endpoint, err := extractEndpoint(config)
	if err != nil {
		return nil, err
	}

	timeout := extractTimeout(config)
	headers := extractHeaders(config)
	dataField := extractDataField(config)
	pagination := extractPagination(config)
	retryConfig := extractRetryConfig(config)

	client := createHTTPClient(timeout)
	authHandler, err := createAuthHandler(config, client)
	if err != nil {
		return nil, err
	}

	// Parse state persistence config
	persistenceConfig := persistence.ParseStatePersistenceConfig(config.Config)

	h := &HTTPPolling{
		endpoint:          endpoint,
		headers:           headers,
		timeout:           timeout,
		dataField:         dataField,
		pagination:        pagination,
		authHandler:       authHandler,
		client:            client,
		retryConfig:       retryConfig,
		persistenceConfig: persistenceConfig,
	}

	// Initialize state store if persistence is enabled
	if persistenceConfig != nil && persistenceConfig.IsEnabled() {
		storagePath := persistenceConfig.StoragePath
		if storagePath == "" {
			storagePath = persistence.DefaultStatePath
		}
		h.stateStore = persistence.NewStateStore(storagePath)

		logger.Debug("state persistence enabled for HTTP polling module",
			"endpoint", endpoint,
			"timestamp_enabled", persistenceConfig.TimestampEnabled(),
			"id_enabled", persistenceConfig.IDEnabled(),
			"storage_path", storagePath,
		)
	}

	logModuleCreation(endpoint, timeout, authHandler, pagination, retryConfig)

	return h, nil
}

// extractEndpoint extracts and validates the endpoint (required).
func extractEndpoint(config *connector.ModuleConfig) (string, error) {
	base := httpconfig.ExtractBaseConfig(config)
	if base.Endpoint == "" {
		return "", ErrMissingEndpoint
	}
	return base.Endpoint, nil
}

// extractTimeout extracts timeout from config (timeoutMs or legacy timeout in seconds).
func extractTimeout(config *connector.ModuleConfig) time.Duration {
	base := httpconfig.ExtractBaseConfig(config)
	return httpconfig.GetTimeoutDuration(base.TimeoutMs, defaultTimeout)
}

// extractHeaders extracts headers from config.
func extractHeaders(config *connector.ModuleConfig) map[string]string {
	return httpconfig.ExtractStringMap(config.Config, "headers")
}

// extractDataField extracts dataField from config.
func extractDataField(config *connector.ModuleConfig) string {
	dec := httpconfig.ExtractDataExtractionConfig(config.Config)
	return dec.DataField
}

// extractPagination extracts pagination config from config.
func extractPagination(config *connector.ModuleConfig) *PaginationConfig {
	if paginationVal, ok := config.Config["pagination"].(map[string]interface{}); ok {
		return parsePaginationConfig(paginationVal)
	}
	return nil
}

// extractRetryConfig extracts retry configuration from config.
func extractRetryConfig(config *connector.ModuleConfig) errhandling.RetryConfig {
	if retryVal, ok := config.Config["retry"].(map[string]interface{}); ok {
		return errhandling.ParseRetryConfig(retryVal)
	}
	return errhandling.DefaultRetryConfig()
}

// createHTTPClient creates an HTTP client with the configured timeout.
func createHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
	}
}

// createAuthHandler creates an authentication handler if configured.
func createAuthHandler(config *connector.ModuleConfig, client *http.Client) (auth.Handler, error) {
	if config.Authentication == nil {
		return nil, nil
	}
	authHandler, err := auth.NewHandler(config.Authentication, client)
	if err != nil {
		return nil, fmt.Errorf("creating auth handler: %w", err)
	}
	return authHandler, nil
}

// logModuleCreation logs module creation details.
func logModuleCreation(endpoint string, timeout time.Duration, authHandler auth.Handler, pagination *PaginationConfig, retryConfig errhandling.RetryConfig) {
	logger.Debug("http polling module created",
		"endpoint", endpoint,
		"timeout", timeout.String(),
		"has_auth", authHandler != nil,
		"has_pagination", pagination != nil,
		"retry_max_attempts", retryConfig.MaxAttempts,
	)
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
// If state persistence is configured and state exists, the endpoint may include
// query parameters for filtering (e.g., ?since=2026-01-26T10:30:00Z or ?after_id=12345).
//
// The context can be used to cancel long-running operations.
//
// Returns:
//   - []map[string]interface{}: The fetched records
//   - error: Any error encountered during fetching
func (h *HTTPPolling) Fetch(ctx context.Context) ([]map[string]interface{}, error) {
	startTime := time.Now()

	// Build endpoint with state-based query params if applicable
	endpoint, err := h.buildEndpointWithState(h.endpoint)
	if err != nil {
		logger.Error("failed to build endpoint with state params",
			"module_type", "httpPolling",
			"endpoint", h.endpoint,
			"error", err.Error(),
		)
		return nil, fmt.Errorf("building endpoint with state: %w", err)
	}

	// Log fetch start with configuration summary
	logger.Info("input fetch started",
		"module_type", "httpPolling",
		"endpoint", endpoint,
		"original_endpoint", h.endpoint,
		"timeout", h.timeout.String(),
		"has_pagination", h.pagination != nil,
		"has_auth", h.authHandler != nil,
		"has_state_persistence", h.persistenceConfig != nil && h.persistenceConfig.IsEnabled(),
	)

	var records []map[string]interface{}

	// Handle pagination if configured
	if h.pagination != nil {
		records, err = h.fetchWithPagination(ctx)
	} else {
		// Single request without pagination
		records, err = h.fetchSingle(ctx, endpoint)
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
	logRequestStart(endpoint)

	req, err := h.buildRequest(ctx, endpoint)
	if err != nil {
		return nil, err
	}

	resp, err := h.executeRequest(req, endpoint, requestStart)
	if err != nil {
		return nil, err
	}
	defer closeResponseBody(resp, endpoint)

	body, err := readResponseBody(resp, endpoint)
	if err != nil {
		return nil, err
	}

	if err := h.handleHTTPError(resp, body, endpoint, requestStart); err != nil {
		return nil, err
	}

	logRequestSuccess(endpoint, resp.StatusCode, requestStart, len(body))
	return body, nil
}

// buildRequest creates and configures the HTTP request.
func (h *HTTPPolling) buildRequest(ctx context.Context, endpoint string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		logger.Error("http request creation failed",
			"module_type", "httpPolling",
			"endpoint", endpoint,
			"error", err.Error(),
		)
		return nil, fmt.Errorf("creating http request: %w", err)
	}

	setRequestHeaders(req)
	for key, value := range h.headers {
		req.Header.Set(key, value)
	}

	if err := h.applyAuthentication(ctx, req); err != nil {
		logger.Error("authentication failed",
			"module_type", "httpPolling",
			"endpoint", endpoint,
			"error", err.Error(),
		)
		return nil, fmt.Errorf("applying authentication: %w", err)
	}

	return req, nil
}

// setRequestHeaders sets default headers on the request.
func setRequestHeaders(req *http.Request) {
	req.Header.Set("User-Agent", defaultUserAgent)
}

// executeRequest executes the HTTP request and handles network errors.
func (h *HTTPPolling) executeRequest(req *http.Request, endpoint string, startTime time.Time) (*http.Response, error) {
	resp, err := h.client.Do(req)
	requestDuration := time.Since(startTime)

	if err != nil {
		logger.Error("http request failed",
			"module_type", "httpPolling",
			"endpoint", endpoint,
			"method", http.MethodGet,
			"duration", requestDuration,
			"error", err.Error(),
		)
		return nil, errhandling.ClassifyNetworkError(err)
	}

	return resp, nil
}

// closeResponseBody safely closes the response body.
func closeResponseBody(resp *http.Response, endpoint string) {
	if closeErr := resp.Body.Close(); closeErr != nil {
		logger.Warn("failed to close response body",
			"endpoint", endpoint,
			"error", closeErr.Error(),
		)
	}
}

// readResponseBody reads the response body.
func readResponseBody(resp *http.Response, endpoint string) ([]byte, error) {
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
	return body, nil
}

// handleHTTPError handles HTTP error responses (status >= 400).
func (h *HTTPPolling) handleHTTPError(resp *http.Response, body []byte, endpoint string, startTime time.Time) error {
	if resp.StatusCode < 400 {
		return nil
	}

	bodySnippet := truncateBodyForLogging(body)
	requestDuration := time.Since(startTime)

	logger.Error("http error response",
		"module_type", "httpPolling",
		"endpoint", endpoint,
		"method", http.MethodGet,
		"status_code", resp.StatusCode,
		"status", resp.Status,
		"duration", requestDuration,
		"response_body", bodySnippet,
	)

	h.handleOAuth2Unauthorized(resp, endpoint)

	classifiedErr := errhandling.ClassifyHTTPStatus(resp.StatusCode, bodySnippet)
	classifiedErr.OriginalErr = &HTTPError{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Endpoint:   endpoint,
		Message:    string(body),
	}
	return classifiedErr
}

// truncateBodyForLogging truncates response body to max 500 chars for logging.
func truncateBodyForLogging(body []byte) string {
	bodySnippet := string(body)
	if len(bodySnippet) > 500 {
		return bodySnippet[:500] + "..."
	}
	return bodySnippet
}

// handleOAuth2Unauthorized handles 401 with OAuth2 by invalidating the token.
func (h *HTTPPolling) handleOAuth2Unauthorized(resp *http.Response, endpoint string) {
	if resp.StatusCode != http.StatusUnauthorized || h.authHandler == nil {
		return
	}

	invalidator, ok := h.authHandler.(interface{ InvalidateToken() })
	if !ok {
		return
	}

	logger.Debug("401 Unauthorized with OAuth2, invalidating cached token",
		"endpoint", endpoint,
	)
	invalidator.InvalidateToken()
}

// logRequestStart logs the start of an HTTP request.
func logRequestStart(endpoint string) {
	logger.Debug("http request started",
		"module_type", "httpPolling",
		"endpoint", endpoint,
		"method", http.MethodGet,
	)
}

// logRequestSuccess logs a successful HTTP request.
func logRequestSuccess(endpoint string, statusCode int, startTime time.Time, bodySize int) {
	logger.Debug("http request completed",
		"module_type", "httpPolling",
		"endpoint", endpoint,
		"method", http.MethodGet,
		"status_code", statusCode,
		"duration", time.Since(startTime),
		"response_size", bodySize,
	)
}

// doRequestWithRetry executes an HTTP request with retry logic.
// It uses the RetryExecutor to retry transient errors.
func (h *HTTPPolling) doRequestWithRetry(ctx context.Context, endpoint string) ([]byte, error) {
	executor := errhandling.NewRetryExecutor(h.retryConfig)

	result, err := executor.ExecuteWithCallback(ctx,
		func(ctx context.Context) (interface{}, error) {
			return h.doRequest(ctx, endpoint)
		},
		func(attempt int, err error, nextDelay time.Duration) {
			if err != nil && nextDelay > 0 {
				logger.Info("retrying http request",
					"module_type", "httpPolling",
					"endpoint", endpoint,
					"attempt", attempt+1,
					"max_attempts", h.retryConfig.MaxAttempts+1,
					"next_delay", nextDelay.String(),
					"error", err.Error(),
					"error_category", errhandling.GetErrorCategory(err),
					"retryable", errhandling.IsRetryable(err),
				)
			}
		},
	)

	info := executor.GetRetryInfo()
	h.lastRetryInfo = retryInfoFromErrhandling(info)

	if err != nil {
		if info.RetryCount > 0 {
			logger.Error("http request failed after retries",
				"module_type", "httpPolling",
				"endpoint", endpoint,
				"total_attempts", info.TotalAttempts,
				"total_duration", info.TotalDuration.String(),
				"error", err.Error(),
			)
		}
		return nil, err
	}

	body, ok := result.([]byte)
	if !ok {
		return nil, fmt.Errorf("unexpected result type from retry executor")
	}

	if info.RetryCount > 0 {
		logger.Info("http request succeeded after retries",
			"module_type", "httpPolling",
			"endpoint", endpoint,
			"total_attempts", info.TotalAttempts,
			"retry_count", info.RetryCount,
			"total_duration", info.TotalDuration.String(),
		)
	}

	return body, nil
}

// GetRetryInfo returns retry information from the last Fetch request (RetryInfoProvider).
func (h *HTTPPolling) GetRetryInfo() *connector.RetryInfo {
	return h.lastRetryInfo
}

func retryInfoFromErrhandling(info errhandling.RetryInfo) *connector.RetryInfo {
	out := &connector.RetryInfo{
		TotalAttempts: info.TotalAttempts,
		RetryCount:    info.RetryCount,
	}
	for _, d := range info.Delays {
		out.RetryDelaysMs = append(out.RetryDelaysMs, d.Milliseconds())
	}
	return out
}

// fetchSingle executes a single HTTP GET request and returns the records
func (h *HTTPPolling) fetchSingle(ctx context.Context, endpoint string) ([]map[string]interface{}, error) {
	body, err := h.doRequestWithRetry(ctx, endpoint)
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
	// Get base endpoint with state params
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

// fetchPageBased handles page-based pagination
func (h *HTTPPolling) fetchPageBased(ctx context.Context, baseEndpoint string) ([]map[string]interface{}, error) {
	var allRecords []map[string]interface{}
	page := 1

	logger.Debug(logMsgPaginationStarted,
		"module_type", "httpPolling",
		"pagination_type", "page",
		"page_param", h.pagination.PageParam,
	)

	for page <= maxPaginationPages {
		// Build URL with page parameter
		pageURL, err := h.buildPaginatedURLFrom(baseEndpoint, h.pagination.PageParam, strconv.Itoa(page))
		if err != nil {
			return nil, err
		}

		// Fetch page
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

		// Check if we've reached the last page
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
func (h *HTTPPolling) fetchOffsetBased(ctx context.Context, baseEndpoint string) ([]map[string]interface{}, error) {
	var allRecords []map[string]interface{}
	offset := 0
	limit := h.pagination.Limit
	if limit == 0 {
		limit = 100 // Default limit
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
		// Build URL with offset and limit parameters
		offsetURL, err := h.buildPaginatedURLMultiFrom(baseEndpoint, map[string]string{
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

		// Check if we've fetched all records
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
		// Build URL with cursor parameter (only if we have a cursor)
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

		// Fetch page
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

		// Check if we've reached the end
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

// buildPaginatedURLFrom adds a query parameter to the given base URL.
// Returns the modified URL or an error if the URL cannot be parsed.
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

// buildPaginatedURLMultiFrom adds multiple query parameters to the given base URL.
// Returns the modified URL or an error if the URL cannot be parsed.
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

// Close releases any resources held by the HTTP polling module.
// Closes idle connections in the HTTP client's connection pool to release network resources.
func (h *HTTPPolling) Close() error {
	if h.client != nil {
		// When h.client.Transport is nil, http.Client uses http.DefaultTransport internally.
		// To ensure we still close idle connections, fall back to http.DefaultTransport.
		rt := h.client.Transport
		if rt == nil {
			rt = http.DefaultTransport
		}

		if transport, ok := rt.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}
	}
	return nil
}

// SetPipelineID sets the pipeline ID for state persistence.
// Must be called before Fetch if state persistence is enabled.
func (h *HTTPPolling) SetPipelineID(pipelineID string) {
	h.pipelineID = pipelineID
}

// LoadState loads the last persisted state for this pipeline.
// Returns nil, nil if no state exists (first execution) or if persistence is disabled.
// Returns nil, error if state loading fails (caller should decide whether to continue).
// Should be called before Fetch to enable state-based filtering.
func (h *HTTPPolling) LoadState() (*persistence.State, error) {
	if h.stateStore == nil || h.pipelineID == "" {
		return nil, nil
	}

	state, err := h.stateStore.Load(h.pipelineID)
	if err != nil {
		logger.Warn("failed to load state",
			"pipeline_id", h.pipelineID,
			"error", err.Error(),
		)
		// Return error instead of silently continuing
		// Caller (pipeline executor) will log and continue gracefully
		return nil, err
	}

	h.lastState = state
	return state, nil
}

// GetPersistenceConfig returns the state persistence configuration.
func (h *HTTPPolling) GetPersistenceConfig() *persistence.StatePersistenceConfig {
	return h.persistenceConfig
}

// GetLastState returns the last loaded state.
func (h *HTTPPolling) GetLastState() *persistence.State {
	return h.lastState
}

// SetStateStore sets the state store to use for persistence.
// Overrides the state store created during module initialization.
func (h *HTTPPolling) SetStateStore(store *persistence.StateStore) {
	h.stateStore = store
}

// buildEndpointWithState builds the endpoint URL with state-based query parameters.
// If state persistence is enabled and state exists, adds appropriate query params.
func (h *HTTPPolling) buildEndpointWithState(endpoint string) (string, error) {
	if h.persistenceConfig == nil || !h.persistenceConfig.IsEnabled() || h.lastState == nil {
		return endpoint, nil
	}

	parsedURL, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf(errMsgParsingEndpointURL, err)
	}

	q := parsedURL.Query()
	modified := false

	// Add timestamp query param if configured
	if h.persistenceConfig.TimestampEnabled() && h.persistenceConfig.Timestamp.QueryParam != "" {
		if h.lastState.LastTimestamp != nil {
			q.Set(h.persistenceConfig.Timestamp.QueryParam, h.lastState.FormatTimestamp())
			modified = true
			logger.Debug("added timestamp query param for state persistence",
				"pipeline_id", h.pipelineID,
				"param", h.persistenceConfig.Timestamp.QueryParam,
				"value", h.lastState.FormatTimestamp(),
			)
		}
	}

	// Add ID query param if configured
	if h.persistenceConfig.IDEnabled() && h.persistenceConfig.ID.QueryParam != "" {
		if h.lastState.LastID != nil {
			q.Set(h.persistenceConfig.ID.QueryParam, *h.lastState.LastID)
			modified = true
			logger.Debug("added ID query param for state persistence",
				"pipeline_id", h.pipelineID,
				"param", h.persistenceConfig.ID.QueryParam,
				"value", *h.lastState.LastID,
			)
		}
	}

	if modified {
		parsedURL.RawQuery = q.Encode()
	}

	return parsedURL.String(), nil
}
