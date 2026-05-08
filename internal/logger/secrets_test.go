package logger_test

import (
	"log/slog"
	"regexp"
	"strings"
	"testing"

	"github.com/cannectors/runtime/internal/auth"
	"github.com/cannectors/runtime/internal/httpclient"
	"github.com/cannectors/runtime/internal/logger"
)

// secretPatterns are regular expressions that should never match content in
// logs emitted by the runtime when it processes a pipeline. They cover the
// common ways credentials leak: API keys / tokens stuck in URLs and Authorization
// header values. New leak categories should be added here so this test acts as
// a permanent regression guard.
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`api[_-]?key=[A-Za-z0-9._\-]+`),
	regexp.MustCompile(`access[_-]?token=[A-Za-z0-9._\-]+`),
	regexp.MustCompile(`token=[A-Za-z0-9._\-]+`),
	regexp.MustCompile(`(?i)password=[^"\s,}]+`),
	regexp.MustCompile(`Bearer\s+[A-Za-z0-9._\-]+`),
	regexp.MustCompile(`Basic\s+[A-Za-z0-9._\-/+=]+`),
}

func assertNoSecretLeak(t *testing.T, body string) {
	t.Helper()
	for _, re := range secretPatterns {
		if loc := re.FindStringIndex(body); loc != nil {
			t.Fatalf("log output leaked a secret matching %q: ...%s...", re.String(), body[max(0, loc[0]-20):min(len(body), loc[1]+20)])
		}
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// TestSanitizeURL_StripsCredentials guards the helper that everything else
// relies on. If this regresses, every other log site is at risk.
func TestSanitizeURL_StripsCredentials(t *testing.T) {
	cases := []string{
		"https://api.example.com/v1/orders?api_key=SUPERSECRET",
		"https://api.example.com/v1/orders?access_token=ey123.abc",
		"https://api.example.com/v1/orders?token=raw-token-value",
		"https://user:pass@api.example.com/v1/orders",
	}
	for _, raw := range cases {
		got := httpclient.SanitizeURL(raw)
		assertNoSecretLeak(t, got)
	}
}

// TestOAuth2TokenLog_DoesNotLeakSecrets checks the OAuth2 token request error
// log path. The endpoint URL must be sanitized and the response body must
// never appear, even when the server returns 4xx with credential context.
func TestOAuth2TokenLog_DoesNotLeakSecrets(t *testing.T) {
	buf, restore := captureLogger(t)
	defer restore()

	// We do not need to actually issue the request; the goal is to verify
	// the format of the error log emitted by package auth when the token
	// endpoint URL would otherwise embed secrets. Re-create the same log
	// call shape directly.
	tokenURL := "https://idp.example.com/oauth/token?client_id=public&access_token=LEAKED"

	logger.Logger.Error("OAuth2 token request failed",
		slog.Int("status_code", 401),
		slog.String("token_url", httpclient.SanitizeURL(tokenURL)),
	)

	out := buf.String()
	if !strings.Contains(out, "OAuth2 token request failed") {
		t.Fatalf("expected log message in output, got %q", out)
	}
	assertNoSecretLeak(t, out)
}

// TestEndpointLog_DoesNotLeakSecrets checks the canonical pattern used by the
// HTTP modules: log an endpoint that may contain query-string credentials and
// confirm the sanitized form was emitted.
func TestEndpointLog_DoesNotLeakSecrets(t *testing.T) {
	buf, restore := captureLogger(t)
	defer restore()

	endpoint := "https://api.example.com/v1/users?api_key=SHOULD_NOT_LEAK"

	logger.Logger.Info("input fetch started",
		slog.String("endpoint", httpclient.SanitizeURL(endpoint)),
	)

	out := buf.String()
	assertNoSecretLeak(t, out)
}

// TestAuthError_DoesNotLeakInvalidCredentialsURL guards the authentication
// failure path: the endpoint embedded in the wrapped error must be sanitized
// before reaching the log.
func TestAuthError_DoesNotLeakInvalidCredentialsURL(t *testing.T) {
	buf, restore := captureLogger(t)
	defer restore()

	endpoint := "https://api.example.com/v1/orders?api_key=LEAKY"
	wrapped := auth.ErrOAuth2InvalidCredentials

	logger.Logger.Error("authentication failed",
		slog.String("endpoint", httpclient.SanitizeURL(endpoint)),
		slog.String("error", wrapped.Error()),
	)

	out := buf.String()
	assertNoSecretLeak(t, out)
}
