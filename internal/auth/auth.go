// Package auth provides shared authentication handling for Cannectors modules.
// It supports API key, Bearer token, Basic auth, and OAuth2 client credentials.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/cannectors/runtime/internal/logger"
	"github.com/cannectors/runtime/pkg/connector"
)

// MaxOAuth2Retries is the maximum number of token-refresh + retry cycles
// performed when an OAuth2-protected endpoint replies with 401. After this
// many consecutive 401s the credentials are considered invalid and the call
// fails fast with ErrOAuth2InvalidCredentials. See Story 17.5.
const MaxOAuth2Retries = 2

// Error types for authentication
var (
	ErrNilConfig          = errors.New("authentication configuration is nil")
	ErrUnknownType        = errors.New("unknown authentication type")
	ErrMissingAPIKey      = errors.New("api key is required for api-key authentication")
	ErrMissingBearerToken = errors.New("token is required for bearer authentication")
	ErrMissingBasicAuth   = errors.New("username and password are required for basic authentication")
	ErrMissingOAuth2Creds = errors.New("tokenUrl, clientId, and clientSecret are required for oauth2 authentication")
	// ErrOAuth2InvalidCredentials is returned when an OAuth2-authenticated
	// request keeps replying 401 after MaxOAuth2Retries token refreshes.
	// The runtime treats it as a fatal authentication error (no further retry).
	ErrOAuth2InvalidCredentials = errors.New("OAuth2 credentials appear invalid after token refresh")
)

// Handler defines the interface for authentication handlers.
// All authentication types implement this interface to apply
// authentication to HTTP requests.
type Handler interface {
	// ApplyAuth applies authentication to the given HTTP request.
	// Returns an error if authentication cannot be applied.
	ApplyAuth(ctx context.Context, req *http.Request) error

	// Type returns the authentication type identifier.
	Type() string
}

// OAuth2Invalidator is an optional interface for handlers that support token invalidation.
// This is primarily used by OAuth2 handlers to invalidate cached tokens when a 401
// Unauthorized response is received, indicating the token has been rejected by the API.
type OAuth2Invalidator interface {
	// InvalidateToken clears the cached token, forcing a refresh on next request.
	// Useful when a token is rejected by the API (e.g., 401 Unauthorized response).
	InvalidateToken()
}

// ErrPreviewUnavailable indicates that a handler cannot produce unmasked preview
// headers without performing network I/O (e.g., OAuth2 with no cached token).
// Callers should fall back to masked headers in this case.
var ErrPreviewUnavailable = errors.New("auth preview unavailable without network I/O")

// PreviewAuthHandler is an optional interface for handlers that can apply
// authentication without any network side effects. It is used by dry-run /
// preview flows that must not emit real HTTP requests.
//
// Handlers whose ApplyAuth is already side-effect-free (api-key, bearer, basic)
// do not need to implement this interface — the preview code falls back to
// ApplyAuth for them. OAuth2 implements it to expose the cached token without
// triggering a token fetch against the tokenUrl.
type PreviewAuthHandler interface {
	// PreviewAuth applies authentication without performing network I/O.
	// Returns ErrPreviewUnavailable if preview is not possible without side
	// effects (e.g., no cached OAuth2 token).
	PreviewAuth(ctx context.Context, req *http.Request) error
}

// NewHandler creates an appropriate authentication handler based on configuration.
// Returns nil, nil if config is nil (no authentication required).
// Returns an error if the authentication type is unknown or configuration is invalid.
func NewHandler(config *connector.AuthConfig, httpClient *http.Client) (Handler, error) {
	if config == nil {
		return nil, nil
	}

	switch config.Type {
	case "api-key":
		return newAPIKeyHandler(config)
	case "bearer":
		return newBearerHandler(config)
	case "basic":
		return newBasicHandler(config)
	case "oauth2":
		return newOAuth2Handler(config, httpClient)
	default:
		logger.Warn("unknown authentication type", "type", config.Type)
		return nil, fmt.Errorf("%w: %s", ErrUnknownType, config.Type)
	}
}

// apiKeyHandler implements API key authentication.
// Supports both header and query parameter locations.
type apiKeyHandler struct {
	key        string
	location   string // "header" or "query"
	paramName  string // parameter/header name
	headerName string // custom header name (for header location)
}

// newAPIKeyHandler creates a new API key authentication handler.
func newAPIKeyHandler(config *connector.AuthConfig) (*apiKeyHandler, error) {
	var creds connector.CredentialsAPIKey
	if err := json.Unmarshal(config.Credentials, &creds); err != nil {
		return nil, fmt.Errorf("invalid api-key credentials: %w", err)
	}

	if creds.Key == "" {
		return nil, ErrMissingAPIKey
	}
	if creds.Location == "" {
		creds.Location = "header"
	}
	if creds.ParamName == "" {
		creds.ParamName = "api_key"
	}
	if creds.HeaderName == "" {
		creds.HeaderName = "X-API-Key"
	}

	return &apiKeyHandler{
		key:        creds.Key,
		location:   creds.Location,
		paramName:  creds.ParamName,
		headerName: creds.HeaderName,
	}, nil
}

// ApplyAuth applies API key authentication to the request.
func (h *apiKeyHandler) ApplyAuth(_ context.Context, req *http.Request) error {
	switch h.location {
	case "query":
		q := req.URL.Query()
		q.Set(h.paramName, h.key)
		req.URL.RawQuery = q.Encode()
	case "header", "":
		req.Header.Set(h.headerName, h.key)
	}
	return nil
}

// Type returns the authentication type identifier.
func (h *apiKeyHandler) Type() string {
	return "api-key"
}

// bearerHandler implements Bearer token authentication.
type bearerHandler struct {
	token string
}

// newBearerHandler creates a new Bearer token authentication handler.
func newBearerHandler(config *connector.AuthConfig) (*bearerHandler, error) {
	var creds connector.CredentialsBearer
	if err := json.Unmarshal(config.Credentials, &creds); err != nil {
		return nil, fmt.Errorf("invalid bearer credentials: %w", err)
	}

	if creds.Token == "" {
		return nil, ErrMissingBearerToken
	}

	return &bearerHandler{
		token: creds.Token,
	}, nil
}

// ApplyAuth applies Bearer token authentication to the request.
func (h *bearerHandler) ApplyAuth(_ context.Context, req *http.Request) error {
	req.Header.Set("Authorization", "Bearer "+h.token)
	return nil
}

// Type returns the authentication type identifier.
func (h *bearerHandler) Type() string {
	return "bearer"
}

// basicHandler implements HTTP Basic authentication.
type basicHandler struct {
	username string
	password string
}

// newBasicHandler creates a new Basic authentication handler.
func newBasicHandler(config *connector.AuthConfig) (*basicHandler, error) {
	var creds connector.CredentialsBasic
	if err := json.Unmarshal(config.Credentials, &creds); err != nil {
		return nil, fmt.Errorf("invalid basic credentials: %w", err)
	}

	if creds.Username == "" || creds.Password == "" {
		return nil, ErrMissingBasicAuth
	}

	return &basicHandler{
		username: creds.Username,
		password: creds.Password,
	}, nil
}

// ApplyAuth applies HTTP Basic authentication to the request.
func (h *basicHandler) ApplyAuth(_ context.Context, req *http.Request) error {
	req.SetBasicAuth(h.username, h.password)
	return nil
}

// Type returns the authentication type identifier.
func (h *basicHandler) Type() string {
	return "basic"
}
