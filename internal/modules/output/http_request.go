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
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"

	"github.com/canectors/runtime/internal/auth"
	"github.com/canectors/runtime/internal/errhandling"
	"github.com/canectors/runtime/internal/httpconfig"
	"github.com/canectors/runtime/internal/logger"
	"github.com/canectors/runtime/pkg/connector"
)

// Default configuration values for HTTP Request module
const (
	defaultHTTPTimeout = 30 * time.Second
	defaultUserAgent   = "Canectors-Runtime/1.0"
	defaultContentType = "application/json"
	defaultBodyFrom    = "records" // batch mode by default
	// MaxRetryHintExpressionLength is the maximum allowed length for retryHintFromBody expressions
	// to prevent DoS attacks through extremely long expressions.
	MaxRetryHintExpressionLength = 10000
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

// Log messages for header validation (extracted for reuse and testability)
const (
	msgInvalidHeaderNameSkipping  = "invalid header name, skipping"
	msgInvalidHeaderValueSkipping = "invalid header value, skipping"
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
	StatusCode      int
	Status          string
	Endpoint        string
	Method          string
	Message         string
	ResponseBody    string
	ResponseHeaders http.Header // HTTP response headers for Retry-After support
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("http error %d (%s) %s %s: %s", e.StatusCode, e.Status, e.Method, e.Endpoint, e.Message)
}

// GetRetryAfter returns the Retry-After header value if present, empty string otherwise.
func (e *HTTPError) GetRetryAfter() string {
	if e.ResponseHeaders == nil {
		return ""
	}
	return e.ResponseHeaders.Get("Retry-After")
}

// RequestConfig holds request-specific configuration
type RequestConfig struct {
	BodyFrom          string            // "records" (batch) or "record" (single)
	PathParams        map[string]string // Path parameter substitution from record
	QueryParams       map[string]string // Static query parameters
	QueryFromRecord   map[string]string // Query parameters extracted from record data
	HeadersFromRecord map[string]string // Headers extracted from record data
	BodyTemplateFile  string            // Path to external template file for request body
	bodyTemplateRaw   string            // Loaded template content (internal use)
}

// RetryConfig is an alias for errhandling.RetryConfig for backward compatibility.
// Use errhandling.RetryConfig directly for new code.
type RetryConfig = errhandling.RetryConfig

// HTTPRequestModule implements HTTP-based data sending.
// It sends transformed records to a target REST API via HTTP requests.
type HTTPRequestModule struct {
	endpoint          string
	method            string
	headers           map[string]string
	timeout           time.Duration
	request           RequestConfig
	retry             RetryConfig
	authHandler       auth.Handler
	client            *http.Client
	onError           errhandling.OnErrorStrategy // "fail", "skip", "log"
	successCodes      []int                       // HTTP status codes considered success
	lastRetryInfo     *connector.RetryInfo
	retryHintProgram  *vm.Program        // Compiled expr program for retryHintFromBody
	templateEvaluator *TemplateEvaluator // Template evaluator for dynamic content
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
		return nil, fmt.Errorf("extracting basic config: %w", err)
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

	// Compile retryHintFromBody expression if configured
	var retryHintProgram *vm.Program
	if retryConfig.RetryHintFromBody != "" {
		// Validate expression length to prevent DoS
		if len(retryConfig.RetryHintFromBody) > MaxRetryHintExpressionLength {
			return nil, fmt.Errorf("retryHintFromBody expression length %d exceeds maximum %d", len(retryConfig.RetryHintFromBody), MaxRetryHintExpressionLength)
		}
		retryHintProgram, err = expr.Compile(retryConfig.RetryHintFromBody, expr.AllowUndefinedVariables(), expr.AsBool())
		if err != nil {
			return nil, fmt.Errorf("compiling retryHintFromBody expression: %w", err)
		}
	}

	// Validate template syntax in endpoint and headers
	if err := validateTemplateConfig(endpoint, headers); err != nil {
		return nil, fmt.Errorf("validating template configuration: %w", err)
	}

	// Load body template file if configured
	if reqConfig.BodyTemplateFile != "" {
		templateContent, err := os.ReadFile(reqConfig.BodyTemplateFile)
		if err != nil {
			return nil, fmt.Errorf("loading body template file %q: %w", reqConfig.BodyTemplateFile, err)
		}
		reqConfig.bodyTemplateRaw = string(templateContent)

		// Validate template syntax in loaded file
		if err := ValidateTemplateSyntax(reqConfig.bodyTemplateRaw); err != nil {
			return nil, fmt.Errorf("invalid template syntax in %q: %w", reqConfig.BodyTemplateFile, err)
		}

		logger.Debug("loaded body template file",
			slog.String("file", reqConfig.BodyTemplateFile),
			slog.Int("size", len(templateContent)),
		)
	}

	module := &HTTPRequestModule{
		endpoint:          endpoint,
		method:            method,
		headers:           headers,
		timeout:           timeout,
		request:           reqConfig,
		retry:             retryConfig,
		authHandler:       authHandler,
		client:            client,
		onError:           onError,
		successCodes:      successCodes,
		retryHintProgram:  retryHintProgram,
		templateEvaluator: NewTemplateEvaluator(),
	}

	// Check if endpoint/headers use templating
	hasTemplating := HasTemplateVariables(endpoint)
	for _, v := range headers {
		if HasTemplateVariables(v) {
			hasTemplating = true
			break
		}
	}

	logger.Debug("http request output module created",
		slog.String("endpoint", endpoint),
		slog.String("method", method),
		slog.String("timeout", timeout.String()),
		slog.Bool("has_auth", authHandler != nil),
		slog.String("body_from", reqConfig.BodyFrom),
		slog.Bool("has_templating", hasTemplating),
		slog.String("body_template_file", reqConfig.BodyTemplateFile),
	)

	return module, nil
}

// validateTemplateConfig validates template syntax in configuration.
func validateTemplateConfig(endpoint string, headers map[string]string) error {
	// Validate endpoint template syntax
	if err := ValidateTemplateSyntax(endpoint); err != nil {
		return fmt.Errorf("invalid endpoint template: %w", err)
	}

	// Validate header template syntax
	for name, value := range headers {
		if err := ValidateTemplateSyntax(value); err != nil {
			return fmt.Errorf("invalid template in header %q: %w", name, err)
		}
	}

	return nil
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

	timeoutMs := 0
	if ms, ok := config["timeoutMs"].(float64); ok && ms > 0 {
		timeoutMs = int(ms)
	}
	timeout = httpconfig.GetTimeoutDuration(timeoutMs, defaultHTTPTimeout)

	return endpoint, method, timeout, nil
}

// extractHeaders extracts custom HTTP headers from configuration
func extractHeaders(config map[string]interface{}) map[string]string {
	return httpconfig.ExtractStringMap(config, "headers")
}

// extractRequestConfig extracts request-specific configuration
func extractRequestConfig(config map[string]interface{}) RequestConfig {
	reqConfig := RequestConfig{
		BodyFrom: defaultBodyFrom,
	}

	// Extract dynamic params using httpconfig
	dynamicParams := httpconfig.ExtractDynamicParamsConfig(config)
	reqConfig.PathParams = dynamicParams.PathParams
	reqConfig.QueryParams = dynamicParams.QueryParams
	reqConfig.QueryFromRecord = dynamicParams.QueryFromRecord
	reqConfig.HeadersFromRecord = dynamicParams.HeadersFromRecord

	// Extract body template from request sub-object
	bodyTemplateConfig := httpconfig.ExtractBodyTemplateConfigFromRequest(config)
	reqConfig.BodyTemplateFile = bodyTemplateConfig.BodyTemplateFile

	// Extract bodyFrom from request sub-object
	if requestVal, ok := config["request"].(map[string]interface{}); ok {
		if bodyFrom, ok := requestVal["bodyFrom"].(string); ok {
			reqConfig.BodyFrom = bodyFrom
		}
	}

	return reqConfig
}

// extractErrorHandling extracts error handling mode from configuration
func extractErrorHandling(config map[string]interface{}) errhandling.OnErrorStrategy {
	ehc := httpconfig.ExtractErrorHandlingConfig(config)
	if ehc.OnError != "" {
		return errhandling.ParseOnErrorStrategy(ehc.OnError)
	}
	return errhandling.OnErrorFail // default
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

// extractRetryConfig extracts retry configuration from config using errhandling.
func extractRetryConfig(config map[string]interface{}) RetryConfig {
	retryVal, ok := config["retry"].(map[string]interface{})
	if !ok {
		return errhandling.DefaultRetryConfig()
	}
	return errhandling.ParseRetryConfig(retryVal)
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
// The context can be used to cancel long-running operations.
//
// Behavior depends on request.bodyFrom configuration:
//   - "records" (default): Sends all records in a single request as JSON array
//   - "record": Sends one request per record, body is single JSON object
//
// Empty or nil records return success with 0 sent.
func (h *HTTPRequestModule) Send(ctx context.Context, records []map[string]interface{}) (int, error) {
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

	// Build request body
	var body []byte
	var err error

	if h.request.bodyTemplateRaw != "" {
		// Use template file: evaluate template with first record (batch context)
		// For batch mode with template, we can only use first record's data
		// The template should be designed accordingly
		if len(records) > 0 {
			bodyStr := h.templateEvaluator.EvaluateTemplate(h.request.bodyTemplateRaw, records[0])
			body = []byte(bodyStr)

			// Validate resulting JSON is well-formed (AC #4)
			// Only validate if Content-Type indicates JSON
			if h.isJSONContentType() {
				if validationErr := validateJSON(body); validationErr != nil {
					logger.Warn("body template produced invalid JSON, continuing anyway",
						slog.String("error", validationErr.Error()),
						slog.String("body_preview", truncateString(string(body), 100)),
					)
					// Continue execution - let HTTP client handle the error if needed
				}
			}
		} else {
			body = []byte(h.request.bodyTemplateRaw)
		}
		logger.Debug("body generated from template file",
			slog.Int("body_size", len(body)),
		)
	} else {
		// Default: marshal records to JSON array
		body, err = json.Marshal(records)
		if err != nil {
			logger.Error("failed to marshal records to JSON",
				slog.String("module_type", "httpRequest"),
				slog.String("endpoint", h.endpoint),
				slog.Int("record_count", len(records)),
				slog.String("error", err.Error()),
			)
			return 0, fmt.Errorf("%w: %w", ErrJSONMarshal, err)
		}
	}

	logger.Debug("request body prepared",
		slog.String("module_type", "httpRequest"),
		slog.Int("body_size", len(body)),
	)

	// Resolve endpoint with template variables and static query parameters
	// In batch mode, templates are evaluated using the first record
	endpoint := h.resolveEndpointForBatch(h.endpoint, records)

	// Evaluate templated headers using first record in batch mode
	var batchHeaders map[string]string
	if len(records) > 0 {
		batchHeaders = h.extractHeadersFromRecord(records[0])
	}

	// Execute request with batch-resolved headers
	err = h.doRequestWithHeaders(ctx, endpoint, body, batchHeaders)
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
		slog.String("on_error", string(h.onError)),
	)

	sent := 0
	failed := 0
	for i, record := range records {
		requestStart := time.Now()

		body, err := h.buildBodyForRecord(record, i)
		if err != nil {
			failed++
			if h.onError == errhandling.OnErrorFail {
				return sent, fmt.Errorf("%w at record %d: %w", ErrJSONMarshal, i, err)
			}
			continue
		}

		endpoint := h.resolveEndpointForRecord(record)
		recordHeaders := h.extractHeadersFromRecord(record)

		ok, err := h.executeRequestAndLog(ctx, endpoint, body, recordHeaders, i, requestStart)
		if !ok {
			failed++
			if h.onError == errhandling.OnErrorFail {
				return sent, err
			}
			continue
		}
		sent++
	}

	logger.Debug("single record mode completed",
		slog.String("module_type", "httpRequest"),
		slog.Int("total_records", len(records)),
		slog.Int("sent", sent),
		slog.Int("failed", failed),
	)

	return sent, nil
}

// buildBodyForRecord builds the request body for a single record (template or JSON marshal).
// Returns an error only on marshal failure; invalid JSON from templates is logged but not returned.
func (h *HTTPRequestModule) buildBodyForRecord(record map[string]interface{}, recordIndex int) ([]byte, error) {
	if h.request.bodyTemplateRaw != "" {
		bodyStr := h.templateEvaluator.EvaluateTemplate(h.request.bodyTemplateRaw, record)
		body := []byte(bodyStr)
		if h.isJSONContentType() {
			if validationErr := validateJSON(body); validationErr != nil {
				logger.Warn("body template produced invalid JSON, continuing anyway",
					slog.Int("record_index", recordIndex),
					slog.String("error", validationErr.Error()),
					slog.String("body_preview", truncateString(string(body), 100)),
				)
			}
		}
		return body, nil
	}
	body, err := json.Marshal(record)
	if err != nil {
		logger.Error("failed to marshal record",
			slog.String("module_type", "httpRequest"),
			slog.Int("record_index", recordIndex),
			slog.String("error", err.Error()),
		)
		return nil, err
	}
	return body, nil
}

// executeRequestAndLog performs the HTTP request for a single record, logs outcome, and returns success.
func (h *HTTPRequestModule) executeRequestAndLog(
	ctx context.Context, endpoint string, body []byte, recordHeaders map[string]string,
	recordIndex int, requestStart time.Time,
) (ok bool, err error) {
	err = h.doRequestWithHeaders(ctx, endpoint, body, recordHeaders)
	duration := time.Since(requestStart)

	if err != nil {
		errorCategory := errhandling.GetErrorCategory(err)
		isFatal := errhandling.IsFatal(err)
		logger.Error("request failed for record",
			slog.String("module_type", "httpRequest"),
			slog.Int("record_index", recordIndex),
			slog.String("endpoint", endpoint),
			slog.Duration("duration", duration),
			slog.String("error", err.Error()),
			slog.String("error_category", string(errorCategory)),
			slog.Bool("is_fatal", isFatal),
			slog.String("on_error", string(h.onError)),
		)
		return false, err
	}

	logger.Debug("record sent successfully",
		slog.String("module_type", "httpRequest"),
		slog.Int("record_index", recordIndex),
		slog.String("endpoint", endpoint),
		slog.Duration("duration", duration),
	)
	return true, nil
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
	startTime := time.Now()
	var delaysMs []int64
	oauth2Retried := false

	lastErr := h.retryLoop(ctx, endpoint, body, recordHeaders, startTime, &delaysMs, &oauth2Retried)

	if lastErr != nil {
		return h.handleRetryFailure(lastErr, delaysMs, startTime, endpoint)
	}
	return nil
}

// retryLoop executes the retry loop for HTTP requests.
// For HTTP errors, it uses h.retry.IsStatusCodeRetryable to decide retryability based on the module's
// configured retryableStatusCodes (AC #1: module config takes precedence over defaults).
// For network errors, it defers to errhandling.IsRetryable (network errors bypass retryableStatusCodes).
func (h *HTTPRequestModule) retryLoop(ctx context.Context, endpoint string, body []byte, recordHeaders map[string]string, startTime time.Time, delaysMs *[]int64, oauth2Retried *bool) error {
	var lastErr error

	for attempt := 0; attempt <= h.retry.MaxAttempts; attempt++ {
		if attempt > 0 {
			// Pass lastErr to allow Retry-After header extraction (AC #2)
			backoff := h.waitForRetry(attempt, delaysMs, endpoint, lastErr)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
				// Continue to next attempt
			}
		}

		err := h.executeHTTPRequest(ctx, endpoint, body, recordHeaders)
		if err == nil {
			h.handleRetrySuccess(attempt, delaysMs, startTime, endpoint)
			return nil
		}

		lastErr = err

		if h.shouldRetryOAuth2(err, oauth2Retried) {
			continue
		}

		// Use module's retryableStatusCodes for HTTP errors (AC #1)
		if !h.isErrorRetryable(err) {
			h.logNonRetryableError(err, endpoint)
			return err
		}

		h.logTransientError(err, attempt, endpoint)
	}

	return lastErr // All attempts exhausted
}

// isErrorRetryable determines if an error should trigger a retry.
// For HTTP errors: uses h.retry.IsStatusCodeRetryable (module's retryableStatusCodes config).
// For network errors: uses errhandling.IsRetryable (network errors are retried regardless of retryableStatusCodes).
func (h *HTTPRequestModule) isErrorRetryable(err error) bool {
	// Extract HTTPError for status code and body hint checking
	var httpErr *HTTPError
	hasHTTPErr := errors.As(err, &httpErr)

	// Check retryHintFromBody if configured (AC #3)
	if h.retry.RetryHintFromBody != "" && hasHTTPErr {
		hint := h.evaluateRetryHintFromBody(httpErr.ResponseBody)
		switch hint {
		case retryHintTrue:
			// Body says retryable, but still respect status code config
			return h.retry.IsStatusCodeRetryable(httpErr.StatusCode)
		case retryHintFalse:
			// Body says NOT retryable, override status code
			return false
		case retryHintAbsent:
			// Fall through to status code only
		}
	}

	// Extract status code from HTTPError if present
	if hasHTTPErr {
		// HTTP error: use module's retryableStatusCodes
		return h.retry.IsStatusCodeRetryable(httpErr.StatusCode)
	}

	// Extract status code from ClassifiedError if present
	var classifiedErr *errhandling.ClassifiedError
	if errors.As(err, &classifiedErr) && classifiedErr.StatusCode > 0 {
		// Check body hint from ClassifiedError's original error
		if h.retry.RetryHintFromBody != "" {
			if origHTTPErr, ok := classifiedErr.OriginalErr.(*HTTPError); ok {
				hint := h.evaluateRetryHintFromBody(origHTTPErr.ResponseBody)
				switch hint {
				case retryHintTrue:
					return h.retry.IsStatusCodeRetryable(classifiedErr.StatusCode)
				case retryHintFalse:
					return false
				case retryHintAbsent:
					// Fall through to status code only
				}
			}
		}
		// HTTP error wrapped in ClassifiedError: use module's retryableStatusCodes
		return h.retry.IsStatusCodeRetryable(classifiedErr.StatusCode)
	}

	// Network error or unknown: defer to errhandling.IsRetryable
	return errhandling.IsRetryable(err)
}

// retryHintResult represents the result of evaluating retryHintFromBody
type retryHintResult int

const (
	retryHintAbsent retryHintResult = iota // Expression returned nil/undefined or body not JSON
	retryHintTrue                          // Expression returned true
	retryHintFalse                         // Expression returned false
)

// evaluateRetryHintFromBody evaluates the retry hint from response body using the configured expr expression.
// The expression has access to a "body" variable containing the parsed JSON response.
// Returns retryHintTrue if expression returns true, retryHintFalse if false, retryHintAbsent if evaluation fails.
func (h *HTTPRequestModule) evaluateRetryHintFromBody(responseBody string) retryHintResult {
	if h.retryHintProgram == nil || responseBody == "" {
		return retryHintAbsent
	}

	// Parse JSON body
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(responseBody), &data); err != nil {
		logger.Warn("failed to parse JSON body for retryHintFromBody evaluation, falling back to status code",
			slog.String("module_type", "httpRequest"),
			slog.String("error", err.Error()),
		)
		return retryHintAbsent
	}

	// Run the compiled expression with body as context
	env := map[string]interface{}{
		"body": data,
	}
	result, err := expr.Run(h.retryHintProgram, env)
	if err != nil {
		logger.Warn("failed to evaluate retryHintFromBody expression, falling back to status code",
			slog.String("module_type", "httpRequest"),
			slog.String("expression", h.retry.RetryHintFromBody),
			slog.String("error", err.Error()),
		)
		return retryHintAbsent
	}

	// Expression must return a boolean
	boolResult, ok := result.(bool)
	if !ok {
		logger.Warn("retryHintFromBody expression returned non-boolean value, falling back to status code",
			slog.String("module_type", "httpRequest"),
			slog.String("expression", h.retry.RetryHintFromBody),
			slog.String("result_type", fmt.Sprintf("%T", result)),
		)
		return retryHintAbsent
	}

	if boolResult {
		return retryHintTrue
	}
	return retryHintFalse
}

// waitForRetry calculates the delay before retrying and logs the retry attempt.
// If UseRetryAfterHeader is enabled and the error contains a valid Retry-After header,
// that value is used (capped by MaxDelayMs). Otherwise, the exponential backoff is used.
func (h *HTTPRequestModule) waitForRetry(attempt int, delaysMs *[]int64, endpoint string, lastErr error) time.Duration {
	backoff := h.retry.CalculateDelay(attempt - 1) // CalculateDelay is 0-indexed

	// Check for Retry-After header if enabled
	if h.retry.UseRetryAfterHeader && lastErr != nil {
		if retryAfterDuration, valid := h.parseRetryAfterFromError(lastErr); valid {
			// Cap at MaxDelayMs (but honor 0 for immediate retry)
			maxDelay := time.Duration(h.retry.MaxDelayMs) * time.Millisecond
			if retryAfterDuration > maxDelay {
				retryAfterDuration = maxDelay
			} else if retryAfterDuration < 0 {
				// HTTP-date in the past or now → immediate retry
				retryAfterDuration = 0
			}
			backoff = retryAfterDuration
		}
	}

	*delaysMs = append(*delaysMs, backoff.Milliseconds())

	logger.Info("retrying request",
		slog.String("module_type", "httpRequest"),
		slog.String("endpoint", endpoint),
		slog.String("method", h.method),
		slog.Int("attempt", attempt),
		slog.Int("max_attempts", h.retry.MaxAttempts),
		slog.Duration("backoff", backoff),
	)

	return backoff
}

// parseRetryAfterFromError extracts and parses the Retry-After header from an HTTPError.
// Returns the duration to wait and a boolean indicating if the header was valid.
// Supports RFC 7231 formats: seconds (integer) or HTTP-date.
// The boolean allows distinguishing "valid but 0" (immediate retry) from "invalid/absent".
func (h *HTTPRequestModule) parseRetryAfterFromError(err error) (time.Duration, bool) {
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		return 0, false
	}

	retryAfter := httpErr.GetRetryAfter()
	if retryAfter == "" {
		return 0, false
	}

	return parseRetryAfterValue(retryAfter)
}

// parseRetryAfterValue parses a Retry-After header value.
// Supports: seconds (e.g., "120", "0") or HTTP-date (e.g., "Fri, 31 Dec 1999 23:59:59 GMT").
// Returns (duration, true) if valid (including 0 for immediate retry), (0, false) if invalid.
func parseRetryAfterValue(value string) (time.Duration, bool) {
	// Try parsing as integer (seconds)
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second, true
	}

	// Try parsing as HTTP-date (RFC 7231)
	// Common formats: RFC1123, RFC850, ANSI C asctime
	httpDateFormats := []string{
		time.RFC1123,
		time.RFC1123Z,
		time.RFC850,
		"Mon Jan _2 15:04:05 2006", // ANSI C asctime format
	}

	for _, format := range httpDateFormats {
		if t, err := time.Parse(format, value); err == nil {
			duration := time.Until(t)
			// Return duration even if 0 or negative (valid header, immediate retry)
			return duration, true
		}
	}

	return 0, false // Invalid format
}

// shouldRetryOAuth2 handles OAuth2 401 by invalidating token and retrying once.
func (h *HTTPRequestModule) shouldRetryOAuth2(err error, oauth2Retried *bool) bool {
	if h.handleOAuth2Unauthorized(err, *oauth2Retried) {
		*oauth2Retried = true
		return true
	}
	return false
}

// handleRetrySuccess handles successful retry and sets retry info.
func (h *HTTPRequestModule) handleRetrySuccess(attempt int, delaysMs *[]int64, startTime time.Time, endpoint string) {
	if attempt > 0 {
		logger.Info("retry succeeded",
			slog.String("module_type", "httpRequest"),
			slog.String("endpoint", endpoint),
			slog.Int("attempts", attempt+1),
			slog.Duration("total_duration", time.Since(startTime)),
		)
		h.lastRetryInfo = &connector.RetryInfo{
			TotalAttempts: attempt + 1,
			RetryCount:    attempt,
			RetryDelaysMs: *delaysMs,
		}
	} else {
		h.lastRetryInfo = nil
	}
}

// handleRetryFailure handles retry failure and sets retry info.
func (h *HTTPRequestModule) handleRetryFailure(lastErr error, delaysMs []int64, startTime time.Time, endpoint string) error {
	safeErr := lastErr
	if safeErr == nil {
		safeErr = fmt.Errorf("all retry attempts exhausted but no error captured (max_attempts=%d)", h.retry.MaxAttempts)
	}

	h.lastRetryInfo = &connector.RetryInfo{
		TotalAttempts: len(delaysMs) + 1,
		RetryCount:    len(delaysMs),
		RetryDelaysMs: delaysMs,
	}

	logger.Error("all retry attempts exhausted",
		slog.String("module_type", "httpRequest"),
		slog.String("endpoint", endpoint),
		slog.Int("attempts", h.retry.MaxAttempts+1),
		slog.Duration("total_duration", time.Since(startTime)),
		slog.String("error", safeErr.Error()),
	)

	return safeErr
}

// logNonRetryableError logs a non-retryable error.
func (h *HTTPRequestModule) logNonRetryableError(err error, endpoint string) {
	logger.Debug("non-transient error, not retrying",
		slog.String("module_type", "httpRequest"),
		slog.String("endpoint", endpoint),
		slog.String("error", err.Error()),
		slog.String("error_category", string(errhandling.GetErrorCategory(err))),
	)
}

// logTransientError logs a transient error that will be retried.
func (h *HTTPRequestModule) logTransientError(err error, attempt int, endpoint string) {
	logger.Warn("transient error, will retry",
		slog.String("module_type", "httpRequest"),
		slog.String("endpoint", endpoint),
		slog.Int("attempt", attempt+1),
		slog.Int("max_attempts", h.retry.MaxAttempts),
		slog.String("error", err.Error()),
		slog.String("error_category", string(errhandling.GetErrorCategory(err))),
	)
}

// GetRetryInfo returns retry information from the last Send request (RetryInfoProvider).
func (h *HTTPRequestModule) GetRetryInfo() *connector.RetryInfo {
	return h.lastRetryInfo
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

	// Apply validated headers (defaults + static config + record; all custom headers validated via tryAddValidHeader)
	headers := h.buildBaseHeadersMap(recordHeaders)
	for key, value := range headers {
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
		// Classify network error for retry logic
		return errhandling.ClassifyNetworkError(err)
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
		// Classify HTTP error for retry logic
		classifiedErr := errhandling.ClassifyHTTPStatus(resp.StatusCode, bodySnippet)
		classifiedErr.OriginalErr = &HTTPError{
			StatusCode:      resp.StatusCode,
			Status:          resp.Status,
			Endpoint:        endpoint,
			Method:          h.method,
			Message:         "request failed",
			ResponseBody:    string(respBody),
			ResponseHeaders: resp.Header.Clone(), // Capture headers for Retry-After support
		}
		return classifiedErr
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

// Note: calculateBackoff and isTransientError methods have been replaced by
// errhandling.RetryConfig.CalculateDelay and errhandling.IsRetryable respectively.

// resolveEndpointWithStaticQuery adds static query parameters to the endpoint.
// In batch mode, template variables can be evaluated using the first record if available.
func (h *HTTPRequestModule) resolveEndpointWithStaticQuery(endpoint string) string {
	// No query params to add
	if len(h.request.QueryParams) == 0 && !HasTemplateVariables(endpoint) {
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

// resolveEndpointForBatch resolves template variables in endpoint for batch mode.
// Uses the first record for template evaluation when in batch mode.
func (h *HTTPRequestModule) resolveEndpointForBatch(endpoint string, records []map[string]interface{}) string {
	// Evaluate template variables using first record (for batch mode)
	if HasTemplateVariables(endpoint) && len(records) > 0 {
		endpoint = h.templateEvaluator.EvaluateTemplateForURL(endpoint, records[0])
		logger.Debug("batch mode: evaluated endpoint template using first record",
			slog.String("endpoint", endpoint),
		)
	}

	finalURL := h.resolveEndpointWithStaticQuery(endpoint)

	// Validate the resulting URL is well-formed (AC #2)
	if err := validateURL(finalURL); err != nil {
		logger.Warn("invalid URL after template evaluation",
			slog.String("url", finalURL),
			slog.String("error", err.Error()),
		)
		// Return the URL anyway to avoid breaking execution, but log the issue
	}

	return finalURL
}

// resolveEndpointForRecord resolves path parameters, template variables, and query params for a single record.
// Template variables ({{record.field}}) are evaluated first, then path parameters ({param}) are substituted.
func (h *HTTPRequestModule) resolveEndpointForRecord(record map[string]interface{}) string {
	endpoint := h.endpoint

	// Evaluate template variables in endpoint ({{record.field}} syntax)
	// This allows dynamic URL construction based on record data
	if HasTemplateVariables(endpoint) {
		endpoint = h.templateEvaluator.EvaluateTemplateForURL(endpoint, record)
	}

	// Substitute path parameters (properly URL-encoded) - legacy {param} syntax
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
	finalURL := parsedURL.String()

	// Validate the resulting URL is well-formed (AC #2)
	if err := validateURL(finalURL); err != nil {
		logger.Warn("invalid URL after template evaluation",
			slog.String("url", finalURL),
			slog.String("error", err.Error()),
		)
		// Return the URL anyway to avoid breaking execution, but log the issue
	}

	return finalURL
}

// validateURL validates that a URL string is well-formed.
// Returns an error if the URL is invalid.
func validateURL(urlStr string) error {
	if urlStr == "" {
		return fmt.Errorf("empty URL")
	}

	parsed, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	// Check that URL has a scheme
	if parsed.Scheme == "" {
		return fmt.Errorf("URL missing scheme")
	}

	// Check that URL has a host (for http/https schemes)
	if parsed.Scheme == "http" || parsed.Scheme == "https" {
		if parsed.Host == "" {
			return fmt.Errorf("URL missing host")
		}
	}

	return nil
}

// validateHeaderName validates an HTTP header name per RFC 7230.
// Header names must be valid token characters (alphanumeric and specific symbols).
func validateHeaderName(name string) error {
	if name == "" {
		return fmt.Errorf("header name cannot be empty")
	}

	// RFC 7230: header names are tokens (alphanumeric and specific symbols)
	// Check for invalid characters: control characters, spaces, and specific invalid chars
	for _, r := range name {
		if r < 0x21 || r > 0x7E {
			return fmt.Errorf("header name contains invalid character: %q", r)
		}
		if r == ':' || r == ' ' || r == '\t' || r == '\r' || r == '\n' {
			return fmt.Errorf("header name contains invalid character: %q", r)
		}
	}

	return nil
}

// validateHeaderValue validates an HTTP header value per RFC 7230.
// Header values must not contain control characters except HTAB.
func validateHeaderValue(value string) error {
	// RFC 7230: header values can contain VCHAR, obs-text, and HTAB
	// Control characters (except HTAB) are not allowed
	for _, r := range value {
		if r < 0x20 && r != '\t' {
			return fmt.Errorf("header value contains invalid control character: %q", r)
		}
		if r == '\r' || r == '\n' {
			return fmt.Errorf("header value contains invalid character: %q", r)
		}
	}

	return nil
}

// tryAddValidHeader validates header name and value per RFC 7230, logs and skips if invalid,
// adds to headers otherwise.
func tryAddValidHeader(headers map[string]string, name, value string) {
	if err := validateHeaderName(name); err != nil {
		logger.Warn(msgInvalidHeaderNameSkipping,
			slog.String("header", name),
			slog.String("error", err.Error()),
		)
		return
	}
	if err := validateHeaderValue(value); err != nil {
		logger.Warn(msgInvalidHeaderValueSkipping,
			slog.String("header", name),
			slog.String("error", err.Error()),
		)
		return
	}
	headers[name] = value
}

// extractHeadersFromRecord extracts header values from record data.
// Supports both HeadersFromRecord (field path lookup) and template syntax ({{record.field}}).
// Validates header names and values per RFC 7230 (AC #3).
func (h *HTTPRequestModule) extractHeadersFromRecord(record map[string]interface{}) map[string]string {
	headers := make(map[string]string)

	for headerName, headerValue := range h.headers {
		value := headerValue
		if HasTemplateVariables(headerValue) {
			value = h.templateEvaluator.EvaluateTemplate(headerValue, record)
			if value == "" {
				continue
			}
		}
		tryAddValidHeader(headers, headerName, value)
	}

	for headerName, fieldPath := range h.request.HeadersFromRecord {
		value := getFieldValue(record, fieldPath)
		if value != "" {
			tryAddValidHeader(headers, headerName, value)
		}
	}

	if len(headers) == 0 {
		return nil
	}
	return headers
}

// validateJSON validates that a byte slice contains valid JSON.
// Returns an error if the JSON is invalid.
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

// isJSONContentType checks if the module's Content-Type header indicates JSON.
func (h *HTTPRequestModule) isJSONContentType() bool {
	contentType := h.headers[headerContentType]
	if contentType == "" {
		contentType = defaultContentType
	}

	// Check if Content-Type is JSON (application/json, application/vnd.api+json, etc.)
	return strings.HasPrefix(strings.ToLower(contentType), "application/json") ||
		strings.Contains(strings.ToLower(contentType), "+json")
}

// truncateString truncates a string to a maximum length for logging.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
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

// buildBaseHeadersMap returns defaults + validated static config headers + record headers.
// Static config headers are validated via tryAddValidHeader; templated ones are omitted
// (they come from recordHeaders, already validated in extractHeadersFromRecord).
func (h *HTTPRequestModule) buildBaseHeadersMap(recordHeaders map[string]string) map[string]string {
	headers := make(map[string]string)
	headers[headerUserAgent] = defaultUserAgent
	headers[headerContentType] = defaultContentType

	for key, value := range h.headers {
		if HasTemplateVariables(value) {
			continue
		}
		tryAddValidHeader(headers, key, value)
	}

	for key, value := range recordHeaders {
		headers[key] = value
	}
	return headers
}

// buildPreviewHeaders constructs the headers map for preview
// If opts.ShowCredentials is false, sensitive auth headers are masked
func (h *HTTPRequestModule) buildPreviewHeaders(recordHeaders map[string]string, opts PreviewOptions) map[string]string {
	headers := h.buildBaseHeadersMap(recordHeaders)

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
