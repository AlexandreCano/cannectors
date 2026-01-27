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

	"github.com/canectors/runtime/internal/auth"
	"github.com/canectors/runtime/internal/cache"
	"github.com/canectors/runtime/internal/errhandling"
	"github.com/canectors/runtime/internal/httpconfig"
	"github.com/canectors/runtime/internal/logger"
	"github.com/canectors/runtime/internal/template"
	"github.com/canectors/runtime/pkg/connector"
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
	//   - A JSON path expression: "$.customerId" or "customerId" (extracts value from record)
	//   - Dot notation path: "user.profile.id" (extracts nested value from record)
	Key string `json:"key"`
}

// HTTPCallConfig represents the configuration for an http_call filter module.
// Fields are organized in logical groups matching httpconfig types for consistency,
// but kept as direct fields for simpler struct literal syntax in tests and usage.
type HTTPCallConfig struct {
	// Base HTTP configuration (from httpconfig.BaseConfig)
	Endpoint  string                `json:"endpoint"`
	Method    string                `json:"method,omitempty"`
	Headers   map[string]string     `json:"headers,omitempty"`
	TimeoutMs int                   `json:"timeoutMs,omitempty"`
	Auth      *connector.AuthConfig `json:"auth,omitempty"`

	// Body template configuration (from httpconfig.BodyTemplateConfig)
	BodyTemplateFile string `json:"bodyTemplateFile,omitempty"`

	// Error handling configuration (from httpconfig.ErrorHandlingConfig)
	OnError string `json:"onError,omitempty"`

	// Data extraction configuration (from httpconfig.DataExtractionConfig)
	DataField string `json:"dataField,omitempty"`

	// Key defines how to extract and use the key value (required for GET, optional for POST/PUT with template)
	Key KeyConfig `json:"key"`
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
type HTTPCallModule struct {
	endpoint          string
	method            string // HTTP method (GET, POST, PUT)
	keyField          string
	keyParamType      string
	keyParamName      string
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
		if err := validateKeyConfig(config.Key); err != nil {
			return nil, err
		}
	}

	mergeStrategy := normalizeHTTPCallMergeStrategy(config.MergeStrategy)
	onError := normalizeHTTPCallOnError(config.OnError)
	timeout := httpconfig.GetTimeoutDuration(config.TimeoutMs, defaultHTTPCallTimeout)

	httpClient := &http.Client{Timeout: timeout}
	authHandler, err := buildHTTPCallAuth(config.Auth, httpClient)
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
		slog.String("key_field", config.Key.Field),
		slog.String("key_param_type", config.Key.ParamType),
		slog.String("merge_strategy", mergeStrategy),
		slog.String("on_error", onError),
		slog.Int("cache_max_size", cacheMaxSize),
		slog.Int("cache_ttl_seconds", cacheTTLSeconds),
		slog.String("cache_key", config.Cache.Key),
		slog.Bool("has_auth", authHandler != nil),
		slog.Bool("has_templating", hasTemplating),
		slog.String("body_template_file", config.BodyTemplateFile),
	)

	return &HTTPCallModule{
		endpoint:          config.Endpoint,
		method:            method,
		keyField:          config.Key.Field,
		keyParamType:      config.Key.ParamType,
		keyParamName:      config.Key.ParamName,
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

// validateKeyConfig validates the key extraction configuration.
func validateKeyConfig(key KeyConfig) error {
	if key.Field == "" {
		return newHTTPCallError(ErrCodeHTTPCallKeyMissing, "http_call key field is required", -1, "", 0, "")
	}
	if key.ParamType == "" {
		return newHTTPCallError(ErrCodeHTTPCallKeyMissing, "http_call key paramType is required", -1, "", 0, "")
	}
	if key.ParamType != "query" && key.ParamType != "path" && key.ParamType != "header" {
		return newHTTPCallError(ErrCodeHTTPCallKeyInvalid, "http_call key paramType must be 'query', 'path', or 'header'", -1, "", 0, "")
	}
	if key.ParamName == "" {
		return newHTTPCallError(ErrCodeHTTPCallKeyMissing, "http_call key paramName is required", -1, "", 0, "")
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
	config.Auth = auth

	// Parse key configuration (required for GET, optional for POST/PUT with template)
	config.Key = parseKeyConfig(cfg)

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

// parseKeyConfig extracts the key configuration from the config map.
func parseKeyConfig(cfg map[string]interface{}) KeyConfig {
	keyConfig := KeyConfig{}
	if keyRaw, ok := cfg["key"].(map[string]interface{}); ok {
		keyConfig.Field = parseStringField(keyRaw, "field")
		keyConfig.ParamType = parseStringField(keyRaw, "paramType")
		keyConfig.ParamName = parseStringField(keyRaw, "paramName")
	}
	return keyConfig
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
	// Extract key value from record (may be empty for POST/PUT with template-only mode)
	var keyValue string
	var err error
	if m.keyField != "" {
		keyValue, err = m.extractKeyValue(record, recordIdx)
		if err != nil {
			return nil, false, err
		}
	}

	// Check cache first
	cacheKey := m.buildCacheKey(keyValue, record)
	if cachedData, found := m.cache.Get(cacheKey); found {
		responseData, ok := cachedData.(map[string]interface{})
		if ok {
			logger.Debug("http_call cache hit",
				slog.String("module_type", "http_call"),
				slog.Int("record_index", recordIdx),
				slog.String("key_value", keyValue),
			)
			enrichedRecord := m.mergeData(record, responseData)
			return enrichedRecord, true, nil
		}
	}

	// Cache miss - make HTTP request
	responseData, err := m.fetchResponseData(ctx, keyValue, recordIdx, record)
	if err != nil {
		return nil, false, err
	}

	// Cache successful response (don't cache errors)
	// Rebuild cache key in case it depends on record values
	cacheKey = m.buildCacheKey(keyValue, record)
	m.cache.Set(cacheKey, responseData, m.cacheTTL)

	logger.Debug("http_call cache miss (fetched and cached)",
		slog.String("module_type", "http_call"),
		slog.Int("record_index", recordIdx),
		slog.String("key_value", keyValue),
	)

	// Merge data into record
	enrichedRecord := m.mergeData(record, responseData)

	return enrichedRecord, false, nil
}

// extractKeyValue extracts the key value from a record using the configured field path.
func (m *HTTPCallModule) extractKeyValue(record map[string]interface{}, recordIdx int) (string, error) {
	value, found := getNestedValue(record, m.keyField)
	if !found {
		return "", newHTTPCallError(
			ErrCodeHTTPCallKeyExtract,
			fmt.Sprintf("http_call failed to extract key from record %d: field '%s' not found", recordIdx, m.keyField),
			recordIdx, m.endpoint, 0, "",
		)
	}

	// Convert value to string
	var keyValue string
	switch v := value.(type) {
	case string:
		keyValue = v
	case float64:
		// Handle JSON numbers
		if v == float64(int64(v)) {
			keyValue = fmt.Sprintf("%d", int64(v))
		} else {
			keyValue = fmt.Sprintf("%v", v)
		}
	case int, int64, int32:
		keyValue = fmt.Sprintf("%v", v)
	case nil:
		return "", newHTTPCallError(
			ErrCodeHTTPCallKeyExtract,
			fmt.Sprintf("http_call failed to extract key from record %d: field '%s' is null", recordIdx, m.keyField),
			recordIdx, m.endpoint, 0, "",
		)
	default:
		keyValue = fmt.Sprintf("%v", v)
	}

	if keyValue == "" {
		return "", newHTTPCallError(
			ErrCodeHTTPCallKeyExtract,
			fmt.Sprintf("http_call failed to extract key from record %d: field '%s' is empty", recordIdx, m.keyField),
			recordIdx, m.endpoint, 0, "",
		)
	}

	return keyValue, nil
}

// buildCacheKey creates a unique cache key from the key value and record.
// If cacheKey is configured, it can be:
//   - A static string: used as-is
//   - A JSON path expression (starting with "$." or dot notation): extracts value from record
//
// If cacheKey is not configured, uses default: endpoint + "::" + keyValue
func (m *HTTPCallModule) buildCacheKey(keyValue string, record map[string]interface{}) string {
	// If cacheKey is configured, use it
	if m.cacheKey != "" {
		// Check if it's a JSON path expression (starts with "$." or contains ".")
		if strings.HasPrefix(m.cacheKey, "$.") {
			// Remove "$." prefix and use dot notation
			path := strings.TrimPrefix(m.cacheKey, "$.")
			if value, found := getNestedValue(record, path); found {
				return fmt.Sprintf("%v", value)
			}
			// If path not found, log warning and fall back to default behavior
			logger.Warn("http_call cache key path not found, using default key",
				slog.String("module_type", "http_call"),
				slog.String("configured_path", m.cacheKey),
				slog.String("fallback_key", m.endpoint+"::"+keyValue),
			)
			return m.endpoint + "::" + keyValue
		} else if strings.Contains(m.cacheKey, ".") {
			// Dot notation path (e.g., "user.profile.id")
			if value, found := getNestedValue(record, m.cacheKey); found {
				return fmt.Sprintf("%v", value)
			}
			// If path not found, log warning and fall back to default behavior
			logger.Warn("http_call cache key path not found, using default key",
				slog.String("module_type", "http_call"),
				slog.String("configured_path", m.cacheKey),
				slog.String("fallback_key", m.endpoint+"::"+keyValue),
			)
			return m.endpoint + "::" + keyValue
		}
		// Static string - use as-is
		return m.cacheKey
	}

	// Default behavior: endpoint + "::" + keyValue
	// Use "::" as delimiter to avoid collisions with ":" in URLs or key values
	// This ensures endpoint="http://api.com" with keyValue="foo:bar" produces
	// a different key than endpoint="http://api.com:foo" with keyValue="bar"
	return m.endpoint + "::" + keyValue
}

// fetchResponseData fetches data from the external API for the given key value and record.
func (m *HTTPCallModule) fetchResponseData(ctx context.Context, keyValue string, recordIdx int, record map[string]interface{}) (map[string]interface{}, error) {
	// Build and execute HTTP request
	body, statusCode, err := m.executeHTTPRequest(ctx, keyValue, recordIdx, record)
	if err != nil {
		return nil, err
	}

	// Parse and extract response data
	return m.parseResponseData(body, statusCode, recordIdx)
}

// executeHTTPRequest builds the HTTP request, executes it, and returns the response body and status code.
func (m *HTTPCallModule) executeHTTPRequest(ctx context.Context, keyValue string, recordIdx int, record map[string]interface{}) ([]byte, int, error) {
	// Build request URL with template evaluation
	requestURL, err := m.buildRequestURL(keyValue, record)
	if err != nil {
		return nil, 0, newHTTPCallError(
			ErrCodeHTTPCallHTTPError,
			fmt.Sprintf("http_call failed to build request URL: %v", err),
			recordIdx, m.endpoint, 0, keyValue,
		)
	}

	// Create and configure HTTP request
	req, err := m.buildHTTPRequest(ctx, requestURL, keyValue, recordIdx, record)
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
			recordIdx, m.endpoint, 0, keyValue,
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
			recordIdx, m.endpoint, resp.StatusCode, keyValue,
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
			recordIdx, m.endpoint, resp.StatusCode, keyValue,
		)
	}

	return body, resp.StatusCode, nil
}

// buildHTTPRequest creates and configures an HTTP request with headers and authentication.
func (m *HTTPCallModule) buildHTTPRequest(ctx context.Context, requestURL, keyValue string, recordIdx int, record map[string]interface{}) (*http.Request, error) {
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
			recordIdx, m.endpoint, 0, keyValue,
		)
	}

	// Set default headers
	req.Header.Set("User-Agent", "Canectors-Runtime/1.0")
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

	// Apply key as header if configured
	if m.keyParamType == "header" && keyValue != "" {
		req.Header.Set(m.keyParamName, keyValue)
	}

	// Apply authentication
	if m.authHandler != nil {
		if authErr := m.authHandler.ApplyAuth(ctx, req); authErr != nil {
			return nil, newHTTPCallError(
				ErrCodeHTTPCallHTTPError,
				fmt.Sprintf("http_call failed to apply auth: %v", authErr),
				recordIdx, m.endpoint, 0, keyValue,
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

// buildRequestURL constructs the HTTP request URL based on the key configuration and template evaluation.
func (m *HTTPCallModule) buildRequestURL(keyValue string, record map[string]interface{}) (string, error) {
	endpoint := m.endpoint

	// Evaluate template variables in endpoint ({{record.field}} syntax)
	if template.HasVariables(endpoint) {
		endpoint = m.templateEvaluator.EvaluateForURL(endpoint, record)
	}

	// If no key configuration, return the templated endpoint as-is
	if m.keyParamType == "" {
		return endpoint, nil
	}

	switch m.keyParamType {
	case "path":
		// Replace {paramName} in endpoint with key value
		placeholder := "{" + m.keyParamName + "}"
		requestURL := strings.Replace(endpoint, placeholder, url.PathEscape(keyValue), 1)
		return requestURL, nil

	case "query":
		// Add key as query parameter
		parsedURL, err := url.Parse(endpoint)
		if err != nil {
			return "", fmt.Errorf(errMsgParsingEndpointURL, err)
		}
		q := parsedURL.Query()
		q.Set(m.keyParamName, keyValue)
		parsedURL.RawQuery = q.Encode()
		return parsedURL.String(), nil

	case "header":
		// Header is added in buildHTTPRequest, return endpoint as-is
		return endpoint, nil

	default:
		return "", fmt.Errorf("unknown key paramType: %s", m.keyParamType)
	}
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
