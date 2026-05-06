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
	"os"
	"strings"
	"time"

	"github.com/expr-lang/expr/vm"

	"github.com/cannectors/runtime/internal/auth"
	"github.com/cannectors/runtime/internal/errhandling"
	"github.com/cannectors/runtime/internal/httpclient"
	"github.com/cannectors/runtime/internal/logger"
	"github.com/cannectors/runtime/internal/metadata"
	"github.com/cannectors/runtime/internal/moduleconfig"
	"github.com/cannectors/runtime/internal/template"
	"github.com/cannectors/runtime/pkg/connector"
)

// Default configuration values for HTTP Request module
const (
	defaultHTTPTimeout = 30 * time.Second
	defaultUserAgent   = "Cannectors-Runtime/1.0"
	defaultContentType = "application/json"
	defaultRequestMode = "batch" // batch mode by default
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

// keyEntry holds parsed key config for request building (internal use).
type keyEntry struct {
	field     string
	paramType string
	paramName string
}

// RequestConfig holds request-specific configuration
type RequestConfig struct {
	RequestMode      string            // "batch" or "single"
	Keys             []keyEntry        // Dynamic params from record (path, query, header)
	QueryParams      map[string]string // Static query parameters
	BodyTemplateFile string            // Path to external template file for request body
	bodyTemplateRaw  string            // Loaded template content (internal use)
}

// HTTPRequestModule implements HTTP-based data sending.
// It sends transformed records to a target REST API via HTTP requests.
type HTTPRequestModule struct {
	endpoint          string
	method            string
	headers           map[string]string
	timeout           time.Duration
	request           RequestConfig
	retry             connector.RetryConfig
	authHandler       auth.Handler
	client            *httpclient.Client
	onError           errhandling.OnErrorStrategy // "fail", "skip", "log"
	successCodes      []int                       // HTTP status codes considered success
	lastRetryInfo     *connector.RetryInfo
	retryHintProgram  *vm.Program         // Compiled expr program for retryHintFromBody
	templateEvaluator *template.Evaluator // Template evaluator for dynamic content
}

// HTTPRequestOutputConfig holds typed configuration for the HTTP request output module.
type HTTPRequestOutputConfig struct {
	connector.ModuleBase
	moduleconfig.HTTPRequestBase
	RequestMode      string                   `json:"requestMode,omitempty"`
	Keys             []moduleconfig.KeyConfig `json:"keys,omitempty"`
	BodyTemplateFile string                   `json:"bodyTemplateFile,omitempty"`
	Success          *SuccessCodeConfig       `json:"success,omitempty"`
	Retry            *connector.RetryConfig   `json:"retry,omitempty"`
}

// SuccessCodeConfig holds success status codes configuration.
type SuccessCodeConfig struct {
	StatusCodes []int `json:"statusCodes,omitempty"`
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
//   - requestMode, keys, queryParams, bodyTemplateFile
//   - onError: Error handling mode ("fail", "skip", "log")
func NewHTTPRequestFromConfig(config *connector.ModuleConfig) (*HTTPRequestModule, error) {
	if config == nil {
		return nil, ErrNilConfig
	}

	cfg, err := moduleconfig.ParseModuleConfig[HTTPRequestOutputConfig](*config)
	if err != nil {
		return nil, err
	}
	if cfg.Endpoint == "" {
		return nil, ErrMissingEndpoint
	}
	if cfg.Method == "" {
		return nil, ErrMissingMethod
	}
	method := strings.ToUpper(cfg.Method)
	if !supportedMethods[method] {
		return nil, fmt.Errorf("%w: %s", ErrInvalidMethod, method)
	}

	timeout := connector.GetTimeoutDuration(cfg.TimeoutMs, defaultHTTPTimeout)
	headers := cfg.Headers
	reqConfig := RequestConfig{
		RequestMode:      cfg.RequestMode,
		QueryParams:      cfg.QueryParams,
		BodyTemplateFile: cfg.BodyTemplateFile,
	}
	if reqConfig.RequestMode == "" {
		reqConfig.RequestMode = defaultRequestMode
	}
	reqConfig.Keys = make([]keyEntry, len(cfg.Keys))
	for i, k := range cfg.Keys {
		reqConfig.Keys[i] = keyEntry{field: k.Field, paramType: k.ParamType, paramName: k.ParamName}
	}

	var onError errhandling.OnErrorStrategy
	if cfg.OnError != "" {
		onError = errhandling.ParseOnErrorStrategy(cfg.OnError)
	} else {
		onError = errhandling.OnErrorFail
	}

	var successCodes []int
	if cfg.Success != nil && len(cfg.Success.StatusCodes) > 0 {
		successCodes = cfg.Success.StatusCodes
	} else {
		successCodes = defaultSuccessCodes
	}

	retryConfig := moduleconfig.ToRetryConfig(cfg.Retry)

	client := httpclient.NewClient(timeout)

	// Create authentication handler if configured
	authHandler, err := auth.NewHandler(cfg.Authentication, client.Client)
	if err != nil {
		return nil, fmt.Errorf("creating auth handler: %w", err)
	}

	retryHintProgram, err := httpclient.CompileRetryHint(retryConfig.RetryHintFromBody)
	if err != nil {
		return nil, err
	}

	// Validate template syntax in endpoint and headers
	if err := validateTemplateConfig(cfg.Endpoint, headers); err != nil {
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
		if err := template.ValidateSyntax(reqConfig.bodyTemplateRaw); err != nil {
			return nil, fmt.Errorf("invalid template syntax in %q: %w", reqConfig.BodyTemplateFile, err)
		}

		logger.Debug("loaded body template file",
			slog.String("file", reqConfig.BodyTemplateFile),
			slog.Int("size", len(templateContent)),
		)
	}

	module := &HTTPRequestModule{
		endpoint:          cfg.Endpoint,
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
		templateEvaluator: template.NewEvaluator(),
	}

	// Check if endpoint/headers use templating
	hasTemplating := template.HasVariables(cfg.Endpoint)
	for _, v := range headers {
		if template.HasVariables(v) {
			hasTemplating = true
			break
		}
	}

	logger.Debug("http request output module created",
		slog.String("endpoint", cfg.Endpoint),
		slog.String("method", method),
		slog.String("timeout", timeout.String()),
		slog.Bool("has_auth", authHandler != nil),
		slog.String("request_mode", reqConfig.RequestMode),
		slog.Bool("has_templating", hasTemplating),
		slog.String("body_template_file", reqConfig.BodyTemplateFile),
	)

	return module, nil
}

// validateTemplateConfig validates template syntax in configuration.
func validateTemplateConfig(endpoint string, headers map[string]string) error {
	// Validate endpoint template syntax
	if err := template.ValidateSyntax(endpoint); err != nil {
		return fmt.Errorf("invalid endpoint template: %w", err)
	}

	// Validate header template syntax
	for name, value := range headers {
		if err := template.ValidateSyntax(value); err != nil {
			return fmt.Errorf("invalid template in header %q: %w", name, err)
		}
	}

	return nil
}

// Send transmits records to the destination via HTTP.
// Returns the number of records successfully sent and any error encountered.
//
// The context can be used to cancel long-running operations.
//
// Behavior depends on requestMode configuration:
//   - "batch" (default): Sends all records in a single request as JSON array
//   - "single": Sends one request per record, body is single JSON object
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
		slog.String("request_mode", h.request.RequestMode),
		slog.Bool("has_auth", h.authHandler != nil),
	)

	var sent int
	var err error

	// Choose send mode based on configuration
	if h.request.RequestMode == "single" {
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
			bodyStr := h.templateEvaluator.Evaluate(h.request.bodyTemplateRaw, records[0])
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
		// Default: marshal records to JSON array (with metadata stripped)
		recordsForBody := metadata.StripFromRecords(records, metadata.DefaultFieldName)
		body, err = json.Marshal(recordsForBody)
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
		bodyStr := h.templateEvaluator.Evaluate(h.request.bodyTemplateRaw, record)
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
	// Strip metadata before serialization
	recordForBody := metadata.StripFromRecord(record, metadata.DefaultFieldName)
	body, err := json.Marshal(recordForBody)
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

// handleOAuth2Unauthorized handles 401 Unauthorized for OAuth2 authentication.
// It invalidates the cached token and asks the retry loop to try again with a
// fresh token, but only up to auth.MaxOAuth2Retries times in the same request
// cycle. After that, returning false stops the retry loop so the caller can
// surface ErrOAuth2InvalidCredentials instead of looping forever (Story 17.5).
func (h *HTTPRequestModule) handleOAuth2Unauthorized(resp *http.Response, retryCount *int) bool {
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		return false
	}
	if h.authHandler == nil {
		return false
	}
	invalidator, ok := h.authHandler.(interface{ InvalidateToken() })
	if !ok {
		return false
	}
	if *retryCount >= auth.MaxOAuth2Retries {
		logger.Warn("401 Unauthorized persists after OAuth2 token refresh, likely invalid credentials",
			slog.String("endpoint", h.endpoint),
			slog.String("method", h.method),
			slog.Int("oauth2_retry_count", *retryCount),
		)
		return false
	}

	logger.Debug("401 Unauthorized with OAuth2, invalidating token and retrying",
		slog.String("endpoint", h.endpoint),
		slog.String("method", h.method),
		slog.Int("oauth2_retry_count", *retryCount),
	)
	invalidator.InvalidateToken()
	*retryCount++
	return true
}

// doRequestWithHeaders executes a single HTTP request with optional
// record-specific headers, delegating the retry loop to httpclient.DoWithRetry.
// Special handling for 401 with OAuth2: invalidates the token and retries
// once with a new token via the OnAttemptFailure hook.
func (h *HTTPRequestModule) doRequestWithHeaders(ctx context.Context, endpoint string, body []byte, recordHeaders map[string]string) error {
	startTime := time.Now()

	req, err := h.buildHTTPRequest(ctx, endpoint, body, recordHeaders)
	if err != nil {
		return err
	}

	var delaysMs []int64
	oauth2RetryCount := 0

	hooks := httpclient.RetryHooks{
		OnRetry: func(attempt int, retryErr error, nextDelay time.Duration) {
			if retryErr == nil {
				return
			}
			if nextDelay > 0 {
				delaysMs = append(delaysMs, nextDelay.Milliseconds())
			}
			logger.Info("retrying request",
				slog.String("module_type", "httpRequest"),
				slog.String("endpoint", endpoint),
				slog.String("method", h.method),
				slog.Int("attempt", attempt+1),
				slog.Int("max_attempts", h.retry.MaxAttempts),
				slog.Duration("backoff", nextDelay),
				slog.String("error", retryErr.Error()),
				slog.String("error_category", string(errhandling.GetErrorCategory(retryErr))),
			)
		},
		ShouldRetryBody: func(respBody []byte) (bool, bool) {
			return httpclient.EvalRetryHint(h.retryHintProgram, respBody)
		},
		OnAttemptFailure: func(_ int, resp *http.Response, _ error) bool {
			return h.handleOAuth2Unauthorized(resp, &oauth2RetryCount)
		},
	}

	resp, err := h.client.DoWithRetry(ctx, req, h.retry, hooks)
	if resp != nil {
		defer func() {
			if closeErr := resp.Body.Close(); closeErr != nil {
				logger.Warn("failed to close response body",
					slog.String("endpoint", endpoint),
					slog.String("error", closeErr.Error()),
				)
			}
		}()
	}
	if err != nil {
		// Story 17.5: surface a typed authentication error when 401 persists
		// after the maximum number of OAuth2 token-refresh retries.
		if oauth2RetryCount >= auth.MaxOAuth2Retries && resp != nil && resp.StatusCode == http.StatusUnauthorized {
			err = fmt.Errorf("%w: endpoint=%s status=%d", auth.ErrOAuth2InvalidCredentials, endpoint, resp.StatusCode)
		}
		return h.recordRetryFailure(err, delaysMs, startTime, endpoint)
	}

	if !h.isSuccessStatusCode(resp.StatusCode) {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024))
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
			slog.String("response_body", bodySnippet),
		)
		// Story 17.5: when 401 persists after the maximum number of OAuth2
		// token-refresh retries, surface a typed authentication error so the
		// runtime classifies it as fatal instead of looping further.
		if resp.StatusCode == http.StatusUnauthorized && oauth2RetryCount >= auth.MaxOAuth2Retries {
			return h.recordRetryFailure(
				fmt.Errorf("%w: endpoint=%s status=%d", auth.ErrOAuth2InvalidCredentials, endpoint, resp.StatusCode),
				delaysMs, startTime, endpoint,
			)
		}
		classified := errhandling.ClassifyHTTPStatus(resp.StatusCode, bodySnippet)
		classified.OriginalErr = &httpclient.Error{
			StatusCode:      resp.StatusCode,
			Status:          resp.Status,
			Endpoint:        endpoint,
			Method:          h.method,
			Message:         "status not in successCodes",
			ResponseBody:    string(respBody),
			ResponseHeaders: resp.Header.Clone(),
		}
		return h.recordRetryFailure(classified, delaysMs, startTime, endpoint)
	}

	h.recordRetrySuccess(len(delaysMs), delaysMs, startTime, endpoint)
	return nil
}

// buildHTTPRequest creates the *http.Request with headers and authentication
// attached. It does not perform the network call.
func (h *HTTPRequestModule) buildHTTPRequest(ctx context.Context, endpoint string, body []byte, recordHeaders map[string]string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, h.method, endpoint, bytes.NewReader(body))
	if err != nil {
		logger.Error("failed to create http request",
			slog.String("module_type", "httpRequest"),
			slog.String("endpoint", endpoint),
			slog.String("method", h.method),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("creating http request: %w", err)
	}

	for key, value := range h.buildBaseHeadersMap(recordHeaders) {
		req.Header.Set(key, value)
	}

	if err := h.applyAuthentication(ctx, req); err != nil {
		logger.Error("failed to apply authentication",
			slog.String("module_type", "httpRequest"),
			slog.String("endpoint", endpoint),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("applying authentication: %w", err)
	}

	logger.Debug("sending http request",
		slog.String("module_type", "httpRequest"),
		slog.String("endpoint", endpoint),
		slog.String("method", h.method),
		slog.Int("body_size", len(body)),
	)
	return req, nil
}

// recordRetrySuccess captures retry metrics after a successful request.
func (h *HTTPRequestModule) recordRetrySuccess(retryCount int, delaysMs []int64, startTime time.Time, endpoint string) {
	if retryCount > 0 {
		logger.Info("retry succeeded",
			slog.String("module_type", "httpRequest"),
			slog.String("endpoint", endpoint),
			slog.Int("attempts", retryCount+1),
			slog.Duration("total_duration", time.Since(startTime)),
		)
		h.lastRetryInfo = &connector.RetryInfo{
			TotalAttempts: retryCount + 1,
			RetryCount:    retryCount,
			RetryDelaysMs: delaysMs,
		}
		return
	}
	h.lastRetryInfo = nil
}

// recordRetryFailure captures retry metrics after a final failure.
func (h *HTTPRequestModule) recordRetryFailure(lastErr error, delaysMs []int64, startTime time.Time, endpoint string) error {
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

// GetRetryInfo returns retry information from the last Send request (RetryInfoProvider).
func (h *HTTPRequestModule) GetRetryInfo() *connector.RetryInfo {
	return h.lastRetryInfo
}

// extractHeadersFromRecord extracts header values from record data.
// Supports both HeadersFromRecord (field path lookup) and template syntax ({{record.field}}).
// Validates header names and values per RFC 7230 (AC #3).
func (h *HTTPRequestModule) extractHeadersFromRecord(record map[string]interface{}) map[string]string {
	headers := make(map[string]string)

	for headerName, headerValue := range h.headers {
		value := headerValue
		if template.HasVariables(headerValue) {
			value = h.templateEvaluator.Evaluate(headerValue, record)
			if value == "" {
				continue
			}
		}
		httpclient.TryAddValidHeader(headers, headerName, value)
	}

	// Add headers from keys (paramType=header)
	for _, k := range h.request.Keys {
		if k.paramType == "header" {
			value := getRecordFieldString(record, k.field)
			if value != "" {
				httpclient.TryAddValidHeader(headers, k.paramName, value)
			}
		}
	}

	if len(headers) == 0 {
		return nil
	}
	return headers
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
	h.client.CloseIdleConnections()
	logger.Debug("http request output module closed",
		slog.String("endpoint", h.endpoint),
	)
	return nil
}

// buildBaseHeadersMap returns defaults + validated static config headers +
// record-derived headers. Static config headers are validated via
// httpclient.TryAddValidHeader; templated ones are skipped here — they come
// from recordHeaders and have already been validated in
// extractHeadersFromRecord.
func (h *HTTPRequestModule) buildBaseHeadersMap(recordHeaders map[string]string) map[string]string {
	headers := make(map[string]string)
	headers[headerUserAgent] = defaultUserAgent
	headers[headerContentType] = defaultContentType

	for key, value := range h.headers {
		if template.HasVariables(value) {
			continue
		}
		httpclient.TryAddValidHeader(headers, key, value)
	}
	for key, value := range recordHeaders {
		headers[key] = value
	}
	return headers
}
