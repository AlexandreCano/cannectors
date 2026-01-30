// Package registry provides module registries for the Cannectors runtime.
// This file registers all built-in modules during initialization.
package registry

import (
	"fmt"

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
	})

	// condition - Conditional routing filter module
	// IMPORTANT: This registry entry provides basic condition support without nested modules.
	// For full support including nested then/else blocks, use factory.CreateFilterModules()
	// which calls factory.createConditionFilterModule() with complete parsing via
	// factory.ParseConditionConfig(). The factory path is the preferred and recommended
	// approach for condition modules.
	//
	// This registry entry exists for API completeness and simple condition cases.
	// It does NOT parse nested then/else modules - those will be ignored.
	RegisterFilter("condition", func(cfg connector.ModuleConfig, index int) (filter.Module, error) {
		// Basic parsing without nested module support
		condConfig := filter.ConditionConfig{}
		expr, ok := cfg.Config["expression"].(string)
		if !ok || expr == "" {
			return nil, fmt.Errorf("required field 'expression' is missing or empty in condition config at index %d", index)
		}
		condConfig.Expression = expr

		if lang, ok := cfg.Config["lang"].(string); ok {
			condConfig.Lang = lang
		}
		if onTrue, ok := cfg.Config["onTrue"].(string); ok {
			condConfig.OnTrue = onTrue
		}
		if onFalse, ok := cfg.Config["onFalse"].(string); ok {
			condConfig.OnFalse = onFalse
		}
		if onError, ok := cfg.Config["onError"].(string); ok {
			condConfig.OnError = onError
		}

		// Note: Nested then/else modules are NOT parsed here.
		// Use factory.CreateFilterModules() for full condition support with nested modules.
		return filter.NewConditionFromConfig(condConfig)
	})

	// script - JavaScript transformation filter module using Goja
	RegisterFilter("script", func(cfg connector.ModuleConfig, index int) (filter.Module, error) {
		scriptConfig, err := filter.ParseScriptConfig(cfg.Config)
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
		httpCallConfig, err := filter.ParseHTTPCallConfig(cfg.Config, cfg.Authentication)
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
		sqlCallConfig, err := filter.ParseSQLCallConfig(cfg.Config)
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
		setConfig, err := filter.ParseSetConfig(cfg.Config)
		if err != nil {
			return nil, fmt.Errorf("invalid set config at index %d: %w", index, err)
		}
		module, err := filter.NewSetFromConfig(setConfig)
		if err != nil {
			return nil, fmt.Errorf("invalid set config at index %d: %w", index, err)
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
