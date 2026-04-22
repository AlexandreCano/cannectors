// Package filter provides implementations for filter modules.
// HTTPCall module makes HTTP requests to external APIs and can enrich or transform records.
package filter

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

	"github.com/cannectors/runtime/internal/auth"
	"github.com/cannectors/runtime/internal/cache"
	"github.com/cannectors/runtime/internal/httpclient"
	"github.com/cannectors/runtime/internal/logger"
	"github.com/cannectors/runtime/internal/moduleconfig"
	"github.com/cannectors/runtime/internal/template"
	"github.com/cannectors/runtime/pkg/connector"
)

// Default configuration values for http_call module
const (
	defaultHTTPCallTimeout  = 30 * time.Second
	defaultCacheMaxSize     = 1000
	defaultCacheTTLSeconds  = 300 // 5 minutes
	defaultHTTPCallStrategy = "merge"
)

// Error codes for http_call module
const (
	ErrCodeHTTPCallEndpointMissing = "HTTP_CALL_ENDPOINT_MISSING"
	ErrCodeHTTPCallKeyMissing      = "HTTP_CALL_KEY_MISSING"
	ErrCodeHTTPCallKeyInvalid      = "HTTP_CALL_KEY_INVALID"
	ErrCodeHTTPCallKeyExtract      = "HTTP_CALL_KEY_EXTRACT"
	ErrCodeHTTPCallHTTPError       = "HTTP_CALL_HTTP_ERROR"
	ErrCodeHTTPCallJSONParse       = "HTTP_CALL_JSON_PARSE"
	ErrCodeHTTPCallMerge           = "HTTP_CALL_MERGE"
)

// Error messages
const (
	errMsgParsingEndpointURL = "parsing endpoint URL: %w"
)

// Error types for http_call module
var (
	ErrHTTPCallEndpointMissing = fmt.Errorf("http_call endpoint is required")
	ErrHTTPCallKeyMissing      = fmt.Errorf("http_call key configuration is required")
	ErrHTTPCallKeyInvalid      = fmt.Errorf("http_call key paramType must be 'query', 'path', or 'header'")
)

// HTTPCallConfig represents the configuration for an http_call filter module.
type HTTPCallConfig struct {
	connector.ModuleBase
	moduleconfig.HTTPRequestBase

	// Body template configuration
	BodyTemplateFile string `json:"bodyTemplateFile,omitempty"`

	// Data extraction configuration
	DataField string `json:"dataField,omitempty"`

	// Keys defines how to extract values from records and use them in requests (required for GET, optional for POST/PUT with template)
	Keys []moduleconfig.KeyConfig `json:"keys"`
	// Cache defines cache behavior (optional, uses defaults if not specified)
	Cache moduleconfig.CacheConfig `json:"cache"`
	// MergeStrategy defines how to merge response data: "merge" (default), "replace", "append"
	MergeStrategy string `json:"mergeStrategy"`
	// Retry defines the retry policy for the HTTP call (optional).
	Retry *connector.RetryConfig `json:"retry,omitempty"`
}

// HTTPCallModule implements a filter that makes HTTP requests and can enrich records with response data.
// It supports caching to avoid redundant API calls for records with the same key value.
//
// Thread Safety:
//   - The cache is thread-safe (uses mutex internally)
//   - Process() can be called from multiple goroutines
//
// Error Handling:
//   - HTTP errors are not cached (only successful responses are cached)
//   - onError mode controls behavior: fail (stop pipeline), skip (drop record), log (continue)
type HTTPCallModule struct {
	endpoint          string
	method            string                   // HTTP method (GET, POST, PUT)
	keys              []moduleconfig.KeyConfig // key configurations for request building
	authHandler       auth.Handler
	httpClient        *httpclient.Client
	retry             connector.RetryConfig
	cache             cache.Cache
	mergeStrategy     string
	dataField         string
	onError           string
	headers           map[string]string
	cacheTTL          time.Duration
	cacheKey          string              // Cache key configuration (optional)
	bodyTemplateRaw   string              // Loaded body template content (for POST/PUT)
	templateEvaluator *template.Evaluator // Template evaluator for dynamic content
}

// HTTPCallError carries structured context for http_call failures.
type HTTPCallError struct {
	Code        string
	Message     string
	RecordIndex int
	Endpoint    string
	StatusCode  int
	KeyValue    string
	Details     map[string]interface{}
}

func (e *HTTPCallError) Error() string {
	return e.Message
}

// sanitizeURL removes sensitive information from URLs for error messages.
// Masks query parameters and fragments to prevent exposing credentials or tokens.
func sanitizeURL(urlStr string) string {
	parsed, err := url.Parse(urlStr)
	if err != nil {
		// If parsing fails, return a safe placeholder
		return "[invalid URL]"
	}
	// Remove query parameters and fragments
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

// newHTTPCallError creates an HTTPCallError with context.
func newHTTPCallError(code, message string, recordIdx int, endpoint string, statusCode int, keyValue string) *HTTPCallError {
	// Sanitize endpoint URL in error message to avoid exposing sensitive data
	sanitizedEndpoint := sanitizeURL(endpoint)
	return &HTTPCallError{
		Code:        code,
		Message:     message,
		RecordIndex: recordIdx,
		Endpoint:    sanitizedEndpoint,
		StatusCode:  statusCode,
		KeyValue:    keyValue,
		Details:     make(map[string]interface{}),
	}
}

// NewHTTPCallFromConfig creates a new http_call filter module from configuration.
// It validates the configuration and initializes the HTTP client and cache.
//
// Required config fields:
//   - endpoint: The HTTP endpoint URL
//   - key: Key extraction configuration (field, paramType, paramName) - required for GET, optional for POST/PUT
//
// Optional config fields:
//   - method: HTTP method (GET, POST, PUT). Defaults to GET.
//   - auth: Authentication configuration
//   - cache: Cache configuration (maxSize, defaultTTL)
//   - mergeStrategy: How to merge data ("merge", "replace", "append")
//   - dataField: JSON field containing the data array
//   - onError: Error handling mode ("fail", "skip", "log")
//   - timeoutMs: Request timeout in milliseconds
//   - headers: Custom HTTP headers (supports {{record.field}} templates)
//   - bodyTemplateFile: Path to external template file for POST/PUT requests
func NewHTTPCallFromConfig(config HTTPCallConfig) (*HTTPCallModule, error) {
	if config.Endpoint == "" {
		return nil, newHTTPCallError(ErrCodeHTTPCallEndpointMissing, "http_call endpoint is required", -1, "", 0, "")
	}

	method, err := normalizeHTTPCallMethod(config.Method)
	if err != nil {
		return nil, err
	}

	keyRequired := method == http.MethodGet || config.BodyTemplateFile == ""
	if keyRequired {
		if keyErr := validateKeysConfig(config.Keys); keyErr != nil {
			return nil, keyErr
		}
	}

	mergeStrategy := normalizeHTTPCallMergeStrategy(config.MergeStrategy)
	onError := normalizeHTTPCallOnError(config.OnError)
	timeout := connector.GetTimeoutDuration(config.TimeoutMs, defaultHTTPCallTimeout)

	httpClient := httpclient.NewClient(timeout)
	authHandler, err := buildHTTPCallAuth(config.Authentication, httpClient.Client)
	if err != nil {
		return nil, err
	}
	// Retry is opt-in on the filter: callers must explicitly set it in the
	// module config. When absent, the zero-valued RetryConfig disables
	// retries (MaxAttempts=0), preserving the historical behavior of
	// http_call.
	var retryConfig connector.RetryConfig
	if config.Retry != nil {
		retryConfig = moduleconfig.ToRetryConfig(config.Retry)
	}

	lruCache, cacheTTL := buildHTTPCallCache(config.Cache.MaxSize, config.Cache.DefaultTTL)

	bodyTemplateRaw, err := loadHTTPCallBodyTemplate(config.BodyTemplateFile)
	if err != nil {
		return nil, err
	}

	if err := validateHTTPCallTemplates(config.Endpoint, config.Headers); err != nil {
		return nil, err
	}

	hasTemplating := httpCallHasTemplating(config.Endpoint, config.Headers, config.BodyTemplateFile != "")
	cacheMaxSize := config.Cache.MaxSize
	if cacheMaxSize <= 0 {
		cacheMaxSize = defaultCacheMaxSize
	}
	cacheTTLSeconds := config.Cache.DefaultTTL
	if cacheTTLSeconds <= 0 {
		cacheTTLSeconds = defaultCacheTTLSeconds
	}

	logger.Debug("http_call module initialized",
		slog.String("endpoint", config.Endpoint),
		slog.String("method", method),
		slog.Int("keys_count", len(config.Keys)),
		slog.String("merge_strategy", mergeStrategy),
		slog.String("on_error", onError),
		slog.Int("cache_max_size", cacheMaxSize),
		slog.Int("cache_ttl_seconds", cacheTTLSeconds),
		slog.String("cache_key", config.Cache.Key),
		slog.Bool("has_auth", authHandler != nil),
		slog.Bool("has_templating", hasTemplating),
		slog.String("body_template_file", config.BodyTemplateFile),
	)

	keyEntries := make([]moduleconfig.KeyConfig, len(config.Keys))
	copy(keyEntries, config.Keys)

	return &HTTPCallModule{
		endpoint:          config.Endpoint,
		method:            method,
		keys:              keyEntries,
		authHandler:       authHandler,
		httpClient:        httpClient,
		retry:             retryConfig,
		cache:             lruCache,
		mergeStrategy:     mergeStrategy,
		dataField:         config.DataField,
		onError:           onError,
		headers:           config.Headers,
		cacheTTL:          cacheTTL,
		cacheKey:          config.Cache.Key,
		bodyTemplateRaw:   bodyTemplateRaw,
		templateEvaluator: template.NewEvaluator(),
	}, nil
}

// Process enriches each input record by performing an HTTP call and merging the response data.
// Returns the records with merged enrichment data according to the configured merge strategy.
func (m *HTTPCallModule) Process(ctx context.Context, records []map[string]interface{}) ([]map[string]interface{}, error) {
	if records == nil {
		return []map[string]interface{}{}, nil
	}

	startTime := time.Now()
	inputCount := len(records)

	logger.Debug("filter processing started",
		slog.String("module_type", "http_call"),
		slog.Int("input_records", inputCount),
		slog.String("on_error", m.onError),
	)

	result := make([]map[string]interface{}, 0, len(records))
	skippedCount := 0
	errorCount := 0
	cacheHits := 0
	cacheMisses := 0

	for recordIdx, record := range records {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		enrichedRecord, wasCacheHit, err := m.processRecord(ctx, record, recordIdx)
		if wasCacheHit {
			cacheHits++
		} else if err == nil {
			cacheMisses++
		}

		if err != nil {
			errorCount++
			switch m.onError {
			case OnErrorFail:
				duration := time.Since(startTime)
				logger.Error("filter processing failed",
					slog.String("module_type", "http_call"),
					slog.Int("record_index", recordIdx),
					slog.Duration("duration", duration),
					slog.String("error", err.Error()),
				)
				return nil, err
			case OnErrorSkip:
				skippedCount++
				logger.Warn("skipping record due to http_call error",
					slog.String("module_type", "http_call"),
					slog.Int("record_index", recordIdx),
					slog.String("error", err.Error()),
				)
				continue
			case OnErrorLog:
				logger.Error("http_call error (continuing)",
					slog.String("module_type", "http_call"),
					slog.Int("record_index", recordIdx),
					slog.String("error", err.Error()),
				)
				// For log mode, add the original record (not enriched)
				result = append(result, record)
				continue
			}
		}
		result = append(result, enrichedRecord)
	}

	duration := time.Since(startTime)
	outputCount := len(result)

	logger.Info("filter processing completed",
		slog.String("module_type", "http_call"),
		slog.Int("input_records", inputCount),
		slog.Int("output_records", outputCount),
		slog.Int("skipped_records", skippedCount),
		slog.Int("error_count", errorCount),
		slog.Int("cache_hits", cacheHits),
		slog.Int("cache_misses", cacheMisses),
		slog.Duration("duration", duration),
	)

	return result, nil
}

// processRecord makes an HTTP call for a single record and enriches it with the response data.
// Returns the enriched record, whether it was a cache hit, and any error.
func (m *HTTPCallModule) processRecord(ctx context.Context, record map[string]interface{}, recordIdx int) (map[string]interface{}, bool, error) {
	// Extract key values from record (may be empty for POST/PUT with template-only mode)
	var keyValues map[string]string
	var err error
	if len(m.keys) > 0 {
		keyValues, err = m.extractKeyValues(record, recordIdx)
		if err != nil {
			return nil, false, err
		}
	}

	// Check cache first
	cacheKey := m.buildCacheKey(keyValues, record)
	if cachedData, found := m.cache.Get(cacheKey); found {
		responseData, ok := cachedData.(map[string]interface{})
		if ok {
			logger.Debug("http_call cache hit",
				slog.String("module_type", "http_call"),
				slog.Int("record_index", recordIdx),
				slog.Any("key_values", keyValues),
			)
			enrichedRecord := m.mergeData(record, responseData)
			return enrichedRecord, true, nil
		}
	}

	// Cache miss - make HTTP request
	responseData, err := m.fetchResponseData(ctx, keyValues, recordIdx, record)
	if err != nil {
		return nil, false, err
	}

	// Cache successful response (don't cache errors)
	// Rebuild cache key in case it depends on record values
	cacheKey = m.buildCacheKey(keyValues, record)
	m.cache.Set(cacheKey, responseData, m.cacheTTL)

	logger.Debug("http_call cache miss (fetched and cached)",
		slog.String("module_type", "http_call"),
		slog.Int("record_index", recordIdx),
		slog.Any("key_values", keyValues),
	)

	// Merge data into record
	enrichedRecord := m.mergeData(record, responseData)

	return enrichedRecord, false, nil
}

// extractKeyValues extracts key values from a record using all configured key definitions.
func (m *HTTPCallModule) extractKeyValues(record map[string]interface{}, recordIdx int) (map[string]string, error) {
	result := make(map[string]string, len(m.keys))
	for _, k := range m.keys {
		value, found := moduleconfig.GetNestedValue(record, k.Field)
		if !found {
			return nil, newHTTPCallError(
				ErrCodeHTTPCallKeyExtract,
				fmt.Sprintf("http_call failed to extract key from record %d: field '%s' not found", recordIdx, k.Field),
				recordIdx, m.endpoint, 0, "",
			)
		}

		var keyValue string
		switch v := value.(type) {
		case string:
			keyValue = v
		case nil:
			return nil, newHTTPCallError(
				ErrCodeHTTPCallKeyExtract,
				fmt.Sprintf("http_call failed to extract key from record %d: field '%s' is null", recordIdx, k.Field),
				recordIdx, m.endpoint, 0, "",
			)
		default:
			keyValue = fmt.Sprintf("%v", v)
		}

		if keyValue == "" {
			return nil, newHTTPCallError(
				ErrCodeHTTPCallKeyExtract,
				fmt.Sprintf("http_call failed to extract key from record %d: field '%s' is empty", recordIdx, k.Field),
				recordIdx, m.endpoint, 0, "",
			)
		}
		result[k.ParamName] = keyValue
	}
	return result, nil
}

// buildCacheKey creates a unique cache key from the key values and record.
// If cacheKey is configured, it can be:
//   - A static string: used as-is
//   - A dot notation path: "customerId" or "user.profile.id" (extracts value from record)
//
// If cacheKey is not configured, uses default: endpoint + "::" + joined key values (in config order)
func (m *HTTPCallModule) buildCacheKey(keyValues map[string]string, record map[string]interface{}) string {
	if m.cacheKey != "" {
		if value, found := moduleconfig.GetNestedValue(record, m.cacheKey); found {
			return fmt.Sprintf("%v", value)
		}
		return m.cacheKey
	}

	return m.endpoint + "::" + m.compositeKeyString(keyValues)
}

// compositeKeyString joins key values in config order for cache key.
func (m *HTTPCallModule) compositeKeyString(keyValues map[string]string) string {
	if keyValues == nil || len(m.keys) == 0 {
		return ""
	}
	parts := make([]string, len(m.keys))
	for i, k := range m.keys {
		parts[i] = keyValues[k.ParamName]
	}
	return strings.Join(parts, "::")
}

// fetchResponseData fetches data from the external API for the given key values and record.
func (m *HTTPCallModule) fetchResponseData(ctx context.Context, keyValues map[string]string, recordIdx int, record map[string]interface{}) (map[string]interface{}, error) {
	// Build and execute HTTP request
	body, statusCode, err := m.executeHTTPRequest(ctx, keyValues, recordIdx, record)
	if err != nil {
		return nil, err
	}

	// Parse and extract response data
	return m.parseResponseData(body, statusCode, recordIdx)
}

// executeHTTPRequest builds the HTTP request, executes it with retry via
// httpclient.DoWithRetry, and returns the response body and status code.
// Story 15.5 introduces retry support (previously absent from this filter).
func (m *HTTPCallModule) executeHTTPRequest(ctx context.Context, keyValues map[string]string, recordIdx int, record map[string]interface{}) ([]byte, int, error) {
	requestURL, err := m.buildRequestURL(keyValues, record)
	if err != nil {
		return nil, 0, newHTTPCallError(
			ErrCodeHTTPCallHTTPError,
			fmt.Sprintf("http_call failed to build request URL: %v", err),
			recordIdx, m.endpoint, 0, m.compositeKeyString(keyValues),
		)
	}

	req, err := m.buildHTTPRequest(ctx, requestURL, keyValues, recordIdx, record)
	if err != nil {
		return nil, 0, err
	}

	hooks := httpclient.RetryHooks{
		OnRetry: func(attempt int, retryErr error, nextDelay time.Duration) {
			if retryErr != nil && nextDelay > 0 {
				logger.Info("retrying http_call request",
					slog.String("module_type", "http_call"),
					slog.String("endpoint", m.endpoint),
					slog.Int("attempt", attempt+1),
					slog.Int("max_attempts", m.retry.MaxAttempts+1),
					slog.Duration("next_delay", nextDelay),
					slog.String("error", retryErr.Error()),
				)
			}
		},
	}

	resp, err := m.httpClient.DoWithRetry(ctx, req, m.retry, hooks)
	if resp != nil {
		defer func() {
			if closeErr := resp.Body.Close(); closeErr != nil {
				logger.Warn("failed to close http_call response body",
					slog.String("error", closeErr.Error()),
				)
			}
		}()
	}
	if err != nil {
		statusCode := 0
		var httpErr *httpclient.Error
		if errors.As(err, &httpErr) {
			statusCode = httpErr.StatusCode
		}
		if resp != nil && statusCode == 0 {
			statusCode = resp.StatusCode
		}
		if statusCode >= 400 {
			var bodySnippet string
			if httpErr != nil {
				bodySnippet = httpErr.ResponseBody
				if len(bodySnippet) > 200 {
					bodySnippet = bodySnippet[:200] + "..."
				}
			}
			return nil, statusCode, newHTTPCallError(
				ErrCodeHTTPCallHTTPError,
				fmt.Sprintf("http_call HTTP error %d: %s", statusCode, bodySnippet),
				recordIdx, m.endpoint, statusCode, m.compositeKeyString(keyValues),
			)
		}
		return nil, statusCode, newHTTPCallError(
			ErrCodeHTTPCallHTTPError,
			fmt.Sprintf("http_call HTTP request failed: %v", err),
			recordIdx, m.endpoint, statusCode, m.compositeKeyString(keyValues),
		)
	}

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, resp.StatusCode, newHTTPCallError(
			ErrCodeHTTPCallHTTPError,
			fmt.Sprintf("http_call failed to read response: %v", readErr),
			recordIdx, m.endpoint, resp.StatusCode, m.compositeKeyString(keyValues),
		)
	}
	return body, resp.StatusCode, nil
}

// buildHTTPRequest creates and configures an HTTP request with headers and authentication.
func (m *HTTPCallModule) buildHTTPRequest(ctx context.Context, requestURL string, keyValues map[string]string, recordIdx int, record map[string]interface{}) (*http.Request, error) {
	// Build request body for POST/PUT
	var bodyReader io.Reader
	if m.method == http.MethodPost || m.method == http.MethodPut {
		if m.bodyTemplateRaw != "" {
			// Evaluate body template with record data
			bodyContent := m.templateEvaluator.Evaluate(m.bodyTemplateRaw, record)
			bodyReader = bytes.NewReader([]byte(bodyContent))
		}
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, m.method, requestURL, bodyReader)
	if err != nil {
		return nil, newHTTPCallError(
			ErrCodeHTTPCallHTTPError,
			fmt.Sprintf("http_call failed to create request: %v", err),
			recordIdx, m.endpoint, 0, m.compositeKeyString(keyValues),
		)
	}

	// Set default headers
	req.Header.Set("User-Agent", "Cannectors-Runtime/1.0")
	req.Header.Set("Accept", "application/json")

	// Set Content-Type for POST/PUT
	if m.method == http.MethodPost || m.method == http.MethodPut {
		req.Header.Set("Content-Type", "application/json")
	}

	// Validate custom headers and key-based headers per RFC 7230 before
	// attaching them to the request (mirrors the output module's safeguard).
	validated := make(map[string]string, len(m.headers)+len(m.keys))
	for key, value := range m.headers {
		evaluatedValue := value
		if template.HasVariables(value) {
			evaluatedValue = m.templateEvaluator.Evaluate(value, record)
		}
		httpclient.TryAddValidHeader(validated, key, evaluatedValue)
	}
	for _, k := range m.keys {
		if k.ParamType == "header" {
			if v := keyValues[k.ParamName]; v != "" {
				httpclient.TryAddValidHeader(validated, k.ParamName, v)
			}
		}
	}
	for key, value := range validated {
		req.Header.Set(key, value)
	}

	// Apply authentication
	if m.authHandler != nil {
		if authErr := m.authHandler.ApplyAuth(ctx, req); authErr != nil {
			return nil, newHTTPCallError(
				ErrCodeHTTPCallHTTPError,
				fmt.Sprintf("http_call failed to apply auth: %v", authErr),
				recordIdx, m.endpoint, 0, m.compositeKeyString(keyValues),
			)
		}
	}

	return req, nil
}

// parseResponseData parses the JSON response and extracts the data field if configured.
func (m *HTTPCallModule) parseResponseData(body []byte, statusCode int, recordIdx int) (map[string]interface{}, error) {
	// Parse JSON response
	var responseData map[string]interface{}
	if err := json.Unmarshal(body, &responseData); err != nil {
		return nil, newHTTPCallError(
			ErrCodeHTTPCallJSONParse,
			fmt.Sprintf("http_call failed to parse response: %v", err),
			recordIdx, m.endpoint, statusCode, "",
		)
	}

	// Extract data field if configured
	if m.dataField != "" {
		return m.extractDataField(responseData, recordIdx), nil
	}

	return responseData, nil
}

// extractDataField extracts the configured data field from the response.
// Returns an empty map if the field is not found or invalid.
func (m *HTTPCallModule) extractDataField(responseData map[string]interface{}, recordIdx int) map[string]interface{} {
	data, ok := responseData[m.dataField]
	if !ok {
		logger.Warn("http_call dataField not found or invalid",
			slog.String("data_field", m.dataField),
			slog.Int("record_index", recordIdx),
		)
		return make(map[string]interface{})
	}

	// If dataField points to a map, return it
	if dataMap, ok := data.(map[string]interface{}); ok {
		return dataMap
	}

	// If dataField points to an array with single element, use that
	if dataArr, ok := data.([]interface{}); ok && len(dataArr) == 1 {
		if dataMap, ok := dataArr[0].(map[string]interface{}); ok {
			return dataMap
		}
	}

	// dataField not an object - return empty
	logger.Warn("http_call dataField not found or invalid",
		slog.String("data_field", m.dataField),
		slog.Int("record_index", recordIdx),
	)
	return make(map[string]interface{})
}

// buildRequestURL constructs the HTTP request URL based on the key configurations and template evaluation.
func (m *HTTPCallModule) buildRequestURL(keyValues map[string]string, record map[string]interface{}) (string, error) {
	endpoint := m.endpoint

	// Evaluate template variables in endpoint ({{record.field}} syntax)
	if template.HasVariables(endpoint) {
		endpoint = m.templateEvaluator.EvaluateForURL(endpoint, record)
	}

	// If no key configuration, return the templated endpoint as-is
	if len(m.keys) == 0 {
		return endpoint, nil
	}

	// Apply path replacements first
	for _, k := range m.keys {
		if k.ParamType == "path" {
			placeholder := "{" + k.ParamName + "}"
			endpoint = strings.Replace(endpoint, placeholder, url.PathEscape(keyValues[k.ParamName]), 1)
		}
	}

	// Apply query parameters
	hasQuery := false
	for _, k := range m.keys {
		if k.ParamType == "query" {
			hasQuery = true
			break
		}
	}
	if hasQuery {
		parsedURL, err := url.Parse(endpoint)
		if err != nil {
			return "", fmt.Errorf(errMsgParsingEndpointURL, err)
		}
		q := parsedURL.Query()
		for _, k := range m.keys {
			if k.ParamType == "query" {
				q.Set(k.ParamName, keyValues[k.ParamName])
			}
		}
		parsedURL.RawQuery = q.Encode()
		return parsedURL.String(), nil
	}

	// Path-only or header-only: return endpoint after path replacements
	return endpoint, nil
}
