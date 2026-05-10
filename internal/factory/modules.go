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

	"github.com/cannectors/runtime/internal/logger"
	"github.com/cannectors/runtime/internal/modules/filter"
	"github.com/cannectors/runtime/internal/modules/input"
	"github.com/cannectors/runtime/internal/modules/output"
	"github.com/cannectors/runtime/internal/registry"
	"github.com/cannectors/runtime/pkg/connector"
)

// moduleEnabled returns the value of the optional `enabled` field in a module
// configuration. It defaults to true when the field is absent or the raw
// payload cannot be parsed (the latter is benign here — schema validation
// runs before the factory and would have surfaced the error already).
func moduleEnabled(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return true
	}
	var probe struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return true
	}
	if probe.Enabled == nil {
		return true
	}
	return *probe.Enabled
}

// CreateInputModule creates an input module instance from configuration.
// Uses the registry to look up the constructor by type.
// Returns a stub module for unregistered types.
// Returns an error if the registered constructor fails or if configuration is invalid.
func CreateInputModule(cfg *connector.ModuleConfig) (input.Module, error) {
	if cfg == nil {
		return nil, nil
	}

	if !moduleEnabled(cfg.Raw) {
		return nil, fmt.Errorf("input module %q has enabled=false: input is required for the pipeline to run, remove it from the config or set enabled=true", cfg.Type)
	}

	constructor := registry.GetInputConstructor(cfg.Type)
	if constructor != nil {
		return constructor(cfg)
	}

	// Error for unregistered types
	return nil, fmt.Errorf("unknown input module type %q: supported types are %v", cfg.Type, registry.ListInputTypes())
}

// CreateFilterModules creates filter module instances from configuration.
// Filters with `enabled: false` are skipped entirely (not constructed, not run).
func CreateFilterModules(cfgs []connector.ModuleConfig) ([]filter.Module, error) {
	if len(cfgs) == 0 {
		return nil, nil
	}

	modules := make([]filter.Module, 0, len(cfgs))
	for i, cfg := range cfgs {
		if !moduleEnabled(cfg.Raw) {
			logger.Debug("skipping disabled filter module",
				"index", i,
				"type", cfg.Type,
			)
			continue
		}
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

	if !moduleEnabled(cfg.Raw) {
		return nil, fmt.Errorf("output module %q has enabled=false: output is required for the pipeline to run, remove it from the config or set enabled=true", cfg.Type)
	}

	constructor := registry.GetOutputConstructor(cfg.Type)
	if constructor != nil {
		return constructor(cfg)
	}

	// Error for unregistered types
	return nil, fmt.Errorf("unknown output module type %q: supported types are %v", cfg.Type, registry.ListOutputTypes())
}
