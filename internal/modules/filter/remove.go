// Package filter provides implementations for filter modules.
// This file implements the "remove" filter module for removing fields from records.
//
// The remove filter mutates each record in place (same map reference is returned).
// If the target field does not exist, the record is left unchanged (no error).
package filter

import (
	"context"
	"errors"

	"github.com/cannectors/runtime/internal/logger"
)

// RemoveConfig represents the configuration for a remove filter module.
type RemoveConfig struct {
	// Target is the field path to remove (supports dot notation for nested paths)
	Target string `json:"target"`
	// Targets is a list of field paths to remove (supports dot notation for nested paths)
	Targets []string `json:"targets"`
}

// RemoveModule implements the remove filter that removes fields from each record.
type RemoveModule struct {
	targets []string
}

// NewRemoveFromConfig creates a new remove filter module from configuration.
// It validates that at least one target is provided (either via Target or Targets).
func NewRemoveFromConfig(config RemoveConfig) (*RemoveModule, error) {
	targets := config.Targets

	// Support single target for backward compatibility
	if config.Target != "" {
		targets = append(targets, config.Target)
	}

	if len(targets) == 0 {
		return nil, errors.New("at least one target field path is required")
	}

	// Remove duplicates while preserving order
	seen := make(map[string]bool)
	uniqueTargets := make([]string, 0, len(targets))
	for _, t := range targets {
		if t != "" && !seen[t] {
			seen[t] = true
			uniqueTargets = append(uniqueTargets, t)
		}
	}

	if len(uniqueTargets) == 0 {
		return nil, errors.New("at least one non-empty target field path is required")
	}

	logger.Debug("remove filter module initialized", "targets", uniqueTargets)

	return &RemoveModule{targets: uniqueTargets}, nil
}

// Process implements the filter.Module interface.
// It removes the configured fields from each record.
func (m *RemoveModule) Process(ctx context.Context, records []map[string]interface{}) ([]map[string]interface{}, error) {
	// Respect context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if len(records) == 0 {
		return records, nil
	}

	result := make([]map[string]interface{}, 0, len(records))

	for i, record := range records {
		// Check context for long-running operations
		if i > 0 && i%100 == 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}

		processed := m.processRecord(record)
		result = append(result, processed)
	}

	return result, nil
}

// processRecord processes a single record and returns the modified record.
func (m *RemoveModule) processRecord(record map[string]interface{}) map[string]interface{} {
	if record == nil {
		return record
	}
	for _, target := range m.targets {
		// Simple case: no dot notation (flat field)
		if !IsNestedPath(target) {
			delete(record, target)
		} else {
			// Nested path - delete the leaf field
			DeleteNestedValue(record, target)
		}
	}
	return record
}

// ParseRemoveConfig parses a raw configuration map into RemoveConfig.
func ParseRemoveConfig(config map[string]interface{}) (RemoveConfig, error) {
	var cfg RemoveConfig

	// Parse single target (backward compatibility)
	if target, ok := config["target"].(string); ok && target != "" {
		cfg.Target = target
	}

	// Parse targets array
	if targets, ok := config["targets"]; ok {
		switch v := targets.(type) {
		case []interface{}:
			cfg.Targets = make([]string, 0, len(v))
			for _, item := range v {
				if s, ok := item.(string); ok && s != "" {
					cfg.Targets = append(cfg.Targets, s)
				}
			}
		case []string:
			cfg.Targets = v
		}
	}

	// Validate at least one target is provided
	if cfg.Target == "" && len(cfg.Targets) == 0 {
		return cfg, errors.New("'target' or 'targets' is required")
	}

	return cfg, nil
}

// Verify interface compliance at compile time
var _ Module = (*RemoveModule)(nil)
