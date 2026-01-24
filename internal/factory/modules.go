// Package factory provides module creation functions for the pipeline runtime.
// It centralizes the logic for instantiating input, filter, and output modules
// from their configuration.
package factory

import (
	"fmt"

	"github.com/canectors/runtime/internal/modules/filter"
	"github.com/canectors/runtime/internal/modules/input"
	"github.com/canectors/runtime/internal/modules/output"
	"github.com/canectors/runtime/pkg/connector"
)

// maxNestingDepth is the maximum allowed depth for nested module configurations.
const maxNestingDepth = 50

// CreateInputModule creates an input module instance from configuration.
// Returns a stub module for unimplemented types.
func CreateInputModule(cfg *connector.ModuleConfig) input.Module {
	if cfg == nil {
		return nil
	}

	switch cfg.Type {
	case "httpPolling":
		endpoint, _ := cfg.Config["endpoint"].(string)
		return input.NewStub(cfg.Type, endpoint)
	default:
		return input.NewStub(cfg.Type, "")
	}
}

// CreateFilterModules creates filter module instances from configuration.
// Supports mapping and condition filter types. Other types use stub implementations.
func CreateFilterModules(cfgs []connector.ModuleConfig) ([]filter.Module, error) {
	if len(cfgs) == 0 {
		return nil, nil
	}

	modules := make([]filter.Module, 0, len(cfgs))
	for i, cfg := range cfgs {
		module, err := createSingleFilterModule(cfg, i)
		if err != nil {
			return nil, err
		}
		modules = append(modules, module)
	}
	return modules, nil
}

// createSingleFilterModule creates a single filter module based on its type.
func createSingleFilterModule(cfg connector.ModuleConfig, index int) (filter.Module, error) {
	switch cfg.Type {
	case "mapping":
		return createMappingFilterModule(cfg, index)
	case "condition":
		return createConditionFilterModule(cfg, index)
	default:
		return filter.NewStub(cfg.Type, index), nil
	}
}

// createMappingFilterModule creates a mapping filter module from configuration.
func createMappingFilterModule(cfg connector.ModuleConfig, index int) (filter.Module, error) {
	mappings, err := filter.ParseFieldMappings(cfg.Config["mappings"])
	if err != nil {
		return nil, fmt.Errorf("invalid mapping config at index %d: %w", index, err)
	}

	onError, _ := cfg.Config["onError"].(string)
	module, err := filter.NewMappingFromConfig(mappings, onError)
	if err != nil {
		return nil, fmt.Errorf("invalid mapping config at index %d: %w", index, err)
	}

	return module, nil
}

// createConditionFilterModule creates a condition filter module from configuration.
func createConditionFilterModule(cfg connector.ModuleConfig, index int) (filter.Module, error) {
	condConfig, err := ParseConditionConfig(cfg.Config)
	if err != nil {
		return nil, fmt.Errorf("invalid condition config at index %d: %w", index, err)
	}

	module, err := filter.NewConditionFromConfig(condConfig)
	if err != nil {
		return nil, fmt.Errorf("invalid condition config at index %d: %w", index, err)
	}

	return module, nil
}

// CreateOutputModule creates an output module instance from configuration.
// Uses real HTTPRequestModule for httpRequest type, stub for unsupported types.
func CreateOutputModule(cfg *connector.ModuleConfig) (output.Module, error) {
	if cfg == nil {
		return nil, nil
	}

	switch cfg.Type {
	case "httpRequest":
		module, err := output.NewHTTPRequestFromConfig(cfg)
		if err != nil {
			return nil, fmt.Errorf("creating httpRequest module: %w", err)
		}
		return module, nil
	default:
		endpoint, _ := cfg.Config["endpoint"].(string)
		method, _ := cfg.Config["method"].(string)
		return output.NewStub(cfg.Type, endpoint, method), nil
	}
}

// ParseConditionConfig parses a condition filter configuration from raw config.
func ParseConditionConfig(cfg map[string]interface{}) (filter.ConditionConfig, error) {
	condConfig := filter.ConditionConfig{}

	expr, ok := cfg["expression"].(string)
	if !ok || expr == "" {
		return condConfig, fmt.Errorf("required field 'expression' is missing or empty in condition config")
	}
	condConfig.Expression = expr

	if lang, ok := cfg["lang"].(string); ok {
		condConfig.Lang = lang
	}
	if onTrue, ok := cfg["onTrue"].(string); ok {
		condConfig.OnTrue = onTrue
	}
	if onFalse, ok := cfg["onFalse"].(string); ok {
		condConfig.OnFalse = onFalse
	}
	if onError, ok := cfg["onError"].(string); ok {
		condConfig.OnError = onError
	}

	if thenCfg, ok := cfg["then"].(map[string]interface{}); ok {
		nestedModule, err := parseNestedModuleConfig(thenCfg)
		if err != nil {
			return condConfig, fmt.Errorf("invalid 'then' config: %w", err)
		}
		condConfig.Then = nestedModule
	}

	if elseCfg, ok := cfg["else"].(map[string]interface{}); ok {
		nestedModule, err := parseNestedModuleConfig(elseCfg)
		if err != nil {
			return condConfig, fmt.Errorf("invalid 'else' config: %w", err)
		}
		condConfig.Else = nestedModule
	}

	return condConfig, nil
}

// parseNestedModuleConfig parses a nested module configuration.
func parseNestedModuleConfig(cfg map[string]interface{}) (*filter.NestedModuleConfig, error) {
	return parseNestedConfig(cfg, 0)
}

// parseNestedConfig parses a nested module configuration with depth tracking.
func parseNestedConfig(cfg map[string]interface{}, depth int) (*filter.NestedModuleConfig, error) {
	if depth >= maxNestingDepth {
		return nil, fmt.Errorf("nested module depth %d exceeds maximum %d", depth, maxNestingDepth)
	}

	nestedConfig := initNestedConfig(cfg)
	nestedConfigMap := extractConfigMap(cfg, nestedConfig)

	switch nestedConfig.Type {
	case "mapping":
		if err := parseMappingNestedConfig(cfg, nestedConfig, nestedConfigMap); err != nil {
			return nil, err
		}
	case "condition":
		if err := parseConditionNestedConfig(cfg, nestedConfig, nestedConfigMap, depth); err != nil {
			return nil, err
		}
	}

	return nestedConfig, nil
}

// initNestedConfig initializes a NestedModuleConfig from raw config.
func initNestedConfig(cfg map[string]interface{}) *filter.NestedModuleConfig {
	nestedConfig := &filter.NestedModuleConfig{}
	if typ, ok := cfg["type"].(string); ok {
		nestedConfig.Type = typ
	}
	if onError, ok := cfg["onError"].(string); ok {
		nestedConfig.OnError = onError
	}
	return nestedConfig
}

// extractConfigMap extracts the nested config map if present.
func extractConfigMap(cfg map[string]interface{}, nestedConfig *filter.NestedModuleConfig) map[string]interface{} {
	if config, ok := cfg["config"].(map[string]interface{}); ok {
		nestedConfig.Config = config
		return config
	}
	return nil
}

// parseMappingNestedConfig parses configuration for a nested mapping module.
func parseMappingNestedConfig(cfg map[string]interface{}, nestedConfig *filter.NestedModuleConfig, nestedConfigMap map[string]interface{}) error {
	mappingsRaw := getFromMaps(cfg, nestedConfigMap, "mappings")
	if mappingsRaw != nil {
		mappings, err := filter.ParseFieldMappings(mappingsRaw)
		if err != nil {
			return err
		}
		nestedConfig.Mappings = mappings
	}

	if nestedConfig.OnError == "" && nestedConfigMap != nil {
		if configOnError, ok := nestedConfigMap["onError"].(string); ok {
			nestedConfig.OnError = configOnError
		}
	}
	return nil
}

// parseConditionNestedConfig parses configuration for a nested condition module.
func parseConditionNestedConfig(cfg map[string]interface{}, nestedConfig *filter.NestedModuleConfig, nestedConfigMap map[string]interface{}, depth int) error {
	if expr, ok := getStringFromMaps(cfg, nestedConfigMap, "expression"); ok {
		nestedConfig.Expression = expr
	}
	if lang, ok := getStringFromMaps(cfg, nestedConfigMap, "lang"); ok {
		nestedConfig.Lang = lang
	}
	if onTrue, ok := getStringFromMaps(cfg, nestedConfigMap, "onTrue"); ok {
		nestedConfig.OnTrue = onTrue
	}
	if onFalse, ok := getStringFromMaps(cfg, nestedConfigMap, "onFalse"); ok {
		nestedConfig.OnFalse = onFalse
	}
	if nestedConfig.OnError == "" {
		if onError, ok := getStringFromMaps(cfg, nestedConfigMap, "onError"); ok {
			nestedConfig.OnError = onError
		}
	}

	return parseNestedThenElse(cfg, nestedConfig, nestedConfigMap, depth)
}

// parseNestedThenElse parses the then and else nested modules recursively.
func parseNestedThenElse(cfg map[string]interface{}, nestedConfig *filter.NestedModuleConfig, nestedConfigMap map[string]interface{}, depth int) error {
	if thenCfg, ok := getMapFromMaps(cfg, nestedConfigMap, "then"); ok {
		then, err := parseNestedConfig(thenCfg, depth+1)
		if err != nil {
			return err
		}
		nestedConfig.Then = then
	}

	if elseCfg, ok := getMapFromMaps(cfg, nestedConfigMap, "else"); ok {
		elseModule, err := parseNestedConfig(elseCfg, depth+1)
		if err != nil {
			return err
		}
		nestedConfig.Else = elseModule
	}

	return nil
}

// getStringFromMaps retrieves a string value from primary or fallback map.
func getStringFromMaps(primary, fallback map[string]interface{}, key string) (string, bool) {
	if val, ok := primary[key].(string); ok {
		return val, true
	}
	if fallback != nil {
		if val, ok := fallback[key].(string); ok {
			return val, true
		}
	}
	return "", false
}

// getMapFromMaps retrieves a map value from primary or fallback map.
func getMapFromMaps(primary, fallback map[string]interface{}, key string) (map[string]interface{}, bool) {
	if val, ok := primary[key].(map[string]interface{}); ok {
		return val, true
	}
	if fallback != nil {
		if val, ok := fallback[key].(map[string]interface{}); ok {
			return val, true
		}
	}
	return nil, false
}

// getFromMaps retrieves any value from primary or fallback map.
func getFromMaps(primary, fallback map[string]interface{}, key string) interface{} {
	if val, ok := primary[key]; ok {
		return val
	}
	if fallback != nil {
		if val, ok := fallback[key]; ok {
			return val
		}
	}
	return nil
}
