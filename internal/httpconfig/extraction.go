// Package httpconfig provides shared HTTP configuration extraction utilities.
package httpconfig

import (
	"time"

	"github.com/cannectors/runtime/pkg/connector"
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

// ExtractKeysConfig extracts keys configuration from a config map.
func ExtractKeysConfig(config map[string]interface{}) []KeyConfig {
	if config == nil {
		return nil
	}
	keysRaw, ok := config["keys"].([]interface{})
	if !ok {
		return nil
	}
	result := make([]KeyConfig, 0, len(keysRaw))
	for _, item := range keysRaw {
		if m, ok := item.(map[string]interface{}); ok {
			result = append(result, keyConfigFromMap(m))
		}
	}
	return result
}

func keyConfigFromMap(m map[string]interface{}) KeyConfig {
	return KeyConfig{
		Field:     extractStringField(m, "field"),
		ParamType: extractStringField(m, "paramType"),
		ParamName: extractStringField(m, "paramName"),
	}
}

func extractStringField(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
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
func extractTimeoutMs(config map[string]interface{}) int {
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

	return 0 // Use default
}

// GetTimeoutDuration returns the timeout as a time.Duration.
// If timeoutMs is 0 or negative, returns the provided default.
// Delegates to connector.GetTimeoutDuration.
func GetTimeoutDuration(timeoutMs int, defaultTimeout time.Duration) time.Duration {
	return connector.GetTimeoutDuration(timeoutMs, defaultTimeout)
}
