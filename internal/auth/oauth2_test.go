package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cannectors/runtime/pkg/connector"
)

func TestOAuth2Handler_ApplyAuth_Success(t *testing.T) {
	// Create mock OAuth2 token server
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Errorf("expected application/x-www-form-urlencoded, got %s", r.Header.Get("Content-Type"))
		}

		// Verify form data
		if err := r.ParseForm(); err != nil {
			t.Errorf("failed to parse form: %v", err)
		}
		if r.Form.Get("grant_type") != "client_credentials" {
			t.Errorf("expected client_credentials grant_type, got %s", r.Form.Get("grant_type"))
		}
		if r.Form.Get("client_id") != "test-client-id" {
			t.Errorf("expected test-client-id, got %s", r.Form.Get("client_id"))
		}
		if r.Form.Get("client_secret") != "test-client-secret" {
			t.Errorf("expected test-client-secret, got %s", r.Form.Get("client_secret"))
		}

		// Return token response
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "test-access-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer tokenServer.Close()

	config := &connector.AuthConfig{
		Type: "oauth2",
		Credentials: toJSON(t, map[string]string{
			"tokenUrl":     tokenServer.URL,
			"clientId":     "test-client-id",
			"clientSecret": "test-client-secret",
		}),
	}

	handler, err := NewHandler(config, nil)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "https://example.com/api", nil)
	err = handler.ApplyAuth(context.Background(), req)
	if err != nil {
		t.Fatalf("ApplyAuth failed: %v", err)
	}

	got := req.Header.Get("Authorization")
	want := "Bearer test-access-token"
	if got != want {
		t.Errorf("Authorization header = %q, want %q", got, want)
	}
}

func TestOAuth2Handler_TokenCaching(t *testing.T) {
	var tokenRequestCount int32

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&tokenRequestCount, 1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "cached-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer tokenServer.Close()

	config := &connector.AuthConfig{
		Type: "oauth2",
		Credentials: toJSON(t, map[string]string{
			"tokenUrl":     tokenServer.URL,
			"clientId":     "client",
			"clientSecret": "secret",
		}),
	}

	handler, err := NewHandler(config, nil)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	// Make multiple requests
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "https://example.com/api", nil)
		if err := handler.ApplyAuth(context.Background(), req); err != nil {
			t.Fatalf("ApplyAuth failed on iteration %d: %v", i, err)
		}
	}

	// Should only have made one token request (caching works)
	if count := atomic.LoadInt32(&tokenRequestCount); count != 1 {
		t.Errorf("expected 1 token request, got %d", count)
	}
}

func TestOAuth2Handler_TokenExpiry(t *testing.T) {
	var tokenRequestCount int32

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&tokenRequestCount, 1)
		w.Header().Set("Content-Type", "application/json")
		// Return token that expires immediately (expires_in: 1s - 60s buffer = 0 effective)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "token-" + string(rune(int('0')+int(count))),
			"token_type":   "Bearer",
			"expires_in":   1, // After 60s buffer subtraction, becomes 0 = expires immediately
		})
	}))
	defer tokenServer.Close()

	config := &connector.AuthConfig{
		Type: "oauth2",
		Credentials: toJSON(t, map[string]string{
			"tokenUrl":     tokenServer.URL,
			"clientId":     "client",
			"clientSecret": "secret",
		}),
	}

	handler, err := NewHandler(config, nil)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	// First request - gets token
	req1 := httptest.NewRequest(http.MethodGet, "https://example.com/api", nil)
	if err := handler.ApplyAuth(context.Background(), req1); err != nil {
		t.Fatalf("first ApplyAuth failed: %v", err)
	}

	// Token expires immediately (effective expiry is now), so wait just a moment
	time.Sleep(10 * time.Millisecond)

	// Second request should trigger new token request (token expired)
	req2 := httptest.NewRequest(http.MethodGet, "https://example.com/api", nil)
	if err := handler.ApplyAuth(context.Background(), req2); err != nil {
		t.Fatalf("second ApplyAuth failed: %v", err)
	}

	// Should have made two token requests (token expired)
	if count := atomic.LoadInt32(&tokenRequestCount); count != 2 {
		t.Errorf("expected 2 token requests, got %d", count)
	}
}

func TestOAuth2Handler_Concurrent(t *testing.T) {
	var tokenRequestCount int32

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&tokenRequestCount, 1)
		// Simulate slow token endpoint
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "concurrent-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer tokenServer.Close()

	config := &connector.AuthConfig{
		Type: "oauth2",
		Credentials: toJSON(t, map[string]string{
			"tokenUrl":     tokenServer.URL,
			"clientId":     "client",
			"clientSecret": "secret",
		}),
	}

	handler, err := NewHandler(config, nil)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	// Launch 10 concurrent requests
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "https://example.com/api", nil)
			if err := handler.ApplyAuth(context.Background(), req); err != nil {
				t.Errorf("ApplyAuth failed: %v", err)
			}
		}()
	}
	wg.Wait()

	// Due to double-check locking, should have minimal token requests
	// (ideally 1, but could be a few if timing is unfortunate)
	if count := atomic.LoadInt32(&tokenRequestCount); count > 3 {
		t.Errorf("expected <=3 token requests for concurrent access, got %d", count)
	}
}

func TestOAuth2Handler_TokenRequestFailure(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error": "invalid_client"}`))
	}))
	defer tokenServer.Close()

	config := &connector.AuthConfig{
		Type: "oauth2",
		Credentials: toJSON(t, map[string]string{
			"tokenUrl":     tokenServer.URL,
			"clientId":     "bad-client",
			"clientSecret": "bad-secret",
		}),
	}

	handler, err := NewHandler(config, nil)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "https://example.com/api", nil)
	err = handler.ApplyAuth(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for failed token request")
	}

	// Error should mention status code
	if !containsString(err.Error(), "401") && !containsString(err.Error(), "status") {
		t.Errorf("error should mention status code: %v", err)
	}
}

func TestOAuth2Handler_EmptyToken(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer tokenServer.Close()

	config := &connector.AuthConfig{
		Type: "oauth2",
		Credentials: toJSON(t, map[string]string{
			"tokenUrl":     tokenServer.URL,
			"clientId":     "client",
			"clientSecret": "secret",
		}),
	}

	handler, err := NewHandler(config, nil)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "https://example.com/api", nil)
	err = handler.ApplyAuth(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for empty access token")
	}

	if !containsString(err.Error(), "empty access_token") {
		t.Errorf("error should mention empty token: %v", err)
	}
}

func TestOAuth2Handler_InvalidJSON(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{invalid json`))
	}))
	defer tokenServer.Close()

	config := &connector.AuthConfig{
		Type: "oauth2",
		Credentials: toJSON(t, map[string]string{
			"tokenUrl":     tokenServer.URL,
			"clientId":     "client",
			"clientSecret": "secret",
		}),
	}

	handler, err := NewHandler(config, nil)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "https://example.com/api", nil)
	err = handler.ApplyAuth(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for invalid JSON response")
	}

	if !containsString(err.Error(), "parsing") {
		t.Errorf("error should mention parsing: %v", err)
	}
}

func TestOAuth2Handler_WithScopes(t *testing.T) {
	var receivedScopes string

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("failed to parse form: %v", err)
		}
		receivedScopes = r.Form.Get("scope")

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "scoped-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer tokenServer.Close()

	config := &connector.AuthConfig{
		Type: "oauth2",
		Credentials: toJSON(t, map[string]interface{}{
			"tokenUrl":     tokenServer.URL,
			"clientId":     "client",
			"clientSecret": "secret",
			"scopes":       []string{"read", "write", "admin"},
		}),
	}

	handler, err := NewHandler(config, nil)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "https://example.com/api", nil)
	if err := handler.ApplyAuth(context.Background(), req); err != nil {
		t.Fatalf("ApplyAuth failed: %v", err)
	}

	// Scopes should be space-separated
	if receivedScopes != "read write admin" {
		t.Errorf("scopes = %q, want %q", receivedScopes, "read write admin")
	}
}

func TestOAuth2Handler_InvalidateToken(t *testing.T) {
	var tokenRequestCount int32

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&tokenRequestCount, 1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "token-" + string(rune(int('0')+int(count))),
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer tokenServer.Close()

	config := &connector.AuthConfig{
		Type: "oauth2",
		Credentials: toJSON(t, map[string]string{
			"tokenUrl":     tokenServer.URL,
			"clientId":     "client",
			"clientSecret": "secret",
		}),
	}

	handler, err := NewHandler(config, nil)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	oauth2Handler, ok := handler.(*oauth2Handler)
	if !ok {
		t.Fatal("expected handler to be *oauth2Handler")
	}

	// First request
	req1 := httptest.NewRequest(http.MethodGet, "https://example.com/api", nil)
	if err := handler.ApplyAuth(context.Background(), req1); err != nil {
		t.Fatalf("first ApplyAuth failed: %v", err)
	}

	// Invalidate token
	oauth2Handler.InvalidateToken()

	// Second request should get new token
	req2 := httptest.NewRequest(http.MethodGet, "https://example.com/api", nil)
	if err := handler.ApplyAuth(context.Background(), req2); err != nil {
		t.Fatalf("second ApplyAuth failed: %v", err)
	}

	if count := atomic.LoadInt32(&tokenRequestCount); count != 2 {
		t.Errorf("expected 2 token requests after invalidation, got %d", count)
	}
}

func TestOAuth2Handler_NetworkError(t *testing.T) {
	config := &connector.AuthConfig{
		Type: "oauth2",
		Credentials: toJSON(t, map[string]string{
			"tokenUrl":     "http://localhost:1", // Port 1 should not be listening
			"clientId":     "client",
			"clientSecret": "secret",
		}),
	}

	handler, err := NewHandler(config, &http.Client{Timeout: 100 * time.Millisecond})
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "https://example.com/api", nil)
	err = handler.ApplyAuth(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for network failure")
	}
}

func TestOAuth2Handler_DeterministicWithCache(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "deterministic-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer tokenServer.Close()

	config := &connector.AuthConfig{
		Type: "oauth2",
		Credentials: toJSON(t, map[string]string{
			"tokenUrl":     tokenServer.URL,
			"clientId":     "client",
			"clientSecret": "secret",
		}),
	}

	handler, err := NewHandler(config, nil)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	// Multiple requests should get the same token (deterministic)
	var tokens []string
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "https://example.com/api", nil)
		if err := handler.ApplyAuth(context.Background(), req); err != nil {
			t.Fatalf("ApplyAuth %d failed: %v", i, err)
		}
		tokens = append(tokens, req.Header.Get("Authorization"))
	}

	// All tokens should be identical
	for i := 1; i < len(tokens); i++ {
		if tokens[i] != tokens[0] {
			t.Errorf("token %d differs from token 0: %q vs %q", i, tokens[i], tokens[0])
		}
	}
}

func TestOAuth2Handler_InvalidateTokenOn401(t *testing.T) {
	var tokenRequestCount int32

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&tokenRequestCount, 1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": fmt.Sprintf("token-%d", count),
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer tokenServer.Close()

	config := &connector.AuthConfig{
		Type: "oauth2",
		Credentials: toJSON(t, map[string]string{
			"tokenUrl":     tokenServer.URL,
			"clientId":     "client",
			"clientSecret": "secret",
		}),
	}

	handler, err := NewHandler(config, nil)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	oauth2Handler, ok := handler.(*oauth2Handler)
	if !ok {
		t.Fatal("expected handler to be *oauth2Handler")
	}

	// First request - gets token-1
	req1 := httptest.NewRequest(http.MethodGet, "https://example.com/api", nil)
	if err := handler.ApplyAuth(context.Background(), req1); err != nil {
		t.Fatalf("first ApplyAuth failed: %v", err)
	}
	if req1.Header.Get("Authorization") != "Bearer token-1" {
		t.Errorf("expected Bearer token-1, got %q", req1.Header.Get("Authorization"))
	}

	// Invalidate token (simulating 401 response)
	oauth2Handler.InvalidateToken()

	// Second request should get new token (token-2)
	req2 := httptest.NewRequest(http.MethodGet, "https://example.com/api", nil)
	if err := handler.ApplyAuth(context.Background(), req2); err != nil {
		t.Fatalf("second ApplyAuth failed: %v", err)
	}
	if req2.Header.Get("Authorization") != "Bearer token-2" {
		t.Errorf("expected Bearer token-2 after invalidation, got %q", req2.Header.Get("Authorization"))
	}

	// Should have made two token requests
	if count := atomic.LoadInt32(&tokenRequestCount); count != 2 {
		t.Errorf("expected 2 token requests after invalidation, got %d", count)
	}
}
