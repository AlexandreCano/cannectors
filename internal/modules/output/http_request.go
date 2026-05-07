// Package output provides implementations for output modules.
// Output modules are responsible for sending data to destination systems.
package output

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
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

// GetRetryInfo returns retry information from the last Send request (RetryInfoProvider).
func (h *HTTPRequestModule) GetRetryInfo() *connector.RetryInfo {
	return h.lastRetryInfo
}

// Close releases any resources held by the HTTP request module.
func (h *HTTPRequestModule) Close() error {
	h.client.CloseIdleConnections()
	logger.Debug("http request output module closed",
		slog.String("endpoint", h.endpoint),
	)
	return nil
}
