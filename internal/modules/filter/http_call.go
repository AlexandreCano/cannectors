// Package filter provides implementations for filter modules.
// HTTPCall module makes HTTP requests to external APIs and can enrich or transform records.
package filter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/cannectors/runtime/internal/auth"
	"github.com/cannectors/runtime/internal/cache"
	"github.com/cannectors/runtime/internal/errhandling"
	"github.com/cannectors/runtime/internal/httpconfig"
	"github.com/cannectors/runtime/internal/logger"
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

// KeyConfig defines how to extract a key from a record and use it in HTTP requests.
type KeyConfig struct {
	// Field is the dot-notation path to extract the key value from the record (e.g., "customer.id")
	Field string `json:"field"`
	// ParamType specifies how to include the key in the request: "query", "path", or "header"
	ParamType string `json:"paramType"`
	// ParamName is the parameter name to use in the request
	ParamName string `json:"paramName"`
}

// CacheConfig defines cache behavior for the http_call module.
type CacheConfig struct {
	// MaxSize is the maximum number of entries in the cache (default 1000)
	MaxSize int `json:"maxSize"`
	// DefaultTTL is the TTL for cache entries in seconds (default 300)
	DefaultTTL int `json:"defaultTTL"`
	// Key is the cache key configuration (optional).
	// If not specified, uses default: endpoint + "::" + keyValue.
	// Can be:
	//   - A static string: "my-cache-key"
	//   - A dot notation path: "customerId" or "user.profile.id" (extracts value from record)
	Key string `json:"key"`
}

// HTTPCallConfig represents the configuration for an http_call filter module.
// Fields are organized in logical groups matching httpconfig types for consistency,
// but kept as direct fields for simpler struct literal syntax in tests and usage.
type HTTPCallConfig struct {
	// Base HTTP configuration (from httpconfig.BaseConfig)
	Endpoint       string                `json:"endpoint"`
	Method         string                `json:"method,omitempty"`
	Headers        map[string]string     `json:"headers,omitempty"`
	TimeoutMs      int                   `json:"timeoutMs,omitempty"`
	Authentication *connector.AuthConfig `json:"authentication,omitempty"`

	// Body template configuration (from httpconfig.BodyTemplateConfig)
	BodyTemplateFile string `json:"bodyTemplateFile,omitempty"`

	// Error handling configuration (from httpconfig.ErrorHandlingConfig)
	OnError string `json:"onError,omitempty"`

	// Data extraction configuration (from httpconfig.DataExtractionConfig)
	DataField string `json:"dataField,omitempty"`

	// Keys defines how to extract values from records and use them in requests (required for GET, optional for POST/PUT with template)
	Keys []KeyConfig `json:"keys"`
	// Cache defines cache behavior (optional, uses defaults if not specified)
	Cache CacheConfig `json:"cache"`
	// MergeStrategy defines how to merge response data: "merge" (default), "replace", "append"
	MergeStrategy string `json:"mergeStrategy"`
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
type keyEntry struct {
	field     string
	paramType string
	paramName string
}

type HTTPCallModule struct {
	endpoint          string
	method            string     // HTTP method (GET, POST, PUT)
	keys              []keyEntry // key configurations for request building
	authHandler       auth.Handler
	httpClient        *http.Client
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
	timeout := httpconfig.GetTimeoutDuration(config.TimeoutMs, defaultHTTPCallTimeout)

	httpClient := &http.Client{Timeout: timeout}
	authHandler, err := buildHTTPCallAuth(config.Authentication, httpClient)
	if err != nil {
		return nil, err
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

	keyEntries := make([]keyEntry, len(config.Keys))
	for i, k := range config.Keys {
		keyEntries[i] = keyEntry{field: k.Field, paramType: k.ParamType, paramName: k.ParamName}
	}

	return &HTTPCallModule{
		endpoint:          config.Endpoint,
		method:            method,
		keys:              keyEntries,
		authHandler:       authHandler,
		httpClient:        httpClient,
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

func normalizeHTTPCallMethod(method string) (string, error) {
	m := strings.ToUpper(method)
	if m == "" {
		m = http.MethodGet
	}
	if m != http.MethodGet && m != http.MethodPost && m != http.MethodPut {
		return "", fmt.Errorf("http_call method must be GET, POST, or PUT, got: %s", method)
	}
	return m, nil
}

func normalizeHTTPCallMergeStrategy(s string) string {
	if s == "" {
		return defaultHTTPCallStrategy
	}
	if s != "merge" && s != "replace" && s != "append" {
		logger.Warn("invalid mergeStrategy for http_call module; defaulting to merge",
			slog.String("merge_strategy", s),
		)
		return defaultHTTPCallStrategy
	}
	return s
}

func normalizeHTTPCallOnError(s string) string {
	if s == "" {
		return OnErrorFail
	}
	if s != OnErrorFail && s != OnErrorSkip && s != OnErrorLog {
		logger.Warn("invalid onError value for http_call module; defaulting to fail",
			slog.String("on_error", s),
		)
		return OnErrorFail
	}
	return s
}

func buildHTTPCallAuth(authCfg *connector.AuthConfig, client *http.Client) (auth.Handler, error) {
	if authCfg == nil {
		return nil, nil
	}
	h, err := auth.NewHandler(authCfg, client)
	if err != nil {
		return nil, fmt.Errorf("creating http_call auth handler: %w", err)
	}
	return h, nil
}

func buildHTTPCallCache(maxSize, ttlSeconds int) (*cache.LRUCache, time.Duration) {
	if maxSize <= 0 {
		maxSize = defaultCacheMaxSize
	}
	if ttlSeconds <= 0 {
		ttlSeconds = defaultCacheTTLSeconds
	}
	return cache.NewLRUCache(maxSize, time.Duration(ttlSeconds)*time.Second), time.Duration(ttlSeconds) * time.Second
}

func loadHTTPCallBodyTemplate(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("loading http_call body template file %q: %w", path, err)
	}
	raw := string(content)
	if err := template.ValidateSyntax(raw); err != nil {
		return "", fmt.Errorf("invalid template syntax in %q: %w", path, err)
	}
	logger.Debug("loaded http_call body template file",
		slog.String("file", path),
		slog.Int("size", len(content)),
	)
	return raw, nil
}

func validateHTTPCallTemplates(endpoint string, headers map[string]string) error {
	if err := template.ValidateSyntax(endpoint); err != nil {
		return fmt.Errorf("invalid template syntax in endpoint: %w", err)
	}
	for name, v := range headers {
		if err := template.ValidateSyntax(v); err != nil {
			return fmt.Errorf("invalid template syntax in header %q: %w", name, err)
		}
	}
	return nil
}

func httpCallHasTemplating(endpoint string, headers map[string]string, hasBodyTemplate bool) bool {
	if template.HasVariables(endpoint) || hasBodyTemplate {
		return true
	}
	for _, v := range headers {
		if template.HasVariables(v) {
			return true
		}
	}
	return false
}

// validateKeysConfig validates the keys extraction configuration.
func validateKeysConfig(keys []KeyConfig) error {
	if len(keys) == 0 {
		return newHTTPCallError(ErrCodeHTTPCallKeyMissing, "http_call keys is required (at least one key config)", -1, "", 0, "")
	}
	for i, key := range keys {
		if key.Field == "" {
			return newHTTPCallError(ErrCodeHTTPCallKeyMissing, fmt.Sprintf("http_call keys[%d] field is required", i), -1, "", 0, "")
		}
		if key.ParamType == "" {
			return newHTTPCallError(ErrCodeHTTPCallKeyMissing, fmt.Sprintf("http_call keys[%d] paramType is required", i), -1, "", 0, "")
		}
		if key.ParamType != "query" && key.ParamType != "path" && key.ParamType != "header" {
			return newHTTPCallError(ErrCodeHTTPCallKeyInvalid, "http_call key paramType must be 'query', 'path', or 'header'", -1, "", 0, "")
		}
		if key.ParamName == "" {
			return newHTTPCallError(ErrCodeHTTPCallKeyMissing, fmt.Sprintf("http_call keys[%d] paramName is required", i), -1, "", 0, "")
		}
	}
	return nil
}

// ParseHTTPCallConfig parses an http_call filter configuration from raw config map.
// The authentication parameter is passed separately (from cfg.Authentication) to match
// the ModuleConfig structure used by input and output modules.
func ParseHTTPCallConfig(cfg map[string]interface{}, auth *connector.AuthConfig) (HTTPCallConfig, error) {
	config := HTTPCallConfig{}

	// Use httpconfig extraction for base config
	config.Endpoint = parseStringField(cfg, "endpoint")
	config.Method = parseStringField(cfg, "method")
	config.Headers = parseHeaders(cfg)
	config.TimeoutMs = parseIntField(cfg, "timeoutMs")
	config.Authentication = auth

	// Parse key configuration (required for GET, optional for POST/PUT with template)
	config.Keys = parseKeysConfig(cfg)

	// Parse cache configuration (optional)
	config.Cache = parseCacheConfig(cfg)

	// Parse optional fields
	config.MergeStrategy = parseStringField(cfg, "mergeStrategy")
	config.DataField = parseStringField(cfg, "dataField")
	config.OnError = parseStringField(cfg, "onError")

	// Parse body template file (optional, for POST/PUT)
	config.BodyTemplateFile = parseStringField(cfg, "bodyTemplateFile")

	return config, nil
}

// parseStringField extracts a string field from the config map.
func parseStringField(cfg map[string]interface{}, fieldName string) string {
	if value, ok := cfg[fieldName].(string); ok {
		return value
	}
	return ""
}

// parseIntField extracts an integer field from the config map.
func parseIntField(cfg map[string]interface{}, fieldName string) int {
	if value, ok := cfg[fieldName].(float64); ok {
		return int(value)
	}
	return 0
}

// parseKeysConfig extracts the keys configuration from the config map.
func parseKeysConfig(cfg map[string]interface{}) []KeyConfig {
	if keysRaw, ok := cfg["keys"].([]interface{}); ok {
		result := make([]KeyConfig, 0, len(keysRaw))
		for _, item := range keysRaw {
			if m, ok := item.(map[string]interface{}); ok {
				result = append(result, keyConfigFromMap(m))
			}
		}
		return result
	}
	return nil
}

func keyConfigFromMap(m map[string]interface{}) KeyConfig {
	return KeyConfig{
		Field:     parseStringField(m, "field"),
		ParamType: parseStringField(m, "paramType"),
		ParamName: parseStringField(m, "paramName"),
	}
}

// parseCacheConfig extracts the cache configuration from the config map.
func parseCacheConfig(cfg map[string]interface{}) CacheConfig {
	cacheConfig := CacheConfig{}
	if cacheRaw, ok := cfg["cache"].(map[string]interface{}); ok {
		if maxSize, ok := cacheRaw["maxSize"].(float64); ok {
			maxSizeInt := int(maxSize)
			if maxSizeInt > 0 {
				cacheConfig.MaxSize = maxSizeInt
			}
		}
		if ttl, ok := cacheRaw["defaultTTL"].(float64); ok {
			ttlInt := int(ttl)
			if ttlInt > 0 {
				cacheConfig.DefaultTTL = ttlInt
			}
		}
		cacheConfig.Key = parseStringField(cacheRaw, "key")
	}
	return cacheConfig
}

// parseHeaders extracts headers from the config map.
func parseHeaders(cfg map[string]interface{}) map[string]string {
	headers := make(map[string]string)
	if headersRaw, ok := cfg["headers"].(map[string]interface{}); ok {
		for k, v := range headersRaw {
			if strVal, ok := v.(string); ok {
				headers[k] = strVal
			}
		}
	}
	return headers
}

// Process makes HTTP requests for each input record and can enrich them with response data.
// For each record:
//  1. Extracts the key value from the record
//  2. Checks the cache for existing data
//  3. If cache miss, makes an HTTP request to fetch the data
//  4. Merges the fetched data into the record
//  5. Caches successful responses
//
// The context can be used to cancel long-running operations.
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
		value, found := GetNestedValue(record, k.field)
		if !found {
			return nil, newHTTPCallError(
				ErrCodeHTTPCallKeyExtract,
				fmt.Sprintf("http_call failed to extract key from record %d: field '%s' not found", recordIdx, k.field),
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
				fmt.Sprintf("http_call failed to extract key from record %d: field '%s' is null", recordIdx, k.field),
				recordIdx, m.endpoint, 0, "",
			)
		default:
			keyValue = fmt.Sprintf("%v", v)
		}

		if keyValue == "" {
			return nil, newHTTPCallError(
				ErrCodeHTTPCallKeyExtract,
				fmt.Sprintf("http_call failed to extract key from record %d: field '%s' is empty", recordIdx, k.field),
				recordIdx, m.endpoint, 0, "",
			)
		}
		result[k.paramName] = keyValue
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
		if value, found := GetNestedValue(record, m.cacheKey); found {
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
		parts[i] = keyValues[k.paramName]
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

// executeHTTPRequest builds the HTTP request, executes it, and returns the response body and status code.
func (m *HTTPCallModule) executeHTTPRequest(ctx context.Context, keyValues map[string]string, recordIdx int, record map[string]interface{}) ([]byte, int, error) {
	// Build request URL with template evaluation
	requestURL, err := m.buildRequestURL(keyValues, record)
	if err != nil {
		return nil, 0, newHTTPCallError(
			ErrCodeHTTPCallHTTPError,
			fmt.Sprintf("http_call failed to build request URL: %v", err),
			recordIdx, m.endpoint, 0, m.compositeKeyString(keyValues),
		)
	}

	// Create and configure HTTP request
	req, err := m.buildHTTPRequest(ctx, requestURL, keyValues, recordIdx, record)
	if err != nil {
		return nil, 0, err
	}

	// Execute request
	resp, err := m.httpClient.Do(req)
	if err != nil {
		classifiedErr := errhandling.ClassifyNetworkError(err)
		return nil, 0, newHTTPCallError(
			ErrCodeHTTPCallHTTPError,
			fmt.Sprintf("http_call HTTP request failed: %v", classifiedErr),
			recordIdx, m.endpoint, 0, m.compositeKeyString(keyValues),
		)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			logger.Warn("failed to close http_call response body",
				slog.String("error", closeErr.Error()),
			)
		}
	}()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, newHTTPCallError(
			ErrCodeHTTPCallHTTPError,
			fmt.Sprintf("http_call failed to read response: %v", err),
			recordIdx, m.endpoint, resp.StatusCode, m.compositeKeyString(keyValues),
		)
	}

	// Check for HTTP errors
	if resp.StatusCode >= 400 {
		bodySnippet := string(body)
		if len(bodySnippet) > 200 {
			bodySnippet = bodySnippet[:200] + "..."
		}
		return nil, resp.StatusCode, newHTTPCallError(
			ErrCodeHTTPCallHTTPError,
			fmt.Sprintf("http_call HTTP error %d: %s", resp.StatusCode, bodySnippet),
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

	// Set custom headers with template evaluation
	for key, value := range m.headers {
		evaluatedValue := value
		if template.HasVariables(value) {
			evaluatedValue = m.templateEvaluator.Evaluate(value, record)
		}
		req.Header.Set(key, evaluatedValue)
	}

	// Apply keys as headers if configured
	for _, k := range m.keys {
		if k.paramType == "header" {
			if v := keyValues[k.paramName]; v != "" {
				req.Header.Set(k.paramName, v)
			}
		}
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
		if k.paramType == "path" {
			placeholder := "{" + k.paramName + "}"
			endpoint = strings.Replace(endpoint, placeholder, url.PathEscape(keyValues[k.paramName]), 1)
		}
	}

	// Apply query parameters
	hasQuery := false
	for _, k := range m.keys {
		if k.paramType == "query" {
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
			if k.paramType == "query" {
				q.Set(k.paramName, keyValues[k.paramName])
			}
		}
		parsedURL.RawQuery = q.Encode()
		return parsedURL.String(), nil
	}

	// Path-only or header-only: return endpoint after path replacements
	return endpoint, nil
}

// mergeData merges response data into the record based on the merge strategy.
func (m *HTTPCallModule) mergeData(record, responseData map[string]interface{}) map[string]interface{} {
	switch m.mergeStrategy {
	case "merge":
		return m.deepMerge(record, responseData)

	case "replace":
		// Replace the entire record with response data
		// Keep original fields that aren't in response data
		result := make(map[string]interface{})
		for k, v := range record {
			result[k] = v
		}
		for k, v := range responseData {
			result[k] = v
		}
		return result

	case "append":
		// Append response data under a special key
		result := make(map[string]interface{})
		for k, v := range record {
			result[k] = v
		}
		result["_response"] = responseData
		return result

	default:
		return m.deepMerge(record, responseData)
	}
}

// deepMerge performs a deep merge of two maps.
// Values from b override values from a, except for nested maps which are merged recursively.
func (m *HTTPCallModule) deepMerge(a, b map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Copy all from a
	for k, v := range a {
		result[k] = v
	}

	// Merge/override from b
	for k, vb := range b {
		if va, exists := result[k]; exists {
			// If both are maps, merge recursively
			if mapA, okA := va.(map[string]interface{}); okA {
				if mapB, okB := vb.(map[string]interface{}); okB {
					result[k] = m.deepMerge(mapA, mapB)
					continue
				}
			}
		}
		// Otherwise, b overrides
		result[k] = vb
	}

	return result
}

// GetCacheStats returns the current cache statistics.
func (m *HTTPCallModule) GetCacheStats() cache.Stats {
	return m.cache.Stats()
}

// ClearCache clears all entries from the cache.
func (m *HTTPCallModule) ClearCache() {
	m.cache.Clear()
}
