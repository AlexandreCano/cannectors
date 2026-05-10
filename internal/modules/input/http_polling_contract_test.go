package input

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cannectors/runtime/pkg/connector"
)

// TestHTTPPolling_Fetch_DeleteWithBody locks Story 24.3 AC1/AC3: httpPolling
// supports any HTTP method and sends a configured body.
func TestHTTPPolling_Fetch_DeleteWithBody(t *testing.T) {
	var gotMethod, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{{"ok": true}})
	}))
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "httpPolling",
		Raw: mustJSON(map[string]any{
			"endpoint": server.URL,
			"method":   "DELETE",
			"body":     `{"reason":"cleanup"}`,
		}),
	}

	polling, err := NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig: %v", err)
	}
	if _, err := polling.Fetch(context.Background()); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("expected DELETE, got %s", gotMethod)
	}
	if !strings.Contains(gotBody, "cleanup") {
		t.Errorf("expected body to contain 'cleanup', got %q", gotBody)
	}
}

// TestHTTPPolling_InvalidMethod locks Story 24.3 AC1: malformed methods are
// rejected at construction time, not silently ignored.
func TestHTTPPolling_InvalidMethod(t *testing.T) {
	config := &connector.ModuleConfig{
		Type: "httpPolling",
		Raw: mustJSON(map[string]any{
			"endpoint": "https://e.com",
			"method":   "BAD METHOD",
		}),
	}
	if _, err := NewHTTPPollingFromConfig(config); err == nil {
		t.Fatal("expected error for invalid method")
	}
}

// TestHTTPPolling_InvalidHeader locks Story 24.3 AC6: invalid header names
// or values cause the request to fail rather than being silently dropped.
func TestHTTPPolling_InvalidHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	config := &connector.ModuleConfig{
		Type: "httpPolling",
		Raw: mustJSON(map[string]any{
			"endpoint": server.URL,
			"headers": map[string]any{
				"X-Bad Header": "value",
			},
		}),
	}
	polling, err := NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig: %v", err)
	}
	if _, err := polling.Fetch(context.Background()); err == nil {
		t.Fatal("expected error for invalid header, got nil")
	}
}

// TestHTTPPolling_InvalidURL locks Story 24.3 AC5: a final URL that is not a
// well-formed http(s) URL fails the request at runtime instead of being sent.
func TestHTTPPolling_InvalidURL(t *testing.T) {
	config := &connector.ModuleConfig{
		Type: "httpPolling",
		Raw: mustJSON(map[string]any{
			"endpoint": "ftp://example.com/data",
		}),
	}
	polling, err := NewHTTPPollingFromConfig(config)
	if err != nil {
		t.Fatalf("NewHTTPPollingFromConfig: %v", err)
	}
	if _, err := polling.Fetch(context.Background()); err == nil {
		t.Fatal("expected error for non-http(s) URL")
	}
}

// TestHTTPPolling_RejectsUnknownPaginationType locks Story 24.6 AC9: an unknown
// pagination type fails at module instantiation (not at Fetch time).
func TestHTTPPolling_RejectsUnknownPaginationType(t *testing.T) {
	config := &connector.ModuleConfig{
		Type: "httpPolling",
		Raw: mustJSON(map[string]any{
			"endpoint": "https://example.com",
			"pagination": map[string]any{
				"type":  "weird",
				"param": "x",
			},
		}),
	}
	if _, err := NewHTTPPollingFromConfig(config); err == nil {
		t.Fatal("expected error for unknown pagination type")
	} else if !strings.Contains(err.Error(), "unknown pagination.type") {
		t.Errorf("error = %v, want substring 'unknown pagination.type'", err)
	}
}
