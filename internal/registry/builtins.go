// Package registry provides module registries for the Cannectors runtime.
// This file registers all built-in modules during initialization.
package registry

import (
	"encoding/json"
	"fmt"

	"github.com/cannectors/runtime/internal/moduleconfig"
	"github.com/cannectors/runtime/internal/modules/filter"
	"github.com/cannectors/runtime/internal/modules/input"
	"github.com/cannectors/runtime/internal/modules/output"
	"github.com/cannectors/runtime/pkg/connector"
)

func init() {
	registerBuiltinInputModules()
	registerBuiltinFilterModules()
	registerBuiltinOutputModules()
}

// registerBuiltinInputModules registers all built-in input module types.
func registerBuiltinInputModules() {
	// httpPolling - HTTP polling input module
	RegisterInput("httpPolling", func(cfg *connector.ModuleConfig) (input.Module, error) {
		if cfg == nil {
			return nil, nil
		}
		return input.NewHTTPPollingFromConfig(cfg)
	})

	// webhook - Webhook input module
	RegisterInput("webhook", func(cfg *connector.ModuleConfig) (input.Module, error) {
		if cfg == nil {
			return nil, nil
		}
		return input.NewWebhookFromConfig(cfg)
	})

	// database - Database input module for SQL queries
	RegisterInput("database", func(cfg *connector.ModuleConfig) (input.Module, error) {
		if cfg == nil {
			return nil, nil
		}
		return input.NewDatabaseInputFromConfig(cfg)
	})
}

// registerBuiltinFilterModules registers all built-in filter module types.
func registerBuiltinFilterModules() {
	// mapping - Field mapping filter module
	RegisterFilter("mapping", func(cfg connector.ModuleConfig, index int) (filter.Module, error) {
		mapCfg, err := moduleconfig.ParseModuleConfig[filter.MappingModuleConfig](cfg)
		if err != nil {
			return nil, fmt.Errorf("invalid mapping config at index %d: %w", index, err)
		}
		module, err := filter.NewMappingFromConfig(mapCfg.Mappings, mapCfg.OnError)
		if err != nil {
			return nil, fmt.Errorf("invalid mapping config at index %d: %w", index, err)
		}
		return module, nil
	})

	// condition - Conditional routing filter module
	//
	// Nested then/else modules are resolved via the registry through
	// resolveNestedFilter, which serializes the nested config and dispatches
	// to the appropriate registered constructor. This means custom filter
	// types registered via RegisterFilter work transparently in nested blocks.
	RegisterFilter("condition", func(cfg connector.ModuleConfig, index int) (filter.Module, error) {
		condConfig, err := moduleconfig.ParseModuleConfig[filter.ConditionConfig](cfg)
		if err != nil {
			return nil, fmt.Errorf("invalid condition config at index %d: %w", index, err)
		}
		if condConfig.Expression == "" {
			return nil, fmt.Errorf("required field 'expression' is missing or empty in condition config at index %d", index)
		}
		return filter.NewConditionFromConfig(condConfig, resolveNestedFilter)
	})

	// script - JavaScript transformation filter module using Goja
	RegisterFilter("script", func(cfg connector.ModuleConfig, index int) (filter.Module, error) {
		scriptConfig, err := moduleconfig.ParseModuleConfig[filter.ScriptConfig](cfg)
		if err != nil {
			return nil, fmt.Errorf("invalid script config at index %d: %w", index, err)
		}
		module, err := filter.NewScriptFromConfig(scriptConfig)
		if err != nil {
			return nil, fmt.Errorf("invalid script config at index %d: %w", index, err)
		}
		return module, nil
	})

	// http_call - HTTP call filter module with HTTP requests and caching
	RegisterFilter("http_call", func(cfg connector.ModuleConfig, index int) (filter.Module, error) {
		httpCallConfig, err := moduleconfig.ParseModuleConfig[filter.HTTPCallConfig](cfg)
		if err != nil {
			return nil, fmt.Errorf("invalid http_call config at index %d: %w", index, err)
		}
		module, err := filter.NewHTTPCallFromConfig(httpCallConfig)
		if err != nil {
			return nil, fmt.Errorf("invalid http_call config at index %d: %w", index, err)
		}
		return module, nil
	})

	// sql_call - SQL call filter module with database queries and caching
	RegisterFilter("sql_call", func(cfg connector.ModuleConfig, index int) (filter.Module, error) {
		sqlCallConfig, err := moduleconfig.ParseModuleConfig[filter.SQLCallConfig](cfg)
		if err != nil {
			return nil, fmt.Errorf("invalid sql_call config at index %d: %w", index, err)
		}
		module, err := filter.NewSQLCallFromConfig(sqlCallConfig)
		if err != nil {
			return nil, fmt.Errorf("invalid sql_call config at index %d: %w", index, err)
		}
		return module, nil
	})

	// set - Set or modify a single field on each record
	RegisterFilter("set", func(cfg connector.ModuleConfig, index int) (filter.Module, error) {
		setConfig, err := moduleconfig.ParseModuleConfig[filter.SetConfig](cfg)
		if err != nil {
			return nil, fmt.Errorf("invalid set config at index %d: %w", index, err)
		}
		module, err := filter.NewSetFromConfig(setConfig)
		if err != nil {
			return nil, fmt.Errorf("invalid set config at index %d: %w", index, err)
		}
		return module, nil
	})

	// remove - Remove one or more fields from each record
	RegisterFilter("remove", func(cfg connector.ModuleConfig, index int) (filter.Module, error) {
		removeConfig, err := moduleconfig.ParseModuleConfig[filter.RemoveConfig](cfg)
		if err != nil {
			return nil, fmt.Errorf("invalid remove config at index %d: %w", index, err)
		}
		module, err := filter.NewRemoveFromConfig(removeConfig)
		if err != nil {
			return nil, fmt.Errorf("invalid remove config at index %d: %w", index, err)
		}
		return module, nil
	})
}

// resolveNestedFilter resolves a NestedModuleConfig by serializing it back to
// JSON and dispatching to the registered filter constructor for the type.
// This is the closure injected into condition modules so nested then/else
// blocks support every registered filter type — including custom ones —
// without the filter package needing to depend on the registry.
func resolveNestedFilter(nestedConfig *filter.NestedModuleConfig, index int) (filter.Module, error) {
	if nestedConfig == nil {
		return nil, nil
	}

	rawMap := make(map[string]interface{})
	for k, v := range nestedConfig.Config {
		rawMap[k] = v
	}

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

	if nestedConfig.OnError != "" {
		rawMap["onError"] = nestedConfig.OnError
	}

	raw, err := json.Marshal(rawMap)
	if err != nil {
		return nil, fmt.Errorf("marshaling nested config for %q: %w", nestedConfig.Type, err)
	}

	moduleConfig := connector.ModuleConfig{Type: nestedConfig.Type, Raw: raw}
	constructor := GetFilterConstructor(moduleConfig.Type)
	if constructor == nil {
		return nil, fmt.Errorf("unknown filter module type %q in nested config: supported types are %v", moduleConfig.Type, ListFilterTypes())
	}
	return constructor(moduleConfig, index)
}

// registerBuiltinOutputModules registers all built-in output module types.
func registerBuiltinOutputModules() {
	// httpRequest - HTTP request output module
	RegisterOutput("httpRequest", func(cfg *connector.ModuleConfig) (output.Module, error) {
		if cfg == nil {
			return nil, nil
		}
		return output.NewHTTPRequestFromConfig(cfg)
	})

	// database - Database output module for SQL operations
	RegisterOutput("database", func(cfg *connector.ModuleConfig) (output.Module, error) {
		if cfg == nil {
			return nil, nil
		}
		return output.NewDatabaseOutputFromConfig(cfg)
	})
}
