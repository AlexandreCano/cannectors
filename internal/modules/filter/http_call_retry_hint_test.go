package filter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/cannectors/runtime/internal/moduleconfig"
	"github.com/cannectors/runtime/pkg/connector"
)

// TestHTTPCall_RetryHintFromBody locks Story 24.3 AC9: the http_call filter
// honors retry.retryHintFromBody (was previously silently ignored).
func TestHTTPCall_RetryHintFromBody(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if n < 3 {
			_, _ = w.Write([]byte(`{"status":"pending"}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"done","country":"US"}`))
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
			MaxAttempts:       5,
			DelayMs:           1,
			BackoffMultiplier: 1,
			MaxDelayMs:        5,
			RetryHintFromBody: `body.status == "pending"`,
		},
	}

	module, err := NewHTTPCallFromConfig(cfg)
	if err != nil {
		t.Fatalf("NewHTTPCallFromConfig: %v", err)
	}

	out, err := module.Process(context.Background(), []map[string]any{{"id": "42"}})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("expected 3 calls (2 retries + final), got %d", got)
	}
	if len(out) != 1 || out[0]["country"] != "US" {
		t.Errorf("expected enriched record with country=US, got %#v", out)
	}
}

// TestHTTPCall_InvalidRetryConfigRejected locks Story 24.3 retry validation:
// an invalid retry.maxAttempts is surfaced at construction time, not silently
// clamped at execution.
func TestHTTPCall_InvalidRetryConfigRejected(t *testing.T) {
	cfg := HTTPCallConfig{
		HTTPRequestBase: moduleconfig.HTTPRequestBase{Endpoint: "https://e.com"},
		Keys:            []moduleconfig.KeyConfig{{Field: "id", ParamType: "query", ParamName: "id"}},
		Retry:           &connector.RetryConfig{MaxAttempts: -1},
	}
	if _, err := NewHTTPCallFromConfig(cfg); err == nil {
		t.Fatal("expected error for negative MaxAttempts")
	}
}
