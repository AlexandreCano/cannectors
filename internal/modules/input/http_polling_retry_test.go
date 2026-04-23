package input

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cannectors/runtime/pkg/connector"
)

// TestHTTPPolling_Fetch_HonorsRetryAfter verifies that Retry-After is
// honored on 429 responses (new capability introduced by Story 15.4).
func TestHTTPPolling_Fetch_HonorsRetryAfter(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 2 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":1}]`))
	}))
	defer srv.Close()

	cfg := &connector.ModuleConfig{
		Type: "http-polling",
		Raw: mustJSON(map[string]interface{}{
			"endpoint": srv.URL,
			"retry": map[string]interface{}{
				"maxAttempts":          float64(3),
				"delayMs":              float64(500),
				"backoffMultiplier":    float64(2),
				"maxDelayMs":           float64(5000),
				"useRetryAfterHeader":  true,
				"retryableStatusCodes": []interface{}{float64(429)},
			},
		}),
	}

	polling, err := NewHTTPPollingFromConfig(cfg)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig: %v", err)
	}

	start := time.Now()
	records, err := polling.Fetch(context.Background())
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	// Retry-After: 0 → immediate retry; must be fast (< 200ms)
	// despite baseline backoff of 500ms.
	if elapsed > 200*time.Millisecond {
		t.Errorf("expected Retry-After=0 to short-circuit backoff, elapsed=%v", elapsed)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Errorf("server called %d times, want 2", atomic.LoadInt32(&calls))
	}
}

// TestHTTPPolling_Fetch_RetryHintFromBody verifies that retryHintFromBody
// short-circuits retries when the body says fatal (new capability).
func TestHTTPPolling_Fetch_RetryHintFromBody(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"fatal": true}`))
	}))
	defer srv.Close()

	cfg := &connector.ModuleConfig{
		Type: "http-polling",
		Raw: mustJSON(map[string]interface{}{
			"endpoint": srv.URL,
			"retry": map[string]interface{}{
				"maxAttempts":          float64(5),
				"delayMs":              float64(1),
				"backoffMultiplier":    float64(1),
				"maxDelayMs":           float64(5),
				"retryableStatusCodes": []interface{}{float64(503)},
				"retryHintFromBody":    "body.fatal != true",
			},
		}),
	}

	polling, err := NewHTTPPollingFromConfig(cfg)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig: %v", err)
	}
	if _, err := polling.Fetch(context.Background()); err == nil {
		t.Fatal("expected error (hint=false forces no retry, server always 503)")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("server called %d times, want 1 (retry vetoed by body hint)", got)
	}
}
