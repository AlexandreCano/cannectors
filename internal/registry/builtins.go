// Package registry provides module registries for the Canectors runtime.
// This file registers all built-in modules during initialization.
package registry

import (
	"fmt"
	"log/slog"

	"github.com/canectors/runtime/internal/logger"
	"github.com/canectors/runtime/internal/modules/filter"
	"github.com/canectors/runtime/internal/modules/input"
	"github.com/canectors/runtime/internal/modules/output"
	"github.com/canectors/runtime/pkg/connector"
)

func init() {
	registerBuiltinInputModules()
	registerBuiltinFilterModules()
	registerBuiltinOutputModules()
}

// registerBuiltinInputModules registers all built-in input module types.
func registerBuiltinInputModules() {
	// httpPolling - HTTP polling input module
	RegisterInput("httpPolling", func(cfg *connector.ModuleConfig) input.Module {
		if cfg == nil {
			return nil
		}
		module, err := input.NewHTTPPollingFromConfig(cfg)
		if err != nil {
			// Log error but still return stub to allow pipeline to continue
			logger.Error("failed to create httpPolling module, falling back to stub",
				slog.String("module_type", cfg.Type),
				slog.String("error", err.Error()),
			)
			endpoint, _ := cfg.Config["endpoint"].(string)
			return input.NewStub(cfg.Type, endpoint)
		}
		return module
	})

	// webhook - Webhook input module
	RegisterInput("webhook", func(cfg *connector.ModuleConfig) input.Module {
		if cfg == nil {
			return nil
		}
		module, err := input.NewWebhookFromConfig(cfg)
		if err != nil {
			// Log error but still return stub to allow pipeline to continue
			logger.Error("failed to create webhook module, falling back to stub",
				slog.String("module_type", cfg.Type),
				slog.String("error", err.Error()),
			)
			endpoint, _ := cfg.Config["endpoint"].(string)
			return input.NewStub(cfg.Type, endpoint)
		}
		return module
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
	// Note: The factory's createConditionFilterModule handles condition modules
	// with special parsing for nested modules. This registry entry provides
	// a fallback implementation that uses the same parsing logic.
	RegisterFilter("condition", func(cfg connector.ModuleConfig, index int) (filter.Module, error) {
		// Parse condition config (supports nested modules)
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

		// Note: Nested then/else modules are not parsed here as they require
		// factory.ParseConditionConfig for full support. This constructor
		// is primarily for registry completeness; the factory's
		// createConditionFilterModule is the preferred path.
		return filter.NewConditionFromConfig(condConfig)
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
}
