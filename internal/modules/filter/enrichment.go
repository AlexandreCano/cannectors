// Package filter provides implementations for filter modules.
// Enrichment module fetches additional data from external APIs and merges it into records.
package filter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/canectors/runtime/internal/auth"
	"github.com/canectors/runtime/internal/cache"
	"github.com/canectors/runtime/internal/errhandling"
	"github.com/canectors/runtime/internal/logger"
	"github.com/canectors/runtime/pkg/connector"
)

// Default configuration values for enrichment module
const (
	defaultEnrichmentTimeout  = 30 * time.Second
	defaultCacheMaxSize       = 1000
	defaultCacheTTLSeconds    = 300 // 5 minutes
	defaultEnrichmentStrategy = "merge"
)

// Error codes for enrichment module
const (
	ErrCodeEnrichmentEndpointMissing = "ENRICHMENT_ENDPOINT_MISSING"
	ErrCodeEnrichmentKeyMissing      = "ENRICHMENT_KEY_MISSING"
	ErrCodeEnrichmentKeyInvalid      = "ENRICHMENT_KEY_INVALID"
	ErrCodeEnrichmentKeyExtract      = "ENRICHMENT_KEY_EXTRACT"
	ErrCodeEnrichmentHTTPError       = "ENRICHMENT_HTTP_ERROR"
	ErrCodeEnrichmentJSONParse       = "ENRICHMENT_JSON_PARSE"
	ErrCodeEnrichmentMerge           = "ENRICHMENT_MERGE"
)

// Error messages
const (
	errMsgParsingEndpointURL = "parsing endpoint URL: %w"
)

// Error types for enrichment module
var (
	ErrEnrichmentEndpointMissing = fmt.Errorf("enrichment endpoint is required")
	ErrEnrichmentKeyMissing      = fmt.Errorf("enrichment key configuration is required")
	ErrEnrichmentKeyInvalid      = fmt.Errorf("enrichment key paramType must be 'query', 'path', or 'header'")
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

// CacheConfig defines cache behavior for the enrichment module.
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

// EnrichmentConfig represents the configuration for an enrichment filter module.
type EnrichmentConfig struct {
	// Endpoint is the HTTP endpoint URL (required). May contain {key} placeholder for path params.
	Endpoint string `json:"endpoint"`
	// Key defines how to extract and use the key value (required)
	Key KeyConfig `json:"key"`
	// Auth is the optional authentication configuration
	Auth *connector.AuthConfig `json:"auth,omitempty"`
	// Cache defines cache behavior (optional, uses defaults if not specified)
	Cache CacheConfig `json:"cache"`
	// MergeStrategy defines how to merge enrichment data: "merge" (default), "replace", "append"
	MergeStrategy string `json:"mergeStrategy"`
	// DataField is the JSON field containing the data in the response (optional)
	DataField string `json:"dataField"`
	// OnError specifies error handling mode: "fail" (default), "skip", "log"
	OnError string `json:"onError"`
	// TimeoutMs is the request timeout in milliseconds (default 30000)
	TimeoutMs int `json:"timeoutMs"`
	// Headers are custom HTTP headers to include in requests
	Headers map[string]string `json:"headers"`
}

// EnrichmentModule implements a filter that enriches records with external API data.
// It supports caching to avoid redundant API calls for records with the same key value.
//
// Thread Safety:
//   - The cache is thread-safe (uses mutex internally)
//   - Process() can be called from multiple goroutines
//
// Error Handling:
//   - HTTP errors are not cached (only successful responses are cached)
//   - onError mode controls behavior: fail (stop pipeline), skip (drop record), log (continue)
type EnrichmentModule struct {
	endpoint      string
	keyField      string
	keyParamType  string
	keyParamName  string
	authHandler   auth.Handler
	httpClient    *http.Client
	cache         cache.Cache
	mergeStrategy string
	dataField     string
	onError       string
	headers       map[string]string
	cacheTTL      time.Duration
	cacheKey      string // Cache key configuration (optional)
}

// EnrichmentError carries structured context for enrichment failures.
type EnrichmentError struct {
	Code        string
	Message     string
	RecordIndex int
	Endpoint    string
	StatusCode  int
	KeyValue    string
	Details     map[string]interface{}
}

func (e *EnrichmentError) Error() string {
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

// newEnrichmentError creates an EnrichmentError with context.
func newEnrichmentError(code, message string, recordIdx int, endpoint string, statusCode int, keyValue string) *EnrichmentError {
	// Sanitize endpoint URL in error message to avoid exposing sensitive data
	sanitizedEndpoint := sanitizeURL(endpoint)
	return &EnrichmentError{
		Code:        code,
		Message:     message,
		RecordIndex: recordIdx,
		Endpoint:    sanitizedEndpoint,
		StatusCode:  statusCode,
		KeyValue:    keyValue,
		Details:     make(map[string]interface{}),
	}
}

// NewEnrichmentFromConfig creates a new enrichment filter module from configuration.
// It validates the configuration and initializes the HTTP client and cache.
//
// Required config fields:
//   - endpoint: The HTTP endpoint URL
//   - key: Key extraction configuration (field, paramType, paramName)
//
// Optional config fields:
//   - auth: Authentication configuration
//   - cache: Cache configuration (maxSize, defaultTTL)
//   - mergeStrategy: How to merge data ("merge", "replace", "append")
//   - dataField: JSON field containing the data array
//   - onError: Error handling mode ("fail", "skip", "log")
//   - timeoutMs: Request timeout in milliseconds
//   - headers: Custom HTTP headers
func NewEnrichmentFromConfig(config EnrichmentConfig) (*EnrichmentModule, error) {
	// Validate endpoint
	if config.Endpoint == "" {
		return nil, newEnrichmentError(ErrCodeEnrichmentEndpointMissing, "enrichment endpoint is required", -1, "", 0, "")
	}

	// Validate key configuration
	if err := validateKeyConfig(config.Key); err != nil {
		return nil, err
	}

	// Normalize merge strategy
	mergeStrategy := config.MergeStrategy
	if mergeStrategy == "" {
		mergeStrategy = defaultEnrichmentStrategy
	}
	if mergeStrategy != "merge" && mergeStrategy != "replace" && mergeStrategy != "append" {
		logger.Warn("invalid mergeStrategy for enrichment module; defaulting to merge",
			slog.String("merge_strategy", mergeStrategy),
		)
		mergeStrategy = defaultEnrichmentStrategy
	}

	// Normalize onError
	onError := config.OnError
	if onError == "" {
		onError = OnErrorFail
	}
	if onError != OnErrorFail && onError != OnErrorSkip && onError != OnErrorLog {
		logger.Warn("invalid onError value for enrichment module; defaulting to fail",
			slog.String("on_error", onError),
		)
		onError = OnErrorFail
	}

	// Set timeout
	timeout := defaultEnrichmentTimeout
	if config.TimeoutMs > 0 {
		timeout = time.Duration(config.TimeoutMs) * time.Millisecond
	}

	// Create HTTP client
	httpClient := &http.Client{
		Timeout: timeout,
	}

	// Create auth handler if configured
	var authHandler auth.Handler
	if config.Auth != nil {
		var err error
		authHandler, err = auth.NewHandler(config.Auth, httpClient)
		if err != nil {
			return nil, fmt.Errorf("creating enrichment auth handler: %w", err)
		}
	}

	// Set cache configuration
	cacheMaxSize := config.Cache.MaxSize
	if cacheMaxSize <= 0 {
		cacheMaxSize = defaultCacheMaxSize
	}
	cacheTTLSeconds := config.Cache.DefaultTTL
	if cacheTTLSeconds <= 0 {
		cacheTTLSeconds = defaultCacheTTLSeconds
	}
	cacheTTL := time.Duration(cacheTTLSeconds) * time.Second

	// Create cache
	lruCache := cache.NewLRUCache(cacheMaxSize, cacheTTL)

	logger.Debug("enrichment module initialized",
		slog.String("endpoint", config.Endpoint),
		slog.String("key_field", config.Key.Field),
		slog.String("key_param_type", config.Key.ParamType),
		slog.String("merge_strategy", mergeStrategy),
		slog.String("on_error", onError),
		slog.Int("cache_max_size", cacheMaxSize),
		slog.Int("cache_ttl_seconds", cacheTTLSeconds),
		slog.String("cache_key", config.Cache.Key),
		slog.Bool("has_auth", authHandler != nil),
	)

	return &EnrichmentModule{
		endpoint:      config.Endpoint,
		keyField:      config.Key.Field,
		keyParamType:  config.Key.ParamType,
		keyParamName:  config.Key.ParamName,
		authHandler:   authHandler,
		httpClient:    httpClient,
		cache:         lruCache,
		mergeStrategy: mergeStrategy,
		dataField:     config.DataField,
		onError:       onError,
		headers:       config.Headers,
		cacheTTL:      cacheTTL,
		cacheKey:      config.Cache.Key,
	}, nil
}

// validateKeyConfig validates the key extraction configuration.
func validateKeyConfig(key KeyConfig) error {
	if key.Field == "" {
		return newEnrichmentError(ErrCodeEnrichmentKeyMissing, "enrichment key field is required", -1, "", 0, "")
	}
	if key.ParamType == "" {
		return newEnrichmentError(ErrCodeEnrichmentKeyMissing, "enrichment key paramType is required", -1, "", 0, "")
	}
	if key.ParamType != "query" && key.ParamType != "path" && key.ParamType != "header" {
		return newEnrichmentError(ErrCodeEnrichmentKeyInvalid, "enrichment key paramType must be 'query', 'path', or 'header'", -1, "", 0, "")
	}
	if key.ParamName == "" {
		return newEnrichmentError(ErrCodeEnrichmentKeyMissing, "enrichment key paramName is required", -1, "", 0, "")
	}
	return nil
}

// ParseEnrichmentConfig parses an enrichment filter configuration from raw config map.
// The authentication parameter is passed separately (from cfg.Authentication) to match
// the ModuleConfig structure used by input and output modules.
func ParseEnrichmentConfig(cfg map[string]interface{}, auth *connector.AuthConfig) (EnrichmentConfig, error) {
	config := EnrichmentConfig{}

	// Parse endpoint (required)
	config.Endpoint = parseStringField(cfg, "endpoint")

	// Parse key configuration (required)
	config.Key = parseKeyConfig(cfg)

	// Set authentication from separate parameter (consistent with input/output modules)
	config.Auth = auth

	// Parse cache configuration (optional)
	config.Cache = parseCacheConfig(cfg)

	// Parse optional fields
	config.MergeStrategy = parseStringField(cfg, "mergeStrategy")
	config.DataField = parseStringField(cfg, "dataField")
	config.OnError = parseStringField(cfg, "onError")
	config.TimeoutMs = parseIntField(cfg, "timeoutMs")
	config.Headers = parseHeaders(cfg)

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

// Process enriches each input record with data from an external API.
// For each record:
//  1. Extracts the key value from the record
//  2. Checks the cache for existing data
//  3. If cache miss, makes an HTTP request to fetch the data
//  4. Merges the fetched data into the record
//  5. Caches successful responses
//
// The context can be used to cancel long-running operations.
func (m *EnrichmentModule) Process(ctx context.Context, records []map[string]interface{}) ([]map[string]interface{}, error) {
	if records == nil {
		return []map[string]interface{}{}, nil
	}

	startTime := time.Now()
	inputCount := len(records)

	logger.Debug("filter processing started",
		slog.String("module_type", "enrichment"),
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
					slog.String("module_type", "enrichment"),
					slog.Int("record_index", recordIdx),
					slog.Duration("duration", duration),
					slog.String("error", err.Error()),
				)
				return nil, err
			case OnErrorSkip:
				skippedCount++
				logger.Warn("skipping record due to enrichment error",
					slog.String("module_type", "enrichment"),
					slog.Int("record_index", recordIdx),
					slog.String("error", err.Error()),
				)
				continue
			case OnErrorLog:
				logger.Error("enrichment error (continuing)",
					slog.String("module_type", "enrichment"),
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
		slog.String("module_type", "enrichment"),
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

// processRecord enriches a single record with data from the external API.
// Returns the enriched record, whether it was a cache hit, and any error.
func (m *EnrichmentModule) processRecord(ctx context.Context, record map[string]interface{}, recordIdx int) (map[string]interface{}, bool, error) {
	// Extract key value from record
	keyValue, err := m.extractKeyValue(record, recordIdx)
	if err != nil {
		return nil, false, err
	}

	// Check cache first
	cacheKey := m.buildCacheKey(keyValue, record)
	if cachedData, found := m.cache.Get(cacheKey); found {
		enrichmentData, ok := cachedData.(map[string]interface{})
		if ok {
			logger.Debug("enrichment cache hit",
				slog.String("module_type", "enrichment"),
				slog.Int("record_index", recordIdx),
				slog.String("key_value", keyValue),
			)
			enrichedRecord := m.mergeData(record, enrichmentData)
			return enrichedRecord, true, nil
		}
	}

	// Cache miss - make HTTP request
	enrichmentData, err := m.fetchEnrichmentData(ctx, keyValue, recordIdx)
	if err != nil {
		return nil, false, err
	}

	// Cache successful response (don't cache errors)
	// Rebuild cache key in case it depends on record values
	cacheKey = m.buildCacheKey(keyValue, record)
	m.cache.Set(cacheKey, enrichmentData, m.cacheTTL)

	logger.Debug("enrichment cache miss (fetched and cached)",
		slog.String("module_type", "enrichment"),
		slog.Int("record_index", recordIdx),
		slog.String("key_value", keyValue),
	)

	// Merge data into record
	enrichedRecord := m.mergeData(record, enrichmentData)

	return enrichedRecord, false, nil
}

// extractKeyValue extracts the key value from a record using the configured field path.
func (m *EnrichmentModule) extractKeyValue(record map[string]interface{}, recordIdx int) (string, error) {
	value, found := getNestedValue(record, m.keyField)
	if !found {
		return "", newEnrichmentError(
			ErrCodeEnrichmentKeyExtract,
			fmt.Sprintf("enrichment failed to extract key from record %d: field '%s' not found", recordIdx, m.keyField),
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
		return "", newEnrichmentError(
			ErrCodeEnrichmentKeyExtract,
			fmt.Sprintf("enrichment failed to extract key from record %d: field '%s' is null", recordIdx, m.keyField),
			recordIdx, m.endpoint, 0, "",
		)
	default:
		keyValue = fmt.Sprintf("%v", v)
	}

	if keyValue == "" {
		return "", newEnrichmentError(
			ErrCodeEnrichmentKeyExtract,
			fmt.Sprintf("enrichment failed to extract key from record %d: field '%s' is empty", recordIdx, m.keyField),
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
func (m *EnrichmentModule) buildCacheKey(keyValue string, record map[string]interface{}) string {
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
			logger.Warn("enrichment cache key path not found, using default key",
				slog.String("module_type", "enrichment"),
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
			logger.Warn("enrichment cache key path not found, using default key",
				slog.String("module_type", "enrichment"),
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

// fetchEnrichmentData fetches data from the external API for the given key value.
func (m *EnrichmentModule) fetchEnrichmentData(ctx context.Context, keyValue string, recordIdx int) (map[string]interface{}, error) {
	// Build and execute HTTP request
	body, statusCode, err := m.executeHTTPRequest(ctx, keyValue, recordIdx)
	if err != nil {
		return nil, err
	}

	// Parse and extract response data
	return m.parseResponseData(body, statusCode, recordIdx)
}

// executeHTTPRequest builds the HTTP request, executes it, and returns the response body and status code.
func (m *EnrichmentModule) executeHTTPRequest(ctx context.Context, keyValue string, recordIdx int) ([]byte, int, error) {
	// Build request URL
	requestURL, err := m.buildRequestURL(keyValue)
	if err != nil {
		return nil, 0, newEnrichmentError(
			ErrCodeEnrichmentHTTPError,
			fmt.Sprintf("enrichment failed to build request URL: %v", err),
			recordIdx, m.endpoint, 0, keyValue,
		)
	}

	// Create and configure HTTP request
	req, err := m.buildHTTPRequest(ctx, requestURL, keyValue, recordIdx)
	if err != nil {
		return nil, 0, err
	}

	// Execute request
	resp, err := m.httpClient.Do(req)
	if err != nil {
		classifiedErr := errhandling.ClassifyNetworkError(err)
		return nil, 0, newEnrichmentError(
			ErrCodeEnrichmentHTTPError,
			fmt.Sprintf("enrichment HTTP request failed: %v", classifiedErr),
			recordIdx, m.endpoint, 0, keyValue,
		)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			logger.Warn("failed to close enrichment response body",
				slog.String("error", closeErr.Error()),
			)
		}
	}()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, newEnrichmentError(
			ErrCodeEnrichmentHTTPError,
			fmt.Sprintf("enrichment failed to read response: %v", err),
			recordIdx, m.endpoint, resp.StatusCode, keyValue,
		)
	}

	// Check for HTTP errors
	if resp.StatusCode >= 400 {
		bodySnippet := string(body)
		if len(bodySnippet) > 200 {
			bodySnippet = bodySnippet[:200] + "..."
		}
		return nil, resp.StatusCode, newEnrichmentError(
			ErrCodeEnrichmentHTTPError,
			fmt.Sprintf("enrichment HTTP error %d: %s", resp.StatusCode, bodySnippet),
			recordIdx, m.endpoint, resp.StatusCode, keyValue,
		)
	}

	return body, resp.StatusCode, nil
}

// buildHTTPRequest creates and configures an HTTP request with headers and authentication.
func (m *EnrichmentModule) buildHTTPRequest(ctx context.Context, requestURL, keyValue string, recordIdx int) (*http.Request, error) {
	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, newEnrichmentError(
			ErrCodeEnrichmentHTTPError,
			fmt.Sprintf("enrichment failed to create request: %v", err),
			recordIdx, m.endpoint, 0, keyValue,
		)
	}

	// Set default headers
	req.Header.Set("User-Agent", "Canectors-Runtime/1.0")
	req.Header.Set("Accept", "application/json")

	// Set custom headers
	for key, value := range m.headers {
		req.Header.Set(key, value)
	}

	// Apply key as header if configured
	if m.keyParamType == "header" {
		req.Header.Set(m.keyParamName, keyValue)
	}

	// Apply authentication
	if m.authHandler != nil {
		if authErr := m.authHandler.ApplyAuth(ctx, req); authErr != nil {
			return nil, newEnrichmentError(
				ErrCodeEnrichmentHTTPError,
				fmt.Sprintf("enrichment failed to apply auth: %v", authErr),
				recordIdx, m.endpoint, 0, keyValue,
			)
		}
	}

	return req, nil
}

// parseResponseData parses the JSON response and extracts the data field if configured.
func (m *EnrichmentModule) parseResponseData(body []byte, statusCode int, recordIdx int) (map[string]interface{}, error) {
	// Parse JSON response
	var responseData map[string]interface{}
	if err := json.Unmarshal(body, &responseData); err != nil {
		return nil, newEnrichmentError(
			ErrCodeEnrichmentJSONParse,
			fmt.Sprintf("enrichment failed to parse response: %v", err),
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
func (m *EnrichmentModule) extractDataField(responseData map[string]interface{}, recordIdx int) map[string]interface{} {
	data, ok := responseData[m.dataField]
	if !ok {
		logger.Warn("enrichment dataField not found or invalid",
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
	logger.Warn("enrichment dataField not found or invalid",
		slog.String("data_field", m.dataField),
		slog.Int("record_index", recordIdx),
	)
	return make(map[string]interface{})
}

// buildRequestURL constructs the HTTP request URL based on the key configuration.
func (m *EnrichmentModule) buildRequestURL(keyValue string) (string, error) {
	switch m.keyParamType {
	case "path":
		// Replace {paramName} in endpoint with key value
		placeholder := "{" + m.keyParamName + "}"
		requestURL := strings.Replace(m.endpoint, placeholder, url.PathEscape(keyValue), 1)
		return requestURL, nil

	case "query":
		// Add key as query parameter
		parsedURL, err := url.Parse(m.endpoint)
		if err != nil {
			return "", fmt.Errorf(errMsgParsingEndpointURL, err)
		}
		q := parsedURL.Query()
		q.Set(m.keyParamName, keyValue)
		parsedURL.RawQuery = q.Encode()
		return parsedURL.String(), nil

	case "header":
		// Header is added in fetchEnrichmentData, return endpoint as-is
		return m.endpoint, nil

	default:
		return "", fmt.Errorf("unknown key paramType: %s", m.keyParamType)
	}
}

// mergeData merges enrichment data into the record based on the merge strategy.
func (m *EnrichmentModule) mergeData(record, enrichmentData map[string]interface{}) map[string]interface{} {
	switch m.mergeStrategy {
	case "merge":
		return m.deepMerge(record, enrichmentData)

	case "replace":
		// Replace the entire record with enrichment data
		// Keep original fields that aren't in enrichment data
		result := make(map[string]interface{})
		for k, v := range record {
			result[k] = v
		}
		for k, v := range enrichmentData {
			result[k] = v
		}
		return result

	case "append":
		// Append enrichment data under a special key
		result := make(map[string]interface{})
		for k, v := range record {
			result[k] = v
		}
		result["_enrichment"] = enrichmentData
		return result

	default:
		return m.deepMerge(record, enrichmentData)
	}
}

// deepMerge performs a deep merge of two maps.
// Values from b override values from a, except for nested maps which are merged recursively.
func (m *EnrichmentModule) deepMerge(a, b map[string]interface{}) map[string]interface{} {
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
func (m *EnrichmentModule) GetCacheStats() cache.Stats {
	return m.cache.Stats()
}

// ClearCache clears all entries from the cache.
func (m *EnrichmentModule) ClearCache() {
	m.cache.Clear()
}
