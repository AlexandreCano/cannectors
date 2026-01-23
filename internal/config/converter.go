// Package config provides functionality for parsing and validating
// pipeline configuration files (JSON/YAML).
package config

import (
	"fmt"
	"time"

	"github.com/canectors/runtime/pkg/connector"
)

// ConvertToPipeline converts parsed configuration data to a Pipeline struct.
// The input data should have been validated against the schema before calling this function.
//
// The configuration is expected to have this structure:
//
//	{
//	  "schemaVersion": "1.1.0",
//	  "connector": {
//	    "name": "...",
//	    "version": "...",
//	    "input": {...},
//	    "filters": [...],
//	    "output": {...}
//	  }
//	}
func ConvertToPipeline(data map[string]interface{}) (*connector.Pipeline, error) {
	if data == nil {
		return nil, fmt.Errorf("configuration data is nil")
	}

	// Extract connector section
	connectorData, ok := data["connector"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'connector' section")
	}

	pipeline := &connector.Pipeline{
		Enabled:   true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Extract required fields
	var name string
	if name, ok = connectorData["name"].(string); !ok {
		return nil, fmt.Errorf("missing required field 'connector.name'")
	}
	pipeline.Name = name
	// Use name as ID if not specified
	pipeline.ID = name

	var version string
	if version, ok = connectorData["version"].(string); !ok {
		return nil, fmt.Errorf("missing required field 'connector.version'")
	}
	pipeline.Version = version

	// Extract optional fields
	if description, okDesc := connectorData["description"].(string); okDesc {
		pipeline.Description = description
	}

	if id, okID := connectorData["id"].(string); okID {
		pipeline.ID = id
	}

	// Extract input module config
	inputData, ok := connectorData["input"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'connector.input' section")
	}
	inputConfig, err := convertModuleConfig(inputData)
	if err != nil {
		return nil, fmt.Errorf("invalid input config: %w", err)
	}
	pipeline.Input = inputConfig

	// Extract schedule from input if present (for pipeline-level schedule)
	if schedule, okSchedule := inputData["schedule"].(string); okSchedule {
		pipeline.Schedule = schedule
	}

	// Extract filters (optional)
	if filtersData, okFilters := connectorData["filters"].([]interface{}); okFilters {
		for i, filterData := range filtersData {
			filterMap, isMap := filterData.(map[string]interface{})
			if !isMap {
				return nil, fmt.Errorf("invalid filter at index %d", i)
			}
			filterConfig, convertErr := convertModuleConfig(filterMap)
			if convertErr != nil {
				return nil, fmt.Errorf("invalid filter at index %d: %w", i, convertErr)
			}
			pipeline.Filters = append(pipeline.Filters, *filterConfig)
		}
	}

	// Extract output module config
	outputData, okOutput := connectorData["output"].(map[string]interface{})
	if !okOutput {
		return nil, fmt.Errorf("missing or invalid 'connector.output' section")
	}
	outputConfig, err := convertModuleConfig(outputData)
	if err != nil {
		return nil, fmt.Errorf("invalid output config: %w", err)
	}
	pipeline.Output = outputConfig

	// Extract defaults (optional). Precedence: module > defaults > errorHandling.
	if defaults, okDefaults := connectorData["defaults"].(map[string]interface{}); okDefaults {
		pipeline.Defaults = convertModuleDefaults(defaults)
	}

	// Extract error handling (optional, legacy)
	if errorHandling, okErrHandling := connectorData["errorHandling"].(map[string]interface{}); okErrHandling {
		pipeline.ErrorHandling = convertErrorHandling(errorHandling)
	}

	// Resolve retry/onError/timeoutMs per module (module > defaults > errorHandling) and inject into Config
	resolveAndInjectErrorHandling(pipeline)

	return pipeline, nil
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

	if v, ok := data["retryCount"].(float64); ok {
		eh.RetryCount = int(v)
	}
	if v, ok := data["retryDelay"].(float64); ok {
		eh.RetryDelay = int(v)
	}
	if v, ok := data["retryDelayMs"].(float64); ok {
		eh.RetryDelay = int(v)
	}
	if v, ok := data["onError"].(string); ok {
		eh.OnError = v
	}
	if v, ok := getIntFromMap(data, "timeoutMs"); ok && v > 0 {
		eh.TimeoutMs = v
	}
	if r, ok := data["retry"].(map[string]interface{}); ok && len(r) > 0 {
		eh.Retry = r
	}
	// Build Retry from legacy fields if no retry object
	if eh.Retry == nil && (eh.RetryCount > 0 || eh.RetryDelay > 0) {
		eh.Retry = map[string]interface{}{
			"maxAttempts": eh.RetryCount,
			"delayMs":     eh.RetryDelay,
		}
		if b, ok := data["backoffMultiplier"].(float64); ok {
			eh.Retry["backoffMultiplier"] = b
		}
		if v, ok := getIntFromMap(data, "maxRetryDelayMs"); ok {
			eh.Retry["maxDelayMs"] = v
		}
		if c, ok := data["retryableStatusCodes"].([]interface{}); ok {
			eh.Retry["retryableStatusCodes"] = c
		}
	}
	return eh
}

// resolveAndInjectErrorHandling resolves retry/onError/timeoutMs per module (module > defaults > errorHandling)
// and injects resolved values into each module's Config so modules read them directly.
func resolveAndInjectErrorHandling(p *connector.Pipeline) {
	defaults := p.Defaults
	eh := p.ErrorHandling

	resolve := func(m *connector.ModuleConfig) {
		if m == nil || m.Config == nil {
			return
		}
		// Precedence: module > defaults > errorHandling
		var onError string
		var timeoutMs int
		var retry map[string]interface{}

		if v, ok := m.Config["onError"].(string); ok && v != "" {
			onError = v
		} else if defaults != nil && defaults.OnError != "" {
			onError = defaults.OnError
		} else if eh != nil && eh.OnError != "" {
			onError = eh.OnError
		}
		if onError != "" {
			m.Config["onError"] = onError
		}

		if v, ok := getIntFromMap(m.Config, "timeoutMs"); ok && v > 0 {
			timeoutMs = v
		} else if v, ok := getIntFromMap(m.Config, "timeout"); ok && v > 0 {
			timeoutMs = v * 1000
		} else if defaults != nil && defaults.TimeoutMs > 0 {
			timeoutMs = defaults.TimeoutMs
		} else if eh != nil && eh.TimeoutMs > 0 {
			timeoutMs = eh.TimeoutMs
		}
		if timeoutMs > 0 {
			m.Config["timeoutMs"] = timeoutMs
		}

		if r, ok := m.Config["retry"].(map[string]interface{}); ok && len(r) > 0 {
			retry = r
		} else if defaults != nil && len(defaults.Retry) > 0 {
			retry = defaults.Retry
		} else if eh != nil && len(eh.Retry) > 0 {
			retry = eh.Retry
		}
		if len(retry) > 0 {
			m.Config["retry"] = retry
		}
	}

	resolve(p.Input)
	for i := range p.Filters {
		resolve(&p.Filters[i])
	}
	resolve(p.Output)
}
