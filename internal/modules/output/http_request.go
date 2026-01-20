// Package output provides implementations for output modules.
// Output modules are responsible for sending data to destination systems.
package output

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/canectors/runtime/internal/logger"
	"github.com/canectors/runtime/pkg/connector"
)

// Default configuration values for HTTP Request module
const (
	defaultHTTPTimeout = 30 * time.Second
	defaultUserAgent   = "Canectors-Runtime/1.0"
	defaultContentType = "application/json"
	defaultBodyFrom    = "records" // batch mode by default
)

// Supported HTTP methods for output module
var supportedMethods = map[string]bool{
	"POST":  true,
	"PUT":   true,
	"PATCH": true,
}

// Error types for HTTP request output module
var (
	ErrNilConfig         = errors.New("module configuration is nil")
	ErrMissingEndpoint   = errors.New("endpoint is required in module configuration")
	ErrMissingMethod     = errors.New("method is required in module configuration")
	ErrInvalidMethod     = errors.New("invalid HTTP method: must be POST, PUT, or PATCH")
	ErrHTTPRequestFailed = errors.New("HTTP request failed")
	ErrJSONMarshal       = errors.New("failed to marshal records to JSON")
)

// HTTPError represents an HTTP error with status code and context
type HTTPError struct {
	StatusCode   int
	Status       string
	Endpoint     string
	Method       string
	Message      string
	ResponseBody string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("http error %d (%s) %s %s: %s", e.StatusCode, e.Status, e.Method, e.Endpoint, e.Message)
}

// RequestConfig holds request-specific configuration
type RequestConfig struct {
	BodyFrom          string            // "records" (batch) or "record" (single)
	PathParams        map[string]string // Path parameter substitution from record
	QueryParams       map[string]string // Static query parameters
	QueryFromRecord   map[string]string // Query parameters extracted from record data
	HeadersFromRecord map[string]string // Headers extracted from record data
}

// RetryConfig holds retry configuration
type RetryConfig struct {
	MaxRetries        int     // Maximum number of retry attempts (default: 3)
	BackoffMs         int     // Initial backoff in milliseconds (default: 1000)
	BackoffMultiplier float64 // Backoff multiplier (default: 2.0)
}

// Default retry configuration
var defaultRetryConfig = RetryConfig{
	MaxRetries:        3,
	BackoffMs:         1000,
	BackoffMultiplier: 2.0,
}

// HTTPRequestModule implements HTTP-based data sending.
// It sends transformed records to a target REST API via HTTP requests.
type HTTPRequestModule struct {
	endpoint     string
	method       string
	headers      map[string]string
	timeout      time.Duration
	request      RequestConfig
	retry        RetryConfig
	auth         *connector.AuthConfig
	client       *http.Client
	onError      string // "fail", "skip", "log"
	successCodes []int  // HTTP status codes considered success

	// OAuth2 token caching
	oauth2Token  string
	oauth2Expiry time.Time
}

// Default success status codes
var defaultSuccessCodes = []int{200, 201, 202, 204}

// NewHTTPRequestFromConfig creates a new HTTP request output module from configuration.
// This is the primary constructor that parses ModuleConfig and creates a ready-to-use module.
//
// Required config fields:
//   - endpoint: The target HTTP endpoint URL
//   - method: HTTP method (POST, PUT, PATCH)
//
// Optional config fields:
//   - headers: Custom HTTP headers (map[string]string)
//   - timeoutMs: Request timeout in milliseconds (default 30000)
//   - request: Request configuration (bodyFrom, pathParams, query)
//   - onError: Error handling mode ("fail", "skip", "log")
func NewHTTPRequestFromConfig(config *connector.ModuleConfig) (*HTTPRequestModule, error) {
	if config == nil {
		return nil, ErrNilConfig
	}

	// Extract endpoint (required)
	endpoint, ok := config.Config["endpoint"].(string)
	if !ok || endpoint == "" {
		return nil, ErrMissingEndpoint
	}

	// Extract method (required)
	method, ok := config.Config["method"].(string)
	if !ok || method == "" {
		return nil, ErrMissingMethod
	}

	// Normalize method to uppercase
	method = strings.ToUpper(method)
	if !supportedMethods[method] {
		return nil, fmt.Errorf("%w: %s", ErrInvalidMethod, method)
	}

	// Extract timeout (optional, default 30s)
	timeout := defaultHTTPTimeout
	if timeoutMs, ok := config.Config["timeoutMs"].(float64); ok && timeoutMs > 0 {
		timeout = time.Duration(timeoutMs) * time.Millisecond
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

	// Extract request config (optional)
	reqConfig := RequestConfig{
		BodyFrom: defaultBodyFrom,
	}
	if requestVal, ok := config.Config["request"].(map[string]interface{}); ok {
		if bodyFrom, ok := requestVal["bodyFrom"].(string); ok {
			reqConfig.BodyFrom = bodyFrom
		}
		if pathParams, ok := requestVal["pathParams"].(map[string]interface{}); ok {
			reqConfig.PathParams = make(map[string]string)
			for k, v := range pathParams {
				if strVal, ok := v.(string); ok {
					reqConfig.PathParams[k] = strVal
				}
			}
		}
		if queryParams, ok := requestVal["query"].(map[string]interface{}); ok {
			reqConfig.QueryParams = make(map[string]string)
			for k, v := range queryParams {
				if strVal, ok := v.(string); ok {
					reqConfig.QueryParams[k] = strVal
				}
			}
		}
		if queryFromRecord, ok := requestVal["queryFromRecord"].(map[string]interface{}); ok {
			reqConfig.QueryFromRecord = make(map[string]string)
			for k, v := range queryFromRecord {
				if strVal, ok := v.(string); ok {
					reqConfig.QueryFromRecord[k] = strVal
				}
			}
		}
		if headersFromRecord, ok := requestVal["headersFromRecord"].(map[string]interface{}); ok {
			reqConfig.HeadersFromRecord = make(map[string]string)
			for k, v := range headersFromRecord {
				if strVal, ok := v.(string); ok {
					reqConfig.HeadersFromRecord[k] = strVal
				}
			}
		}
	}

	// Extract onError mode (optional, default "fail")
	onError := "fail"
	if onErrorVal, ok := config.Config["onError"].(string); ok {
		onError = onErrorVal
	}

	// Extract success status codes (optional, default [200, 201, 202, 204])
	successCodes := defaultSuccessCodes
	if successConfig, ok := config.Config["success"].(map[string]interface{}); ok {
		if statusCodes, ok := successConfig["statusCodes"].([]interface{}); ok && len(statusCodes) > 0 {
			successCodes = make([]int, 0, len(statusCodes))
			for _, code := range statusCodes {
				if codeFloat, ok := code.(float64); ok {
					successCodes = append(successCodes, int(codeFloat))
				}
			}
		}
	}

	// Extract retry configuration (optional, uses defaults)
	retryConfig := defaultRetryConfig
	if retryVal, ok := config.Config["retry"].(map[string]interface{}); ok {
		if maxRetries, ok := retryVal["maxRetries"].(float64); ok {
			retryConfig.MaxRetries = int(maxRetries)
		}
		if backoffMs, ok := retryVal["backoffMs"].(float64); ok {
			retryConfig.BackoffMs = int(backoffMs)
		}
		if backoffMult, ok := retryVal["backoffMultiplier"].(float64); ok {
			retryConfig.BackoffMultiplier = backoffMult
		}
	}

	// Create HTTP client with configured timeout
	client := &http.Client{
		Timeout: timeout,
	}

	module := &HTTPRequestModule{
		endpoint:     endpoint,
		method:       method,
		headers:      headers,
		timeout:      timeout,
		request:      reqConfig,
		retry:        retryConfig,
		auth:         config.Authentication,
		client:       client,
		onError:      onError,
		successCodes: successCodes,
	}

	logger.Debug("http request output module created",
		slog.String("endpoint", endpoint),
		slog.String("method", method),
		slog.String("timeout", timeout.String()),
		slog.Bool("has_auth", config.Authentication != nil),
		slog.String("body_from", reqConfig.BodyFrom),
	)

	return module, nil
}

// Send transmits records to the destination via HTTP.
// Returns the number of records successfully sent and any error encountered.
//
// Behavior depends on request.bodyFrom configuration:
//   - "records" (default): Sends all records in a single request as JSON array
//   - "record": Sends one request per record, body is single JSON object
//
// Empty or nil records return success with 0 sent.
func (h *HTTPRequestModule) Send(records []map[string]interface{}) (int, error) {
	// Handle empty/nil records gracefully
	if len(records) == 0 {
		logger.Debug("no records to send, returning success",
			slog.String("endpoint", h.endpoint),
		)
		return 0, nil
	}

	ctx := context.Background()

	// Choose send mode based on configuration
	if h.request.BodyFrom == "record" {
		return h.sendSingleRecordMode(ctx, records)
	}

	return h.sendBatchMode(ctx, records)
}

// sendBatchMode sends all records in a single HTTP request as JSON array
func (h *HTTPRequestModule) sendBatchMode(ctx context.Context, records []map[string]interface{}) (int, error) {
	logger.Debug("sending records in batch mode",
		slog.String("endpoint", h.endpoint),
		slog.String("method", h.method),
		slog.Int("record_count", len(records)),
	)

	// Marshal records to JSON array
	body, err := json.Marshal(records)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", ErrJSONMarshal, err)
	}

	// Resolve endpoint with static query parameters
	endpoint := h.resolveEndpointWithStaticQuery(h.endpoint)

	// Execute request (no record-specific headers in batch mode)
	err = h.doRequestWithHeaders(ctx, endpoint, body, nil)
	if err != nil {
		return 0, err
	}

	return len(records), nil
}

// sendSingleRecordMode sends one HTTP request per record
func (h *HTTPRequestModule) sendSingleRecordMode(ctx context.Context, records []map[string]interface{}) (int, error) {
	logger.Debug("sending records in single record mode",
		slog.String("endpoint", h.endpoint),
		slog.String("method", h.method),
		slog.Int("record_count", len(records)),
	)

	sent := 0
	for i, record := range records {
		// Marshal single record to JSON object
		body, err := json.Marshal(record)
		if err != nil {
			logger.Error("failed to marshal record",
				slog.Int("record_index", i),
				slog.String("error", err.Error()),
			)
			if h.onError == "fail" {
				return sent, fmt.Errorf("%w at record %d: %w", ErrJSONMarshal, i, err)
			}
			// skip or log mode: continue
			continue
		}

		// Resolve endpoint with path parameters and query params from record
		endpoint := h.resolveEndpointForRecord(record)

		// Extract headers from record data
		recordHeaders := h.extractHeadersFromRecord(record)

		// Execute request with record-specific headers
		err = h.doRequestWithHeaders(ctx, endpoint, body, recordHeaders)
		if err != nil {
			logger.Error("request failed for record",
				slog.Int("record_index", i),
				slog.String("error", err.Error()),
			)
			if h.onError == "fail" {
				return sent, err
			}
			// skip or log mode: continue
			continue
		}

		sent++
	}

	return sent, nil
}

// doRequestWithHeaders executes a single HTTP request with optional record-specific headers
// Implements retry logic for transient errors (5xx, network errors)
func (h *HTTPRequestModule) doRequestWithHeaders(ctx context.Context, endpoint string, body []byte, recordHeaders map[string]string) error {
	var lastErr error

	for attempt := 0; attempt <= h.retry.MaxRetries; attempt++ {
		// Wait before retry (not on first attempt)
		if attempt > 0 {
			backoff := h.calculateBackoff(attempt)
			logger.Debug("retrying request",
				slog.String("endpoint", endpoint),
				slog.Int("attempt", attempt),
				slog.Duration("backoff", backoff),
			)
			time.Sleep(backoff)
		}

		err := h.executeHTTPRequest(ctx, endpoint, body, recordHeaders)
		if err == nil {
			return nil // Success
		}

		lastErr = err

		// Check if error is transient (should retry)
		if !h.isTransientError(err) {
			// Non-transient error - don't retry (e.g., 4xx client errors)
			return err
		}

		logger.Debug("transient error, will retry",
			slog.String("endpoint", endpoint),
			slog.Int("attempt", attempt),
			slog.String("error", err.Error()),
		)
	}

	// All retries exhausted
	return lastErr
}

// executeHTTPRequest executes a single HTTP request without retry logic
func (h *HTTPRequestModule) executeHTTPRequest(ctx context.Context, endpoint string, body []byte, recordHeaders map[string]string) error {
	req, err := http.NewRequestWithContext(ctx, h.method, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating http request: %w", err)
	}

	// Set default headers
	req.Header.Set("User-Agent", defaultUserAgent)
	req.Header.Set("Content-Type", defaultContentType)

	// Set custom headers from config (may override defaults)
	for key, value := range h.headers {
		req.Header.Set(key, value)
	}

	// Set headers from record data (may override config headers)
	for key, value := range recordHeaders {
		req.Header.Set(key, value)
	}

	// Apply authentication if configured
	if err = h.applyAuthentication(ctx, req); err != nil {
		return fmt.Errorf("applying authentication: %w", err)
	}

	// Execute request
	resp, err := h.client.Do(req)
	if err != nil {
		logger.Error("http request failed",
			slog.String("endpoint", endpoint),
			slog.String("method", h.method),
			slog.String("error", err.Error()),
		)
		// Network errors are transient
		return &HTTPError{
			StatusCode:   0,
			Status:       "network error",
			Endpoint:     endpoint,
			Method:       h.method,
			Message:      err.Error(),
			ResponseBody: "",
		}
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			logger.Error("failed to close response body", slog.String("error", closeErr.Error()))
		}
	}()

	// Read response body for error messages
	respBody, _ := io.ReadAll(resp.Body)

	// Check if status code is in success codes
	if !h.isSuccessStatusCode(resp.StatusCode) {
		return &HTTPError{
			StatusCode:   resp.StatusCode,
			Status:       resp.Status,
			Endpoint:     endpoint,
			Method:       h.method,
			Message:      "request failed",
			ResponseBody: string(respBody),
		}
	}

	logger.Debug("http request completed",
		slog.String("endpoint", endpoint),
		slog.String("method", h.method),
		slog.Int("status_code", resp.StatusCode),
	)

	return nil
}

// calculateBackoff calculates the backoff duration for a retry attempt
func (h *HTTPRequestModule) calculateBackoff(attempt int) time.Duration {
	backoffMs := float64(h.retry.BackoffMs)
	for i := 1; i < attempt; i++ {
		backoffMs *= h.retry.BackoffMultiplier
	}
	return time.Duration(backoffMs) * time.Millisecond
}

// isTransientError determines if an error is transient and should be retried
func (h *HTTPRequestModule) isTransientError(err error) bool {
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		// Non-HTTP errors (e.g., network errors) are transient
		return true
	}

	// Network errors (status code 0) are transient
	if httpErr.StatusCode == 0 {
		return true
	}

	// 5xx server errors are transient
	if httpErr.StatusCode >= 500 && httpErr.StatusCode < 600 {
		return true
	}

	// 429 Too Many Requests is transient
	if httpErr.StatusCode == 429 {
		return true
	}

	// 4xx client errors are NOT transient (don't retry)
	return false
}

// resolveEndpointWithStaticQuery adds static query parameters to the endpoint
func (h *HTTPRequestModule) resolveEndpointWithStaticQuery(endpoint string) string {
	if len(h.request.QueryParams) == 0 {
		return endpoint
	}

	// Add static query parameters
	separator := "?"
	if strings.Contains(endpoint, "?") {
		separator = "&"
	}

	for param, value := range h.request.QueryParams {
		endpoint += separator + param + "=" + value
		separator = "&"
	}

	return endpoint
}

// resolveEndpointForRecord resolves path parameters and query params for a single record
func (h *HTTPRequestModule) resolveEndpointForRecord(record map[string]interface{}) string {
	endpoint := h.endpoint

	// Substitute path parameters
	for param, fieldPath := range h.request.PathParams {
		value := getFieldValue(record, fieldPath)
		if value != "" {
			placeholder := "{" + param + "}"
			endpoint = strings.ReplaceAll(endpoint, placeholder, value)
		}
	}

	// Add static query parameters
	separator := "?"
	if strings.Contains(endpoint, "?") {
		separator = "&"
	}

	for param, value := range h.request.QueryParams {
		endpoint += separator + param + "=" + value
		separator = "&"
	}

	// Add query parameters from record data
	for param, fieldPath := range h.request.QueryFromRecord {
		value := getFieldValue(record, fieldPath)
		if value != "" {
			endpoint += separator + param + "=" + value
			separator = "&"
		}
	}

	return endpoint
}

// extractHeadersFromRecord extracts header values from record data
func (h *HTTPRequestModule) extractHeadersFromRecord(record map[string]interface{}) map[string]string {
	if len(h.request.HeadersFromRecord) == 0 {
		return nil
	}

	headers := make(map[string]string)
	for headerName, fieldPath := range h.request.HeadersFromRecord {
		value := getFieldValue(record, fieldPath)
		if value != "" {
			headers[headerName] = value
		}
	}

	return headers
}

// getFieldValue extracts a string value from a record using dot notation path
func getFieldValue(record map[string]interface{}, path string) string {
	parts := strings.Split(path, ".")
	current := interface{}(record)

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]interface{}:
			val, ok := v[part]
			if !ok {
				return ""
			}
			current = val
		default:
			return ""
		}
	}

	// Convert to string
	switch v := current.(type) {
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

// applyAuthentication applies authentication to the HTTP request
// Supports: api-key, bearer, basic, oauth2
func (h *HTTPRequestModule) applyAuthentication(ctx context.Context, req *http.Request) error {
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
			slog.String("type", h.auth.Type),
		)
	}

	return nil
}

// applyAPIKeyAuth applies API key authentication
func (h *HTTPRequestModule) applyAPIKeyAuth(req *http.Request) error {
	key := h.auth.Credentials["key"]
	if key == "" {
		return errors.New("api key is required for api-key authentication")
	}

	location := h.auth.Credentials["location"]
	paramName := h.auth.Credentials["paramName"]
	if paramName == "" {
		paramName = "api_key"
	}

	switch location {
	case "query":
		q := req.URL.Query()
		q.Set(paramName, key)
		req.URL.RawQuery = q.Encode()
	case "header", "":
		headerName := h.auth.Credentials["headerName"]
		if headerName == "" {
			headerName = "X-API-Key"
		}
		req.Header.Set(headerName, key)
	}

	return nil
}

// applyBearerAuth applies bearer token authentication
func (h *HTTPRequestModule) applyBearerAuth(req *http.Request) error {
	token := h.auth.Credentials["token"]
	if token == "" {
		return errors.New("token is required for bearer authentication")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return nil
}

// applyBasicAuth applies HTTP basic authentication
func (h *HTTPRequestModule) applyBasicAuth(req *http.Request) error {
	username := h.auth.Credentials["username"]
	password := h.auth.Credentials["password"]
	if username == "" || password == "" {
		return errors.New("username and password are required for basic authentication")
	}
	req.SetBasicAuth(username, password)
	return nil
}

// applyOAuth2Auth applies OAuth2 client credentials authentication
func (h *HTTPRequestModule) applyOAuth2Auth(ctx context.Context, req *http.Request) error {
	// Check if we have a valid cached token
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
func (h *HTTPRequestModule) obtainOAuth2Token(ctx context.Context) (string, time.Time, error) {
	tokenURL := h.auth.Credentials["tokenUrl"]
	clientID := h.auth.Credentials["clientId"]
	clientSecret := h.auth.Credentials["clientSecret"]

	if tokenURL == "" || clientID == "" || clientSecret == "" {
		return "", time.Time{}, errors.New("tokenUrl, clientId, and clientSecret are required for oauth2 authentication")
	}

	// Build form data
	formData := "grant_type=client_credentials&client_id=" + clientID + "&client_secret=" + clientSecret

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(formData))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("creating token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Execute request
	resp, err := h.client.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("executing token request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return "", time.Time{}, fmt.Errorf("token endpoint returned %d", resp.StatusCode)
	}

	// Parse response
	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", time.Time{}, fmt.Errorf("parsing token response: %w", err)
	}

	// Calculate expiry with buffer
	expiresIn := tokenResp.ExpiresIn - 60
	if expiresIn < 0 {
		expiresIn = 0
	}
	expiry := time.Now().Add(time.Duration(expiresIn) * time.Second)

	logger.Debug("oauth2 token obtained",
		slog.Int("expires_in", tokenResp.ExpiresIn),
	)

	return tokenResp.AccessToken, expiry, nil
}

// isSuccessStatusCode checks if a status code is considered success
func (h *HTTPRequestModule) isSuccessStatusCode(statusCode int) bool {
	for _, code := range h.successCodes {
		if statusCode == code {
			return true
		}
	}
	return false
}

// Close releases any resources held by the HTTP request module.
func (h *HTTPRequestModule) Close() error {
	// HTTP client connections are automatically managed by Go's http.Transport
	// No explicit cleanup needed for the default transport
	logger.Debug("http request output module closed",
		slog.String("endpoint", h.endpoint),
	)
	return nil
}
