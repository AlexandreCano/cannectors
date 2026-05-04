// Package config provides functionality for parsing and validating
// pipeline configuration files (JSON/YAML).
package config

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/cannectors/runtime/pkg/connector"
)

// moduleBuilder holds intermediate state during conversion.
// The raw map is kept for defaults resolution before serialization to Raw.
type moduleBuilder struct {
	mc     *connector.ModuleConfig
	rawMap map[string]interface{}
}

// finalize serializes the raw map to ModuleConfig.Raw.
func (b *moduleBuilder) finalize() error {
	raw, err := json.Marshal(b.rawMap)
	if err != nil {
		return fmt.Errorf("marshaling module config for type %q: %w", b.mc.Type, err)
	}
	b.mc.Raw = raw
	return nil
}

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

	builders, err := extractModules(pipeline, data)
	if err != nil {
		return nil, fmt.Errorf("extracting modules: %w", err)
	}

	extractDefaults(pipeline, data)
	applyDefaults(builders, pipeline.Defaults)

	for _, b := range builders {
		if err := b.finalize(); err != nil {
			return nil, err
		}
	}

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
// Returns all module builders for later defaults resolution.
func extractModules(p *connector.Pipeline, data map[string]interface{}) ([]*moduleBuilder, error) {
	var builders []*moduleBuilder

	// Extract input
	inputData, ok := data["input"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'input' section")
	}
	inputBuilder, err := newModuleBuilder(inputData)
	if err != nil {
		return nil, fmt.Errorf("invalid input config: %w", err)
	}
	p.Input = inputBuilder.mc
	builders = append(builders, inputBuilder)

	// Extract filters
	if filtersData, okFilters := data["filters"].([]interface{}); okFilters {
		for i, filterData := range filtersData {
			filterMap, isMap := filterData.(map[string]interface{})
			if !isMap {
				return nil, fmt.Errorf("invalid filter at index %d", i)
			}
			fb, convertErr := newModuleBuilder(filterMap)
			if convertErr != nil {
				return nil, fmt.Errorf("invalid filter at index %d: %w", i, convertErr)
			}
			p.Filters = append(p.Filters, *fb.mc)
			// Point builder at the actual slice element so finalize writes to it
			builders = append(builders, &moduleBuilder{
				mc:     &p.Filters[len(p.Filters)-1],
				rawMap: fb.rawMap,
			})
		}
	}

	// Extract output
	outputData, ok := data["output"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'output' section")
	}
	outputBuilder, err := newModuleBuilder(outputData)
	if err != nil {
		return nil, fmt.Errorf("invalid output config: %w", err)
	}
	p.Output = outputBuilder.mc
	builders = append(builders, outputBuilder)

	return builders, nil
}

// extractDefaults extracts defaults (optional).
func extractDefaults(p *connector.Pipeline, data map[string]interface{}) {
	if defaults, ok := data["defaults"].(map[string]interface{}); ok {
		p.Defaults = convertModuleDefaults(defaults)
	}
}

// newModuleBuilder creates a module builder from raw config data.
// All fields except "type" are stored in rawMap for later serialization.
func newModuleBuilder(data map[string]interface{}) (*moduleBuilder, error) {
	moduleType, ok := data["type"].(string)
	if !ok {
		return nil, fmt.Errorf("missing required field 'type'")
	}

	rawMap := make(map[string]interface{}, len(data)-1)
	for key, value := range data {
		if key != "type" {
			rawMap[key] = value
		}
	}

	return &moduleBuilder{
		mc:     &connector.ModuleConfig{Type: moduleType},
		rawMap: rawMap,
	}, nil
}

// convertModuleDefaults converts connector.defaults to ModuleDefaults.
func convertModuleDefaults(data map[string]interface{}) *connector.ModuleDefaults {
	d := &connector.ModuleDefaults{}
	d.OnError, d.TimeoutMs, d.Retry = extractErrorHandlingFields(data)
	return d
}

// extractErrorHandlingFields extracts the common onError/timeoutMs/retry fields from a map.
func extractErrorHandlingFields(data map[string]interface{}) (string, int, *connector.RetryConfig) {
	var onError string
	var timeoutMs int
	var retry *connector.RetryConfig

	if v, ok := data["onError"].(string); ok {
		onError = v
	}
	if v, ok := getIntFromMap(data, "timeoutMs"); ok && v > 0 {
		timeoutMs = v
	}
	if r, ok := data["retry"].(map[string]interface{}); ok && len(r) > 0 {
		retry = parseRetryConfigFromMap(r)
	}
	return onError, timeoutMs, retry
}

// parseRetryConfigFromMap parses a retry config from a raw map into *connector.RetryConfig.
func parseRetryConfigFromMap(m map[string]interface{}) *connector.RetryConfig {
	rc := &connector.RetryConfig{}
	if v, ok := getIntFromMap(m, "maxAttempts"); ok {
		rc.MaxAttempts = v
	}
	if v, ok := getIntFromMap(m, "delayMs"); ok {
		rc.DelayMs = v
	}
	if v, ok := getFloatFromMap(m, "backoffMultiplier"); ok {
		rc.BackoffMultiplier = v
	}
	if v, ok := getIntFromMap(m, "maxDelayMs"); ok {
		rc.MaxDelayMs = v
	}
	if codes, ok := getIntSliceFromMap(m, "retryableStatusCodes"); ok {
		rc.RetryableStatusCodes = codes
	}
	if v, ok := m["useRetryAfterHeader"].(bool); ok {
		rc.UseRetryAfterHeader = v
	}
	if v, ok := m["retryHintFromBody"].(string); ok {
		rc.RetryHintFromBody = v
	}
	return rc
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

func getFloatFromMap(m map[string]interface{}, key string) (float64, bool) {
	if v, ok := m[key]; !ok {
		return 0, false
	} else if f, ok := v.(float64); ok {
		return f, true
	} else if i, ok := v.(int); ok {
		return float64(i), true
	}
	return 0, false
}

func getIntSliceFromMap(m map[string]interface{}, key string) ([]int, bool) {
	v, ok := m[key]
	if !ok {
		return nil, false
	}
	slice, ok := v.([]interface{})
	if !ok {
		return nil, false
	}
	result := make([]int, 0, len(slice))
	for _, item := range slice {
		switch val := item.(type) {
		case float64:
			result = append(result, int(val))
		case int:
			result = append(result, val)
		}
	}
	return result, len(result) > 0
}

// applyDefaults resolves retry/onError/timeoutMs per module (module > defaults)
// and injects resolved values into each module's raw map before serialization.
func applyDefaults(builders []*moduleBuilder, defaults *connector.ModuleDefaults) {
	for _, b := range builders {
		resolveOnErrorInheritance(b.rawMap, defaults)
		resolveTimeout(b.rawMap, defaults)
		resolveRetry(b.rawMap, defaults)
	}
}

// resolveOnErrorInheritance applies inheritance for the onError field with
// precedence module > defaults. Distinct from errhandling.ParseOnErrorStrategy,
// which parses a user-provided string into a typed strategy.
func resolveOnErrorInheritance(m map[string]interface{}, defaults *connector.ModuleDefaults) {
	if v, ok := m["onError"].(string); ok && v != "" {
		return
	}
	if defaults != nil && defaults.OnError != "" {
		m["onError"] = defaults.OnError
	}
}

// resolveTimeout resolves timeoutMs with precedence: module > defaults.
func resolveTimeout(m map[string]interface{}, defaults *connector.ModuleDefaults) {
	if v, ok := getIntFromMap(m, "timeoutMs"); ok && v > 0 {
		return
	}
	if defaults != nil && defaults.TimeoutMs > 0 {
		m["timeoutMs"] = defaults.TimeoutMs
	}
}

// resolveRetry resolves retry config with granular merge.
// Base config is resolved from defaults, then module fields override individually.
func resolveRetry(m map[string]interface{}, defaults *connector.ModuleDefaults) {
	moduleRetry, hasModule := m["retry"].(map[string]interface{})

	var base *connector.RetryConfig
	if defaults != nil && defaults.Retry != nil {
		base = defaults.Retry
	}

	if !hasModule || len(moduleRetry) == 0 {
		if base != nil {
			m["retry"] = retryConfigToMap(base)
		}
		return
	}

	if base == nil {
		return
	}

	// Merge: serialize base to map, then overlay module fields
	baseMap := retryConfigToMap(base)
	for k, v := range moduleRetry {
		baseMap[k] = v
	}
	m["retry"] = baseMap
}

// retryConfigToMap converts a *connector.RetryConfig to a map for merging.
func retryConfigToMap(rc *connector.RetryConfig) map[string]interface{} {
	m := make(map[string]interface{})
	if rc.MaxAttempts != 0 {
		m["maxAttempts"] = rc.MaxAttempts
	}
	if rc.DelayMs != 0 {
		m["delayMs"] = rc.DelayMs
	}
	if rc.BackoffMultiplier != 0 {
		m["backoffMultiplier"] = rc.BackoffMultiplier
	}
	if rc.MaxDelayMs != 0 {
		m["maxDelayMs"] = rc.MaxDelayMs
	}
	if len(rc.RetryableStatusCodes) > 0 {
		codes := make([]interface{}, len(rc.RetryableStatusCodes))
		for i, c := range rc.RetryableStatusCodes {
			codes[i] = c
		}
		m["retryableStatusCodes"] = codes
	}
	if rc.UseRetryAfterHeader {
		m["useRetryAfterHeader"] = true
	}
	if rc.RetryHintFromBody != "" {
		m["retryHintFromBody"] = rc.RetryHintFromBody
	}
	return m
}
