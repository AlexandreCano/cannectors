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
	"fmt"

	"github.com/cannectors/runtime/internal/modules/filter"
	"github.com/cannectors/runtime/internal/modules/input"
	"github.com/cannectors/runtime/internal/modules/output"
	"github.com/cannectors/runtime/internal/registry"
	"github.com/cannectors/runtime/pkg/connector"
)

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

// createSingleFilterModule creates a single filter module by looking up the
// registered constructor for its type. The condition filter is registered
// like any other type and resolves its nested then/else modules via the
// registry — no special-casing needed here.
func createSingleFilterModule(cfg connector.ModuleConfig, index int) (filter.Module, error) {
	constructor := registry.GetFilterConstructor(cfg.Type)
	if constructor != nil {
		return constructor(cfg, index)
	}

	return nil, fmt.Errorf("unknown filter module type %q at index %d: supported types are %v", cfg.Type, index, registry.ListFilterTypes())
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
