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
	"strings"
	"time"

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
	ErrNilConfig         = errors.New("module configuration is nil")
	ErrMissingEndpoint   = errors.New("endpoint is required in module configuration")
	ErrHTTPRequest       = errors.New("http request failed")
	ErrJSONParse         = errors.New("failed to parse JSON response")
	ErrInvalidDataField  = errors.New("dataField does not contain an array")
	ErrOAuth2TokenFailed = errors.New("failed to obtain OAuth2 access token")
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
	endpoint     string
	headers      map[string]string
	timeout      time.Duration
	dataField    string
	pagination   *PaginationConfig
	auth         *connector.AuthConfig
	client       *http.Client
	oauth2Token  string
	oauth2Expiry time.Time
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

	h := &HTTPPolling{
		endpoint:   endpoint,
		headers:    headers,
		timeout:    timeout,
		dataField:  dataField,
		pagination: pagination,
		auth:       config.Authentication,
		client:     client,
	}

	logger.Debug("http polling module created",
		"endpoint", endpoint,
		"timeout", timeout.String(),
		"has_auth", config.Authentication != nil,
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

	logger.Debug("starting http fetch",
		"endpoint", h.endpoint,
	)

	// Handle pagination if configured
	if h.pagination != nil {
		return h.fetchWithPagination(ctx)
	}

	// Single request without pagination
	return h.fetchSingle(ctx, h.endpoint)
}

// doRequest executes an HTTP GET request and returns the raw response body
func (h *HTTPPolling) doRequest(ctx context.Context, endpoint string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("creating http request: %w", err)
	}

	req.Header.Set("User-Agent", defaultUserAgent)
	for key, value := range h.headers {
		req.Header.Set(key, value)
	}

	if err := h.applyAuthentication(ctx, req); err != nil {
		return nil, fmt.Errorf("applying authentication: %w", err)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		logger.Error("http request failed",
			"endpoint", endpoint,
			"error", err.Error(),
		)
		return nil, fmt.Errorf("%w: %w", ErrHTTPRequest, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, &HTTPError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Endpoint:   endpoint,
			Message:    string(body),
		}
	}

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

// applyAuthentication applies authentication to the HTTP request
func (h *HTTPPolling) applyAuthentication(ctx context.Context, req *http.Request) error {
	if h.auth == nil {
		return nil
	}

	switch h.auth.Type {
	case "api-key":
		return h.applyAPIKeyAuth(req)
	case "bearer":
		return h.applyBearerAuth(req)
	case "basic":
		return h.applyBasicAuth(req)
	case "oauth2":
		return h.applyOAuth2Auth(ctx, req)
	default:
		logger.Warn("unknown authentication type",
			"type", h.auth.Type,
		)
	}

	return nil
}

// applyAPIKeyAuth applies API key authentication
func (h *HTTPPolling) applyAPIKeyAuth(req *http.Request) error {
	key := h.auth.Credentials["key"]
	location := h.auth.Credentials["location"]

	switch location {
	case "query":
		paramName := h.auth.Credentials["paramName"]
		if paramName == "" {
			paramName = "api_key"
		}
		q := req.URL.Query()
		q.Set(paramName, key)
		req.URL.RawQuery = q.Encode()
	case "header", "":
		// Default to header if not specified
		req.Header.Set("Authorization", "Bearer "+key)
	}

	return nil
}

// applyBearerAuth applies bearer token authentication
func (h *HTTPPolling) applyBearerAuth(req *http.Request) error {
	token := h.auth.Credentials["token"]
	req.Header.Set("Authorization", "Bearer "+token)
	return nil
}

// applyBasicAuth applies HTTP basic authentication
func (h *HTTPPolling) applyBasicAuth(req *http.Request) error {
	username := h.auth.Credentials["username"]
	password := h.auth.Credentials["password"]
	req.SetBasicAuth(username, password)
	return nil
}

// applyOAuth2Auth applies OAuth2 client credentials authentication
func (h *HTTPPolling) applyOAuth2Auth(ctx context.Context, req *http.Request) error {
	// Check if we have a valid token
	if h.oauth2Token != "" && time.Now().Before(h.oauth2Expiry) {
		req.Header.Set("Authorization", "Bearer "+h.oauth2Token)
		return nil
	}

	// Obtain new token
	token, expiry, err := h.obtainOAuth2Token(ctx)
	if err != nil {
		return err
	}

	h.oauth2Token = token
	h.oauth2Expiry = expiry
	req.Header.Set("Authorization", "Bearer "+token)

	return nil
}

// obtainOAuth2Token obtains an OAuth2 access token using client credentials flow
func (h *HTTPPolling) obtainOAuth2Token(ctx context.Context) (string, time.Time, error) {
	tokenURL := h.auth.Credentials["tokenUrl"]
	clientID := h.auth.Credentials["clientId"]
	clientSecret := h.auth.Credentials["clientSecret"]

	// Build form data
	formData := url.Values{}
	formData.Set("grant_type", "client_credentials")
	formData.Set("client_id", clientID)
	formData.Set("client_secret", clientSecret)

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("%w: creating token request: %v", ErrOAuth2TokenFailed, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Execute request
	resp, err := h.client.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("%w: executing token request: %v", ErrOAuth2TokenFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", time.Time{}, fmt.Errorf("%w: token endpoint returned %d", ErrOAuth2TokenFailed, resp.StatusCode)
	}

	// Parse response
	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", time.Time{}, fmt.Errorf("%w: parsing token response: %v", ErrOAuth2TokenFailed, err)
	}

	// Calculate expiry (with 60s buffer)
	expiry := time.Now().Add(time.Duration(tokenResp.ExpiresIn-60) * time.Second)

	logger.Debug("oauth2 token obtained",
		"expires_in", tokenResp.ExpiresIn,
	)

	return tokenResp.AccessToken, expiry, nil
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

	logger.Debug("page-based pagination completed",
		"total_pages", page-1,
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

// fetchPageWithMeta fetches a page and extracts metadata
func (h *HTTPPolling) fetchPageWithMeta(ctx context.Context, endpoint, totalPagesField string) ([]map[string]interface{}, int, error) {
	obj, err := h.fetchAndParseObject(ctx, endpoint)
	if err != nil {
		return nil, 0, err
	}

	// Extract records: use dataField if specified, otherwise try common field names
	var records []map[string]interface{}
	if h.dataField != "" {
		records, err = h.extractDataFromField(obj, h.dataField)
		if err != nil {
			return nil, 0, err
		}
	} else {
		// Fall back to parseResponse logic for common field names
		for _, field := range []string{"data", "items", "results", "records"} {
			if data, ok := obj[field]; ok {
				if converted, err := h.convertToRecords(data); err == nil {
					records = converted
					break
				}
			}
		}
		// If no common field found and dataField not set, return error for clarity
		if len(records) == 0 {
			return nil, 0, fmt.Errorf("%w: pagination requires dataField when response is object", ErrInvalidDataField)
		}
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

	for offset < maxPaginationPages*limit {
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

	logger.Debug("offset-based pagination completed",
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

	// Extract records: use dataField if specified, otherwise try common field names
	var records []map[string]interface{}
	if h.dataField != "" {
		records, err = h.extractDataFromField(obj, h.dataField)
		if err != nil {
			return nil, 0, err
		}
	} else {
		// Fall back to parseResponse logic for common field names
		for _, field := range []string{"data", "items", "results", "records"} {
			if data, ok := obj[field]; ok {
				if converted, err := h.convertToRecords(data); err == nil {
					records = converted
					break
				}
			}
		}
		// If no common field found and dataField not set, return error for clarity
		if len(records) == 0 {
			return nil, 0, fmt.Errorf("%w: pagination requires dataField when response is object", ErrInvalidDataField)
		}
	}

	return records, extractIntField(obj, totalField), nil
}

// fetchCursorBased handles cursor-based pagination
func (h *HTTPPolling) fetchCursorBased(ctx context.Context) ([]map[string]interface{}, error) {
	var allRecords []map[string]interface{}
	cursor := ""
	iterations := 0

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

		allRecords = append(allRecords, records...)

		// Check if we've reached the end
		if nextCursor == "" {
			break
		}

		cursor = nextCursor
		iterations++
	}

	logger.Debug("cursor-based pagination completed",
		"iterations", iterations,
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

	// Extract records: use dataField if specified, otherwise try common field names
	var records []map[string]interface{}
	if h.dataField != "" {
		records, err = h.extractDataFromField(obj, h.dataField)
		if err != nil {
			return nil, "", err
		}
	} else {
		// Fall back to parseResponse logic for common field names
		for _, field := range []string{"data", "items", "results", "records"} {
			if data, ok := obj[field]; ok {
				if converted, err := h.convertToRecords(data); err == nil {
					records = converted
					break
				}
			}
		}
		// If no common field found and dataField not set, return error for clarity
		if len(records) == 0 {
			return nil, "", fmt.Errorf("%w: pagination requires dataField when response is object", ErrInvalidDataField)
		}
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
