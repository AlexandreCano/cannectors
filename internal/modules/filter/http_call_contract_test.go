package filter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cannectors/runtime/internal/moduleconfig"
)

// TestHTTPCall_RejectsInvalidMergeStrategy locks Story 24.11 AC7.
func TestHTTPCall_RejectsInvalidMergeStrategy(t *testing.T) {
	_, err := NewHTTPCallFromConfig(HTTPCallConfig{
		HTTPRequestBase: moduleconfig.HTTPRequestBase{Endpoint: "https://e/x"},
		Keys: []moduleconfig.KeyConfig{{
			Field: "id", ParamType: "query", ParamName: "id",
		}},
		MergeStrategy: "concat",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid mergeStrategy") {
		t.Fatalf("expected invalid mergeStrategy error, got %v", err)
	}
}

// TestHTTPCall_CacheDisabledByDefault locks Story 24.11 AC4: when
// cache.enabled is absent or false, the cache must not be used regardless
// of records sharing the same key.
func TestHTTPCall_CacheDisabledByDefault(t *testing.T) {
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": "x"})
	}))
	defer server.Close()

	module, err := NewHTTPCallFromConfig(HTTPCallConfig{
		HTTPRequestBase: moduleconfig.HTTPRequestBase{Endpoint: server.URL},
		Keys: []moduleconfig.KeyConfig{{
			Field: "id", ParamType: "query", ParamName: "id",
		}},
		// Cache omitted → disabled.
	})
	if err != nil {
		t.Fatalf("NewHTTPCallFromConfig: %v", err)
	}
	records := []map[string]any{{"id": "k"}, {"id": "k"}, {"id": "k"}}
	if _, err := module.Process(context.Background(), records); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if got := atomic.LoadInt32(&requestCount); got != 3 {
		t.Fatalf("expected 3 HTTP requests (cache off), got %d", got)
	}
}

// TestHTTPCall_CacheEnabledAppliesDefaults locks Story 24.11 AC5.
func TestHTTPCall_CacheEnabledAppliesDefaults(t *testing.T) {
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": "x"})
	}))
	defer server.Close()

	module, err := NewHTTPCallFromConfig(HTTPCallConfig{
		HTTPRequestBase: moduleconfig.HTTPRequestBase{Endpoint: server.URL},
		Keys: []moduleconfig.KeyConfig{{
			Field: "id", ParamType: "query", ParamName: "id",
		}},
		Cache: moduleconfig.CacheConfig{Enabled: true}, // no maxSize/ttlSeconds
	})
	if err != nil {
		t.Fatalf("NewHTTPCallFromConfig: %v", err)
	}
	records := []map[string]any{{"id": "k"}, {"id": "k"}}
	if _, err := module.Process(context.Background(), records); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if got := atomic.LoadInt32(&requestCount); got != 1 {
		t.Fatalf("expected 1 HTTP request (cache on with defaults), got %d", got)
	}
}

// TestHTTPCall_KeyMissingFieldErrors locks Story 24.11 AC12.
func TestHTTPCall_KeyMissingFieldErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	defer server.Close()

	module, err := NewHTTPCallFromConfig(HTTPCallConfig{
		HTTPRequestBase: moduleconfig.HTTPRequestBase{Endpoint: server.URL},
		Keys: []moduleconfig.KeyConfig{{
			Field: "id", ParamType: "query", ParamName: "id",
		}},
	})
	if err != nil {
		t.Fatalf("NewHTTPCallFromConfig: %v", err)
	}
	cases := []struct {
		name   string
		record map[string]any
	}{
		{"absent", map[string]any{}},
		{"null", map[string]any{"id": nil}},
		{"empty", map[string]any{"id": ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := module.Process(context.Background(), []map[string]any{tc.record})
			if err == nil {
				t.Fatalf("expected error for %s key", tc.name)
			}
		})
	}
}

// TestHTTPCall_RejectsInvalidEndpointTemplate locks Story 24.11 AC9.
func TestHTTPCall_RejectsInvalidEndpointTemplate(t *testing.T) {
	_, err := NewHTTPCallFromConfig(HTTPCallConfig{
		HTTPRequestBase: moduleconfig.HTTPRequestBase{Endpoint: "https://e/{{record.id"},
		Keys: []moduleconfig.KeyConfig{{
			Field: "id", ParamType: "query", ParamName: "id",
		}},
	})
	if err == nil {
		t.Fatal("expected error for invalid endpoint template")
	}
}

// TestHTTPCall_RejectsInvalidCacheKeyTemplate locks Story 24.11 AC9
// (cache.key must also be validated as a template at construction).
func TestHTTPCall_RejectsInvalidCacheKeyTemplate(t *testing.T) {
	_, err := NewHTTPCallFromConfig(HTTPCallConfig{
		HTTPRequestBase: moduleconfig.HTTPRequestBase{Endpoint: "https://e/x"},
		Keys: []moduleconfig.KeyConfig{{
			Field: "id", ParamType: "query", ParamName: "id",
		}},
		Cache: moduleconfig.CacheConfig{Enabled: true, Key: "{{record.id"},
	})
	if err == nil || !strings.Contains(err.Error(), "cache key") {
		t.Fatalf("expected cache key template error, got %v", err)
	}
}

// TestHTTPCall_CacheKeyEvaluatedAsTemplate verifies cache.key supports
// {{record.X}} templating so distinct records produce distinct cache slots.
func TestHTTPCall_CacheKeyEvaluatedAsTemplate(t *testing.T) {
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": "x"})
	}))
	defer server.Close()

	module, err := NewHTTPCallFromConfig(HTTPCallConfig{
		HTTPRequestBase: moduleconfig.HTTPRequestBase{Endpoint: server.URL},
		Keys: []moduleconfig.KeyConfig{{
			Field: "id", ParamType: "query", ParamName: "id",
		}},
		Cache: moduleconfig.CacheConfig{Enabled: true, Key: "{{record.customerId}}"},
	})
	if err != nil {
		t.Fatalf("NewHTTPCallFromConfig: %v", err)
	}
	records := []map[string]any{
		{"id": "1", "customerId": "A"},
		{"id": "2", "customerId": "B"},
		{"id": "3", "customerId": "A"}, // hits cache from record 1
	}
	if _, err := module.Process(context.Background(), records); err != nil {
		t.Fatalf("Process: %v", err)
	}
	if got := atomic.LoadInt32(&requestCount); got != 2 {
		t.Fatalf("expected 2 HTTP requests (A cached on 3rd record), got %d", got)
	}
}

// TestHTTPCall_KeyEntriesValidatedEvenWithBody locks Story 24.11 AC11:
// invalid key entries must be rejected at construction even when a body is
// configured (which makes keys optional but not exempt from validation).
func TestHTTPCall_KeyEntriesValidatedEvenWithBody(t *testing.T) {
	cases := []struct {
		name string
		key  moduleconfig.KeyConfig
	}{
		{"invalid paramType", moduleconfig.KeyConfig{Field: "id", ParamType: "body", ParamName: "id"}},
		{"empty field", moduleconfig.KeyConfig{Field: "", ParamType: "query", ParamName: "id"}},
		{"empty paramName", moduleconfig.KeyConfig{Field: "id", ParamType: "query", ParamName: ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewHTTPCallFromConfig(HTTPCallConfig{
				HTTPRequestBase: moduleconfig.HTTPRequestBase{
					Endpoint: "https://e/x",
					Body:     `{"a":1}`,
				},
				Keys: []moduleconfig.KeyConfig{tc.key},
			})
			if err == nil {
				t.Fatalf("expected key validation error with body present (%s)", tc.name)
			}
		})
	}
}
