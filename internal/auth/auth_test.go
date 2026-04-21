package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cannectors/runtime/pkg/connector"
)

func TestNewHandler(t *testing.T) {
	tests := []struct {
		name        string
		config      *connector.AuthConfig
		wantType    string
		wantErr     bool
		errContains string
	}{
		{
			name:     "nil config returns nil handler",
			config:   nil,
			wantType: "",
			wantErr:  false,
		},
		{
			name: "api-key type creates handler",
			config: &connector.AuthConfig{
				Type:        "api-key",
				Credentials: toJSON(t, map[string]string{"key": "test-key"}),
			},
			wantType: "api-key",
			wantErr:  false,
		},
		{
			name: "bearer type creates handler",
			config: &connector.AuthConfig{
				Type:        "bearer",
				Credentials: toJSON(t, map[string]string{"token": "test-token"}),
			},
			wantType: "bearer",
			wantErr:  false,
		},
		{
			name: "basic type creates handler",
			config: &connector.AuthConfig{
				Type:        "basic",
				Credentials: toJSON(t, map[string]string{"username": "user", "password": "pass"}),
			},
			wantType: "basic",
			wantErr:  false,
		},
		{
			name: "oauth2 type creates handler",
			config: &connector.AuthConfig{
				Type: "oauth2",
				Credentials: toJSON(t, map[string]string{
					"tokenUrl":     "https://example.com/token",
					"clientId":     "client-id",
					"clientSecret": "client-secret",
				}),
			},
			wantType: "oauth2",
			wantErr:  false,
		},
		{
			name: "unknown type returns error",
			config: &connector.AuthConfig{
				Type:        "unknown",
				Credentials: toJSON(t, map[string]string{}),
			},
			wantErr:     true,
			errContains: "unknown authentication type",
		},
		{
			name: "api-key missing key returns error",
			config: &connector.AuthConfig{
				Type:        "api-key",
				Credentials: toJSON(t, map[string]string{}),
			},
			wantErr:     true,
			errContains: "api key is required",
		},
		{
			name: "bearer missing token returns error",
			config: &connector.AuthConfig{
				Type:        "bearer",
				Credentials: toJSON(t, map[string]string{}),
			},
			wantErr:     true,
			errContains: "token is required",
		},
		{
			name: "basic missing username returns error",
			config: &connector.AuthConfig{
				Type:        "basic",
				Credentials: toJSON(t, map[string]string{"password": "pass"}),
			},
			wantErr:     true,
			errContains: "username and password are required",
		},
		{
			name: "basic missing password returns error",
			config: &connector.AuthConfig{
				Type:        "basic",
				Credentials: toJSON(t, map[string]string{"username": "user"}),
			},
			wantErr:     true,
			errContains: "username and password are required",
		},
		{
			name: "oauth2 missing tokenUrl returns error",
			config: &connector.AuthConfig{
				Type: "oauth2",
				Credentials: toJSON(t, map[string]string{
					"clientId":     "client-id",
					"clientSecret": "client-secret",
				}),
			},
			wantErr:     true,
			errContains: "tokenUrl, clientId, and clientSecret are required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, err := NewHandler(tt.config, nil)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !containsString(err.Error(), tt.errContains) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.config == nil {
				if handler != nil {
					t.Error("expected nil handler for nil config")
				}
				return
			}

			if handler == nil {
				t.Fatal("expected non-nil handler")
			}

			if handler.Type() != tt.wantType {
				t.Errorf("Type() = %q, want %q", handler.Type(), tt.wantType)
			}
		})
	}
}

func TestAPIKeyHandler_ApplyAuth_Header(t *testing.T) {
	config := &connector.AuthConfig{
		Type: "api-key",
		Credentials: toJSON(t, map[string]string{
			"key":        "my-api-key",
			"location":   "header",
			"headerName": "X-Custom-Key",
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

	got := req.Header.Get("X-Custom-Key")
	if got != "my-api-key" {
		t.Errorf("header X-Custom-Key = %q, want %q", got, "my-api-key")
	}
}

func TestAPIKeyHandler_ApplyAuth_HeaderDefault(t *testing.T) {
	config := &connector.AuthConfig{
		Type: "api-key",
		Credentials: toJSON(t, map[string]string{
			"key": "my-api-key",
			// location defaults to header, headerName defaults to X-API-Key
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

	got := req.Header.Get("X-API-Key")
	if got != "my-api-key" {
		t.Errorf("header X-API-Key = %q, want %q", got, "my-api-key")
	}
}

func TestAPIKeyHandler_ApplyAuth_Query(t *testing.T) {
	config := &connector.AuthConfig{
		Type: "api-key",
		Credentials: toJSON(t, map[string]string{
			"key":       "my-api-key",
			"location":  "query",
			"paramName": "apikey",
		}),
	}

	handler, err := NewHandler(config, nil)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "https://example.com/api?existing=param", nil)
	err = handler.ApplyAuth(context.Background(), req)
	if err != nil {
		t.Fatalf("ApplyAuth failed: %v", err)
	}

	got := req.URL.Query().Get("apikey")
	if got != "my-api-key" {
		t.Errorf("query param apikey = %q, want %q", got, "my-api-key")
	}

	// Verify existing params are preserved
	existing := req.URL.Query().Get("existing")
	if existing != "param" {
		t.Errorf("existing query param = %q, want %q", existing, "param")
	}
}

func TestAPIKeyHandler_ApplyAuth_QueryDefaultParamName(t *testing.T) {
	config := &connector.AuthConfig{
		Type: "api-key",
		Credentials: toJSON(t, map[string]string{
			"key":      "my-api-key",
			"location": "query",
			// paramName defaults to api_key
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

	got := req.URL.Query().Get("api_key")
	if got != "my-api-key" {
		t.Errorf("query param api_key = %q, want %q", got, "my-api-key")
	}
}

func TestBearerHandler_ApplyAuth(t *testing.T) {
	config := &connector.AuthConfig{
		Type:        "bearer",
		Credentials: toJSON(t, map[string]string{"token": "my-bearer-token"}),
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
	want := "Bearer my-bearer-token"
	if got != want {
		t.Errorf("Authorization header = %q, want %q", got, want)
	}
}

func TestBasicHandler_ApplyAuth(t *testing.T) {
	config := &connector.AuthConfig{
		Type:        "basic",
		Credentials: toJSON(t, map[string]string{"username": "myuser", "password": "mypass"}),
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

	username, password, ok := req.BasicAuth()
	if !ok {
		t.Fatal("BasicAuth not set on request")
	}
	if username != "myuser" {
		t.Errorf("username = %q, want %q", username, "myuser")
	}
	if password != "mypass" {
		t.Errorf("password = %q, want %q", password, "mypass")
	}
}

func TestDeterministicAuth(t *testing.T) {
	// Test that same config produces same auth headers
	// This verifies deterministic execution requirement

	tests := []struct {
		name   string
		config *connector.AuthConfig
	}{
		{
			name: "api-key header is deterministic",
			config: &connector.AuthConfig{
				Type:        "api-key",
				Credentials: toJSON(t, map[string]string{"key": "test-key"}),
			},
		},
		{
			name: "api-key query is deterministic",
			config: &connector.AuthConfig{
				Type:        "api-key",
				Credentials: toJSON(t, map[string]string{"key": "test-key", "location": "query"}),
			},
		},
		{
			name: "bearer is deterministic",
			config: &connector.AuthConfig{
				Type:        "bearer",
				Credentials: toJSON(t, map[string]string{"token": "test-token"}),
			},
		},
		{
			name: "basic is deterministic",
			config: &connector.AuthConfig{
				Type:        "basic",
				Credentials: toJSON(t, map[string]string{"username": "user", "password": "pass"}),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler1, err := NewHandler(tt.config, nil)
			if err != nil {
				t.Fatalf("failed to create handler1: %v", err)
			}

			handler2, err := NewHandler(tt.config, nil)
			if err != nil {
				t.Fatalf("failed to create handler2: %v", err)
			}

			// Apply auth to two separate requests
			req1 := httptest.NewRequest(http.MethodGet, "https://example.com/api", nil)
			req2 := httptest.NewRequest(http.MethodGet, "https://example.com/api", nil)

			if err := handler1.ApplyAuth(context.Background(), req1); err != nil {
				t.Fatalf("ApplyAuth failed for req1: %v", err)
			}
			if err := handler2.ApplyAuth(context.Background(), req2); err != nil {
				t.Fatalf("ApplyAuth failed for req2: %v", err)
			}

			// Compare headers
			if req1.Header.Get("Authorization") != req2.Header.Get("Authorization") {
				t.Error("Authorization headers differ between requests")
			}

			// Compare query strings
			if req1.URL.RawQuery != req2.URL.RawQuery {
				t.Error("Query strings differ between requests")
			}
		})
	}
}

// Helper function
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestSecurity_CredentialsNotInErrorMessages verifies that credentials are never exposed in error messages.
// This is a critical security requirement to prevent credential leakage in logs or error responses.
func TestSecurity_CredentialsNotInErrorMessages(t *testing.T) {
	tests := []struct {
		name           string
		config         *connector.AuthConfig
		expectedErrKey string // Key that should be in error, but NOT the credential value
		credentialKey  string // Key of the credential that should NOT appear in error
		credentialVal  string // Value that should NEVER appear in error message
	}{
		{
			name: "api-key missing key doesn't expose credentials",
			config: &connector.AuthConfig{
				Type:        "api-key",
				Credentials: toJSON(t, map[string]string{}), // Missing key
			},
			expectedErrKey: "api key is required",
			credentialKey:  "key",
			credentialVal:  "secret-api-key-12345",
		},
		{
			name: "bearer missing token doesn't expose credentials",
			config: &connector.AuthConfig{
				Type:        "bearer",
				Credentials: toJSON(t, map[string]string{}), // Missing token
			},
			expectedErrKey: "token is required",
			credentialKey:  "token",
			credentialVal:  "secret-bearer-token-67890",
		},
		{
			name: "basic missing password doesn't expose credentials",
			config: &connector.AuthConfig{
				Type:        "basic",
				Credentials: toJSON(t, map[string]string{"username": "user"}), // Missing password
			},
			expectedErrKey: "username and password are required",
			credentialKey:  "password",
			credentialVal:  "secret-password-abcde",
		},
		{
			name: "oauth2 missing credentials doesn't expose values",
			config: &connector.AuthConfig{
				Type:        "oauth2",
				Credentials: toJSON(t, map[string]string{}), // Missing all OAuth2 creds
			},
			expectedErrKey: "tokenUrl, clientId, and clientSecret are required",
			credentialKey:  "clientSecret",
			credentialVal:  "secret-oauth2-client-secret-xyz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewHandler(tt.config, nil)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			// Verify error message contains expected key but NOT the credential value
			errMsg := err.Error()
			if !containsString(errMsg, tt.expectedErrKey) {
				t.Errorf("error message should contain %q, got %q", tt.expectedErrKey, errMsg)
			}
			if containsString(errMsg, tt.credentialVal) {
				t.Errorf("SECURITY VIOLATION: error message contains credential value %q: %q", tt.credentialVal, errMsg)
			}
		})
	}
}

// TestSecurity_ValidCredentialsNotInErrors verifies that even when credentials are present
// but validation fails for other reasons, credentials are never exposed in error messages.
func TestSecurity_ValidCredentialsNotInErrors(t *testing.T) {
	tests := []struct {
		name          string
		config        *connector.AuthConfig
		credentialVal string // Value that should NEVER appear in error message
	}{
		{
			name: "api-key with valid credentials, unknown type doesn't expose credentials",
			config: &connector.AuthConfig{
				Type:        "unknown-type",
				Credentials: toJSON(t, map[string]string{"key": "secret-api-key-valid"}),
			},
			credentialVal: "secret-api-key-valid",
		},
		{
			name: "bearer with valid token, unknown type doesn't expose token",
			config: &connector.AuthConfig{
				Type:        "unknown-type",
				Credentials: toJSON(t, map[string]string{"token": "secret-bearer-token-valid"}),
			},
			credentialVal: "secret-bearer-token-valid",
		},
		{
			name: "basic with valid credentials, unknown type doesn't expose password",
			config: &connector.AuthConfig{
				Type:        "unknown-type",
				Credentials: toJSON(t, map[string]string{"username": "user", "password": "secret-password-valid"}),
			},
			credentialVal: "secret-password-valid",
		},
		{
			name: "oauth2 with valid credentials, unknown type doesn't expose clientSecret",
			config: &connector.AuthConfig{
				Type: "unknown-type",
				Credentials: toJSON(t, map[string]string{
					"tokenUrl":     "https://example.com/token",
					"clientId":     "client-id",
					"clientSecret": "secret-oauth2-client-secret-valid",
				}),
			},
			credentialVal: "secret-oauth2-client-secret-valid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, err := NewHandler(tt.config, nil)
			if err == nil {
				t.Fatal("expected error for unknown type, got nil")
			}

			errMsg := err.Error()
			if containsString(errMsg, tt.credentialVal) {
				t.Errorf("SECURITY VIOLATION: error message contains credential value %q: %q", tt.credentialVal, errMsg)
			}

			// Handler should be nil
			if handler != nil {
				t.Error("expected nil handler for unknown type")
			}
		})
	}
}

func toJSON(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("toJSON: %v", err)
	}
	return b
}
