package filter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/canectors/runtime/pkg/connector"
)

func TestNewHTTPCallFromConfig(t *testing.T) {
	t.Run("creates module with valid config", func(t *testing.T) {
		config := HTTPCallConfig{
			Endpoint: "https://api.example.com/customers/{id}",
			Key: KeyConfig{
				Field:     "customerId",
				ParamType: "path",
				ParamName: "id",
			},
		}

		module, err := NewHTTPCallFromConfig(config)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if module == nil {
			t.Fatal("expected module, got nil")
		}
		if module.endpoint != config.Endpoint {
			t.Errorf("expected endpoint %s, got %s", config.Endpoint, module.endpoint)
		}
		if module.keyField != "customerId" {
			t.Errorf("expected keyField customerId, got %s", module.keyField)
		}
		if module.keyParamType != "path" {
			t.Errorf("expected keyParamType path, got %s", module.keyParamType)
		}
		if module.mergeStrategy != "merge" {
			t.Errorf("expected default mergeStrategy merge, got %s", module.mergeStrategy)
		}
		if module.onError != "fail" {
			t.Errorf("expected default onError fail, got %s", module.onError)
		}
	})

	t.Run("returns error for missing endpoint", func(t *testing.T) {
		config := HTTPCallConfig{
			Key: KeyConfig{
				Field:     "customerId",
				ParamType: "path",
				ParamName: "id",
			},
		}

		_, err := NewHTTPCallFromConfig(config)
		if err == nil {
			t.Error("expected error for missing endpoint")
		}
	})

	t.Run("returns error for missing key field", func(t *testing.T) {
		config := HTTPCallConfig{
			Endpoint: "https://api.example.com/customers",
			Key: KeyConfig{
				ParamType: "query",
				ParamName: "id",
			},
		}

		_, err := NewHTTPCallFromConfig(config)
		if err == nil {
			t.Error("expected error for missing key field")
		}
	})

	t.Run("returns error for invalid key paramType", func(t *testing.T) {
		config := HTTPCallConfig{
			Endpoint: "https://api.example.com/customers",
			Key: KeyConfig{
				Field:     "customerId",
				ParamType: "invalid",
				ParamName: "id",
			},
		}

		_, err := NewHTTPCallFromConfig(config)
		if err == nil {
			t.Error("expected error for invalid key paramType")
		}
	})

	t.Run("accepts valid paramTypes", func(t *testing.T) {
		for _, paramType := range []string{"query", "path", "header"} {
			config := HTTPCallConfig{
				Endpoint: "https://api.example.com/customers",
				Key: KeyConfig{
					Field:     "customerId",
					ParamType: paramType,
					ParamName: "id",
				},
			}

			module, err := NewHTTPCallFromConfig(config)
			if err != nil {
				t.Errorf("expected no error for paramType %s, got %v", paramType, err)
			}
			if module.keyParamType != paramType {
				t.Errorf("expected keyParamType %s, got %s", paramType, module.keyParamType)
			}
		}
	})

	t.Run("uses custom cache configuration", func(t *testing.T) {
		config := HTTPCallConfig{
			Endpoint: "https://api.example.com/customers",
			Key: KeyConfig{
				Field:     "customerId",
				ParamType: "query",
				ParamName: "id",
			},
			Cache: CacheConfig{
				MaxSize:    500,
				DefaultTTL: 60,
			},
		}

		module, err := NewHTTPCallFromConfig(config)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if module.cacheTTL != 60*time.Second {
			t.Errorf("expected cacheTTL 60s, got %v", module.cacheTTL)
		}
	})

	t.Run("creates module with authentication", func(t *testing.T) {
		config := HTTPCallConfig{
			Endpoint: "https://api.example.com/customers",
			Key: KeyConfig{
				Field:     "customerId",
				ParamType: "query",
				ParamName: "id",
			},
			Auth: &connector.AuthConfig{
				Type:        "bearer",
				Credentials: map[string]string{"token": "test-token"},
			},
		}

		module, err := NewHTTPCallFromConfig(config)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if module.authHandler == nil {
			t.Error("expected auth handler to be configured")
		}
	})
}

func TestParseHTTPCallConfig(t *testing.T) {
	t.Run("parses complete config", func(t *testing.T) {
		raw := map[string]interface{}{
			"endpoint": "https://api.example.com/customers/{id}",
			"key": map[string]interface{}{
				"field":     "customerId",
				"paramType": "path",
				"paramName": "id",
			},
			"cache": map[string]interface{}{
				"maxSize":    float64(500),
				"defaultTTL": float64(60),
			},
			"mergeStrategy": "replace",
			"dataField":     "data",
			"onError":       "skip",
			"timeoutMs":     float64(5000),
			"headers": map[string]interface{}{
				"X-Custom": "value",
			},
		}

		config, err := ParseHTTPCallConfig(raw, nil)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if config.Endpoint != "https://api.example.com/customers/{id}" {
			t.Errorf("unexpected endpoint: %s", config.Endpoint)
		}
		if config.Key.Field != "customerId" {
			t.Errorf("unexpected key field: %s", config.Key.Field)
		}
		if config.Key.ParamType != "path" {
			t.Errorf("unexpected key paramType: %s", config.Key.ParamType)
		}
		if config.Cache.MaxSize != 500 {
			t.Errorf("unexpected cache maxSize: %d", config.Cache.MaxSize)
		}
		if config.MergeStrategy != "replace" {
			t.Errorf("unexpected mergeStrategy: %s", config.MergeStrategy)
		}
		if config.OnError != "skip" {
			t.Errorf("unexpected onError: %s", config.OnError)
		}
		if config.Headers["X-Custom"] != "value" {
			t.Errorf("unexpected header: %v", config.Headers)
		}
	})

	t.Run("parses auth config from separate parameter", func(t *testing.T) {
		raw := map[string]interface{}{
			"endpoint": "https://api.example.com/customers",
			"key": map[string]interface{}{
				"field":     "id",
				"paramType": "query",
				"paramName": "id",
			},
		}

		authConfig := &connector.AuthConfig{
			Type: "api-key",
			Credentials: map[string]string{
				"key":        "secret",
				"headerName": "X-API-Key",
			},
		}

		config, err := ParseHTTPCallConfig(raw, authConfig)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if config.Auth == nil {
			t.Fatal("expected auth config")
		}
		if config.Auth.Type != "api-key" {
			t.Errorf("unexpected auth type: %s", config.Auth.Type)
		}
		if config.Auth.Credentials["key"] != "secret" {
			t.Errorf("unexpected auth key: %s", config.Auth.Credentials["key"])
		}
	})

	t.Run("ignores auth in config map when separate auth parameter provided", func(t *testing.T) {
		// This test verifies that when authentication is provided via cfg.Authentication
		// (as done by the registry), any "auth" field in cfg.Config is ignored.
		// This matches the behavior of input/output modules.
		raw := map[string]interface{}{
			"endpoint": "https://api.example.com/customers",
			"key": map[string]interface{}{
				"field":     "id",
				"paramType": "query",
				"paramName": "id",
			},
			"auth": map[string]interface{}{
				"type": "ignored",
			},
		}

		authConfig := &connector.AuthConfig{
			Type: "bearer",
			Credentials: map[string]string{
				"token": "correct-token",
			},
		}

		config, err := ParseHTTPCallConfig(raw, authConfig)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Should use the separate auth parameter, not the one in the map
		if config.Auth == nil {
			t.Fatal("expected auth config")
		}
		if config.Auth.Type != "bearer" {
			t.Errorf("expected auth type 'bearer' from separate parameter, got '%s'", config.Auth.Type)
		}
		if config.Auth.Credentials["token"] != "correct-token" {
			t.Errorf("expected token 'correct-token' from separate parameter, got '%s'", config.Auth.Credentials["token"])
		}
	})
}

func TestHTTPCallModule_Process(t *testing.T) {
	t.Run("enriches records with API data", func(t *testing.T) {
		// Create mock server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.URL.Query().Get("id")
			response := map[string]interface{}{
				"customerId": id,
				"name":       "Customer " + id,
				"email":      id + "@example.com",
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(response); err != nil {
				t.Errorf("failed to encode response: %v", err)
			}
		}))
		defer server.Close()

		config := HTTPCallConfig{
			Endpoint: server.URL,
			Key: KeyConfig{
				Field:     "customerId",
				ParamType: "query",
				ParamName: "id",
			},
		}

		module, err := NewHTTPCallFromConfig(config)
		if err != nil {
			t.Fatalf("failed to create module: %v", err)
		}

		records := []map[string]interface{}{
			{"customerId": "123", "amount": 100},
			{"customerId": "456", "amount": 200},
		}

		result, err := module.Process(context.Background(), records)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if len(result) != 2 {
			t.Fatalf("expected 2 records, got %d", len(result))
		}

		// Check first record was enriched
		if result[0]["name"] != "Customer 123" {
			t.Errorf("expected name 'Customer 123', got %v", result[0]["name"])
		}
		if result[0]["email"] != "123@example.com" {
			t.Errorf("expected email '123@example.com', got %v", result[0]["email"])
		}
		// Original field should be preserved
		if result[0]["amount"] != 100 {
			t.Errorf("expected amount 100, got %v", result[0]["amount"])
		}
	})

	t.Run("uses cache for duplicate keys", func(t *testing.T) {
		var requestCount int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&requestCount, 1)
			response := map[string]interface{}{
				"data": "enriched",
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(response); err != nil {
				t.Errorf("failed to encode response: %v", err)
			}
		}))
		defer server.Close()

		config := HTTPCallConfig{
			Endpoint: server.URL,
			Key: KeyConfig{
				Field:     "id",
				ParamType: "query",
				ParamName: "id",
			},
		}

		module, err := NewHTTPCallFromConfig(config)
		if err != nil {
			t.Fatalf("failed to create module: %v", err)
		}

		// Three records with same key
		records := []map[string]interface{}{
			{"id": "same-key"},
			{"id": "same-key"},
			{"id": "same-key"},
		}

		result, err := module.Process(context.Background(), records)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if len(result) != 3 {
			t.Fatalf("expected 3 records, got %d", len(result))
		}

		// Should only make one HTTP request (cache hits for the rest)
		if atomic.LoadInt32(&requestCount) != 1 {
			t.Errorf("expected 1 HTTP request, got %d", requestCount)
		}

		// Verify cache stats
		stats := module.GetCacheStats()
		if stats.Hits != 2 {
			t.Errorf("expected 2 cache hits, got %d", stats.Hits)
		}
		if stats.Misses != 1 {
			t.Errorf("expected 1 cache miss, got %d", stats.Misses)
		}
	})

	t.Run("path parameter substitution", func(t *testing.T) {
		var receivedPath string

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedPath = r.URL.Path
			response := map[string]interface{}{"status": "ok"}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(response); err != nil {
				t.Errorf("failed to encode response: %v", err)
			}
		}))
		defer server.Close()

		config := HTTPCallConfig{
			Endpoint: server.URL + "/customers/{id}/details",
			Key: KeyConfig{
				Field:     "customerId",
				ParamType: "path",
				ParamName: "id",
			},
		}

		module, err := NewHTTPCallFromConfig(config)
		if err != nil {
			t.Fatalf("failed to create module: %v", err)
		}

		records := []map[string]interface{}{
			{"customerId": "abc123"},
		}

		_, err = module.Process(context.Background(), records)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if receivedPath != "/customers/abc123/details" {
			t.Errorf("expected path /customers/abc123/details, got %s", receivedPath)
		}
	})

	t.Run("header parameter", func(t *testing.T) {
		var receivedHeader string

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedHeader = r.Header.Get("X-Customer-ID")
			response := map[string]interface{}{"status": "ok"}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(response); err != nil {
				t.Errorf("failed to encode response: %v", err)
			}
		}))
		defer server.Close()

		config := HTTPCallConfig{
			Endpoint: server.URL,
			Key: KeyConfig{
				Field:     "customerId",
				ParamType: "header",
				ParamName: "X-Customer-ID",
			},
		}

		module, err := NewHTTPCallFromConfig(config)
		if err != nil {
			t.Fatalf("failed to create module: %v", err)
		}

		records := []map[string]interface{}{
			{"customerId": "header-value-123"},
		}

		_, err = module.Process(context.Background(), records)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if receivedHeader != "header-value-123" {
			t.Errorf("expected header value 'header-value-123', got %s", receivedHeader)
		}
	})

	t.Run("extracts nested key field", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.URL.Query().Get("id")
			response := map[string]interface{}{"enriched": id}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(response); err != nil {
				t.Errorf("failed to encode response: %v", err)
			}
		}))
		defer server.Close()

		config := HTTPCallConfig{
			Endpoint: server.URL,
			Key: KeyConfig{
				Field:     "customer.id",
				ParamType: "query",
				ParamName: "id",
			},
		}

		module, err := NewHTTPCallFromConfig(config)
		if err != nil {
			t.Fatalf("failed to create module: %v", err)
		}

		records := []map[string]interface{}{
			{
				"customer": map[string]interface{}{
					"id":   "nested-123",
					"name": "Test",
				},
			},
		}

		result, err := module.Process(context.Background(), records)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if result[0]["enriched"] != "nested-123" {
			t.Errorf("expected enriched 'nested-123', got %v", result[0]["enriched"])
		}
	})

	t.Run("extracts dataField from response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := map[string]interface{}{
				"status": "success",
				"data": map[string]interface{}{
					"name":  "Extracted Data",
					"value": 42,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(response); err != nil {
				t.Errorf("failed to encode response: %v", err)
			}
		}))
		defer server.Close()

		config := HTTPCallConfig{
			Endpoint: server.URL,
			Key: KeyConfig{
				Field:     "id",
				ParamType: "query",
				ParamName: "id",
			},
			DataField: "data",
		}

		module, err := NewHTTPCallFromConfig(config)
		if err != nil {
			t.Fatalf("failed to create module: %v", err)
		}

		records := []map[string]interface{}{
			{"id": "123"},
		}

		result, err := module.Process(context.Background(), records)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if result[0]["name"] != "Extracted Data" {
			t.Errorf("expected name 'Extracted Data', got %v", result[0]["name"])
		}
		// Should not include top-level "status" field
		if _, ok := result[0]["status"]; ok {
			t.Error("should not include top-level status field")
		}
	})

	t.Run("merge strategy: replace", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := map[string]interface{}{
				"newField": "added",
				"id":       "overwritten",
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(response); err != nil {
				t.Errorf("failed to encode response: %v", err)
			}
		}))
		defer server.Close()

		config := HTTPCallConfig{
			Endpoint: server.URL,
			Key: KeyConfig{
				Field:     "id",
				ParamType: "query",
				ParamName: "id",
			},
			MergeStrategy: "replace",
		}

		module, err := NewHTTPCallFromConfig(config)
		if err != nil {
			t.Fatalf("failed to create module: %v", err)
		}

		records := []map[string]interface{}{
			{"id": "original", "existing": "kept"},
		}

		result, err := module.Process(context.Background(), records)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// id should be overwritten
		if result[0]["id"] != "overwritten" {
			t.Errorf("expected id 'overwritten', got %v", result[0]["id"])
		}
		// newField should be added
		if result[0]["newField"] != "added" {
			t.Errorf("expected newField 'added', got %v", result[0]["newField"])
		}
		// existing should be preserved
		if result[0]["existing"] != "kept" {
			t.Errorf("expected existing 'kept', got %v", result[0]["existing"])
		}
	})

	t.Run("merge strategy: append", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := map[string]interface{}{
				"enrichedData": "value",
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(response); err != nil {
				t.Errorf("failed to encode response: %v", err)
			}
		}))
		defer server.Close()

		config := HTTPCallConfig{
			Endpoint: server.URL,
			Key: KeyConfig{
				Field:     "id",
				ParamType: "query",
				ParamName: "id",
			},
			MergeStrategy: "append",
		}

		module, err := NewHTTPCallFromConfig(config)
		if err != nil {
			t.Fatalf("failed to create module: %v", err)
		}

		records := []map[string]interface{}{
			{"id": "123", "existing": "value"},
		}

		result, err := module.Process(context.Background(), records)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Enrichment data should be under _response key
		enrichment, ok := result[0]["_response"].(map[string]interface{})
		if !ok {
			t.Fatal("expected _response to be a map")
		}
		if enrichment["enrichedData"] != "value" {
			t.Errorf("expected enrichedData 'value', got %v", enrichment["enrichedData"])
		}
		// Original fields should be preserved
		if result[0]["id"] != "123" {
			t.Errorf("expected id '123', got %v", result[0]["id"])
		}
	})

	t.Run("handles nil records", func(t *testing.T) {
		config := HTTPCallConfig{
			Endpoint: "https://api.example.com",
			Key: KeyConfig{
				Field:     "id",
				ParamType: "query",
				ParamName: "id",
			},
		}

		module, err := NewHTTPCallFromConfig(config)
		if err != nil {
			t.Fatalf("failed to create module: %v", err)
		}

		result, err := module.Process(context.Background(), nil)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(result) != 0 {
			t.Errorf("expected empty result, got %d records", len(result))
		}
	})

	t.Run("handles context cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		config := HTTPCallConfig{
			Endpoint: server.URL,
			Key: KeyConfig{
				Field:     "id",
				ParamType: "query",
				ParamName: "id",
			},
		}

		module, err := NewHTTPCallFromConfig(config)
		if err != nil {
			t.Fatalf("failed to create module: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		records := []map[string]interface{}{
			{"id": "123"},
		}

		_, err = module.Process(ctx, records)
		if err == nil {
			t.Error("expected error for canceled context")
		}
	})
}

func TestHTTPCallModule_ErrorHandling(t *testing.T) {
	t.Run("onError fail: stops on HTTP error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		config := HTTPCallConfig{
			Endpoint: server.URL,
			Key: KeyConfig{
				Field:     "id",
				ParamType: "query",
				ParamName: "id",
			},
			OnError: "fail",
		}

		module, err := NewHTTPCallFromConfig(config)
		if err != nil {
			t.Fatalf("failed to create module: %v", err)
		}

		records := []map[string]interface{}{
			{"id": "123"},
			{"id": "456"},
		}

		_, err = module.Process(context.Background(), records)
		if err == nil {
			t.Error("expected error for HTTP 500")
		}
	})

	t.Run("onError skip: skips failed records", func(t *testing.T) {
		var requestCount int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count := atomic.AddInt32(&requestCount, 1)
			if count == 1 {
				// First request fails
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			// Subsequent requests succeed
			response := map[string]interface{}{"status": "ok"}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(response); err != nil {
				t.Errorf("failed to encode response: %v", err)
			}
		}))
		defer server.Close()

		config := HTTPCallConfig{
			Endpoint: server.URL,
			Key: KeyConfig{
				Field:     "id",
				ParamType: "query",
				ParamName: "id",
			},
			OnError: "skip",
		}

		module, err := NewHTTPCallFromConfig(config)
		if err != nil {
			t.Fatalf("failed to create module: %v", err)
		}

		records := []map[string]interface{}{
			{"id": "fail"},
			{"id": "success"},
		}

		result, err := module.Process(context.Background(), records)
		if err != nil {
			t.Fatalf("expected no error with onError=skip, got %v", err)
		}

		// Only second record should be in result
		if len(result) != 1 {
			t.Errorf("expected 1 record (skipped first), got %d", len(result))
		}
	})

	t.Run("onError log: includes original record on failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		config := HTTPCallConfig{
			Endpoint: server.URL,
			Key: KeyConfig{
				Field:     "id",
				ParamType: "query",
				ParamName: "id",
			},
			OnError: "log",
		}

		module, err := NewHTTPCallFromConfig(config)
		if err != nil {
			t.Fatalf("failed to create module: %v", err)
		}

		records := []map[string]interface{}{
			{"id": "123", "original": "data"},
		}

		result, err := module.Process(context.Background(), records)
		if err != nil {
			t.Fatalf("expected no error with onError=log, got %v", err)
		}

		// Should include original record
		if len(result) != 1 {
			t.Fatalf("expected 1 record, got %d", len(result))
		}
		if result[0]["original"] != "data" {
			t.Errorf("expected original data to be preserved")
		}
	})

	t.Run("handles missing key field", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := map[string]interface{}{"status": "ok"}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(response); err != nil {
				t.Errorf("failed to encode response: %v", err)
			}
		}))
		defer server.Close()

		config := HTTPCallConfig{
			Endpoint: server.URL,
			Key: KeyConfig{
				Field:     "customerId",
				ParamType: "query",
				ParamName: "id",
			},
			OnError: "fail",
		}

		module, err := NewHTTPCallFromConfig(config)
		if err != nil {
			t.Fatalf("failed to create module: %v", err)
		}

		records := []map[string]interface{}{
			{"id": "123"}, // Missing customerId
		}

		_, err = module.Process(context.Background(), records)
		if err == nil {
			t.Error("expected error for missing key field")
		}

		enrichErr, ok := err.(*HTTPCallError)
		if !ok {
			t.Fatalf("expected HTTPCallError, got %T", err)
		}
		if enrichErr.Code != ErrCodeHTTPCallKeyExtract {
			t.Errorf("expected error code %s, got %s", ErrCodeHTTPCallKeyExtract, enrichErr.Code)
		}
	})

	t.Run("handles null key value", func(t *testing.T) {
		config := HTTPCallConfig{
			Endpoint: "https://api.example.com",
			Key: KeyConfig{
				Field:     "id",
				ParamType: "query",
				ParamName: "id",
			},
			OnError: "fail",
		}

		module, err := NewHTTPCallFromConfig(config)
		if err != nil {
			t.Fatalf("failed to create module: %v", err)
		}

		records := []map[string]interface{}{
			{"id": nil},
		}

		_, err = module.Process(context.Background(), records)
		if err == nil {
			t.Error("expected error for null key value")
		}
	})

	t.Run("handles invalid JSON response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("not valid json"))
		}))
		defer server.Close()

		config := HTTPCallConfig{
			Endpoint: server.URL,
			Key: KeyConfig{
				Field:     "id",
				ParamType: "query",
				ParamName: "id",
			},
			OnError: "fail",
		}

		module, err := NewHTTPCallFromConfig(config)
		if err != nil {
			t.Fatalf("failed to create module: %v", err)
		}

		records := []map[string]interface{}{
			{"id": "123"},
		}

		_, err = module.Process(context.Background(), records)
		if err == nil {
			t.Error("expected error for invalid JSON")
		}

		enrichErr, ok := err.(*HTTPCallError)
		if !ok {
			t.Fatalf("expected HTTPCallError, got %T", err)
		}
		if enrichErr.Code != ErrCodeHTTPCallJSONParse {
			t.Errorf("expected error code %s, got %s", ErrCodeHTTPCallJSONParse, enrichErr.Code)
		}
	})
}

func TestHTTPCallModule_CacheIsolation(t *testing.T) {
	t.Run("separate modules have isolated caches", func(t *testing.T) {
		var module1Requests, module2Requests int32

		server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&module1Requests, 1)
			response := map[string]interface{}{"source": "server1"}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(response); err != nil {
				t.Errorf("failed to encode response: %v", err)
			}
		}))
		defer server1.Close()

		server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&module2Requests, 1)
			response := map[string]interface{}{"source": "server2"}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(response); err != nil {
				t.Errorf("failed to encode response: %v", err)
			}
		}))
		defer server2.Close()

		config1 := HTTPCallConfig{
			Endpoint: server1.URL,
			Key: KeyConfig{
				Field:     "id",
				ParamType: "query",
				ParamName: "id",
			},
		}
		config2 := HTTPCallConfig{
			Endpoint: server2.URL,
			Key: KeyConfig{
				Field:     "id",
				ParamType: "query",
				ParamName: "id",
			},
		}

		module1, _ := NewHTTPCallFromConfig(config1)
		module2, _ := NewHTTPCallFromConfig(config2)

		records := []map[string]interface{}{
			{"id": "same-key"},
		}

		// Process with module1
		result1, _ := module1.Process(context.Background(), records)

		// Process with module2 - should NOT use module1's cache
		result2, _ := module2.Process(context.Background(), records)

		// Both should have made requests
		if atomic.LoadInt32(&module1Requests) != 1 {
			t.Errorf("expected 1 request to server1, got %d", module1Requests)
		}
		if atomic.LoadInt32(&module2Requests) != 1 {
			t.Errorf("expected 1 request to server2, got %d", module2Requests)
		}

		// Results should be different
		if result1[0]["source"] == result2[0]["source"] {
			t.Error("modules should have different results from different servers")
		}
	})
}

func TestHTTPCallModule_DeepMerge(t *testing.T) {
	t.Run("deep merges nested objects", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := map[string]interface{}{
				"profile": map[string]interface{}{
					"email":   "enriched@example.com",
					"phone":   "123-456-7890",
					"address": map[string]interface{}{"city": "New York"},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(response); err != nil {
				t.Errorf("failed to encode response: %v", err)
			}
		}))
		defer server.Close()

		config := HTTPCallConfig{
			Endpoint: server.URL,
			Key: KeyConfig{
				Field:     "id",
				ParamType: "query",
				ParamName: "id",
			},
			MergeStrategy: "merge",
		}

		module, _ := NewHTTPCallFromConfig(config)

		records := []map[string]interface{}{
			{
				"id": "123",
				"profile": map[string]interface{}{
					"name":  "John Doe",
					"email": "original@example.com",
				},
			},
		}

		result, err := module.Process(context.Background(), records)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		profile, ok := result[0]["profile"].(map[string]interface{})
		if !ok {
			t.Fatal("expected profile to be a map")
		}

		// Original field should be preserved
		if profile["name"] != "John Doe" {
			t.Errorf("expected name 'John Doe', got %v", profile["name"])
		}
		// Enriched field should override
		if profile["email"] != "enriched@example.com" {
			t.Errorf("expected email 'enriched@example.com', got %v", profile["email"])
		}
		// New field should be added
		if profile["phone"] != "123-456-7890" {
			t.Errorf("expected phone '123-456-7890', got %v", profile["phone"])
		}
		// Nested object should be added
		address, ok := profile["address"].(map[string]interface{})
		if !ok || address["city"] != "New York" {
			t.Errorf("expected address.city 'New York', got %v", profile["address"])
		}
	})
}

func TestHTTPCallModule_NumericKeyValues(t *testing.T) {
	t.Run("handles numeric key values", func(t *testing.T) {
		var receivedID string

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedID = r.URL.Query().Get("id")
			response := map[string]interface{}{"status": "ok"}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(response); err != nil {
				t.Errorf("failed to encode response: %v", err)
			}
		}))
		defer server.Close()

		config := HTTPCallConfig{
			Endpoint: server.URL,
			Key: KeyConfig{
				Field:     "id",
				ParamType: "query",
				ParamName: "id",
			},
		}

		module, _ := NewHTTPCallFromConfig(config)

		// Test with float64 (how JSON numbers are decoded)
		records := []map[string]interface{}{
			{"id": float64(12345)},
		}

		_, err := module.Process(context.Background(), records)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Should convert to integer string without decimal
		if receivedID != "12345" {
			t.Errorf("expected id '12345', got '%s'", receivedID)
		}
	})
}

func TestHTTPCallModule_ClearCache(t *testing.T) {
	t.Run("ClearCache removes all entries", func(t *testing.T) {
		var requestCount int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&requestCount, 1)
			response := map[string]interface{}{"status": "ok"}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(response); err != nil {
				t.Errorf("failed to encode response: %v", err)
			}
		}))
		defer server.Close()

		config := HTTPCallConfig{
			Endpoint: server.URL,
			Key: KeyConfig{
				Field:     "id",
				ParamType: "query",
				ParamName: "id",
			},
		}

		module, _ := NewHTTPCallFromConfig(config)

		records := []map[string]interface{}{{"id": "123"}}

		// First call
		_, _ = module.Process(context.Background(), records)
		// Second call - should use cache
		_, _ = module.Process(context.Background(), records)

		if atomic.LoadInt32(&requestCount) != 1 {
			t.Errorf("expected 1 request before clear, got %d", requestCount)
		}

		// Clear cache
		module.ClearCache()

		// Third call - should make new request
		_, _ = module.Process(context.Background(), records)

		if atomic.LoadInt32(&requestCount) != 2 {
			t.Errorf("expected 2 requests after clear, got %d", requestCount)
		}
	})
}

func TestHTTPCallModule_Authentication(t *testing.T) {
	t.Run("API key authentication in query parameter", func(t *testing.T) {
		var receivedAPIKey string

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedAPIKey = r.URL.Query().Get("api_key")
			response := map[string]interface{}{"enriched": "data"}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(response); err != nil {
				t.Errorf("failed to encode response: %v", err)
			}
		}))
		defer server.Close()

		config := HTTPCallConfig{
			Endpoint: server.URL,
			Key: KeyConfig{
				Field:     "id",
				ParamType: "query",
				ParamName: "id",
			},
			Auth: &connector.AuthConfig{
				Type: "api-key",
				Credentials: map[string]string{
					"key":       "my-secret-api-key",
					"location":  "query",
					"paramName": "api_key",
				},
			},
		}

		module, err := NewHTTPCallFromConfig(config)
		if err != nil {
			t.Fatalf("failed to create module: %v", err)
		}

		records := []map[string]interface{}{{"id": "123"}}
		_, err = module.Process(context.Background(), records)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if receivedAPIKey != "my-secret-api-key" {
			t.Errorf("expected api_key query param 'my-secret-api-key', got '%s'", receivedAPIKey)
		}
	})

	t.Run("Bearer token authentication", func(t *testing.T) {
		var receivedAuth string

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedAuth = r.Header.Get("Authorization")
			response := map[string]interface{}{"enriched": "data"}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(response); err != nil {
				t.Errorf("failed to encode response: %v", err)
			}
		}))
		defer server.Close()

		config := HTTPCallConfig{
			Endpoint: server.URL,
			Key: KeyConfig{
				Field:     "id",
				ParamType: "query",
				ParamName: "id",
			},
			Auth: &connector.AuthConfig{
				Type: "bearer",
				Credentials: map[string]string{
					"token": "my-bearer-token-123",
				},
			},
		}

		module, err := NewHTTPCallFromConfig(config)
		if err != nil {
			t.Fatalf("failed to create module: %v", err)
		}

		records := []map[string]interface{}{{"id": "123"}}
		_, err = module.Process(context.Background(), records)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		expectedAuth := "Bearer my-bearer-token-123"
		if receivedAuth != expectedAuth {
			t.Errorf("expected Authorization header '%s', got '%s'", expectedAuth, receivedAuth)
		}
	})

	t.Run("Basic authentication", func(t *testing.T) {
		var receivedUser, receivedPass string
		var authOK bool

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedUser, receivedPass, authOK = r.BasicAuth()
			response := map[string]interface{}{"enriched": "data"}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(response); err != nil {
				t.Errorf("failed to encode response: %v", err)
			}
		}))
		defer server.Close()

		config := HTTPCallConfig{
			Endpoint: server.URL,
			Key: KeyConfig{
				Field:     "id",
				ParamType: "query",
				ParamName: "id",
			},
			Auth: &connector.AuthConfig{
				Type: "basic",
				Credentials: map[string]string{
					"username": "testuser",
					"password": "testpass",
				},
			},
		}

		module, err := NewHTTPCallFromConfig(config)
		if err != nil {
			t.Fatalf("failed to create module: %v", err)
		}

		records := []map[string]interface{}{{"id": "123"}}
		_, err = module.Process(context.Background(), records)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if !authOK {
			t.Error("basic auth was not sent")
		}
		if receivedUser != "testuser" {
			t.Errorf("expected username 'testuser', got '%s'", receivedUser)
		}
		if receivedPass != "testpass" {
			t.Errorf("expected password 'testpass', got '%s'", receivedPass)
		}
	})

	t.Run("OAuth2 client credentials flow", func(t *testing.T) {
		// Token server
		tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("token endpoint: expected POST, got %s", r.Method)
			}
			if err := r.ParseForm(); err != nil {
				t.Errorf("token endpoint: failed to parse form: %v", err)
			}
			if r.FormValue("grant_type") != "client_credentials" {
				t.Errorf("token endpoint: expected grant_type 'client_credentials', got '%s'", r.FormValue("grant_type"))
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": "test-oauth2-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			}); err != nil {
				t.Errorf("failed to encode token response: %v", err)
			}
		}))
		defer tokenServer.Close()

		var receivedAuth string
		// API server
		apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedAuth = r.Header.Get("Authorization")
			response := map[string]interface{}{"enriched": "data"}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(response); err != nil {
				t.Errorf("failed to encode response: %v", err)
			}
		}))
		defer apiServer.Close()

		config := HTTPCallConfig{
			Endpoint: apiServer.URL,
			Key: KeyConfig{
				Field:     "id",
				ParamType: "query",
				ParamName: "id",
			},
			Auth: &connector.AuthConfig{
				Type: "oauth2",
				Credentials: map[string]string{
					"clientId":     "test-client-id",
					"clientSecret": "test-client-secret",
					"tokenUrl":     tokenServer.URL,
				},
			},
		}

		module, err := NewHTTPCallFromConfig(config)
		if err != nil {
			t.Fatalf("failed to create module: %v", err)
		}

		records := []map[string]interface{}{{"id": "123"}}
		_, err = module.Process(context.Background(), records)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		expectedAuth := "Bearer test-oauth2-token"
		if receivedAuth != expectedAuth {
			t.Errorf("expected Authorization header '%s', got '%s'", expectedAuth, receivedAuth)
		}
	})
}

func TestHTTPCallModule_CacheTTLExpiration(t *testing.T) {
	t.Run("cache entries expire according to TTL", func(t *testing.T) {
		var requestCount int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&requestCount, 1)
			response := map[string]interface{}{"enriched": "data"}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(response); err != nil {
				t.Errorf("failed to encode response: %v", err)
			}
		}))
		defer server.Close()

		config := HTTPCallConfig{
			Endpoint: server.URL,
			Key: KeyConfig{
				Field:     "id",
				ParamType: "query",
				ParamName: "id",
			},
			Cache: CacheConfig{
				MaxSize:    100,
				DefaultTTL: 1, // 1 second TTL for testing
			},
		}

		module, err := NewHTTPCallFromConfig(config)
		if err != nil {
			t.Fatalf("failed to create module: %v", err)
		}

		records := []map[string]interface{}{{"id": "123"}}

		// First call - should make HTTP request
		_, err = module.Process(context.Background(), records)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if atomic.LoadInt32(&requestCount) != 1 {
			t.Errorf("expected 1 request, got %d", requestCount)
		}

		// Second call immediately - should use cache
		_, err = module.Process(context.Background(), records)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if atomic.LoadInt32(&requestCount) != 1 {
			t.Errorf("expected 1 request (cache hit), got %d", requestCount)
		}

		// Wait for TTL expiration
		time.Sleep(1100 * time.Millisecond)

		// Third call after expiration - should make new HTTP request
		_, err = module.Process(context.Background(), records)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if atomic.LoadInt32(&requestCount) != 2 {
			t.Errorf("expected 2 requests (cache expired), got %d", requestCount)
		}
	})
}

func TestHTTPCallModule_CacheSizeLimitAndLRU(t *testing.T) {
	t.Run("cache respects maxSize and evicts LRU entries", func(t *testing.T) {
		var requestCount int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&requestCount, 1)
			key := r.URL.Query().Get("id")
			response := map[string]interface{}{"enriched": key}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(response); err != nil {
				t.Errorf("failed to encode response: %v", err)
			}
		}))
		defer server.Close()

		config := HTTPCallConfig{
			Endpoint: server.URL,
			Key: KeyConfig{
				Field:     "id",
				ParamType: "query",
				ParamName: "id",
			},
			Cache: CacheConfig{
				MaxSize:    3, // Small cache size for testing
				DefaultTTL: 300,
			},
		}

		module, err := NewHTTPCallFromConfig(config)
		if err != nil {
			t.Fatalf("failed to create module: %v", err)
		}

		// Add 3 different keys - all should be cached
		records1 := []map[string]interface{}{{"id": "key1"}}
		records2 := []map[string]interface{}{{"id": "key2"}}
		records3 := []map[string]interface{}{{"id": "key3"}}

		_, _ = module.Process(context.Background(), records1)
		time.Sleep(5 * time.Millisecond)
		_, _ = module.Process(context.Background(), records2)
		time.Sleep(5 * time.Millisecond)
		_, _ = module.Process(context.Background(), records3)

		// Should have made 3 requests
		if atomic.LoadInt32(&requestCount) != 3 {
			t.Errorf("expected 3 requests, got %d", requestCount)
		}

		// Access key1 to make it most recently used
		_, _ = module.Process(context.Background(), records1)
		time.Sleep(5 * time.Millisecond)

		// Add 4th key - should evict key2 (LRU, not key1)
		records4 := []map[string]interface{}{{"id": "key4"}}
		_, _ = module.Process(context.Background(), records4)

		// Should have made 4 requests total
		if atomic.LoadInt32(&requestCount) != 4 {
			t.Errorf("expected 4 requests, got %d", requestCount)
		}

		// Verify key2 was evicted (should make new request)
		atomic.StoreInt32(&requestCount, 0)
		_, _ = module.Process(context.Background(), records2)

		if atomic.LoadInt32(&requestCount) != 1 {
			t.Errorf("expected 1 request for evicted key2, got %d", requestCount)
		}

		// Verify key1 still cached (no new request)
		atomic.StoreInt32(&requestCount, 0)
		_, _ = module.Process(context.Background(), records1)

		if atomic.LoadInt32(&requestCount) != 0 {
			t.Errorf("expected 0 requests for cached key1, got %d", requestCount)
		}
	})
}

func TestHTTPCallModule_CacheKeyCollision(t *testing.T) {
	t.Run("cache key delimiter prevents collisions", func(t *testing.T) {
		// Test that cache keys with "::" delimiter prevent collisions
		// when keyValue contains ":" or endpoint contains ":"
		var requestCount int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&requestCount, 1)
			key := r.URL.Query().Get("id")
			response := map[string]interface{}{"enriched": key}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(response); err != nil {
				t.Errorf("failed to encode response: %v", err)
			}
		}))
		defer server.Close()

		// Create module with endpoint that contains ":"
		config := HTTPCallConfig{
			Endpoint: server.URL, // Contains "http://" which has ":"
			Key: KeyConfig{
				Field:     "id",
				ParamType: "query",
				ParamName: "id",
			},
			Cache: CacheConfig{
				MaxSize:    10,
				DefaultTTL: 300,
			},
		}

		module1, err := NewHTTPCallFromConfig(config)
		if err != nil {
			t.Fatalf("failed to create module: %v", err)
		}

		// Test with keyValue that contains ":"
		records1 := []map[string]interface{}{{"id": "foo:bar"}}
		result1, err := module1.Process(context.Background(), records1)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if len(result1) != 1 {
			t.Fatalf("expected 1 record, got %d", len(result1))
		}

		// Verify cache key is unique (should make request)
		if atomic.LoadInt32(&requestCount) != 1 {
			t.Errorf("expected 1 request, got %d", requestCount)
		}

		// Second call with same key - should use cache
		_, err = module1.Process(context.Background(), records1)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if atomic.LoadInt32(&requestCount) != 1 {
			t.Errorf("expected 1 request (cache hit), got %d", requestCount)
		}

		// Test with different keyValue that would collide with old delimiter
		records2 := []map[string]interface{}{{"id": "different"}}
		_, err = module1.Process(context.Background(), records2)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		// Should make new request (different key)
		if atomic.LoadInt32(&requestCount) != 2 {
			t.Errorf("expected 2 requests (different key), got %d", requestCount)
		}

		// Verify first key still cached
		atomic.StoreInt32(&requestCount, 0)
		_, err = module1.Process(context.Background(), records1)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if atomic.LoadInt32(&requestCount) != 0 {
			t.Errorf("expected 0 requests (cache hit for first key), got %d", requestCount)
		}
	})
}

func TestHTTPCallModule_ConfigurableCacheKey(t *testing.T) {
	t.Run("static cache key", func(t *testing.T) {
		var requestCount int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&requestCount, 1)
			response := map[string]interface{}{"enriched": "data"}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(response); err != nil {
				t.Errorf("failed to encode response: %v", err)
			}
		}))
		defer server.Close()

		config := HTTPCallConfig{
			Endpoint: server.URL,
			Key: KeyConfig{
				Field:     "id",
				ParamType: "query",
				ParamName: "id",
			},
			Cache: CacheConfig{
				MaxSize:    10,
				DefaultTTL: 300,
				Key:        "static-cache-key",
			},
		}

		module, err := NewHTTPCallFromConfig(config)
		if err != nil {
			t.Fatalf("failed to create module: %v", err)
		}

		// Process records with different IDs - should all use same cache key
		records1 := []map[string]interface{}{{"id": "123"}}
		records2 := []map[string]interface{}{{"id": "456"}}

		_, err = module.Process(context.Background(), records1)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if atomic.LoadInt32(&requestCount) != 1 {
			t.Errorf("expected 1 request, got %d", requestCount)
		}

		// Second record with different ID should use same cache (static key)
		_, err = module.Process(context.Background(), records2)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if atomic.LoadInt32(&requestCount) != 1 {
			t.Errorf("expected 1 request (cache hit with static key), got %d", requestCount)
		}
	})

	t.Run("cache key from JSON path expression with $.", func(t *testing.T) {
		var requestCount int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&requestCount, 1)
			response := map[string]interface{}{"enriched": "data"}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(response); err != nil {
				t.Errorf("failed to encode response: %v", err)
			}
		}))
		defer server.Close()

		config := HTTPCallConfig{
			Endpoint: server.URL,
			Key: KeyConfig{
				Field:     "id",
				ParamType: "query",
				ParamName: "id",
			},
			Cache: CacheConfig{
				MaxSize:    10,
				DefaultTTL: 300,
				Key:        "$.customerId",
			},
		}

		module, err := NewHTTPCallFromConfig(config)
		if err != nil {
			t.Fatalf("failed to create module: %v", err)
		}

		// Process records with same customerId - should use cache
		records1 := []map[string]interface{}{
			{"id": "123", "customerId": "CUST-001"},
		}
		records2 := []map[string]interface{}{
			{"id": "456", "customerId": "CUST-001"}, // Same customerId
		}

		_, err = module.Process(context.Background(), records1)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if atomic.LoadInt32(&requestCount) != 1 {
			t.Errorf("expected 1 request, got %d", requestCount)
		}

		// Second record with same customerId should use cache
		_, err = module.Process(context.Background(), records2)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if atomic.LoadInt32(&requestCount) != 1 {
			t.Errorf("expected 1 request (cache hit with same customerId), got %d", requestCount)
		}

		// Third record with different customerId should make new request
		records3 := []map[string]interface{}{
			{"id": "789", "customerId": "CUST-002"},
		}
		_, err = module.Process(context.Background(), records3)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if atomic.LoadInt32(&requestCount) != 2 {
			t.Errorf("expected 2 requests (different customerId), got %d", requestCount)
		}
	})

	t.Run("cache key from dot notation path", func(t *testing.T) {
		var requestCount int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&requestCount, 1)
			response := map[string]interface{}{"enriched": "data"}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(response); err != nil {
				t.Errorf("failed to encode response: %v", err)
			}
		}))
		defer server.Close()

		config := HTTPCallConfig{
			Endpoint: server.URL,
			Key: KeyConfig{
				Field:     "id",
				ParamType: "query",
				ParamName: "id",
			},
			Cache: CacheConfig{
				MaxSize:    10,
				DefaultTTL: 300,
				Key:        "user.profile.id",
			},
		}

		module, err := NewHTTPCallFromConfig(config)
		if err != nil {
			t.Fatalf("failed to create module: %v", err)
		}

		records1 := []map[string]interface{}{
			{
				"id": "123",
				"user": map[string]interface{}{
					"profile": map[string]interface{}{
						"id": "PROFILE-001",
					},
				},
			},
		}
		records2 := []map[string]interface{}{
			{
				"id": "456",
				"user": map[string]interface{}{
					"profile": map[string]interface{}{
						"id": "PROFILE-001", // Same profile ID
					},
				},
			},
		}

		_, err = module.Process(context.Background(), records1)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if atomic.LoadInt32(&requestCount) != 1 {
			t.Errorf("expected 1 request, got %d", requestCount)
		}

		// Second record with same profile ID should use cache
		_, err = module.Process(context.Background(), records2)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if atomic.LoadInt32(&requestCount) != 1 {
			t.Errorf("expected 1 request (cache hit with same profile ID), got %d", requestCount)
		}
	})

	t.Run("cache key falls back to default when path not found", func(t *testing.T) {
		var requestCount int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&requestCount, 1)
			response := map[string]interface{}{"enriched": "data"}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(response); err != nil {
				t.Errorf("failed to encode response: %v", err)
			}
		}))
		defer server.Close()

		config := HTTPCallConfig{
			Endpoint: server.URL,
			Key: KeyConfig{
				Field:     "id",
				ParamType: "query",
				ParamName: "id",
			},
			Cache: CacheConfig{
				MaxSize:    10,
				DefaultTTL: 300,
				Key:        "$.nonexistent",
			},
		}

		module, err := NewHTTPCallFromConfig(config)
		if err != nil {
			t.Fatalf("failed to create module: %v", err)
		}

		records := []map[string]interface{}{
			{"id": "123"}, // Missing "nonexistent" field
		}

		_, err = module.Process(context.Background(), records)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		// Should make request (path not found, falls back to default key)
		if atomic.LoadInt32(&requestCount) != 1 {
			t.Errorf("expected 1 request, got %d", requestCount)
		}

		// Second call with same ID should use cache (default key behavior)
		_, err = module.Process(context.Background(), records)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if atomic.LoadInt32(&requestCount) != 1 {
			t.Errorf("expected 1 request (cache hit with default key), got %d", requestCount)
		}
	})
}
