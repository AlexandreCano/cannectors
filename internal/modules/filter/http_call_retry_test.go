package filter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cannectors/runtime/internal/moduleconfig"
	"github.com/cannectors/runtime/pkg/connector"
)

// TestHTTPCall_RetryTransient verifies that the filter retries transient
// 5xx errors when retry is configured (new capability in Story 15.5).
func TestHTTPCall_RetryTransient(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"country":"US"}`))
	}))
	defer srv.Close()

	cfg := HTTPCallConfig{
		HTTPRequestBase: moduleconfig.HTTPRequestBase{
			Endpoint:  srv.URL,
			TimeoutMs: 2000,
		},
		Keys: []moduleconfig.KeyConfig{
			{Field: "id", ParamType: "query", ParamName: "id"},
		},
		Retry: &connector.RetryConfig{
			MaxAttempts:          5,
			DelayMs:              1,
			BackoffMultiplier:    1,
			MaxDelayMs:           5,
			RetryableStatusCodes: []int{503},
		},
	}

	module, err := NewHTTPCallFromConfig(cfg)
	if err != nil {
		t.Fatalf("NewHTTPCallFromConfig: %v", err)
	}

	records := []map[string]interface{}{
		{"id": "42"},
	}
	out, err := module.Process(context.Background(), records)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 record, got %d", len(out))
	}
	if out[0]["country"] != "US" {
		t.Errorf("expected enriched country=US, got %v", out[0]["country"])
	}
	if atomic.LoadInt32(&calls) != 3 {
		t.Errorf("server called %d times, want 3", atomic.LoadInt32(&calls))
	}
}

// TestHTTPCall_RetryAfter429 verifies that a 429 with Retry-After is
// respected when useRetryAfterHeader is enabled.
func TestHTTPCall_RetryAfter429(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 2 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	cfg := HTTPCallConfig{
		HTTPRequestBase: moduleconfig.HTTPRequestBase{
			Endpoint:  srv.URL,
			TimeoutMs: 2000,
		},
		Keys: []moduleconfig.KeyConfig{
			{Field: "id", ParamType: "query", ParamName: "id"},
		},
		Retry: &connector.RetryConfig{
			MaxAttempts:          3,
			DelayMs:              500,
			BackoffMultiplier:    2,
			MaxDelayMs:           5000,
			UseRetryAfterHeader:  true,
			RetryableStatusCodes: []int{429},
		},
	}

	module, err := NewHTTPCallFromConfig(cfg)
	if err != nil {
		t.Fatalf("NewHTTPCallFromConfig: %v", err)
	}

	records := []map[string]interface{}{{"id": "1"}}
	start := time.Now()
	out, err := module.Process(context.Background(), records)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 record, got %d", len(out))
	}
	// Retry-After: 0 short-circuits 500ms baseline backoff.
	if elapsed > 200*time.Millisecond {
		t.Errorf("expected Retry-After=0 to short-circuit, elapsed=%v", elapsed)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Errorf("server called %d times, want 2", atomic.LoadInt32(&calls))
	}
}

// TestHTTPCall_NoRetryByDefault verifies backward-compat: when Retry is not
// configured, a single attempt is made (historical http_call behavior).
func TestHTTPCall_NoRetryByDefault(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	cfg := HTTPCallConfig{
		HTTPRequestBase: moduleconfig.HTTPRequestBase{
			Endpoint:  srv.URL,
			TimeoutMs: 2000,
		},
		Keys: []moduleconfig.KeyConfig{
			{Field: "id", ParamType: "query", ParamName: "id"},
		},
	}

	module, err := NewHTTPCallFromConfig(cfg)
	if err != nil {
		t.Fatalf("NewHTTPCallFromConfig: %v", err)
	}

	records := []map[string]interface{}{{"id": "1"}}
	if _, err := module.Process(context.Background(), records); err == nil {
		t.Fatal("expected error (500 from server)")
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Errorf("server called %d times, want 1 (no retry when Retry is nil)", atomic.LoadInt32(&calls))
	}
}
