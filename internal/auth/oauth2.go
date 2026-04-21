package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/cannectors/runtime/internal/logger"
	"github.com/cannectors/runtime/pkg/connector"
)

// OAuth2 error types
var (
	ErrOAuth2TokenFailed = fmt.Errorf("failed to obtain OAuth2 access token")
)

// Default values for OAuth2
const (
	// tokenExpiryBuffer is subtracted from token expiry to refresh before actual expiry
	tokenExpiryBuffer = 60 * time.Second
	// maxTokenResponseSize limits the size of token responses to prevent memory exhaustion
	maxTokenResponseSize = 64 * 1024 // 64KB
)

// oauth2Handler implements OAuth2 client credentials authentication.
// It handles token acquisition, caching, and automatic refresh.
// Thread-safe for concurrent access.
//
// Security considerations:
//   - Credentials (clientId, clientSecret) are never logged
//   - Token response bodies are never logged (may contain sensitive error details)
//   - Tokens are cached in memory only (not persisted to disk)
//   - Token invalidation is automatic on 401 Unauthorized responses
//   - All error messages are sanitized to prevent credential leakage
type oauth2Handler struct {
	tokenURL     string
	clientID     string
	clientSecret string
	scopes       []string
	httpClient   *http.Client

	// Token cache protected by mutex for thread-safety
	mu          sync.RWMutex
	cachedToken string
	tokenExpiry time.Time
}

// newOAuth2Handler creates a new OAuth2 client credentials authentication handler.
func newOAuth2Handler(config *connector.AuthConfig, httpClient *http.Client) (*oauth2Handler, error) {
	var creds connector.CredentialsOAuth2
	if err := json.Unmarshal(config.Credentials, &creds); err != nil {
		return nil, fmt.Errorf("invalid oauth2 credentials: %w", err)
	}

	if creds.TokenURL == "" || creds.ClientID == "" || creds.ClientSecret == "" {
		return nil, ErrMissingOAuth2Creds
	}

	// Use provided client or create default
	client := httpClient
	if client == nil {
		client = &http.Client{
			Timeout: 30 * time.Second,
		}
	}

	return &oauth2Handler{
		tokenURL:     creds.TokenURL,
		clientID:     creds.ClientID,
		clientSecret: creds.ClientSecret,
		scopes:       creds.Scopes,
		httpClient:   client,
	}, nil
}

// ApplyAuth applies OAuth2 authentication to the request.
// It uses cached token if valid, otherwise obtains a new one.
func (h *oauth2Handler) ApplyAuth(ctx context.Context, req *http.Request) error {
	token, err := h.getValidToken(ctx)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	return nil
}

// Type returns the authentication type identifier.
func (h *oauth2Handler) Type() string {
	return "oauth2"
}

// getValidToken returns a valid token, refreshing if necessary.
// Thread-safe implementation using RWMutex.
func (h *oauth2Handler) getValidToken(ctx context.Context) (string, error) {
	// Try to use cached token (read lock)
	h.mu.RLock()
	if h.cachedToken != "" && time.Now().Before(h.tokenExpiry) {
		token := h.cachedToken
		h.mu.RUnlock()
		return token, nil
	}
	h.mu.RUnlock()

	// Need to refresh token (write lock)
	h.mu.Lock()
	defer h.mu.Unlock()

	// Double-check after acquiring write lock (another goroutine may have refreshed)
	if h.cachedToken != "" && time.Now().Before(h.tokenExpiry) {
		return h.cachedToken, nil
	}

	// Obtain new token
	token, expiry, err := h.fetchToken(ctx)
	if err != nil {
		return "", err
	}

	h.cachedToken = token
	h.tokenExpiry = expiry

	return token, nil
}

// tokenResponse represents the OAuth2 token endpoint response.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope,omitempty"`
}

// fetchToken obtains a new OAuth2 access token using client credentials flow.
func (h *oauth2Handler) fetchToken(ctx context.Context) (string, time.Time, error) {
	// Build form data
	formData := url.Values{}
	formData.Set("grant_type", "client_credentials")
	formData.Set("client_id", h.clientID)
	formData.Set("client_secret", h.clientSecret)

	if len(h.scopes) > 0 {
		formData.Set("scope", strings.Join(h.scopes, " "))
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.tokenURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("%w: creating token request: %v", ErrOAuth2TokenFailed, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Execute request
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("%w: executing token request: %v", ErrOAuth2TokenFailed, err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			logger.Error("failed to close OAuth2 token response body", "error", closeErr.Error())
		}
	}()

	// Read response body with size limit
	limitedReader := io.LimitReader(resp.Body, maxTokenResponseSize)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("%w: reading token response: %v", ErrOAuth2TokenFailed, err)
	}

	// Check for error response
	if resp.StatusCode != http.StatusOK {
		// Log only status code, never the response body (may contain error details with credentials).
		// Security: Never log clientId, clientSecret, or response body to prevent credential leakage.
		logger.Error("OAuth2 token request failed",
			"status_code", resp.StatusCode,
			"token_url", h.tokenURL,
		)
		return "", time.Time{}, fmt.Errorf("%w: token endpoint returned status %d", ErrOAuth2TokenFailed, resp.StatusCode)
	}

	// Parse response
	var tokenResp tokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", time.Time{}, fmt.Errorf("%w: parsing token response: %v", ErrOAuth2TokenFailed, err)
	}

	// Validate access token
	if strings.TrimSpace(tokenResp.AccessToken) == "" {
		return "", time.Time{}, fmt.Errorf("%w: token endpoint returned empty access_token", ErrOAuth2TokenFailed)
	}

	// Calculate expiry with buffer to refresh before actual expiry
	expiresIn := tokenResp.ExpiresIn
	if expiresIn > 0 {
		// Subtract buffer, but ensure we don't go negative
		bufferedExpiry := time.Duration(expiresIn)*time.Second - tokenExpiryBuffer
		if bufferedExpiry < 0 {
			bufferedExpiry = 0
		}
		expiry := time.Now().Add(bufferedExpiry)

		logger.Debug("OAuth2 token obtained",
			"expires_in_seconds", expiresIn,
			"token_type", tokenResp.TokenType,
		)

		return tokenResp.AccessToken, expiry, nil
	}

	// No expiry information - token valid indefinitely (unusual but possible)
	logger.Warn("OAuth2 token has no expiry information")
	return tokenResp.AccessToken, time.Now().Add(24 * time.Hour), nil
}

// InvalidateToken clears the cached token, forcing a refresh on next request.
// This method is called automatically when a 401 Unauthorized response is received,
// indicating the token has been rejected or revoked by the API server.
//
// Error handling behavior:
//   - If token refresh fails after invalidation, the error is returned to caller
//   - Common error scenarios:
//   - Network failure: Returns error with context about token URL
//   - Invalid credentials: Returns error with status code (never logs response body)
//   - Invalid JSON response: Returns parsing error
//   - Empty token: Returns validation error
//   - Errors never contain credential values (clientId, clientSecret) for security
func (h *oauth2Handler) InvalidateToken() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cachedToken = ""
	h.tokenExpiry = time.Time{}
}

// TokenExpiry returns the current token expiry time.
// Returns zero time if no token is cached.
func (h *oauth2Handler) TokenExpiry() time.Time {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.tokenExpiry
}
