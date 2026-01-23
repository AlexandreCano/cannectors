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
	"net/url"
	"strings"
	"time"

	"github.com/canectors/runtime/internal/auth"
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

// HTTP header names
const (
	headerUserAgent   = "User-Agent"
	headerContentType = "Content-Type"
)

// Authentication type constants
const (
	authTypeAPIKey      = "api-key"
	defaultAPIKeyHeader = "X-API-Key"
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
	authHandler  auth.Handler
	client       *http.Client
	onError      string // "fail", "skip", "log"
	successCodes []int  // HTTP status codes considered success
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

	endpoint, method, timeout, err := extractBasicConfig(config.Config)
	if err != nil {
		return nil, err
	}

	headers := extractHeaders(config.Config)
	reqConfig := extractRequestConfig(config.Config)
	onError := extractErrorHandling(config.Config)
	successCodes := extractSuccessCodes(config.Config)
	retryConfig := extractRetryConfig(config.Config)
	client := createHTTPClient(timeout)

	// Create authentication handler if configured
	authHandler, err := auth.NewHandler(config.Authentication, client)
	if err != nil {
		return nil, fmt.Errorf("creating auth handler: %w", err)
	}

	module := &HTTPRequestModule{
		endpoint:     endpoint,
		method:       method,
		headers:      headers,
		timeout:      timeout,
		request:      reqConfig,
		retry:        retryConfig,
		authHandler:  authHandler,
		client:       client,
		onError:      onError,
		successCodes: successCodes,
	}

	logger.Debug("http request output module created",
		slog.String("endpoint", endpoint),
		slog.String("method", method),
		slog.String("timeout", timeout.String()),
		slog.Bool("has_auth", authHandler != nil),
		slog.String("body_from", reqConfig.BodyFrom),
	)

	return module, nil
}

// extractBasicConfig extracts required configuration: endpoint, method, and timeout
func extractBasicConfig(config map[string]interface{}) (endpoint, method string, timeout time.Duration, err error) {
	endpoint, ok := config["endpoint"].(string)
	if !ok || endpoint == "" {
		return "", "", 0, ErrMissingEndpoint
	}

	method, ok = config["method"].(string)
	if !ok || method == "" {
		return "", "", 0, ErrMissingMethod
	}

	method = strings.ToUpper(method)
	if !supportedMethods[method] {
		return "", "", 0, fmt.Errorf("%w: %s", ErrInvalidMethod, method)
	}

	timeout = defaultHTTPTimeout
	if timeoutMs, ok := config["timeoutMs"].(float64); ok && timeoutMs > 0 {
		timeout = time.Duration(timeoutMs) * time.Millisecond
	}

	return endpoint, method, timeout, nil
}

// extractHeaders extracts custom HTTP headers from configuration
func extractHeaders(config map[string]interface{}) map[string]string {
	headers := make(map[string]string)
	if headersVal, ok := config["headers"].(map[string]interface{}); ok {
		for k, v := range headersVal {
			if strVal, ok := v.(string); ok {
				headers[k] = strVal
			}
		}
	}
	return headers
}

// extractRequestConfig extracts request-specific configuration
func extractRequestConfig(config map[string]interface{}) RequestConfig {
	reqConfig := RequestConfig{
		BodyFrom: defaultBodyFrom,
	}

	requestVal, ok := config["request"].(map[string]interface{})
	if !ok {
		return reqConfig
	}

	if bodyFrom, ok := requestVal["bodyFrom"].(string); ok {
		reqConfig.BodyFrom = bodyFrom
	}

	if pathParams, ok := requestVal["pathParams"].(map[string]interface{}); ok {
		reqConfig.PathParams = extractStringMap(pathParams)
	}

	if queryParams, ok := requestVal["query"].(map[string]interface{}); ok {
		reqConfig.QueryParams = extractStringMap(queryParams)
	}

	if queryFromRecord, ok := requestVal["queryFromRecord"].(map[string]interface{}); ok {
		reqConfig.QueryFromRecord = extractStringMap(queryFromRecord)
	}

	if headersFromRecord, ok := requestVal["headersFromRecord"].(map[string]interface{}); ok {
		reqConfig.HeadersFromRecord = extractStringMap(headersFromRecord)
	}

	return reqConfig
}

// extractStringMap converts map[string]interface{} to map[string]string
func extractStringMap(m map[string]interface{}) map[string]string {
	result := make(map[string]string)
	for k, v := range m {
		if strVal, ok := v.(string); ok {
			result[k] = strVal
		}
	}
	return result
}

// extractErrorHandling extracts error handling mode from configuration
func extractErrorHandling(config map[string]interface{}) string {
	if onErrorVal, ok := config["onError"].(string); ok {
		return onErrorVal
	}
	return "fail" // default
}

// extractSuccessCodes extracts custom success status codes from configuration
func extractSuccessCodes(config map[string]interface{}) []int {
	successConfig, ok := config["success"].(map[string]interface{})
	if !ok {
		return defaultSuccessCodes
	}

	statusCodes, ok := successConfig["statusCodes"].([]interface{})
	if !ok || len(statusCodes) == 0 {
		return defaultSuccessCodes
	}

	successCodes := make([]int, 0, len(statusCodes))
	for _, code := range statusCodes {
		if codeFloat, ok := code.(float64); ok {
			successCodes = append(successCodes, int(codeFloat))
		}
	}

	if len(successCodes) == 0 {
		return defaultSuccessCodes
	}

	return successCodes
}

// extractRetryConfig extracts retry configuration from config
func extractRetryConfig(config map[string]interface{}) RetryConfig {
	retryConfig := defaultRetryConfig

	retryVal, ok := config["retry"].(map[string]interface{})
	if !ok {
		return retryConfig
	}

	if maxRetries, ok := retryVal["maxRetries"].(float64); ok {
		retryConfig.MaxRetries = int(maxRetries)
	}

	if backoffMs, ok := retryVal["backoffMs"].(float64); ok {
		retryConfig.BackoffMs = int(backoffMs)
	}

	if backoffMult, ok := retryVal["backoffMultiplier"].(float64); ok {
		retryConfig.BackoffMultiplier = backoffMult
	}

	return retryConfig
}

// createHTTPClient creates an HTTP client with configured timeout and transport settings
func createHTTPClient(timeout time.Duration) *http.Client {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
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
	startTime := time.Now()

	// Handle empty/nil records gracefully
	if len(records) == 0 {
		logger.Debug("no records to send, returning success",
			slog.String("module_type", "httpRequest"),
			slog.String("endpoint", h.endpoint),
		)
		return 0, nil
	}

	logger.Info("output send started",
		slog.String("module_type", "httpRequest"),
		slog.String("endpoint", h.endpoint),
		slog.String("method", h.method),
		slog.Int("record_count", len(records)),
		slog.String("body_from", h.request.BodyFrom),
		slog.Bool("has_auth", h.authHandler != nil),
	)

	ctx := context.Background()
	var sent int
	var err error

	// Choose send mode based on configuration
	if h.request.BodyFrom == "record" {
		sent, err = h.sendSingleRecordMode(ctx, records)
	} else {
		sent, err = h.sendBatchMode(ctx, records)
	}

	duration := time.Since(startTime)

	if err != nil {
		logger.Error("output send failed",
			slog.String("module_type", "httpRequest"),
			slog.String("endpoint", h.endpoint),
			slog.String("method", h.method),
			slog.Int("records_sent", sent),
			slog.Int("records_failed", len(records)-sent),
			slog.Duration("duration", duration),
			slog.String("error", err.Error()),
		)
		return sent, err
	}

	logger.Info("output send completed",
		slog.String("module_type", "httpRequest"),
		slog.String("endpoint", h.endpoint),
		slog.String("method", h.method),
		slog.Int("records_sent", sent),
		slog.Int("records_failed", len(records)-sent),
		slog.Duration("duration", duration),
	)

	return sent, nil
}

// sendBatchMode sends all records in a single HTTP request as JSON array
func (h *HTTPRequestModule) sendBatchMode(ctx context.Context, records []map[string]interface{}) (int, error) {
	requestStart := time.Now()

	logger.Debug("sending records in batch mode",
		slog.String("module_type", "httpRequest"),
		slog.String("endpoint", h.endpoint),
		slog.String("method", h.method),
		slog.Int("record_count", len(records)),
	)

	// Marshal records to JSON array
	body, err := json.Marshal(records)
	if err != nil {
		logger.Error("failed to marshal records to JSON",
			slog.String("module_type", "httpRequest"),
			slog.String("endpoint", h.endpoint),
			slog.Int("record_count", len(records)),
			slog.String("error", err.Error()),
		)
		return 0, fmt.Errorf("%w: %w", ErrJSONMarshal, err)
	}

	logger.Debug("request body prepared",
		slog.String("module_type", "httpRequest"),
		slog.Int("body_size", len(body)),
	)

	// Resolve endpoint with static query parameters
	endpoint := h.resolveEndpointWithStaticQuery(h.endpoint)

	// Execute request (no record-specific headers in batch mode)
	err = h.doRequestWithHeaders(ctx, endpoint, body, nil)
	requestDuration := time.Since(requestStart)

	if err != nil {
		logger.Debug("batch request failed",
			slog.String("module_type", "httpRequest"),
			slog.String("endpoint", endpoint),
			slog.Duration("duration", requestDuration),
			slog.String("error", err.Error()),
		)
		return 0, err
	}

	logger.Debug("batch request completed",
		slog.String("module_type", "httpRequest"),
		slog.String("endpoint", endpoint),
		slog.Int("records_sent", len(records)),
		slog.Duration("duration", requestDuration),
	)

	return len(records), nil
}

// sendSingleRecordMode sends one HTTP request per record
func (h *HTTPRequestModule) sendSingleRecordMode(ctx context.Context, records []map[string]interface{}) (int, error) {
	logger.Debug("sending records in single record mode",
		slog.String("module_type", "httpRequest"),
		slog.String("endpoint", h.endpoint),
		slog.String("method", h.method),
		slog.Int("record_count", len(records)),
	)

	sent := 0
	failed := 0
	for i, record := range records {
		requestStart := time.Now()

		// Marshal single record to JSON object
		body, err := json.Marshal(record)
		if err != nil {
			failed++
			logger.Error("failed to marshal record",
				slog.String("module_type", "httpRequest"),
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
		requestDuration := time.Since(requestStart)

		if err != nil {
			failed++
			logger.Error("request failed for record",
				slog.String("module_type", "httpRequest"),
				slog.Int("record_index", i),
				slog.String("endpoint", endpoint),
				slog.Duration("duration", requestDuration),
				slog.String("error", err.Error()),
			)
			if h.onError == "fail" {
				return sent, err
			}
			// skip or log mode: continue
			continue
		}

		sent++
		logger.Debug("record sent successfully",
			slog.String("module_type", "httpRequest"),
			slog.Int("record_index", i),
			slog.String("endpoint", endpoint),
			slog.Duration("duration", requestDuration),
		)
	}

	logger.Debug("single record mode completed",
		slog.String("module_type", "httpRequest"),
		slog.Int("total_records", len(records)),
		slog.Int("sent", sent),
		slog.Int("failed", failed),
	)

	return sent, nil
}

// handleOAuth2Unauthorized handles 401 Unauthorized for OAuth2 authentication
// Returns true if token was invalidated and request should be retried
// Returns false and logs a warning if token was already invalidated and 401 persists
func (h *HTTPRequestModule) handleOAuth2Unauthorized(err error, alreadyRetried bool) bool {
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusUnauthorized {
		return false
	}

	if h.authHandler == nil {
		return false
	}

	invalidator, ok := h.authHandler.(interface{ InvalidateToken() })
	if !ok {
		return false
	}

	if alreadyRetried {
		// Token was already invalidated and refreshed, but 401 persists
		logger.Warn("401 Unauthorized persists after OAuth2 token refresh, likely invalid credentials",
			slog.String("endpoint", httpErr.Endpoint),
			slog.String("method", h.method),
		)
		return false
	}

	logger.Debug("401 Unauthorized with OAuth2, invalidating token and retrying once",
		slog.String("endpoint", httpErr.Endpoint),
		slog.String("method", h.method),
	)
	invalidator.InvalidateToken()
	return true
}

// doRequestWithHeaders executes a single HTTP request with optional record-specific headers
// Implements retry logic for transient errors (5xx, network errors)
// Special handling for 401 with OAuth2: invalidates token and retries once with new token
func (h *HTTPRequestModule) doRequestWithHeaders(ctx context.Context, endpoint string, body []byte, recordHeaders map[string]string) error {
	var lastErr error
	oauth2Retried := false // Track if we've already retried once for OAuth2 401
	startTime := time.Now()

	for attempt := 0; attempt <= h.retry.MaxRetries; attempt++ {
		// Wait before retry (not on first attempt)
		if attempt > 0 {
			backoff := h.calculateBackoff(attempt)
			logger.Info("retrying request",
				slog.String("module_type", "httpRequest"),
				slog.String("endpoint", endpoint),
				slog.String("method", h.method),
				slog.Int("attempt", attempt),
				slog.Int("max_retries", h.retry.MaxRetries),
				slog.Duration("backoff", backoff),
			)
			time.Sleep(backoff)
		}

		err := h.executeHTTPRequest(ctx, endpoint, body, recordHeaders)
		if err == nil {
			if attempt > 0 {
				logger.Info("retry succeeded",
					slog.String("module_type", "httpRequest"),
					slog.String("endpoint", endpoint),
					slog.Int("attempts", attempt+1),
					slog.Duration("total_duration", time.Since(startTime)),
				)
			}
			return nil // Success
		}

		lastErr = err

		// Handle OAuth2 401: invalidate token and retry once
		if h.handleOAuth2Unauthorized(err, oauth2Retried) {
			oauth2Retried = true
			continue // Retry immediately without counting as retry attempt
		}

		// Non-transient errors don't retry
		if !h.isTransientError(err) {
			logger.Debug("non-transient error, not retrying",
				slog.String("module_type", "httpRequest"),
				slog.String("endpoint", endpoint),
				slog.String("error", err.Error()),
			)
			return err
		}

		logger.Warn("transient error, will retry",
			slog.String("module_type", "httpRequest"),
			slog.String("endpoint", endpoint),
			slog.Int("attempt", attempt+1),
			slog.Int("max_retries", h.retry.MaxRetries),
			slog.String("error", err.Error()),
		)
	}

	logger.Error("all retry attempts exhausted",
		slog.String("module_type", "httpRequest"),
		slog.String("endpoint", endpoint),
		slog.Int("attempts", h.retry.MaxRetries+1),
		slog.Duration("total_duration", time.Since(startTime)),
		slog.String("error", lastErr.Error()),
	)

	return lastErr
}

// executeHTTPRequest executes a single HTTP request without retry logic
func (h *HTTPRequestModule) executeHTTPRequest(ctx context.Context, endpoint string, body []byte, recordHeaders map[string]string) error {
	requestStart := time.Now()

	req, err := http.NewRequestWithContext(ctx, h.method, endpoint, bytes.NewReader(body))
	if err != nil {
		logger.Error("failed to create http request",
			slog.String("module_type", "httpRequest"),
			slog.String("endpoint", endpoint),
			slog.String("method", h.method),
			slog.String("error", err.Error()),
		)
		return fmt.Errorf("creating http request: %w", err)
	}

	// Set default headers
	req.Header.Set(headerUserAgent, defaultUserAgent)
	req.Header.Set(headerContentType, defaultContentType)

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
		logger.Error("failed to apply authentication",
			slog.String("module_type", "httpRequest"),
			slog.String("endpoint", endpoint),
			slog.String("error", err.Error()),
		)
		return fmt.Errorf("applying authentication: %w", err)
	}

	logger.Debug("sending http request",
		slog.String("module_type", "httpRequest"),
		slog.String("endpoint", endpoint),
		slog.String("method", h.method),
		slog.Int("body_size", len(body)),
	)

	// Execute request
	resp, err := h.client.Do(req)
	requestDuration := time.Since(requestStart)

	if err != nil {
		logger.Error("http request network error",
			slog.String("module_type", "httpRequest"),
			slog.String("endpoint", endpoint),
			slog.String("method", h.method),
			slog.Duration("duration", requestDuration),
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
			logger.Warn("failed to close response body",
				slog.String("endpoint", endpoint),
				slog.String("error", closeErr.Error()),
			)
		}
	}()

	// Read response body for error messages (with size limit to prevent memory exhaustion)
	const maxResponseBodySize = 1 * 1024 * 1024 // 1MB limit
	limitedReader := io.LimitReader(resp.Body, maxResponseBodySize)
	respBody, _ := io.ReadAll(limitedReader)

	// Check if status code is in success codes
	if !h.isSuccessStatusCode(resp.StatusCode) {
		// Truncate response body for logging (max 500 chars)
		bodySnippet := string(respBody)
		if len(bodySnippet) > 500 {
			bodySnippet = bodySnippet[:500] + "..."
		}

		logger.Error("http error response",
			slog.String("module_type", "httpRequest"),
			slog.String("endpoint", endpoint),
			slog.String("method", h.method),
			slog.Int("status_code", resp.StatusCode),
			slog.String("status", resp.Status),
			slog.Duration("duration", requestDuration),
			slog.String("response_body", bodySnippet),
		)

		// Note: OAuth2 token invalidation for 401 responses is handled by
		// handleOAuth2Unauthorized in doRequestWithHeaders to avoid duplicate invalidation
		return &HTTPError{
			StatusCode:   resp.StatusCode,
			Status:       resp.Status,
			Endpoint:     endpoint,
			Method:       h.method,
			Message:      "request failed",
			ResponseBody: string(respBody),
		}
	}

	logger.Debug("http request completed successfully",
		slog.String("module_type", "httpRequest"),
		slog.String("endpoint", endpoint),
		slog.String("method", h.method),
		slog.Int("status_code", resp.StatusCode),
		slog.Duration("duration", requestDuration),
		slog.Int("response_size", len(respBody)),
	)

	return nil
}

// Default maximum backoff duration (5 minutes)
const maxBackoffDuration = 5 * time.Minute

// calculateBackoff calculates the backoff duration for a retry attempt
// Protects against integer overflow and caps the maximum backoff duration
func (h *HTTPRequestModule) calculateBackoff(attempt int) time.Duration {
	backoffMs := float64(h.retry.BackoffMs)
	for i := 1; i < attempt; i++ {
		backoffMs *= h.retry.BackoffMultiplier
		// Prevent overflow - cap at reasonable maximum
		if backoffMs > float64(maxBackoffDuration.Milliseconds()) {
			backoffMs = float64(maxBackoffDuration.Milliseconds())
			break
		}
	}

	// Cap final result to maxBackoffDuration
	result := time.Duration(backoffMs) * time.Millisecond
	if result > maxBackoffDuration {
		return maxBackoffDuration
	}
	return result
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

	// Parse URL to properly encode query parameters
	parsedURL, err := url.Parse(endpoint)
	if err != nil {
		// If parsing fails, return original endpoint (shouldn't happen with valid config)
		logger.Warn("failed to parse endpoint URL for query params",
			slog.String("endpoint", endpoint),
			slog.String("error", err.Error()),
		)
		return endpoint
	}

	// Add static query parameters using url.Values for proper encoding
	q := parsedURL.Query()
	for param, value := range h.request.QueryParams {
		q.Set(param, value)
	}
	parsedURL.RawQuery = q.Encode()

	return parsedURL.String()
}

// resolveEndpointForRecord resolves path parameters and query params for a single record
func (h *HTTPRequestModule) resolveEndpointForRecord(record map[string]interface{}) string {
	endpoint := h.endpoint

	// Substitute path parameters (properly URL-encoded)
	for param, fieldPath := range h.request.PathParams {
		value := getFieldValue(record, fieldPath)
		if value != "" {
			// URL-encode path parameter values
			encodedValue := url.PathEscape(value)
			placeholder := "{" + param + "}"
			endpoint = strings.ReplaceAll(endpoint, placeholder, encodedValue)
		}
	}

	// Parse URL to properly encode query parameters
	parsedURL, err := url.Parse(endpoint)
	if err != nil {
		// If parsing fails, return endpoint as-is (shouldn't happen with valid config)
		logger.Warn("failed to parse endpoint URL for record",
			slog.String("endpoint", endpoint),
			slog.String("error", err.Error()),
		)
		return endpoint
	}

	// Add static query parameters using url.Values for proper encoding
	q := parsedURL.Query()
	for param, value := range h.request.QueryParams {
		q.Set(param, value)
	}

	// Add query parameters from record data
	for param, fieldPath := range h.request.QueryFromRecord {
		value := getFieldValue(record, fieldPath)
		if value != "" {
			q.Set(param, value)
		}
	}

	parsedURL.RawQuery = q.Encode()
	return parsedURL.String()
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
			// Protect against nil maps
			if v == nil {
				return ""
			}
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

// applyAuthentication applies authentication to the HTTP request using the shared auth package
func (h *HTTPRequestModule) applyAuthentication(ctx context.Context, req *http.Request) error {
	if h.authHandler == nil {
		return nil
	}
	return h.authHandler.ApplyAuth(ctx, req)
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
	// Close idle connections in the transport to ensure timely cleanup
	if transport, ok := h.client.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}
	logger.Debug("http request output module closed",
		slog.String("endpoint", h.endpoint),
	)
	return nil
}

// PreviewRequest prepares request previews without actually sending HTTP requests.
// This is used in dry-run mode to show what would be sent to the target system.
//
// Returns one preview per request that would be made:
//   - Batch mode (bodyFrom="records"): returns 1 preview for all records
//   - Single record mode (bodyFrom="record"): returns N previews (one per record)
//
// By default, authentication headers are masked for security.
// Set opts.ShowCredentials to true to display actual credential values (for debugging).
func (h *HTTPRequestModule) PreviewRequest(records []map[string]interface{}, opts PreviewOptions) ([]RequestPreview, error) {
	// Handle empty/nil records gracefully
	if len(records) == 0 {
		return []RequestPreview{}, nil
	}

	// Choose preview mode based on configuration
	if h.request.BodyFrom == "record" {
		return h.previewSingleRecordMode(records, opts)
	}

	return h.previewBatchMode(records, opts)
}

// previewBatchMode creates a single preview for all records (batch mode)
func (h *HTTPRequestModule) previewBatchMode(records []map[string]interface{}, opts PreviewOptions) ([]RequestPreview, error) {
	// Resolve endpoint with static query parameters
	endpoint := h.resolveEndpointWithStaticQuery(h.endpoint)

	// Marshal records to formatted JSON
	bodyPreview, err := formatJSONPreview(records)
	if err != nil {
		return nil, fmt.Errorf("formatting body preview: %w", err)
	}

	// Build headers (masked or unmasked based on options)
	headers := h.buildPreviewHeaders(nil, opts)

	preview := RequestPreview{
		Endpoint:    endpoint,
		Method:      h.method,
		Headers:     headers,
		BodyPreview: bodyPreview,
		RecordCount: len(records),
	}

	return []RequestPreview{preview}, nil
}

// previewSingleRecordMode creates one preview per record
func (h *HTTPRequestModule) previewSingleRecordMode(records []map[string]interface{}, opts PreviewOptions) ([]RequestPreview, error) {
	previews := make([]RequestPreview, 0, len(records))

	for _, record := range records {
		// Resolve endpoint with path parameters and query params from record
		endpoint := h.resolveEndpointForRecord(record)

		// Marshal single record to formatted JSON
		bodyPreview, err := formatJSONPreview(record)
		if err != nil {
			return nil, fmt.Errorf("formatting body preview: %w", err)
		}

		// Extract headers from record data
		recordHeaders := h.extractHeadersFromRecord(record)

		// Build headers (masked or unmasked based on options)
		headers := h.buildPreviewHeaders(recordHeaders, opts)

		preview := RequestPreview{
			Endpoint:    endpoint,
			Method:      h.method,
			Headers:     headers,
			BodyPreview: bodyPreview,
			RecordCount: 1,
		}

		previews = append(previews, preview)
	}

	return previews, nil
}

// buildPreviewHeaders constructs the headers map for preview
// If opts.ShowCredentials is false, sensitive auth headers are masked
func (h *HTTPRequestModule) buildPreviewHeaders(recordHeaders map[string]string, opts PreviewOptions) map[string]string {
	headers := make(map[string]string)

	// Set default headers
	headers[headerUserAgent] = defaultUserAgent
	headers[headerContentType] = defaultContentType

	// Set custom headers from config (may override defaults)
	for key, value := range h.headers {
		headers[key] = value
	}

	// Set headers from record data (may override config headers)
	for key, value := range recordHeaders {
		headers[key] = value
	}

	// Add authentication headers LAST (masked unless showCredentials is enabled)
	// This ensures auth headers are always present and masked, even if custom/record headers tried to override
	if opts.ShowCredentials {
		h.addUnmaskedAuthHeaders(headers)
	} else {
		h.addMaskedAuthHeaders(headers)
	}

	return headers
}

// addMaskedAuthHeaders adds authentication headers to the map with sensitive values masked
func (h *HTTPRequestModule) addMaskedAuthHeaders(headers map[string]string) {
	if h.authHandler == nil {
		return
	}

	authType := h.authHandler.Type()
	switch authType {
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

// addUnmaskedAuthHeaders adds authentication headers with actual values (for debugging)
// WARNING: This exposes sensitive credentials - only use in secure debugging environments
func (h *HTTPRequestModule) addUnmaskedAuthHeaders(headers map[string]string) {
	if h.authHandler == nil {
		return
	}

	// Create a mock request to see what headers the auth handler would add
	ctx := context.Background()
	mockReq, err := http.NewRequestWithContext(ctx, "GET", "http://example.com", nil)
	if err != nil {
		return
	}

	// Apply auth to mock request
	if err := h.authHandler.ApplyAuth(ctx, mockReq); err != nil {
		return
	}

	// Copy all headers from the mock request (including auth headers with real values)
	for key, values := range mockReq.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}
}

// maskValue returns a masked version of a sensitive value
func maskValue(valueType string) string {
	return "[MASKED-" + strings.ToUpper(valueType) + "]"
}

// Maximum size for body preview (1MB) to prevent memory issues with very large payloads
const maxBodyPreviewSize = 1 * 1024 * 1024

// formatJSONPreview formats data as indented JSON for preview.
// If the formatted JSON exceeds maxBodyPreviewSize, it truncates with an indication.
func formatJSONPreview(data interface{}) (string, error) {
	formatted, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", err
	}

	formattedStr := string(formatted)
	if len(formattedStr) > maxBodyPreviewSize {
		// Truncate and add indication
		truncated := formattedStr[:maxBodyPreviewSize]
		// Try to truncate at a line boundary
		if lastNewline := strings.LastIndex(truncated, "\n"); lastNewline > maxBodyPreviewSize-100 {
			truncated = truncated[:lastNewline]
		}
		formattedStr = truncated + fmt.Sprintf("\n... (truncated, %d bytes total, %d bytes shown)", len(formatted), len(truncated))
	}

	return formattedStr, nil
}
