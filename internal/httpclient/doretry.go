package httpclient

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/cannectors/runtime/internal/errhandling"
)

// ErrNilClient is returned by DoWithRetry when invoked on a nil Client.
var ErrNilClient = errors.New("httpclient: nil client")

// maxErrorBodyBytes caps the body read by DoWithRetry on error responses
// (status >= 400) to prevent memory exhaustion when a server misbehaves.
// Success responses are read in full so that callers ingesting large JSON
// payloads (e.g. the polling input on multi-MiB paginated pages) are not
// silently truncated.
const maxErrorBodyBytes int64 = 1 * 1024 * 1024

// RetryHooks groups the optional callbacks used by DoWithRetry to let the
// caller customize observability and the retry decision.
//
// Any hook may be nil; DoWithRetry handles that case.
type RetryHooks struct {
	// OnRetry is invoked after every attempt (success or failure):
	//   - attempt: 0-based index of the attempt that just ran.
	//   - err    : error returned by the attempt (nil on success).
	//   - delay  : delay before the next attempt (0 when no further retry
	//              will be attempted).
	OnRetry func(attempt int, err error, delay time.Duration)

	// ShouldRetryBody lets the caller override the retry decision by
	// inspecting the response body (e.g. retryHintFromBody). Returns
	// (retry, hinted): when hinted is true, retry overrides the default
	// classification; when hinted is false, the HTTP status code is used.
	//
	// The hook is called for every response (including 2xx) so that a body
	// containing `retry: true` can force a retry even on a 200 OK.
	ShouldRetryBody func(body []byte) (retry, hinted bool)

	// OnAttemptFailure is invoked after each failed attempt (resp is the
	// response that triggered the failure; err is the classified error).
	// If it returns true, the attempt is treated as retryable even when
	// the status code would normally mark it fatal (e.g. 401 for OAuth2
	// token invalidation). The hook also receives the attempt index so
	// that callers can bound the override (one-shot OAuth2 retry).
	OnAttemptFailure func(attempt int, resp *http.Response, err error) (forceRetry bool)
}

// DoWithRetry executes req using the retry policy described by cfg. The
// retry loop is delegated to errhandling.RetryExecutor; errors are
// classified via errhandling.ClassifyHTTPStatus and
// errhandling.ClassifyNetworkError.
//
// The request is cloned per attempt, so callers must pass a request whose
// body is rewindable (e.g. a bytes.Reader via http.NewRequestWithContext).
// Body setup is the caller's responsibility.
//
// DoWithRetry reads the response body and rewinds it via
// io.NopCloser(bytes.NewReader(body)) so the caller sees resp.Body as a
// standard io.ReadCloser. Callers can read it normally. For safety, error
// bodies (status >= 400) are capped at maxErrorBodyBytes; success bodies are
// read in full.
//
// When cfg.UseRetryAfterHeader is true, a valid Retry-After header on the
// error path overrides the exponential-backoff delay (capped by MaxDelayMs,
// clamped to 0 for past HTTP-dates).
func (c *Client) DoWithRetry(ctx context.Context, req *http.Request, cfg errhandling.RetryConfig, hooks RetryHooks) (*http.Response, error) {
	if c == nil || c.Client == nil {
		return nil, ErrNilClient
	}

	executor := errhandling.NewRetryExecutor(cfg)

	var lastResp *http.Response
	attempt := -1

	fn := func(ctx context.Context) (any, error) {
		attempt++
		reqClone := req.Clone(ctx)
		if req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return nil, errhandling.NewNetworkError("rewinding request body", err)
			}
			reqClone.Body = body
		}
		resp, err := c.Do(reqClone)
		if err != nil {
			return nil, errhandling.ClassifyNetworkError(err)
		}

		var bodyReader io.Reader = resp.Body
		if resp.StatusCode >= 400 {
			bodyReader = io.LimitReader(resp.Body, maxErrorBodyBytes)
		}
		body, readErr := io.ReadAll(bodyReader)
		_ = resp.Body.Close()
		if readErr != nil {
			return resp, errhandling.NewNetworkError("reading response body", readErr)
		}
		resp.Body = io.NopCloser(bytes.NewReader(body))
		lastResp = resp

		if resp.StatusCode >= 400 {
			classified := errhandling.ClassifyHTTPStatus(resp.StatusCode, resp.Status)
			classified.OriginalErr = &Error{
				StatusCode:      resp.StatusCode,
				Status:          resp.Status,
				Endpoint:        req.URL.String(),
				Method:          req.Method,
				Message:         resp.Status,
				ResponseBody:    string(body),
				ResponseHeaders: resp.Header.Clone(),
			}

			// Baseline: when cfg provides an explicit allowlist, use it;
			// otherwise fall back to ClassifyHTTPStatus defaults.
			if len(cfg.RetryableStatusCodes) > 0 {
				classified.Retryable = cfg.IsStatusCodeRetryable(resp.StatusCode)
			}

			// retryHintFromBody: true keeps status-code-driven decision,
			// false forces no-retry (matches output module semantics).
			if hooks.ShouldRetryBody != nil {
				if wantRetry, hinted := hooks.ShouldRetryBody(body); hinted {
					if !wantRetry {
						classified.Retryable = false
					}
				}
			}

			if hooks.OnAttemptFailure != nil {
				if hooks.OnAttemptFailure(attempt, resp, classified) {
					classified.Retryable = true
				}
			}
			return resp, classified
		}

		if hooks.ShouldRetryBody != nil {
			if retry, hinted := hooks.ShouldRetryBody(body); hinted && retry {
				synth := &errhandling.ClassifiedError{
					Category:   errhandling.CategoryServer,
					Retryable:  true,
					StatusCode: resp.StatusCode,
					Message:    "retry forced by retryHintFromBody",
				}
				return resp, synth
			}
		}
		return resp, nil
	}

	executorHooks := errhandling.Hooks{}
	if cfg.UseRetryAfterHeader {
		executorHooks.ExtractRetryAfter = RetryAfterFromError
	}

	callback := func(a int, err error, nextDelay time.Duration) {
		if hooks.OnRetry != nil {
			hooks.OnRetry(a, err, nextDelay)
		}
	}

	_, err := executor.ExecuteWithHooks(ctx, fn, executorHooks, callback)
	return lastResp, err
}
