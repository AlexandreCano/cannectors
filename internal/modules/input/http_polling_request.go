package input

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cannectors/runtime/internal/auth"
	"github.com/cannectors/runtime/internal/errhandling"
	"github.com/cannectors/runtime/internal/httpclient"
	"github.com/cannectors/runtime/internal/logger"
	"github.com/cannectors/runtime/pkg/connector"
)

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
		// Story 17.5: surface a typed authentication error when 401 persists
		// after the maximum number of OAuth2 token-refresh retries.
		if resp != nil && resp.StatusCode == http.StatusUnauthorized && h.oauth2RetryCount >= auth.MaxOAuth2Retries {
			return nil, fmt.Errorf("%w: endpoint=%s", auth.ErrOAuth2InvalidCredentials, endpoint)
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

// handleOAuth2Unauthorized invalidates the cached OAuth2 token on 401 and
// signals the retry loop to retry with a fresh token, but only up to
// auth.MaxOAuth2Retries times per Fetch cycle. Beyond that, returning false
// stops the loop so the caller can wrap the failure as
// auth.ErrOAuth2InvalidCredentials (Story 17.5).
func (h *HTTPPolling) handleOAuth2Unauthorized(resp *http.Response, endpoint string) bool {
	if resp == nil || resp.StatusCode != http.StatusUnauthorized || h.authHandler == nil {
		return false
	}
	invalidator, ok := h.authHandler.(interface{ InvalidateToken() })
	if !ok {
		return false
	}
	if h.oauth2RetryCount >= auth.MaxOAuth2Retries {
		logger.Warn("401 Unauthorized persists after OAuth2 token refresh, likely invalid credentials",
			"endpoint", endpoint,
			"oauth2_retry_count", h.oauth2RetryCount,
		)
		return false
	}
	logger.Debug("401 Unauthorized with OAuth2, invalidating cached token",
		"endpoint", endpoint,
		"oauth2_retry_count", h.oauth2RetryCount,
	)
	invalidator.InvalidateToken()
	h.oauth2RetryCount++
	return true
}
