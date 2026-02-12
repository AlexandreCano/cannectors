// Package config provides functionality for parsing and validating
// pipeline configuration files (JSON/YAML).
package config

import (
	"fmt"
	"time"

	"github.com/cannectors/runtime/pkg/connector"
)

// ConvertToPipeline converts parsed configuration data to a Pipeline struct.
// The input data should have been validated against the schema before calling this function.
//
// The configuration is expected to have this structure:
//
//	{
//	  "name": "...",
//	  "version": "...",
//	  "input": {...},
//	  "filters": [...],
//	  "output": {...}
//	}
//
// Note: version is optional.
func ConvertToPipeline(data map[string]interface{}) (*connector.Pipeline, error) {
	if data == nil {
		return nil, fmt.Errorf("configuration data is nil")
	}

	pipeline := newPipeline()
	if err := extractPipelineMetadata(pipeline, data); err != nil {
		return nil, fmt.Errorf("extracting pipeline metadata: %w", err)
	}

	if err := extractModules(pipeline, data); err != nil {
		return nil, fmt.Errorf("extracting modules: %w", err)
	}

	extractDefaultsAndErrorHandling(pipeline, data)
	applyErrorHandling(pipeline)

	return pipeline, nil
}

// newPipeline creates a new Pipeline with default values.
func newPipeline() *connector.Pipeline {
	return &connector.Pipeline{
		Enabled:   true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// extractPipelineMetadata extracts name, version, description, and id.
func extractPipelineMetadata(p *connector.Pipeline, data map[string]interface{}) error {
	name, ok := data["name"].(string)
	if !ok {
		return fmt.Errorf("missing required field 'name'")
	}
	p.Name = name
	p.ID = name // Use name as ID if not specified

	if version, ok := data["version"].(string); ok {
		p.Version = version
	}

	if description, ok := data["description"].(string); ok {
		p.Description = description
	}

	if id, ok := data["id"].(string); ok {
		p.ID = id
	}

	return nil
}

// extractModules extracts input, filters, and output modules.
func extractModules(p *connector.Pipeline, data map[string]interface{}) error {
	// Extract input
	inputData, ok := data["input"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("missing or invalid 'input' section")
	}
	inputConfig, err := convertModuleConfig(inputData)
	if err != nil {
		return fmt.Errorf("invalid input config: %w", err)
	}
	p.Input = inputConfig

	// Extract filters
	if filtersData, okFilters := data["filters"].([]interface{}); okFilters {
		for i, filterData := range filtersData {
			filterMap, isMap := filterData.(map[string]interface{})
			if !isMap {
				return fmt.Errorf("invalid filter at index %d", i)
			}
			filterConfig, convertErr := convertModuleConfig(filterMap)
			if convertErr != nil {
				return fmt.Errorf("invalid filter at index %d: %w", i, convertErr)
			}
			p.Filters = append(p.Filters, *filterConfig)
		}
	}

	// Extract output
	outputData, ok := data["output"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("missing or invalid 'output' section")
	}
	outputConfig, err := convertModuleConfig(outputData)
	if err != nil {
		return fmt.Errorf("invalid output config: %w", err)
	}
	p.Output = outputConfig

	return nil
}

// extractDefaultsAndErrorHandling extracts defaults and errorHandling (optional).
func extractDefaultsAndErrorHandling(p *connector.Pipeline, data map[string]interface{}) {
	if defaults, ok := data["defaults"].(map[string]interface{}); ok {
		p.Defaults = convertModuleDefaults(defaults)
	}

	if errorHandling, ok := data["errorHandling"].(map[string]interface{}); ok {
		p.ErrorHandling = convertErrorHandling(errorHandling)
	}
}

// convertModuleConfig converts a raw module configuration map to ModuleConfig.
func convertModuleConfig(data map[string]interface{}) (*connector.ModuleConfig, error) {
	moduleConfig := &connector.ModuleConfig{
		Config: make(map[string]interface{}),
	}

	// Extract type (required)
	moduleType, ok := data["type"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required field 'type'")
	}
	moduleConfig.Type = moduleType

	// Copy all fields except 'type' and 'authentication' to Config
	for key, value := range data {
		if key != "type" && key != "authentication" {
			moduleConfig.Config[key] = value
		}
	}

	// Extract authentication (optional)
	if authData, ok := data["authentication"].(map[string]interface{}); ok {
		authConfig, err := convertAuthConfig(authData)
		if err != nil {
			return nil, fmt.Errorf("invalid authentication config: %w", err)
		}
		moduleConfig.Authentication = authConfig
	}

	return moduleConfig, nil
}

// convertAuthConfig converts a raw authentication configuration map to AuthConfig.
func convertAuthConfig(data map[string]interface{}) (*connector.AuthConfig, error) {
	authConfig := &connector.AuthConfig{
		Credentials: make(map[string]string),
	}

	// Extract type (required)
	authType, ok := data["type"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required field 'type'")
	}
	authConfig.Type = authType

	// Extract credentials
	if credentials, ok := data["credentials"].(map[string]interface{}); ok {
		for key, value := range credentials {
			strValue, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("invalid credential value for key %q: expected string, got %T", key, value)
			}
			authConfig.Credentials[key] = strValue
		}
	}

	return authConfig, nil
}

// convertModuleDefaults converts connector.defaults to ModuleDefaults.
func convertModuleDefaults(data map[string]interface{}) *connector.ModuleDefaults {
	d := &connector.ModuleDefaults{}
	if v, ok := data["onError"].(string); ok {
		d.OnError = v
	}
	if v, ok := getIntFromMap(data, "timeoutMs"); ok && v > 0 {
		d.TimeoutMs = v
	}
	if r, ok := data["retry"].(map[string]interface{}); ok && len(r) > 0 {
		d.Retry = r
	}
	return d
}

func getIntFromMap(m map[string]interface{}, key string) (int, bool) {
	if v, ok := m[key]; !ok {
		return 0, false
	} else if f, ok := v.(float64); ok {
		return int(f), true
	} else if i, ok := v.(int); ok {
		return i, true
	}
	return 0, false
}

// convertErrorHandling converts a raw error handling configuration map to ErrorHandling.
// Supports legacy (retryCount, retryDelay) and full retryConfig (retry.*, timeoutMs).
func convertErrorHandling(data map[string]interface{}) *connector.ErrorHandling {
	eh := &connector.ErrorHandling{}

	parseLegacyRetryFields(eh, data)
	eh.OnError = extractStringField(data, "onError")
	eh.TimeoutMs = extractTimeoutMs(data)
	eh.Retry = extractRetryConfig(data)

	// Build Retry from legacy fields if no retry object
	if eh.Retry == nil && (eh.RetryCount > 0 || eh.RetryDelay > 0) {
		eh.Retry = buildRetryFromLegacy(eh, data)
	}

	return eh
}

// parseLegacyRetryFields extracts legacy retry fields (retryCount, retryDelay, retryDelayMs).
func parseLegacyRetryFields(eh *connector.ErrorHandling, data map[string]interface{}) {
	if v, ok := data["retryCount"].(float64); ok {
		eh.RetryCount = int(v)
	}
	if v, ok := data["retryDelay"].(float64); ok {
		eh.RetryDelay = int(v)
	}
	if v, ok := data["retryDelayMs"].(float64); ok {
		eh.RetryDelay = int(v)
	}
}

// extractStringField extracts a string value from a map.
func extractStringField(data map[string]interface{}, key string) string {
	if v, ok := data[key].(string); ok {
		return v
	}
	return ""
}

// extractTimeoutMs extracts timeoutMs from data.
func extractTimeoutMs(data map[string]interface{}) int {
	if v, ok := getIntFromMap(data, "timeoutMs"); ok && v > 0 {
		return v
	}
	return 0
}

// extractRetryConfig extracts retry config map from data.
func extractRetryConfig(data map[string]interface{}) map[string]interface{} {
	if r, ok := data["retry"].(map[string]interface{}); ok && len(r) > 0 {
		return r
	}
	return nil
}

// buildRetryFromLegacy builds a retry config map from legacy fields.
func buildRetryFromLegacy(eh *connector.ErrorHandling, data map[string]interface{}) map[string]interface{} {
	retry := map[string]interface{}{
		"maxAttempts": eh.RetryCount,
		"delayMs":     eh.RetryDelay,
	}

	if b, ok := data["backoffMultiplier"].(float64); ok {
		retry["backoffMultiplier"] = b
	}
	if v, ok := getIntFromMap(data, "maxRetryDelayMs"); ok {
		retry["maxDelayMs"] = v
	}
	if c, ok := data["retryableStatusCodes"].([]interface{}); ok {
		retry["retryableStatusCodes"] = c
	}

	return retry
}

// applyErrorHandling resolves retry/onError/timeoutMs per module (module > defaults > errorHandling)
// and injects resolved values into each module's Config so modules read them directly.
func applyErrorHandling(p *connector.Pipeline) {
	resolve := func(m *connector.ModuleConfig) {
		if m == nil || m.Config == nil {
			return
		}
		resolveOnError(m, p.Defaults, p.ErrorHandling)
		resolveTimeout(m, p.Defaults, p.ErrorHandling)
		resolveRetry(m, p.Defaults, p.ErrorHandling)
	}

	resolve(p.Input)
	for i := range p.Filters {
		resolve(&p.Filters[i])
	}
	resolve(p.Output)
}

// resolveOnError resolves onError with precedence: module > defaults > errorHandling.
func resolveOnError(m *connector.ModuleConfig, defaults *connector.ModuleDefaults, eh *connector.ErrorHandling) {
	if v, ok := m.Config["onError"].(string); ok && v != "" {
		m.Config["onError"] = v
		return
	}
	if defaults != nil && defaults.OnError != "" {
		m.Config["onError"] = defaults.OnError
		return
	}
	if eh != nil && eh.OnError != "" {
		m.Config["onError"] = eh.OnError
	}
}

// resolveTimeout resolves timeoutMs with precedence: module > defaults > errorHandling.
// Also handles legacy "timeout" in seconds (converts to milliseconds).
func resolveTimeout(m *connector.ModuleConfig, defaults *connector.ModuleDefaults, eh *connector.ErrorHandling) {
	// Check module config first (timeoutMs or legacy timeout)
	if v, ok := getIntFromMap(m.Config, "timeoutMs"); ok && v > 0 {
		m.Config["timeoutMs"] = v
		return
	}
	if v, ok := getIntFromMap(m.Config, "timeout"); ok && v > 0 {
		m.Config["timeoutMs"] = v * 1000
		return
	}
	// Fall back to defaults
	if defaults != nil && defaults.TimeoutMs > 0 {
		m.Config["timeoutMs"] = defaults.TimeoutMs
		return
	}
	// Fall back to errorHandling
	if eh != nil && eh.TimeoutMs > 0 {
		m.Config["timeoutMs"] = eh.TimeoutMs
	}
}

// resolveRetry resolves retry config with precedence: module > defaults > errorHandling.
func resolveRetry(m *connector.ModuleConfig, defaults *connector.ModuleDefaults, eh *connector.ErrorHandling) {
	if r, ok := m.Config["retry"].(map[string]interface{}); ok && len(r) > 0 {
		m.Config["retry"] = r
		return
	}
	if defaults != nil && len(defaults.Retry) > 0 {
		m.Config["retry"] = defaults.Retry
		return
	}
	if eh != nil && len(eh.Retry) > 0 {
		m.Config["retry"] = eh.Retry
	}
}
