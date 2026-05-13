package filter

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/cannectors/runtime/internal/auth"
	"github.com/cannectors/runtime/internal/cache"
	"github.com/cannectors/runtime/internal/errhandling"
	"github.com/cannectors/runtime/internal/httpclient"
	"github.com/cannectors/runtime/internal/logger"
	"github.com/cannectors/runtime/internal/moduleconfig"
	"github.com/cannectors/runtime/internal/modules/soaputil"
	"github.com/cannectors/runtime/internal/soapclient"
	"github.com/cannectors/runtime/internal/template"
	"github.com/cannectors/runtime/pkg/connector"
)

// SOAPCallConfig represents the configuration for a soap_call filter module.
type SOAPCallConfig struct {
	connector.ModuleBase
	moduleconfig.SOAPRequestBase
	DataField     string                   `json:"dataField,omitempty"`
	Keys          []moduleconfig.KeyConfig `json:"keys,omitempty"`
	Cache         moduleconfig.CacheConfig `json:"cache,omitempty"`
	MergeStrategy string                   `json:"mergeStrategy,omitempty"`
	ResultKey     string                   `json:"resultKey,omitempty"`
	Retry         *connector.RetryConfig   `json:"retry,omitempty"`
}

// SOAPCallModule enriches records with data returned by a SOAP operation.
type SOAPCallModule struct {
	base              moduleconfig.SOAPRequestBase
	keys              []moduleconfig.KeyConfig
	dataField         string
	mergeStrategy     string
	resultKey         string
	onError           errhandling.OnErrorStrategy
	httpClient        *httpclient.Client
	soapClient        soapclient.SOAPClient
	authHandler       auth.Handler
	retry             connector.RetryConfig
	cache             cache.Cache
	cacheEnabled      bool
	cacheTTL          time.Duration
	cacheKey          string
	templateEvaluator *template.Evaluator
}

// NewSOAPCallFromConfig creates a SOAP enrichment filter.
func NewSOAPCallFromConfig(config SOAPCallConfig) (*SOAPCallModule, error) {
	if err := soaputil.ValidateBase(config.SOAPRequestBase); err != nil {
		return nil, err
	}
	if err := validateKeyEntries(config.Keys); err != nil {
		return nil, err
	}
	mergeStrategy, err := normalizeMergeStrategy("soap_call", config.MergeStrategy)
	if err != nil {
		return nil, err
	}
	if mergeStrategy == "append" && strings.TrimSpace(config.ResultKey) == "" {
		return nil, fmt.Errorf("soap_call resultKey is required when mergeStrategy is append")
	}
	onError, err := errhandling.ParseOnErrorStrategy(config.OnError)
	if err != nil {
		return nil, err
	}
	if config.Cache.Key != "" {
		if cacheKeyErr := template.ValidateSyntax(config.Cache.Key); cacheKeyErr != nil {
			return nil, fmt.Errorf("invalid template syntax in soap_call cache key: %w", cacheKeyErr)
		}
	}

	var retryConfig connector.RetryConfig
	if config.Retry != nil {
		retryConfig = soaputil.ToRetryConfig(config.Retry)
		if retryErr := retryConfig.Validate(); retryErr != nil {
			return nil, fmt.Errorf("soap_call retry config invalid: %w", retryErr)
		}
	}

	timeout := connector.GetTimeoutDuration(config.TimeoutMs, defaultHTTPCallTimeout)
	httpClient := httpclient.NewClient(timeout)
	authHandler, err := auth.NewHandler(config.Authentication, httpClient.Client)
	if err != nil {
		return nil, fmt.Errorf("creating soap_call auth handler: %w", err)
	}
	lruCache, cacheTTL, cacheEnabled := buildHTTPCallCache(config.Cache)

	logger.Debug("soap_call module initialized",
		slog.String("endpoint", httpclient.SanitizeURL(config.Endpoint)),
		slog.String("operation", config.Operation),
		slog.Int("keys_count", len(config.Keys)),
		slog.String("merge_strategy", mergeStrategy),
		slog.Bool("cache_enabled", cacheEnabled),
		slog.Bool("has_auth", authHandler != nil),
	)

	keys := make([]moduleconfig.KeyConfig, len(config.Keys))
	copy(keys, config.Keys)
	return &SOAPCallModule{
		base:              config.SOAPRequestBase,
		keys:              keys,
		dataField:         config.DataField,
		mergeStrategy:     mergeStrategy,
		resultKey:         config.ResultKey,
		onError:           onError,
		httpClient:        httpClient,
		soapClient:        soapclient.NewClient(httpClient),
		authHandler:       authHandler,
		retry:             retryConfig,
		cache:             lruCache,
		cacheEnabled:      cacheEnabled,
		cacheTTL:          cacheTTL,
		cacheKey:          config.Cache.Key,
		templateEvaluator: template.NewEvaluator(),
	}, nil
}

// Process enriches each record by calling the configured SOAP operation.
func (m *SOAPCallModule) Process(ctx context.Context, records []map[string]any) ([]map[string]any, error) {
	if records == nil {
		return []map[string]any{}, nil
	}
	result := make([]map[string]any, 0, len(records))
	for i, record := range records {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		enriched, err := m.processRecord(ctx, record)
		if err != nil {
			switch m.onError {
			case errhandling.OnErrorFail:
				return nil, fmt.Errorf("soap_call record %d: %w", i, err)
			case errhandling.OnErrorSkip:
				logger.Warn("skipping record due to soap_call error",
					slog.Int("record_index", i),
					slog.String("operation", m.base.Operation),
					slog.String("error", err.Error()),
				)
				continue
			case errhandling.OnErrorLog:
				logger.Error("soap_call error (continuing)",
					slog.Int("record_index", i),
					slog.String("operation", m.base.Operation),
					slog.String("error", err.Error()),
				)
				result = append(result, record)
				continue
			}
		}
		result = append(result, enriched)
	}
	return result, nil
}

func (m *SOAPCallModule) processRecord(ctx context.Context, record map[string]any) (map[string]any, error) {
	keyValues, err := soaputil.ExtractKeyValues(record, m.keys)
	if err != nil {
		return nil, err
	}
	if m.cacheEnabled {
		cacheKey := m.buildCacheKey(keyValues, record)
		if cached, found := m.cache.Get(cacheKey); found {
			if responseData, ok := cached.(map[string]any); ok {
				logger.Debug("soap_call cache hit",
					slog.String("operation", m.base.Operation),
					slog.Any("key_values", keyValues),
				)
				return m.mergeData(record, responseData), nil
			}
		}
	}

	responseData, err := m.fetchResponseData(ctx, record, keyValues)
	if err != nil {
		return nil, err
	}
	if m.cacheEnabled {
		m.cache.Set(m.buildCacheKey(keyValues, record), responseData, m.cacheTTL)
	}
	return m.mergeData(record, responseData), nil
}

func (m *SOAPCallModule) fetchResponseData(ctx context.Context, record map[string]any, keyValues map[string]string) (map[string]any, error) {
	endpoint, err := soaputil.BuildEndpointWithKeys(m.base.Endpoint, record, m.keys, keyValues)
	if err != nil {
		return nil, err
	}
	headerOverrides := soaputil.BuildHeaderOverridesWithKeys(m.keys, keyValues)
	retry := &m.retry
	if m.retry.MaxAttempts == 0 {
		retry = nil
	}
	op, err := soaputil.BuildOperation(soaputil.OperationOptions{
		Base:        m.base,
		Record:      record,
		AuthHandler: m.authHandler,
		Retry:       retry,
		Endpoint:    endpoint,
		HTTPHeaders: headerOverrides,
	})
	if err != nil {
		return nil, err
	}
	resp, err := m.soapClient.Call(ctx, op)
	if err != nil {
		return nil, err
	}
	value, err := soaputil.ExtractDataField(resp.Data, m.dataField)
	if err != nil {
		return nil, err
	}
	return soaputil.ValueAsRecordMap(value)
}

func (m *SOAPCallModule) buildCacheKey(keyValues map[string]string, record map[string]any) string {
	if m.cacheKey != "" {
		return m.templateEvaluator.Evaluate(m.cacheKey, record)
	}
	parts := make([]string, 0, len(m.keys))
	for _, key := range m.keys {
		parts = append(parts, keyValues[key.ParamName])
	}
	return m.base.Endpoint + "::" + m.base.Operation + "::" + strings.Join(parts, "::")
}

func (m *SOAPCallModule) mergeData(record, responseData map[string]any) map[string]any {
	switch m.mergeStrategy {
	case "replace":
		result := make(map[string]any, len(record)+len(responseData))
		for k, v := range record {
			result[k] = v
		}
		for k, v := range responseData {
			result[k] = v
		}
		return result
	case "append":
		result := make(map[string]any, len(record)+1)
		for k, v := range record {
			result[k] = v
		}
		result[m.resultKey] = responseData
		return result
	default:
		return m.deepMerge(record, responseData)
	}
}

func (m *SOAPCallModule) deepMerge(a, b map[string]any) map[string]any {
	result := make(map[string]any, len(a)+len(b))
	for k, v := range a {
		result[k] = v
	}
	for k, vb := range b {
		if va, exists := result[k]; exists {
			if mapA, okA := va.(map[string]any); okA {
				if mapB, okB := vb.(map[string]any); okB {
					result[k] = m.deepMerge(mapA, mapB)
					continue
				}
			}
		}
		result[k] = vb
	}
	return result
}

// GetCacheStats returns cache stats when caching is enabled.
func (m *SOAPCallModule) GetCacheStats() cache.Stats {
	if !m.cacheEnabled {
		return cache.Stats{}
	}
	return m.cache.Stats()
}

// ClearCache clears the SOAP call cache.
func (m *SOAPCallModule) ClearCache() {
	if m.cacheEnabled {
		m.cache.Clear()
	}
}
