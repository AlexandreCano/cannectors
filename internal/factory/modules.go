// Package factory provides module creation functions for the pipeline runtime.
// It centralizes the logic for instantiating input, filter, and output modules
// from their configuration using the module registry.
//
// # Module Creation
//
// The factory uses the registry package to look up module constructors by type.
// Built-in modules (httpPolling, webhook, mapping, condition, httpRequest) are
// registered automatically at startup. Unknown types resolve to stub implementations.
//
// # Adding New Module Types
//
// To add a new module type, see the documentation in internal/registry.
// You do NOT need to modify this factory; just register your constructor.
package factory

import (
	"encoding/json"
	"fmt"

	"github.com/cannectors/runtime/internal/moduleconfig"
	"github.com/cannectors/runtime/internal/modules/filter"
	"github.com/cannectors/runtime/internal/modules/input"
	"github.com/cannectors/runtime/internal/modules/output"
	"github.com/cannectors/runtime/internal/registry"
	"github.com/cannectors/runtime/pkg/connector"
)

func init() {
	// Initialize the nested module creator in the filter package to enable
	// registry-based module creation in nested condition blocks.
	filter.NestedModuleCreator = CreateFilterModuleFromNestedConfig
}

// CreateInputModule creates an input module instance from configuration.
// Uses the registry to look up the constructor by type.
// Returns a stub module for unregistered types.
// Returns an error if the registered constructor fails or if configuration is invalid.
func CreateInputModule(cfg *connector.ModuleConfig) (input.Module, error) {
	if cfg == nil {
		return nil, nil
	}

	constructor := registry.GetInputConstructor(cfg.Type)
	if constructor != nil {
		return constructor(cfg)
	}

	// Error for unregistered types
	return nil, fmt.Errorf("unknown input module type %q: supported types are %v", cfg.Type, registry.ListInputTypes())
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
// Uses the registry to look up the constructor, with special handling for
// condition modules that require config parsing.
func createSingleFilterModule(cfg connector.ModuleConfig, index int) (filter.Module, error) {
	// Special handling for condition modules due to nested config parsing
	if cfg.Type == "condition" {
		return createConditionFilterModule(cfg, index)
	}

	constructor := registry.GetFilterConstructor(cfg.Type)
	if constructor != nil {
		return constructor(cfg, index)
	}

	// Error for unregistered types
	return nil, fmt.Errorf("unknown filter module type %q at index %d: supported types are %v", cfg.Type, index, registry.ListFilterTypes())
}

// createConditionFilterModule creates a condition filter module from configuration.
func createConditionFilterModule(cfg connector.ModuleConfig, index int) (filter.Module, error) {
	condConfig, err := moduleconfig.ParseModuleConfig[filter.ConditionConfig](cfg)
	if err != nil {
		return nil, fmt.Errorf("invalid condition config at index %d: %w", index, err)
	}
	if condConfig.Expression == "" {
		return nil, fmt.Errorf("invalid condition config at index %d: required field 'expression' is missing or empty in condition config", index)
	}

	module, err := filter.NewConditionFromConfig(condConfig)
	if err != nil {
		return nil, fmt.Errorf("invalid condition config at index %d: %w", index, err)
	}

	return module, nil
}

// CreateOutputModule creates an output module instance from configuration.
// Uses the registry to look up the constructor by type.
// Returns a stub module for unregistered types.
func CreateOutputModule(cfg *connector.ModuleConfig) (output.Module, error) {
	if cfg == nil {
		return nil, nil
	}

	constructor := registry.GetOutputConstructor(cfg.Type)
	if constructor != nil {
		return constructor(cfg)
	}

	// Error for unregistered types
	return nil, fmt.Errorf("unknown output module type %q: supported types are %v", cfg.Type, registry.ListOutputTypes())
}

// CreateFilterModuleFromNestedConfig creates a filter module from a NestedModuleConfig
// using the registry. This enables custom filter types to work in nested condition blocks.
func CreateFilterModuleFromNestedConfig(nestedConfig *filter.NestedModuleConfig, index int) (filter.Module, error) {
	if nestedConfig == nil {
		return nil, nil
	}

	// Special handling for condition modules due to nested config structure
	if nestedConfig.Type == "condition" {
		condConfig := filter.ConditionConfig{
			ModuleBase: connector.ModuleBase{OnError: nestedConfig.OnError},
			Expression: nestedConfig.Expression,
			Lang:       nestedConfig.Lang,
			OnTrue:     nestedConfig.OnTrue,
			OnFalse:    nestedConfig.OnFalse,
			Then:       nestedConfig.Then,
			Else:       nestedConfig.Else,
		}
		return filter.NewConditionFromConfig(condConfig)
	}

	// Build a raw config map from NestedModuleConfig
	rawMap := make(map[string]interface{})
	if nestedConfig.Config != nil {
		for k, v := range nestedConfig.Config {
			rawMap[k] = v
		}
	}

	// Add mappings to config if present (for mapping modules)
	if len(nestedConfig.Mappings) > 0 {
		mappings := make([]interface{}, len(nestedConfig.Mappings))
		for i, m := range nestedConfig.Mappings {
			mappings[i] = map[string]interface{}{
				"source": m.Source,
				"target": m.Target,
			}
		}
		rawMap["mappings"] = mappings
	}

	// Add onError to config if present
	if nestedConfig.OnError != "" {
		rawMap["onError"] = nestedConfig.OnError
	}

	raw, err := json.Marshal(rawMap)
	if err != nil {
		return nil, fmt.Errorf("marshaling nested config for %q: %w", nestedConfig.Type, err)
	}

	moduleConfig := connector.ModuleConfig{
		Type: nestedConfig.Type,
		Raw:  raw,
	}

	// Use registry to create the module
	constructor := registry.GetFilterConstructor(moduleConfig.Type)
	if constructor != nil {
		return constructor(moduleConfig, index)
	}

	// Error for unregistered types
	return nil, fmt.Errorf("unknown filter module type %q in nested config: supported types are %v", moduleConfig.Type, registry.ListFilterTypes())
}
