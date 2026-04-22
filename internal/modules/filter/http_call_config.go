package filter

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cannectors/runtime/internal/auth"
	"github.com/cannectors/runtime/internal/cache"
	"github.com/cannectors/runtime/internal/logger"
	"github.com/cannectors/runtime/internal/moduleconfig"
	"github.com/cannectors/runtime/internal/template"
	"github.com/cannectors/runtime/pkg/connector"
)

// normalizeHTTPCallMethod uppercases and validates the configured HTTP
// method. GET is used when empty.
func normalizeHTTPCallMethod(method string) (string, error) {
	m := strings.ToUpper(method)
	if m == "" {
		m = http.MethodGet
	}
	if m != http.MethodGet && m != http.MethodPost && m != http.MethodPut {
		return "", fmt.Errorf("http_call method must be GET, POST, or PUT, got: %s", method)
	}
	return m, nil
}

// normalizeHTTPCallMergeStrategy falls back to the default strategy on empty
// input and logs a warning for unknown values.
func normalizeHTTPCallMergeStrategy(s string) string {
	if s == "" {
		return defaultHTTPCallStrategy
	}
	if s != "merge" && s != "replace" && s != "append" {
		logger.Warn("invalid mergeStrategy for http_call module; defaulting to merge",
			slog.String("merge_strategy", s),
		)
		return defaultHTTPCallStrategy
	}
	return s
}

// normalizeHTTPCallOnError falls back to "fail" on empty input and logs a
// warning for unknown values.
func normalizeHTTPCallOnError(s string) string {
	if s == "" {
		return OnErrorFail
	}
	if s != OnErrorFail && s != OnErrorSkip && s != OnErrorLog {
		logger.Warn("invalid onError value for http_call module; defaulting to fail",
			slog.String("on_error", s),
		)
		return OnErrorFail
	}
	return s
}

// buildHTTPCallAuth wraps auth.NewHandler with a module-specific error
// message.
func buildHTTPCallAuth(authCfg *connector.AuthConfig, client *http.Client) (auth.Handler, error) {
	if authCfg == nil {
		return nil, nil
	}
	h, err := auth.NewHandler(authCfg, client)
	if err != nil {
		return nil, fmt.Errorf("creating http_call auth handler: %w", err)
	}
	return h, nil
}

// buildHTTPCallCache creates the LRU cache used to memoize responses,
// applying module defaults when inputs are non-positive.
func buildHTTPCallCache(maxSize, ttlSeconds int) (*cache.LRUCache, time.Duration) {
	if maxSize <= 0 {
		maxSize = defaultCacheMaxSize
	}
	if ttlSeconds <= 0 {
		ttlSeconds = defaultCacheTTLSeconds
	}
	ttl := time.Duration(ttlSeconds) * time.Second
	return cache.NewLRUCache(maxSize, ttl), ttl
}

// loadHTTPCallBodyTemplate loads a POST/PUT body template from disk and
// validates its syntax. Returns ("", nil) when path is empty.
func loadHTTPCallBodyTemplate(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("loading http_call body template file %q: %w", path, err)
	}
	raw := string(content)
	if err := template.ValidateSyntax(raw); err != nil {
		return "", fmt.Errorf("invalid template syntax in %q: %w", path, err)
	}
	logger.Debug("loaded http_call body template file",
		slog.String("file", path),
		slog.Int("size", len(content)),
	)
	return raw, nil
}

// validateHTTPCallTemplates validates template syntax in endpoint and
// headers up-front.
func validateHTTPCallTemplates(endpoint string, headers map[string]string) error {
	if err := template.ValidateSyntax(endpoint); err != nil {
		return fmt.Errorf("invalid template syntax in endpoint: %w", err)
	}
	for name, v := range headers {
		if err := template.ValidateSyntax(v); err != nil {
			return fmt.Errorf("invalid template syntax in header %q: %w", name, err)
		}
	}
	return nil
}

// httpCallHasTemplating returns true when the module configuration contains
// any {{ }} template variables worth evaluating per-record.
func httpCallHasTemplating(endpoint string, headers map[string]string, hasBodyTemplate bool) bool {
	if template.HasVariables(endpoint) || hasBodyTemplate {
		return true
	}
	for _, v := range headers {
		if template.HasVariables(v) {
			return true
		}
	}
	return false
}

// validateKeysConfig validates the keys extraction configuration.
func validateKeysConfig(keys []moduleconfig.KeyConfig) error {
	if len(keys) == 0 {
		return newHTTPCallError(ErrCodeHTTPCallKeyMissing, "http_call keys is required (at least one key config)", -1, "", 0, "")
	}
	for i, key := range keys {
		if key.Field == "" {
			return newHTTPCallError(ErrCodeHTTPCallKeyMissing, fmt.Sprintf("http_call keys[%d] field is required", i), -1, "", 0, "")
		}
		if key.ParamType == "" {
			return newHTTPCallError(ErrCodeHTTPCallKeyMissing, fmt.Sprintf("http_call keys[%d] paramType is required", i), -1, "", 0, "")
		}
		if key.ParamType != "query" && key.ParamType != "path" && key.ParamType != "header" {
			return newHTTPCallError(ErrCodeHTTPCallKeyInvalid, "http_call key paramType must be 'query', 'path', or 'header'", -1, "", 0, "")
		}
		if key.ParamName == "" {
			return newHTTPCallError(ErrCodeHTTPCallKeyMissing, fmt.Sprintf("http_call keys[%d] paramName is required", i), -1, "", 0, "")
		}
	}
	return nil
}
