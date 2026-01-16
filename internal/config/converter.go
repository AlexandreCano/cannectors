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

	// Extract error handling (optional)
	if errorHandling, okErrHandling := connectorData["errorHandling"].(map[string]interface{}); okErrHandling {
		pipeline.ErrorHandling = convertErrorHandling(errorHandling)
	}

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

// convertErrorHandling converts a raw error handling configuration map to ErrorHandling.
func convertErrorHandling(data map[string]interface{}) *connector.ErrorHandling {
	errorHandling := &connector.ErrorHandling{}

	if retryCount, ok := data["retryCount"].(float64); ok {
		errorHandling.RetryCount = int(retryCount)
	}

	if retryDelay, ok := data["retryDelay"].(float64); ok {
		errorHandling.RetryDelay = int(retryDelay)
	}

	if onError, ok := data["onError"].(string); ok {
		errorHandling.OnError = onError
	}

	return errorHandling
}
