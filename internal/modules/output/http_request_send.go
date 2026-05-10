package output

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/cannectors/runtime/internal/auth"
	"github.com/cannectors/runtime/internal/errhandling"
	"github.com/cannectors/runtime/internal/httpclient"
	"github.com/cannectors/runtime/internal/logger"
	"github.com/cannectors/runtime/pkg/connector"
)

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
			slog.String("endpoint", httpclient.SanitizeURL(endpoint)),
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
		slog.String("endpoint", httpclient.SanitizeURL(endpoint)),
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
			slog.String("endpoint", httpclient.SanitizeURL(h.endpoint)),
			slog.String("method", h.method),
			slog.Int("oauth2_retry_count", *retryCount),
		)
		return false
	}

	logger.Debug("401 Unauthorized with OAuth2, invalidating token and retrying",
		slog.String("endpoint", httpclient.SanitizeURL(h.endpoint)),
		slog.String("method", h.method),
		slog.Int("oauth2_retry_count", *retryCount),
	)
	invalidator.InvalidateToken()
	(*retryCount)++
	return true
}

// doRequestWithHeaders executes a single HTTP request with optional
// record-specific headers, delegating the retry loop to httpclient.DoWithRetry.
// Special handling for 401 with OAuth2: invalidates the token and can retry up
// to auth.MaxOAuth2Retries with a new token via the OnAttemptFailure hook.
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
				slog.String("endpoint", httpclient.SanitizeURL(endpoint)),
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
		// Story 24.12 AC8/AC9: when success is configured (status codes
		// and/or expression), responses must be allowed to flow through to
		// the caller for success evaluation, including 4xx/5xx codes.
		IsSuccessStatus: h.statusAllowedAsSuccess,
	}

	resp, err := h.client.DoWithRetry(ctx, req, h.retry, hooks)
	if resp != nil {
		defer func() {
			if closeErr := resp.Body.Close(); closeErr != nil {
				logger.Warn("failed to close response body",
					slog.String("endpoint", httpclient.SanitizeURL(endpoint)),
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

	respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024))
	if readErr != nil {
		return h.recordRetryFailure(
			fmt.Errorf("reading response body: %w", readErr),
			delaysMs, startTime, endpoint,
		)
	}
	success, successErr := h.isResponseSuccess(resp.StatusCode, resp.Header, respBody)
	if successErr != nil {
		return h.recordRetryFailure(successErr, delaysMs, startTime, endpoint)
	}
	if !success {
		bodySnippet := string(respBody)
		if len(bodySnippet) > 500 {
			bodySnippet = bodySnippet[:500] + "..."
		}
		logger.Error("http error response",
			slog.String("module_type", "httpRequest"),
			slog.String("endpoint", httpclient.SanitizeURL(endpoint)),
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
	if err := httpclient.ValidateAbsoluteURL(endpoint); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, h.method, endpoint, bytes.NewReader(body))
	if err != nil {
		logger.Error("failed to create http request",
			slog.String("module_type", "httpRequest"),
			slog.String("endpoint", httpclient.SanitizeURL(endpoint)),
			slog.String("method", h.method),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("creating http request: %w", err)
	}

	baseHeaders, err := h.buildBaseHeadersMap(recordHeaders)
	if err != nil {
		return nil, err
	}
	for key, value := range baseHeaders {
		req.Header.Set(key, value)
	}

	if err := h.applyAuthentication(ctx, req); err != nil {
		logger.Error("failed to apply authentication",
			slog.String("module_type", "httpRequest"),
			slog.String("endpoint", httpclient.SanitizeURL(endpoint)),
			slog.String("error", err.Error()),
		)
		return nil, fmt.Errorf("applying authentication: %w", err)
	}

	logger.Debug("sending http request",
		slog.String("module_type", "httpRequest"),
		slog.String("endpoint", httpclient.SanitizeURL(endpoint)),
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
			slog.String("endpoint", httpclient.SanitizeURL(endpoint)),
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
		slog.String("endpoint", httpclient.SanitizeURL(endpoint)),
		slog.Int("attempts", h.retry.MaxAttempts+1),
		slog.Duration("total_duration", time.Since(startTime)),
		slog.String("error", safeErr.Error()),
	)
	return safeErr
}

// applyAuthentication applies authentication to the HTTP request using the shared auth package.
func (h *HTTPRequestModule) applyAuthentication(ctx context.Context, req *http.Request) error {
	if h.authHandler == nil {
		return nil
	}
	return h.authHandler.ApplyAuth(ctx, req)
}

// statusAllowedAsSuccess returns true when the configured success contract
// can plausibly accept the status code as success. The actual decision is
// finalized by isResponseSuccess after evaluating the expression on the
// body. This hook only widens the >=400 default of httpclient.DoWithRetry
// so that 4xx/5xx codes listed in success.statusCodes (or covered by
// success.expression alone) flow through to the caller.
func (h *HTTPRequestModule) statusAllowedAsSuccess(statusCode int) bool {
	// Expression-only success: any status flows through; expression decides.
	if h.successCodes == nil {
		return true
	}
	for _, code := range h.successCodes {
		if statusCode == code {
			return true
		}
	}
	// Fall back to default behavior (status<400 = success, >=400 = error).
	return statusCode < 400
}

// isResponseSuccess returns true when the response satisfies both the
// configured status codes (if any) AND the configured success.expression
// (if any). When neither is configured, defaults from defaultSuccessCodes
// apply via successCodes. Story 24.12 AC8/AC9/AC10/AC11.
func (h *HTTPRequestModule) isResponseSuccess(statusCode int, headers map[string][]string, body []byte) (bool, error) {
	statusOK := true
	if h.successCodes != nil {
		statusOK = false
		for _, code := range h.successCodes {
			if statusCode == code {
				statusOK = true
				break
			}
		}
	}
	exprOK, err := h.evaluateSuccessExpression(statusCode, headers, body)
	if err != nil {
		return false, err
	}
	return statusOK && exprOK, nil
}
