// Package registry provides module registries for the Cannectors runtime.
// This file registers all built-in modules during initialization.
package registry

import (
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
	// IMPORTANT: This registry entry provides basic condition support without nested modules.
	// For full support including nested then/else blocks, use factory.CreateFilterModules()
	// which calls factory.createConditionFilterModule() with complete parsing.
	RegisterFilter("condition", func(cfg connector.ModuleConfig, index int) (filter.Module, error) {
		condConfig, err := moduleconfig.ParseModuleConfig[filter.ConditionConfig](cfg)
		if err != nil {
			return nil, fmt.Errorf("invalid condition config at index %d: %w", index, err)
		}
		if condConfig.Expression == "" {
			return nil, fmt.Errorf("required field 'expression' is missing or empty in condition config at index %d", index)
		}
		// Note: Nested then/else modules are NOT parsed here.
		// Use factory.CreateFilterModules() for full condition support with nested modules.
		return filter.NewConditionFromConfig(condConfig)
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
