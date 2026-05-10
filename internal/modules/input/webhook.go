// Package input provides implementations for input modules.
// Input modules are responsible for fetching data from source systems.
package input

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/cannectors/runtime/internal/errhandling"
	"github.com/cannectors/runtime/internal/logger"
	"github.com/cannectors/runtime/internal/moduleconfig"
	"github.com/cannectors/runtime/pkg/connector"
)

// Default configuration values for webhook
const (
	defaultListenAddress   = "0.0.0.0:8080"
	defaultSignatureHeader = "X-Webhook-Signature"
	defaultReadTimeout     = 15 * time.Second
	defaultWriteTimeout    = 15 * time.Second
	defaultShutdownTimeout = 5 * time.Second
	defaultQueueSize       = 0
	defaultMaxConcurrent   = 0
)

// Webhook-specific error types
var (
	ErrWebhookServerClosed    = errors.New("webhook server closed")
	ErrInvalidSignature       = errors.New("invalid webhook signature")
	ErrMissingSignature       = errors.New("missing required signature header")
	ErrEmptyRequestBody       = errors.New("request body is empty")
	ErrInvalidJSONPayload     = errors.New("invalid JSON payload")
	ErrMissingSignatureSecret = errors.New("signature validation requires secret")
	ErrUnsupportedSignature   = errors.New("unsupported signature type")
	ErrMissingSignatureType   = errors.New("signature type is required")
	ErrMissingPath            = errors.New("path is required")
	ErrRateLimited            = errors.New("rate limit exceeded")
	ErrQueueFull              = errors.New("webhook queue is full")
	ErrInvalidQueueSize       = errors.New("queueSize must be >= 0")
	ErrInvalidMaxConcurrent   = errors.New("maxConcurrent must be >= 0")
)

// SignatureConfig holds webhook signature validation configuration
type SignatureConfig struct {
	Type   string `json:"type"`             // "hmac-sha256"
	Header string `json:"header,omitempty"` // Header name containing signature
	Secret string `json:"secret,omitempty"` // Secret key for signature validation
}

// RateLimitConfig holds basic rate limiting configuration
type RateLimitConfig struct {
	RequestsPerSecond int `json:"requestsPerSecond,omitempty"`
	Burst             int `json:"burst,omitempty"`
}

type rateLimiter struct {
	tokens chan struct{}
	stop   chan struct{}
}

// WebhookHandler is a callback function that processes webhook data.
// It receives a context carrying a trace ID derived from the incoming request
// (X-Request-Id header, W3C traceparent, or a freshly generated UUID v4),
// the parsed payload, and returns an error if processing fails.
type WebhookHandler func(ctx context.Context, data []map[string]any) error

// queuedRequest pairs a webhook payload with the per-request context that
// carries its trace ID, so workers can preserve correlation when handling
// queued items asynchronously.
type queuedRequest struct {
	ctx  context.Context
	data []map[string]any
}

// Webhook implements an HTTP server that receives webhook POST requests.
// It supports HMAC-SHA256 signature validation and callback-based processing.
//
// Unlike HTTPPolling which is pull-based, Webhook is push-based.
// Data is received via HTTP POST and immediately passed to a handler callback.
type Webhook struct {
	endpoint      string
	listenAddress string
	dataField     string
	timeout       time.Duration
	signature     *SignatureConfig
	queueSize     int
	maxConcurrent int
	rateLimit     *RateLimitConfig

	// Server components
	server     *http.Server
	listener   net.Listener
	actualAddr string // Actual address after binding (useful for port 0)

	// State management
	mu         sync.RWMutex
	running    bool
	queue      chan queuedRequest
	workerStop chan struct{}
	workerWG   sync.WaitGroup
	limiter    *rateLimiter

	// Safe shutdown (prevents double close panic)
	shutdownOnce sync.Once
	workersOnce  sync.Once
}

// WebhookInputConfig holds the parsed configuration for the webhook input module.
type WebhookInputConfig struct {
	connector.ModuleBase
	Path          string           `json:"path,omitempty"`
	ListenAddress string           `json:"listenAddress,omitempty"`
	DataField     string           `json:"dataField,omitempty"`
	TimeoutMs     int              `json:"timeoutMs,omitempty"`
	Signature     *SignatureConfig `json:"signature,omitempty"`
	QueueSize     int              `json:"queueSize,omitempty"`
	MaxConcurrent int              `json:"maxConcurrent,omitempty"`
	RateLimit     *RateLimitConfig `json:"rateLimit,omitempty"`
}

// NewWebhookFromConfig creates a new Webhook input module from configuration.
//
// Required config fields:
//   - path: The HTTP endpoint path (e.g., "/webhook/orders").
//
// Optional config fields:
//   - listenAddress: Server listen address (default: "0.0.0.0:8080")
//   - dataField: JSON field containing the array of records (for nested payloads)
//   - timeoutMs: Request timeout in milliseconds (default: 15000)
//   - signature: Signature validation configuration
//   - type: "hmac-sha256"
//   - header: Header name for signature (default: "X-Webhook-Signature")
//   - secret: Secret key for validation
func NewWebhookFromConfig(config *connector.ModuleConfig) (*Webhook, error) {
	if config == nil {
		return nil, ErrNilConfig
	}

	cfg, err := moduleconfig.ParseModuleConfig[WebhookInputConfig](*config)
	if err != nil {
		return nil, err
	}

	if cfg.Path == "" {
		return nil, ErrMissingPath
	}
	endpoint := cfg.Path

	if _, err := errhandling.ParseOnErrorStrategy(cfg.OnError); err != nil {
		return nil, err
	}

	if cfg.Signature != nil {
		if cfg.Signature.Header == "" {
			cfg.Signature.Header = defaultSignatureHeader
		}
		if err := validateSignatureConfig(cfg.Signature); err != nil {
			return nil, err
		}
	}

	queueSize := cfg.QueueSize
	maxConcurrent := cfg.MaxConcurrent
	if queueSize < 0 {
		return nil, ErrInvalidQueueSize
	}
	if maxConcurrent < 0 {
		return nil, ErrInvalidMaxConcurrent
	}
	if queueSize > 0 && maxConcurrent == 0 {
		maxConcurrent = 1
	}

	listenAddress := cfg.ListenAddress
	if listenAddress == "" {
		listenAddress = defaultListenAddress
	}

	timeout := connector.GetTimeoutDuration(cfg.TimeoutMs, defaultReadTimeout)

	if cfg.RateLimit != nil && cfg.RateLimit.RequestsPerSecond > 0 && cfg.RateLimit.Burst <= 0 {
		cfg.RateLimit.Burst = cfg.RateLimit.RequestsPerSecond
	}

	w := &Webhook{
		endpoint:      endpoint,
		listenAddress: listenAddress,
		dataField:     cfg.DataField,
		timeout:       timeout,
		signature:     cfg.Signature,
		queueSize:     queueSize,
		maxConcurrent: maxConcurrent,
		rateLimit:     cfg.RateLimit,
	}

	logWebhookModuleCreated(w)
	return w, nil
}

func logWebhookModuleCreated(w *Webhook) {
	logger.Debug("webhook module created",
		"endpoint", w.endpoint,
		"listenAddress", w.listenAddress,
		"has_signature", w.signature != nil,
		"queueSize", w.queueSize,
		"maxConcurrent", w.maxConcurrent,
		"rateLimit", w.rateLimit != nil,
	)
}

func validateSignatureConfig(signature *SignatureConfig) error {
	if signature.Type == "" {
		return ErrMissingSignatureType
	}
	if signature.Type != "hmac-sha256" {
		return ErrUnsupportedSignature
	}
	if signature.Secret == "" {
		return ErrMissingSignatureSecret
	}
	if signature.Header == "" {
		signature.Header = defaultSignatureHeader
	}
	return nil
}

// newRateLimiter creates a token bucket rate limiter with the specified rate and burst.
//
// The rate limiter uses a channel-based token bucket implementation:
//   - tokens channel acts as the bucket with capacity = burst
//   - A background goroutine refills tokens at the specified rate
//   - Allow() consumes a token (non-blocking)
//
// Lifecycle:
//   - The goroutine starts immediately when newRateLimiter is called
//   - Stop() MUST be called to clean up the goroutine (e.g., in defer or shutdown)
//   - If Stop() is not called, the goroutine will leak
//
// Edge cases:
//   - Returns nil if requestsPerSecond <= 0 or burst <= 0
//   - For very high requestsPerSecond values (> 1 billion), interval may be very small
//     but is clamped to at least 1 nanosecond to prevent ticker issues
//
// Parameters:
//   - requestsPerSecond: rate at which tokens are refilled (tokens/second)
//   - burst: maximum number of tokens (bucket capacity)
func newRateLimiter(requestsPerSecond int, burst int) *rateLimiter {
	if requestsPerSecond <= 0 || burst <= 0 {
		return nil
	}
	limiter := &rateLimiter{
		tokens: make(chan struct{}, burst),
		stop:   make(chan struct{}),
	}
	// Pre-fill bucket to burst capacity
	for i := 0; i < burst; i++ {
		limiter.tokens <- struct{}{}
	}
	// Calculate refill interval
	interval := time.Second / time.Duration(requestsPerSecond)
	if interval <= 0 {
		// Safety: ensure interval is at least 1ns for extreme requestsPerSecond values
		interval = time.Nanosecond
	}
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				// Try to add a token (non-blocking to avoid deadlock if bucket is full)
				select {
				case limiter.tokens <- struct{}{}:
				default:
					// Bucket full, discard token
				}
			case <-limiter.stop:
				return
			}
		}
	}()
	return limiter
}

func (l *rateLimiter) Allow() bool {
	if l == nil {
		return true
	}
	select {
	case <-l.tokens:
		return true
	default:
		return false
	}
}

func (l *rateLimiter) Stop() {
	if l == nil {
		return
	}
	close(l.stop)
}

// Fetch implements the input.Module interface.
// For webhooks, this method is not applicable as data is pushed via HTTP POST.
// Use Start() with a callback handler instead.
//
// Returns ErrNotImplemented as webhooks use callback-based pattern.
func (w *Webhook) Fetch(_ context.Context) ([]map[string]any, error) {
	return nil, ErrNotImplemented
}

// Start starts the webhook HTTP server and begins listening for requests.
// This method blocks until the context is canceled or an error occurs.
//
// The handler callback is invoked for each valid webhook request with the
// parsed payload data. If the handler returns an error, a 500 response is sent.
//
// Parameters:
//   - ctx: Context for cancellation and graceful shutdown
//   - handler: Callback function to process webhook payloads
//
// Returns an error if the server fails to start or encounters a fatal error.
func (w *Webhook) Start(ctx context.Context, handler WebhookHandler) error {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return errors.New("webhook server already running")
	}
	w.running = true
	w.mu.Unlock()

	// Create HTTP handler
	mux := http.NewServeMux()
	mux.Handle(w.endpoint, w.createHandler(handler))

	if w.rateLimit != nil && w.rateLimit.RequestsPerSecond > 0 {
		w.limiter = newRateLimiter(w.rateLimit.RequestsPerSecond, w.rateLimit.Burst)
	}

	// Create HTTP server with timeouts
	w.server = &http.Server{
		Addr:         w.listenAddress,
		Handler:      mux,
		ReadTimeout:  w.timeout,
		WriteTimeout: w.timeout,
	}

	// Create listener (allows getting actual address for port 0)
	listener, err := net.Listen("tcp", w.listenAddress)
	if err != nil {
		// Clean up rate limiter if it was created
		if w.limiter != nil {
			w.limiter.Stop()
			w.limiter = nil
		}
		w.mu.Lock()
		w.running = false
		w.mu.Unlock()
		logger.Error("failed to start webhook server",
			"listenAddress", w.listenAddress,
			"error", err.Error(),
		)
		return fmt.Errorf("starting webhook listener: %w", err)
	}
	w.listener = listener

	// Store actual address (important when using port 0)
	w.mu.Lock()
	w.actualAddr = listener.Addr().String()
	w.mu.Unlock()

	logger.Info("webhook server started",
		"endpoint", w.endpoint,
		"address", w.actualAddr,
	)

	// Start queue workers if enabled. Use the server ctx (not the per-request
	// ctx) so workers survive past the HTTP response that triggered enqueue.
	w.startWorkers(ctx, handler)

	// Start server in goroutine
	serverErr := make(chan error, 1)
	go func() {
		if err := w.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
		close(serverErr)
	}()

	// Listen for OS shutdown signals
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signalChan)

	// Wait for context cancellation, OS signal, or server error
	select {
	case <-ctx.Done():
		logger.Info("webhook server shutdown requested",
			"endpoint", w.endpoint,
		)
		return w.shutdown()
	case sig := <-signalChan:
		logger.Info("webhook server shutdown requested by signal",
			"endpoint", w.endpoint,
			"signal", sig.String(),
		)
		return w.shutdown()
	case err := <-serverErr:
		w.mu.Lock()
		w.running = false
		w.mu.Unlock()

		// Ensure proper cleanup of workers, rate limiter, and server resources.
		shutdownErr := w.shutdown()

		if err != nil {
			if shutdownErr != nil {
				logger.Error("error during webhook shutdown after server error",
					"endpoint", w.endpoint,
					"serverError", err.Error(),
					"shutdownError", shutdownErr.Error(),
				)
			}
			return fmt.Errorf("webhook server error: %w", err)
		}
		// If the server stopped without an explicit error, propagate any shutdown error.
		return shutdownErr
	}
}

// Close stops the webhook server and releases resources.
// This is an alias for Stop() to satisfy the input.Module interface.
func (w *Webhook) Close() error {
	return w.Stop()
}

// Stop gracefully stops the webhook server.
// It waits for in-flight requests to complete before returning.
func (w *Webhook) Stop() error {
	return w.shutdown()
}

// shutdown performs graceful shutdown of the webhook server.
// Uses sync.Once to ensure shutdown only executes once, even if called
// concurrently from multiple goroutines (e.g., Stop() and context cancellation).
func (w *Webhook) shutdown() error {
	var shutdownErr error

	w.shutdownOnce.Do(func() {
		w.mu.Lock()
		if !w.running {
			w.mu.Unlock()
			return
		}
		// Mark as not running immediately to prevent new operations
		w.running = false
		w.mu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), defaultShutdownTimeout)
		defer cancel()

		logger.Info("shutting down webhook server",
			"endpoint", w.endpoint,
		)

		// Shutdown HTTP server first (stops accepting new requests)
		shutdownErr = w.server.Shutdown(ctx)

		// Stop workers after server is shut down
		w.stopWorkers()

		if shutdownErr != nil {
			logger.Error("webhook server shutdown error",
				"error", shutdownErr.Error(),
			)
			shutdownErr = fmt.Errorf("shutting down webhook server: %w", shutdownErr)
			return
		}

		logger.Info("webhook server stopped",
			"endpoint", w.endpoint,
		)
	})

	return shutdownErr
}

// Address returns the actual address the server is listening on.
// This is useful when using port 0 for dynamic port allocation.
// Returns empty string if server is not running.
func (w *Webhook) Address() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.actualAddr
}

// IsRunning returns true if the webhook server is currently running.
func (w *Webhook) IsRunning() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.running
}

// createHandler creates the HTTP handler for webhook requests.
func (w *Webhook) createHandler(handler WebhookHandler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		startTime := time.Now()

		// Mint or extract a trace ID for this request before any logging so
		// every subsequent log entry can be correlated.
		reqCtx, traceID := logger.EnsureTraceID(
			logger.WithTraceID(r.Context(), logger.TraceIDFromHTTPHeader(r.Header)),
		)

		if !w.checkMethodAndPath(rw, r, traceID) {
			return
		}
		if !w.checkRateLimit(rw, traceID) {
			return
		}
		body, ok := w.readRequestBody(rw, r, traceID)
		if !ok {
			return
		}
		if !w.checkSignature(rw, r, body, traceID) {
			return
		}
		data, ok := w.parsePayloadAndLog(rw, body, traceID)
		if !ok {
			return
		}
		if !w.dispatchToHandler(reqCtx, rw, data, handler, traceID) {
			return
		}

		logger.Debug("webhook request processed",
			slog.String(logger.TraceIDField, traceID),
			"endpoint", w.endpoint,
			"recordCount", len(data),
			"duration", time.Since(startTime).String(),
		)
		w.writeSuccessResponse(rw)
	})
}

// checkMethodAndPath validates HTTP method (POST only) and URL path. Returns false if invalid (response already written).
func (w *Webhook) checkMethodAndPath(rw http.ResponseWriter, r *http.Request, traceID string) bool {
	if r.Method != http.MethodPost {
		logger.Warn("webhook received non-POST request", slog.String(logger.TraceIDField, traceID), "method", r.Method, "endpoint", w.endpoint)
		http.Error(rw, "Method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	if r.URL.Path != w.endpoint {
		logger.Warn("webhook received request on wrong endpoint", slog.String(logger.TraceIDField, traceID), "expected", w.endpoint, "received", r.URL.Path)
		http.Error(rw, "Not found", http.StatusNotFound)
		return false
	}
	return true
}

// checkRateLimit applies rate limiting if configured. Returns false if rate limited (response already written).
func (w *Webhook) checkRateLimit(rw http.ResponseWriter, traceID string) bool {
	if w.limiter != nil && !w.limiter.Allow() {
		logger.Warn("webhook rate limit exceeded", slog.String(logger.TraceIDField, traceID), "endpoint", w.endpoint)
		http.Error(rw, "Rate limit exceeded", http.StatusTooManyRequests)
		return false
	}
	return true
}

// readRequestBody reads and returns the request body. Returns (nil, false) on error (response already written).
func (w *Webhook) readRequestBody(rw http.ResponseWriter, r *http.Request, traceID string) ([]byte, bool) {
	defer func() {
		if closeErr := r.Body.Close(); closeErr != nil {
			logger.Error("failed to close request body", slog.String(logger.TraceIDField, traceID), "endpoint", w.endpoint, "error", closeErr.Error())
		}
	}()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error("failed to read webhook request body", slog.String(logger.TraceIDField, traceID), "endpoint", w.endpoint, "error", err.Error())
		http.Error(rw, "Failed to read request body", http.StatusBadRequest)
		return nil, false
	}
	if len(body) == 0 {
		logger.Warn("webhook received empty body", slog.String(logger.TraceIDField, traceID), "endpoint", w.endpoint)
		http.Error(rw, "Request body is empty", http.StatusBadRequest)
		return nil, false
	}
	return body, true
}

// checkSignature validates HMAC signature if configured. Returns false if invalid (response already written).
func (w *Webhook) checkSignature(rw http.ResponseWriter, r *http.Request, body []byte, traceID string) bool {
	if w.signature == nil || w.signature.Type != "hmac-sha256" {
		return true
	}
	if sigErr := w.validateSignature(r, body); sigErr != nil {
		logger.Warn("webhook signature validation failed", slog.String(logger.TraceIDField, traceID), "endpoint", w.endpoint, "error", sigErr.Error())
		http.Error(rw, "Invalid signature", http.StatusUnauthorized)
		return false
	}
	return true
}

// parsePayloadAndLog parses the JSON body into records. Returns (nil, false) on error (response already written).
func (w *Webhook) parsePayloadAndLog(rw http.ResponseWriter, body []byte, traceID string) ([]map[string]any, bool) {
	data, err := w.parsePayload(body)
	if err != nil {
		logger.Error("failed to parse webhook payload", slog.String(logger.TraceIDField, traceID), "endpoint", w.endpoint, "error", err.Error(), "bodySize", len(body))
		http.Error(rw, "Invalid JSON payload", http.StatusBadRequest)
		return nil, false
	}
	return data, true
}

// dispatchToHandler invokes the handler or enqueues data. Returns false on error (response already written).
func (w *Webhook) dispatchToHandler(ctx context.Context, rw http.ResponseWriter, data []map[string]any, handler WebhookHandler, traceID string) bool {
	if handler == nil {
		return true
	}
	if w.queue != nil {
		if !w.enqueue(queuedRequest{ctx: ctx, data: data}) {
			logger.Warn("webhook queue full", slog.String(logger.TraceIDField, traceID), "endpoint", w.endpoint, "queueSize", w.queueSize)
			http.Error(rw, "Queue full", http.StatusTooManyRequests)
			return false
		}
		return true
	}
	if err := handler(ctx, data); err != nil {
		logger.Error("webhook handler returned error", slog.String(logger.TraceIDField, traceID), "endpoint", w.endpoint, "error", err.Error(), "recordCount", len(data))
		http.Error(rw, "Internal server error", http.StatusInternalServerError)
		return false
	}
	return true
}

// writeSuccessResponse writes 200 OK or 202 Accepted and JSON body.
func (w *Webhook) writeSuccessResponse(rw http.ResponseWriter) {
	if w.queue != nil {
		rw.WriteHeader(http.StatusAccepted)
	} else {
		rw.WriteHeader(http.StatusOK)
	}
	if _, writeErr := rw.Write([]byte(`{"status":"ok"}`)); writeErr != nil {
		logger.Warn("failed to write response", "endpoint", w.endpoint, "error", writeErr.Error())
	}
}

// validateSignature validates the HMAC-SHA256 signature of the request
func (w *Webhook) validateSignature(r *http.Request, body []byte) error {
	if w.signature.Secret == "" {
		return ErrMissingSignatureSecret
	}

	// Get signature from header
	receivedSig := r.Header.Get(w.signature.Header)
	if receivedSig == "" {
		return ErrMissingSignature
	}

	// Compute expected signature
	mac := hmac.New(sha256.New, []byte(w.signature.Secret))
	mac.Write(body)
	expectedSig := hex.EncodeToString(mac.Sum(nil))

	// Constant-time comparison to prevent timing attacks
	if subtle.ConstantTimeCompare([]byte(receivedSig), []byte(expectedSig)) != 1 {
		return ErrInvalidSignature
	}

	return nil
}

// parsePayload parses the JSON request body into records
func (w *Webhook) parsePayload(body []byte) ([]map[string]any, error) {
	// Try parsing as array first
	var arrayResult []map[string]any
	if err := json.Unmarshal(body, &arrayResult); err == nil {
		return arrayResult, nil
	}

	// Try parsing as object
	var objectResult map[string]any
	if err := json.Unmarshal(body, &objectResult); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidJSONPayload, err)
	}

	// If dataField is specified, extract array from that field
	if w.dataField != "" {
		return w.extractDataFromField(objectResult, w.dataField)
	}

	// Try common data field names
	for _, field := range []string{"data", "items", "results", "records"} {
		if data, ok := objectResult[field]; ok {
			if records, err := w.convertToRecords(data); err == nil {
				return records, nil
			}
		}
	}

	// Return single object as single-element array
	return []map[string]any{objectResult}, nil
}

// extractDataFromField extracts array data from a specific field in the object
func (w *Webhook) extractDataFromField(obj map[string]any, field string) ([]map[string]any, error) {
	data, ok := obj[field]
	if !ok {
		return nil, fmt.Errorf("field '%s' not found in payload", field)
	}

	return w.convertToRecords(data)
}

// convertToRecords converts any to []map[string]any
func (w *Webhook) convertToRecords(data any) ([]map[string]any, error) {
	switch v := data.(type) {
	case []any:
		records := make([]map[string]any, 0, len(v))
		for _, item := range v {
			if record, ok := item.(map[string]any); ok {
				records = append(records, record)
			} else {
				return nil, fmt.Errorf("%w: array contains non-object", ErrInvalidJSONPayload)
			}
		}
		return records, nil
	case []map[string]any:
		return v, nil
	default:
		return nil, fmt.Errorf("expected array, got %T", data)
	}
}

func (w *Webhook) enqueue(req queuedRequest) bool {
	select {
	case w.queue <- req:
		return true
	default:
		return false
	}
}

func (w *Webhook) startWorkers(serverCtx context.Context, handler WebhookHandler) {
	if handler == nil || w.queueSize <= 0 {
		return
	}

	// Check and set queue under mutex to prevent race conditions
	w.mu.Lock()
	if w.queue != nil {
		w.mu.Unlock()
		return
	}
	w.queue = make(chan queuedRequest, w.queueSize)
	w.workerStop = make(chan struct{})
	queue := w.queue
	workerStop := w.workerStop
	w.mu.Unlock()

	workers := w.maxConcurrent
	if workers <= 0 {
		workers = 1
	}

	for i := 0; i < workers; i++ {
		w.workerWG.Add(1)
		go func() {
			defer w.workerWG.Done()
			for {
				select {
				case req, ok := <-queue:
					if !ok {
						return
					}
					// Use the server context so the handler isn't canceled by
					// the HTTP response that originally enqueued the request.
					// Preserve the trace ID from the request context.
					workerCtx := logger.WithTraceID(serverCtx, logger.TraceIDFrom(req.ctx))
					if err := handler(workerCtx, req.data); err != nil {
						logger.Error("webhook handler returned error",
							slog.String(logger.TraceIDField, logger.TraceIDFrom(req.ctx)),
							"endpoint", w.endpoint,
							"error", err.Error(),
							"recordCount", len(req.data),
						)
					}
				case <-workerStop:
					return
				}
			}
		}()
	}
}

// stopWorkers stops all worker goroutines and cleans up resources.
// Uses sync.Once to ensure this only executes once.
//
// Shutdown behavior:
//   - Closes the queue channel first, signaling workers no more data will arrive
//   - Closes workerStop channel to interrupt any workers blocked on select
//   - Waits for all workers to finish via workerWG
//
// Note: Any pending items in the queue at shutdown time will be dropped.
// This is intentional for fast graceful shutdown. If at-least-once delivery
// is required, implement a persistent queue or acknowledgment mechanism.
func (w *Webhook) stopWorkers() {
	w.workersOnce.Do(func() {
		// Stop rate limiter first (safe to call even if nil)
		if w.limiter != nil {
			w.limiter.Stop()
			w.limiter = nil
		}

		// Stop workers if queue was initialized
		w.mu.Lock()
		queue := w.queue
		workerStop := w.workerStop
		w.queue = nil
		w.workerStop = nil
		w.mu.Unlock()

		// Close queue first - workers will exit when they see closed channel
		if queue != nil {
			close(queue)
		}
		// Close workerStop to interrupt workers blocked in select
		if workerStop != nil {
			close(workerStop)
		}

		// Wait for workers to finish processing and exit
		w.workerWG.Wait()
	})
}
