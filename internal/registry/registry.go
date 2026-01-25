// Package registry provides module registries for input, filter, and output modules.
//
// # Overview
//
// The registry package enables extensible module registration for the Canectors runtime.
// Instead of hard-coded switch statements, modules register their constructors by type string.
// This allows contributors to add new module types without modifying core factory code.
//
// # Adding a New Module
//
// To add a new module type (e.g., a "kafka" input module):
//
//  1. Implement the appropriate interface (input.Module, filter.Module, or output.Module)
//  2. Create a constructor function matching the registry signature
//  3. Register the constructor in an init() function
//
// Example for a new input module:
//
//	package kafka
//
//	import (
//	    "github.com/canectors/runtime/internal/modules/input"
//	    "github.com/canectors/runtime/internal/registry"
//	    "github.com/canectors/runtime/pkg/connector"
//	)
//
//	func init() {
//	    registry.RegisterInput("kafka", NewKafkaModule)
//	}
//
//	func NewKafkaModule(cfg *connector.ModuleConfig) input.Module {
//	    // Parse cfg.Config and return your implementation
//	    return &KafkaModule{...}
//	}
//
// # Built-in Modules
//
// Built-in modules (httpPolling, webhook, mapping, condition, httpRequest) are
// registered automatically at runtime startup via init() functions.
//
// # Stub Fallback
//
// Unknown types resolve to stub implementations that log their invocation
// and return sample data. This allows pipelines to run even with unimplemented
// module types (useful for testing and development).
package registry

import (
	"sync"

	"github.com/canectors/runtime/internal/modules/filter"
	"github.com/canectors/runtime/internal/modules/input"
	"github.com/canectors/runtime/internal/modules/output"
	"github.com/canectors/runtime/pkg/connector"
)

// InputConstructor is a function that creates an input module from configuration.
// The constructor receives the full ModuleConfig and returns an input.Module.
// Return nil if the configuration is nil.
type InputConstructor func(cfg *connector.ModuleConfig) input.Module

// FilterConstructor is a function that creates a filter module from configuration.
// The constructor receives the ModuleConfig and the filter's index in the pipeline.
// Returns an error if the configuration is invalid.
type FilterConstructor func(cfg connector.ModuleConfig, index int) (filter.Module, error)

// OutputConstructor is a function that creates an output module from configuration.
// The constructor receives the full ModuleConfig and returns an output.Module.
// Returns an error if the configuration is invalid.
type OutputConstructor func(cfg *connector.ModuleConfig) (output.Module, error)

// inputRegistry holds registered input module constructors.
var (
	inputMu       sync.RWMutex
	inputRegistry = make(map[string]InputConstructor)
)

// filterRegistry holds registered filter module constructors.
var (
	filterMu       sync.RWMutex
	filterRegistry = make(map[string]FilterConstructor)
)

// outputRegistry holds registered output module constructors.
var (
	outputMu       sync.RWMutex
	outputRegistry = make(map[string]OutputConstructor)
)

// RegisterInput registers an input module constructor by type string.
// Calling RegisterInput with an already registered type will overwrite
// the previous constructor.
//
// This function is safe for concurrent use and is typically called from
// init() functions in module packages.
//
// Example:
//
//	func init() {
//	    registry.RegisterInput("httpPolling", NewHTTPPollingModule)
//	}
func RegisterInput(moduleType string, constructor InputConstructor) {
	inputMu.Lock()
	defer inputMu.Unlock()
	inputRegistry[moduleType] = constructor
}

// RegisterFilter registers a filter module constructor by type string.
// Calling RegisterFilter with an already registered type will overwrite
// the previous constructor.
//
// This function is safe for concurrent use and is typically called from
// init() functions in module packages.
//
// Example:
//
//	func init() {
//	    registry.RegisterFilter("mapping", NewMappingModule)
//	}
func RegisterFilter(moduleType string, constructor FilterConstructor) {
	filterMu.Lock()
	defer filterMu.Unlock()
	filterRegistry[moduleType] = constructor
}

// RegisterOutput registers an output module constructor by type string.
// Calling RegisterOutput with an already registered type will overwrite
// the previous constructor.
//
// This function is safe for concurrent use and is typically called from
// init() functions in module packages.
//
// Example:
//
//	func init() {
//	    registry.RegisterOutput("httpRequest", NewHTTPRequestModule)
//	}
func RegisterOutput(moduleType string, constructor OutputConstructor) {
	outputMu.Lock()
	defer outputMu.Unlock()
	outputRegistry[moduleType] = constructor
}

// GetInputConstructor returns the registered constructor for an input module type.
// Returns nil if no constructor is registered for the given type.
func GetInputConstructor(moduleType string) InputConstructor {
	inputMu.RLock()
	defer inputMu.RUnlock()
	return inputRegistry[moduleType]
}

// GetFilterConstructor returns the registered constructor for a filter module type.
// Returns nil if no constructor is registered for the given type.
func GetFilterConstructor(moduleType string) FilterConstructor {
	filterMu.RLock()
	defer filterMu.RUnlock()
	return filterRegistry[moduleType]
}

// GetOutputConstructor returns the registered constructor for an output module type.
// Returns nil if no constructor is registered for the given type.
func GetOutputConstructor(moduleType string) OutputConstructor {
	outputMu.RLock()
	defer outputMu.RUnlock()
	return outputRegistry[moduleType]
}

// ListInputTypes returns all registered input module type names.
// Useful for documentation and debugging.
func ListInputTypes() []string {
	inputMu.RLock()
	defer inputMu.RUnlock()
	types := make([]string, 0, len(inputRegistry))
	for t := range inputRegistry {
		types = append(types, t)
	}
	return types
}

// ListFilterTypes returns all registered filter module type names.
// Useful for documentation and debugging.
func ListFilterTypes() []string {
	filterMu.RLock()
	defer filterMu.RUnlock()
	types := make([]string, 0, len(filterRegistry))
	for t := range filterRegistry {
		types = append(types, t)
	}
	return types
}

// ListOutputTypes returns all registered output module type names.
// Useful for documentation and debugging.
func ListOutputTypes() []string {
	outputMu.RLock()
	defer outputMu.RUnlock()
	types := make([]string, 0, len(outputRegistry))
	for t := range outputRegistry {
		types = append(types, t)
	}
	return types
}

// ClearRegistries removes all registered constructors.
// This is intended for testing purposes only.
func ClearRegistries() {
	inputMu.Lock()
	inputRegistry = make(map[string]InputConstructor)
	inputMu.Unlock()

	filterMu.Lock()
	filterRegistry = make(map[string]FilterConstructor)
	filterMu.Unlock()

	outputMu.Lock()
	outputRegistry = make(map[string]OutputConstructor)
	outputMu.Unlock()
}
