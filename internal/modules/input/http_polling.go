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
	"time"

	"github.com/expr-lang/expr/vm"

	"github.com/cannectors/runtime/internal/auth"
	"github.com/cannectors/runtime/internal/errhandling"
	"github.com/cannectors/runtime/internal/httpclient"
	"github.com/cannectors/runtime/internal/logger"
	"github.com/cannectors/runtime/internal/moduleconfig"
	"github.com/cannectors/runtime/internal/persistence"
	"github.com/cannectors/runtime/pkg/connector"
)

// Default configuration values
const (
	defaultTimeout     = 30 * time.Second
	defaultUserAgent   = "Cannectors-Runtime/1.0"
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

// HTTPPollingInputConfig holds typed configuration for the HTTP polling module.
type HTTPPollingInputConfig struct {
	connector.ModuleBase
	moduleconfig.HTTPRequestBase
	DataField        string                              `json:"dataField,omitempty"`
	Pagination       *moduleconfig.PaginationConfig      `json:"pagination,omitempty"`
	Retry            *connector.RetryConfig              `json:"retry,omitempty"`
	StatePersistence *persistence.StatePersistenceConfig `json:"statePersistence,omitempty"`
}

// HTTPPolling implements polling-based HTTP data fetching.
// It supports HTTP GET requests with authentication, pagination, and retry logic.
// State persistence can be configured to track last timestamp and/or last ID
// for reliable resumption after restarts.
type HTTPPolling struct {
	endpoint         string
	headers          map[string]string
	timeout          time.Duration
	dataField        string
	pagination       *moduleconfig.PaginationConfig
	authHandler      auth.Handler
	client           *httpclient.Client
	retryConfig      connector.RetryConfig
	retryHintProgram *vm.Program // Compiled retryHintFromBody expression (may be nil).
	lastRetryInfo    *connector.RetryInfo

	// OAuth2 token invalidation tracking
	oauth2Invalidated bool

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
//   - timeoutMs: Request timeout in milliseconds (default 30000)
//   - dataField: JSON field containing the array of records (for object responses)
//   - pagination: Pagination configuration (map with type, params, etc.)
func NewHTTPPollingFromConfig(config *connector.ModuleConfig) (*HTTPPolling, error) {
	if config == nil {
		return nil, ErrNilConfig
	}

	cfg, err := moduleconfig.ParseModuleConfig[HTTPPollingInputConfig](*config)
	if err != nil {
		return nil, err
	}
	if cfg.Endpoint == "" {
		return nil, ErrMissingEndpoint
	}

	timeout := connector.GetTimeoutDuration(cfg.TimeoutMs, defaultTimeout)
	retryConfig := moduleconfig.ToRetryConfig(cfg.Retry)

	client := httpclient.NewClient(timeout)
	authHandler, err := auth.NewHandler(cfg.Authentication, client.Client)
	if err != nil {
		return nil, fmt.Errorf("creating auth handler: %w", err)
	}

	retryHintProgram, err := httpclient.CompileRetryHint(retryConfig.RetryHintFromBody)
	if err != nil {
		return nil, err
	}

	h := &HTTPPolling{
		endpoint:          cfg.Endpoint,
		headers:           cfg.Headers,
		timeout:           timeout,
		dataField:         cfg.DataField,
		pagination:        cfg.Pagination,
		authHandler:       authHandler,
		client:            client,
		retryConfig:       retryConfig,
		retryHintProgram:  retryHintProgram,
		persistenceConfig: cfg.StatePersistence,
	}

	// Initialize state store if persistence is enabled
	if cfg.StatePersistence != nil && cfg.StatePersistence.IsEnabled() {
		storagePath := cfg.StatePersistence.StoragePath
		if storagePath == "" {
			storagePath = persistence.DefaultStatePath
		}
		h.stateStore = persistence.NewStateStore(storagePath)

		logger.Debug("state persistence enabled for HTTP polling module",
			"endpoint", cfg.Endpoint,
			"timestamp_enabled", cfg.StatePersistence.TimestampEnabled(),
			"id_enabled", cfg.StatePersistence.IDEnabled(),
			"storage_path", storagePath,
		)
	}

	logModuleCreation(cfg.Endpoint, timeout, authHandler, cfg.Pagination, retryConfig)

	return h, nil
}

// logModuleCreation logs module creation details.
func logModuleCreation(endpoint string, timeout time.Duration, authHandler auth.Handler, pagination *moduleconfig.PaginationConfig, retryConfig connector.RetryConfig) {
	logger.Debug("http polling module created",
		"endpoint", endpoint,
		"timeout", timeout.String(),
		"has_auth", authHandler != nil,
		"has_pagination", pagination != nil,
		"retry_max_attempts", retryConfig.MaxAttempts,
	)
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

	// Reset OAuth2 invalidation tracking for this fetch cycle
	h.oauth2Invalidated = false

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

// buildRequest creates and configures the HTTP GET request.
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

	req.Header.Set("User-Agent", defaultUserAgent)
	validated := make(map[string]string, len(h.headers))
	for key, value := range h.headers {
		httpclient.TryAddValidHeader(validated, key, value)
	}
	for key, value := range validated {
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

// doRequestWithRetry performs an HTTP GET with the module's retry policy via
// httpclient.DoWithRetry. Honors Retry-After (when UseRetryAfterHeader is
// enabled) and retryHintFromBody (when configured).
func (h *HTTPPolling) doRequestWithRetry(ctx context.Context, endpoint string) ([]byte, error) {
	startTime := time.Now()
	req, err := h.buildRequest(ctx, endpoint)
	if err != nil {
		return nil, err
	}

	var delaysMs []int64
	hooks := httpclient.RetryHooks{
		OnRetry: func(attempt int, retryErr error, nextDelay time.Duration) {
			if retryErr == nil {
				return
			}
			if nextDelay > 0 {
				delaysMs = append(delaysMs, nextDelay.Milliseconds())
				logger.Info("retrying http request",
					"module_type", "httpPolling",
					"endpoint", endpoint,
					"attempt", attempt+1,
					"max_attempts", h.retryConfig.MaxAttempts+1,
					"next_delay", nextDelay.String(),
					"error", retryErr.Error(),
					"error_category", errhandling.GetErrorCategory(retryErr),
				)
			}
		},
		ShouldRetryBody: func(body []byte) (bool, bool) {
			return httpclient.EvalRetryHint(h.retryHintProgram, body)
		},
		OnAttemptFailure: func(_ int, resp *http.Response, _ error) bool {
			return h.handleOAuth2Unauthorized(resp, endpoint)
		},
	}

	resp, err := h.client.DoWithRetry(ctx, req, h.retryConfig, hooks)
	if resp != nil {
		defer func() {
			if closeErr := resp.Body.Close(); closeErr != nil {
				logger.Warn("failed to close response body",
					"endpoint", endpoint,
					"error", closeErr.Error(),
				)
			}
		}()
	}
	if len(delaysMs) > 0 {
		h.lastRetryInfo = &connector.RetryInfo{
			TotalAttempts: len(delaysMs) + 1,
			RetryCount:    len(delaysMs),
			RetryDelaysMs: delaysMs,
		}
	} else {
		h.lastRetryInfo = nil
	}

	if err != nil {
		if len(delaysMs) > 0 {
			logger.Error("http request failed after retries",
				"module_type", "httpPolling",
				"endpoint", endpoint,
				"total_attempts", len(delaysMs)+1,
				"total_duration", time.Since(startTime).String(),
				"error", err.Error(),
			)
		}
		return nil, err
	}

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, fmt.Errorf("reading response body: %w", readErr)
	}

	if len(delaysMs) > 0 {
		logger.Info("http request succeeded after retries",
			"module_type", "httpPolling",
			"endpoint", endpoint,
			"total_attempts", len(delaysMs)+1,
			"retry_count", len(delaysMs),
			"total_duration", time.Since(startTime).String(),
		)
	}
	logger.Debug("http request completed",
		"module_type", "httpPolling",
		"endpoint", endpoint,
		"method", http.MethodGet,
		"status_code", resp.StatusCode,
		"response_size", len(body),
	)
	return body, nil
}

// handleOAuth2Unauthorized invalidates the cached OAuth2 token on 401 the
// first time it is seen during a Fetch cycle and signals the retry loop to
// retry once with a fresh token. Subsequent 401s return false to prevent
// infinite loops when credentials are actually invalid.
func (h *HTTPPolling) handleOAuth2Unauthorized(resp *http.Response, endpoint string) bool {
	if resp == nil || resp.StatusCode != http.StatusUnauthorized || h.authHandler == nil {
		return false
	}
	invalidator, ok := h.authHandler.(interface{ InvalidateToken() })
	if !ok {
		return false
	}
	if h.oauth2Invalidated {
		logger.Warn("401 Unauthorized persists after OAuth2 token refresh, likely invalid credentials",
			"endpoint", endpoint,
		)
		return false
	}
	logger.Debug("401 Unauthorized with OAuth2, invalidating cached token",
		"endpoint", endpoint,
	)
	invalidator.InvalidateToken()
	h.oauth2Invalidated = true
	return true
}

// GetRetryInfo returns retry information from the last Fetch request
// (RetryInfoProvider).
func (h *HTTPPolling) GetRetryInfo() *connector.RetryInfo {
	return h.lastRetryInfo
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

// Close releases any resources held by the HTTP polling module, closing
// idle connections in the connection pool.
func (h *HTTPPolling) Close() error {
	h.client.CloseIdleConnections()
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
