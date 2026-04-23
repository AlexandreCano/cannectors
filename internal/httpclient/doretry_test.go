package httpclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cannectors/runtime/internal/errhandling"
	"github.com/cannectors/runtime/pkg/connector"
)

func drainResp(t *testing.T, resp *http.Response) {
	t.Helper()
	if resp == nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

func fastRetryConfig(maxAttempts int) errhandling.RetryConfig {
	return connector.RetryConfig{
		MaxAttempts:       maxAttempts,
		DelayMs:           1,
		BackoffMultiplier: 1,
		MaxDelayMs:        5,
	}
}

func TestDoWithRetry_SuccessFirstAttempt(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(2 * time.Second)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}

	resp, err := c.DoWithRetry(context.Background(), req, fastRetryConfig(3), RetryHooks{})
	defer drainResp(t, resp)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil || resp.StatusCode != http.StatusOK {
		t.Errorf("status = %v, want 200", resp)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("server called %d times, want 1", got)
	}
}

func TestDoWithRetry_RetriesOn5xxThenSucceeds(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(2 * time.Second)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}

	var hookInvocations int32
	hooks := RetryHooks{
		OnRetry: func(_ int, _ error, _ time.Duration) {
			atomic.AddInt32(&hookInvocations, 1)
		},
	}

	resp, err := c.DoWithRetry(context.Background(), req, fastRetryConfig(5), hooks)
	defer drainResp(t, resp)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("server called %d times, want 3", got)
	}
	if atomic.LoadInt32(&hookInvocations) == 0 {
		t.Error("OnRetry hook was never invoked")
	}
}

func TestDoWithRetry_StopsOn4xx(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	c := NewClient(2 * time.Second)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}

	resp, err := c.DoWithRetry(context.Background(), req, fastRetryConfig(5), RetryHooks{})
	defer drainResp(t, resp)

	if err == nil {
		t.Fatal("expected error for 4xx, got nil")
	}

	var httpErr *Error
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected *Error in chain, got %T", err)
	}
	if httpErr.StatusCode != http.StatusBadRequest {
		t.Errorf("StatusCode = %d, want 400", httpErr.StatusCode)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("server called %d times, want 1 (no retry on 4xx)", got)
	}
}

func TestDoWithRetry_ExhaustsRetries(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	c := NewClient(2 * time.Second)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}

	resp, err := c.DoWithRetry(context.Background(), req, fastRetryConfig(2), RetryHooks{})
	defer drainResp(t, resp)

	if err == nil {
		t.Fatal("expected exhaustion error, got nil")
	}
	if got := atomic.LoadInt32(&calls); got != 3 { // 1 initial + 2 retries
		t.Errorf("server called %d times, want 3", got)
	}
}

func TestDoWithRetry_HonorsRetryAfter(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 2 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := connector.RetryConfig{
		MaxAttempts:          3,
		DelayMs:              100,
		BackoffMultiplier:    2,
		MaxDelayMs:           5000,
		UseRetryAfterHeader:  true,
		RetryableStatusCodes: []int{429},
	}

	c := NewClient(2 * time.Second)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)

	var delays []time.Duration
	hooks := RetryHooks{
		OnRetry: func(_ int, err error, d time.Duration) {
			if err != nil && d >= 0 {
				delays = append(delays, d)
			}
		},
	}

	start := time.Now()
	resp, err := c.DoWithRetry(context.Background(), req, cfg, hooks)
	defer drainResp(t, resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Retry-After: 0 → immediate retry (delay near zero, not 100ms backoff)
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Errorf("expected near-immediate retry via Retry-After, elapsed=%v", elapsed)
	}
	if len(delays) == 0 {
		t.Fatal("OnRetry hook was never invoked")
	}
	if delays[0] > 50*time.Millisecond {
		t.Errorf("first retry delay = %v, want ~0 (Retry-After override)", delays[0])
	}
}

func TestDoWithRetry_ShouldRetryBodyForcesNoRetry(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"fatal": true}`))
	}))
	defer srv.Close()

	cfg := connector.RetryConfig{
		MaxAttempts:          5,
		DelayMs:              1,
		BackoffMultiplier:    1,
		MaxDelayMs:           5,
		RetryableStatusCodes: []int{503},
	}
	c := NewClient(2 * time.Second)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)

	hooks := RetryHooks{
		ShouldRetryBody: func(body []byte) (bool, bool) {
			// Body says "fatal", force no retry.
			return false, true
		},
	}
	resp, err := c.DoWithRetry(context.Background(), req, cfg, hooks)
	defer drainResp(t, resp)
	if err == nil {
		t.Fatal("expected error")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("server called %d times, want 1 (retry vetoed by body hint)", got)
	}
}

func TestDoWithRetry_OnAttemptFailureOverridesFatal(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 2 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := connector.RetryConfig{
		MaxAttempts:          3,
		DelayMs:              1,
		BackoffMultiplier:    1,
		MaxDelayMs:           5,
		RetryableStatusCodes: []int{},
	}
	c := NewClient(2 * time.Second)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)

	var overrideCalls int
	hooks := RetryHooks{
		OnAttemptFailure: func(_ int, resp *http.Response, _ error) bool {
			if resp.StatusCode == http.StatusUnauthorized {
				overrideCalls++
				return true
			}
			return false
		},
	}

	resp, err := c.DoWithRetry(context.Background(), req, cfg, hooks)
	defer drainResp(t, resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if overrideCalls == 0 {
		t.Error("OnAttemptFailure was not invoked")
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("server called %d times, want 2 (initial 401 + retry success)", got)
	}
}

func TestDoWithRetry_NilClient(t *testing.T) {
	var c *Client
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.invalid", nil)
	_, err := c.DoWithRetry(context.Background(), req, fastRetryConfig(1), RetryHooks{})
	if !errors.Is(err, ErrNilClient) {
		t.Errorf("err = %v, want ErrNilClient", err)
	}
}
