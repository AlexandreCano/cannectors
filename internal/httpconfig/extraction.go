// Package httpconfig provides shared HTTP configuration extraction utilities.
package httpconfig

import (
	"time"

	"github.com/canectors/runtime/pkg/connector"
)

// ExtractBaseConfig extracts BaseConfig from a ModuleConfig.
func ExtractBaseConfig(config *connector.ModuleConfig) BaseConfig {
	if config == nil || config.Config == nil {
		return BaseConfig{}
	}

	base := BaseConfig{
		Auth: config.Authentication,
	}

	if endpoint, ok := config.Config["endpoint"].(string); ok {
		base.Endpoint = endpoint
	}

	if method, ok := config.Config["method"].(string); ok {
		base.Method = method
	}

	base.Headers = ExtractStringMap(config.Config, "headers")
	base.TimeoutMs = extractTimeoutMs(config.Config)

	return base
}

// ExtractBodyTemplateConfig extracts BodyTemplateConfig from a config map.
func ExtractBodyTemplateConfig(config map[string]interface{}) BodyTemplateConfig {
	btc := BodyTemplateConfig{}
	if config == nil {
		return btc
	}

	if bodyTemplateFile, ok := config["bodyTemplateFile"].(string); ok {
		btc.BodyTemplateFile = bodyTemplateFile
	}

	return btc
}

// ExtractBodyTemplateConfigFromRequest extracts BodyTemplateConfig from a request sub-config.
func ExtractBodyTemplateConfigFromRequest(config map[string]interface{}) BodyTemplateConfig {
	btc := BodyTemplateConfig{}
	if config == nil {
		return btc
	}

	requestVal, ok := config["request"].(map[string]interface{})
	if !ok {
		return btc
	}

	if bodyTemplateFile, ok := requestVal["bodyTemplateFile"].(string); ok {
		btc.BodyTemplateFile = bodyTemplateFile
	}

	return btc
}

// ExtractDynamicParamsConfig extracts DynamicParamsConfig from a config map.
// It looks for parameters in both root level and "request" sub-object.
func ExtractDynamicParamsConfig(config map[string]interface{}) DynamicParamsConfig {
	dpc := DynamicParamsConfig{}
	if config == nil {
		return dpc
	}

	// First try root level
	dpc.PathParams = ExtractStringMap(config, "pathParams")
	dpc.QueryParams = ExtractStringMap(config, "queryParams")
	dpc.QueryFromRecord = ExtractStringMap(config, "queryFromRecord")
	dpc.HeadersFromRecord = ExtractStringMap(config, "headersFromRecord")

	// If not found at root, try "request" sub-object
	if requestVal, ok := config["request"].(map[string]interface{}); ok {
		if len(dpc.PathParams) == 0 {
			dpc.PathParams = ExtractStringMap(requestVal, "pathParams")
		}
		if len(dpc.QueryParams) == 0 {
			// Support both "query" and "queryParams" keys
			dpc.QueryParams = ExtractStringMap(requestVal, "query")
			if len(dpc.QueryParams) == 0 {
				dpc.QueryParams = ExtractStringMap(requestVal, "queryParams")
			}
		}
		if len(dpc.QueryFromRecord) == 0 {
			dpc.QueryFromRecord = ExtractStringMap(requestVal, "queryFromRecord")
		}
		if len(dpc.HeadersFromRecord) == 0 {
			dpc.HeadersFromRecord = ExtractStringMap(requestVal, "headersFromRecord")
		}
	}

	return dpc
}

// ExtractErrorHandlingConfig extracts ErrorHandlingConfig from a config map.
func ExtractErrorHandlingConfig(config map[string]interface{}) ErrorHandlingConfig {
	ehc := ErrorHandlingConfig{}
	if config == nil {
		return ehc
	}

	if onError, ok := config["onError"].(string); ok {
		ehc.OnError = onError
	}

	return ehc
}

// ExtractDataExtractionConfig extracts DataExtractionConfig from a config map.
func ExtractDataExtractionConfig(config map[string]interface{}) DataExtractionConfig {
	dec := DataExtractionConfig{}
	if config == nil {
		return dec
	}

	if dataField, ok := config["dataField"].(string); ok {
		dec.DataField = dataField
	}

	return dec
}

// ExtractStringMap extracts a map[string]string from a config map at the given key.
func ExtractStringMap(config map[string]interface{}, key string) map[string]string {
	result := make(map[string]string)
	if config == nil {
		return result
	}

	mapVal, ok := config[key].(map[string]interface{})
	if !ok {
		return result
	}

	for k, v := range mapVal {
		if strVal, ok := v.(string); ok {
			result[k] = strVal
		}
	}

	return result
}

// extractTimeoutMs extracts timeout in milliseconds from config.
// Supports both "timeoutMs" (preferred) and legacy "timeout" in seconds.
func extractTimeoutMs(config map[string]interface{}) int {
	// Try timeoutMs first (preferred)
	if ms, ok := config["timeoutMs"]; ok {
		switch v := ms.(type) {
		case float64:
			if v > 0 {
				return int(v)
			}
		case int:
			if v > 0 {
				return v
			}
		}
	}

	// Legacy: timeout in seconds
	if timeoutVal, ok := config["timeout"].(float64); ok && timeoutVal > 0 {
		return int(timeoutVal * 1000)
	}

	return 0 // Use default
}

// GetTimeoutDuration returns the timeout as a time.Duration.
// If timeoutMs is 0 or negative, returns the provided default.
func GetTimeoutDuration(timeoutMs int, defaultTimeout time.Duration) time.Duration {
	if timeoutMs > 0 {
		return time.Duration(timeoutMs) * time.Millisecond
	}
	return defaultTimeout
}
