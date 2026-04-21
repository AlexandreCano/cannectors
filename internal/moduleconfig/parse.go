// Package moduleconfig provides generic configuration parsing for pipeline modules.
// It centralizes the deserialization of module configs from JSON raw data into typed Go structs.
package moduleconfig

import (
	"encoding/json"
	"fmt"

	"github.com/cannectors/runtime/pkg/connector"
)

// ParseConfig deserializes JSON raw data into a typed Go struct.
// This is the single entry point for all module config parsing.
// Modules call ParseConfig[T](raw) in their constructor to get a fully typed config.
func ParseConfig[T any](raw json.RawMessage) (T, error) {
	var cfg T
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("parsing module config: %w", err)
	}
	return cfg, nil
}

// ParseModuleConfig deserializes a ModuleConfig's Raw field into a typed Go struct.
func ParseModuleConfig[T any](mc connector.ModuleConfig) (T, error) {
	if mc.Raw == nil {
		var zero T
		return zero, fmt.Errorf("no config data available (Raw is nil)")
	}
	return ParseConfig[T](mc.Raw)
}

// ToRetryConfig dereferences a *connector.RetryConfig pointer and applies defaults
// for zero/nil values. Returns DefaultRetryConfig if rc is nil.
func ToRetryConfig(rc *connector.RetryConfig) connector.RetryConfig {
	if rc == nil {
		return connector.DefaultRetryConfig()
	}
	defaults := connector.DefaultRetryConfig()
	cfg := *rc
	if cfg.DelayMs == 0 {
		cfg.DelayMs = defaults.DelayMs
	}
	if cfg.BackoffMultiplier == 0 {
		cfg.BackoffMultiplier = defaults.BackoffMultiplier
	}
	if cfg.MaxDelayMs == 0 {
		cfg.MaxDelayMs = defaults.MaxDelayMs
	}
	if cfg.RetryableStatusCodes == nil {
		cfg.RetryableStatusCodes = defaults.RetryableStatusCodes
	}
	return cfg
}
