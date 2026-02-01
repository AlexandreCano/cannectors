// Package filter provides implementations for filter modules.
// This file implements the "set" filter module for setting or modifying record fields.
//
// The set filter mutates each record in place (same map reference is returned).
// Other filter modules (e.g. mapping) may create a copy; this design choice is intentional
// so that the pipeline receives the same record with the field set or updated.
package filter

import (
	"context"
	"errors"
	"fmt"

	"github.com/cannectors/runtime/internal/logger"
)

// SetConfig represents the configuration for a set filter module.
type SetConfig struct {
	// Target is the field path to set (supports dot notation for nested paths)
	Target string `json:"target"`
	// Value is the literal value to set
	Value interface{} `json:"value"`
}

// SetModule implements the set filter that sets or modifies a single field on each record.
type SetModule struct {
	config SetConfig
}

// NewSetFromConfig creates a new set filter module from configuration.
// It validates Target only; for full validation (including required value), use ParseSetConfig
// which is used by the registry when building modules from pipeline config.
func NewSetFromConfig(config SetConfig) (*SetModule, error) {
	if config.Target == "" {
		return nil, errors.New("target field path is required")
	}

	logger.Debug("set filter module initialized", "target", config.Target)

	return &SetModule{config: config}, nil
}

// Process implements the filter.Module interface.
// It sets or modifies a single field on each record.
func (m *SetModule) Process(ctx context.Context, records []map[string]interface{}) ([]map[string]interface{}, error) {
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

		processed, err := m.processRecord(record)
		if err != nil {
			return nil, fmt.Errorf("processing record %d: %w", i, err)
		}
		result = append(result, processed)
	}

	return result, nil
}

// processRecord processes a single record and returns the modified record.
func (m *SetModule) processRecord(record map[string]interface{}) (map[string]interface{}, error) {
	target := m.config.Target
	value := m.config.Value

	// Simple case: no dot notation (flat field)
	if !IsNestedPath(target) {
		record[target] = value
		return record, nil
	}

	// Nested path - always create intermediate objects
	if err := SetNestedValue(record, target, value); err != nil {
		return nil, err
	}
	return record, nil
}

// ParseSetConfig parses a raw configuration map into SetConfig.
func ParseSetConfig(config map[string]interface{}) (SetConfig, error) {
	var cfg SetConfig

	// Parse and validate target (required)
	target, ok := config["target"].(string)
	if !ok || target == "" {
		return cfg, errors.New("'target' is required and must be a non-empty string")
	}
	cfg.Target = target

	// Check if value key exists (including nil values)
	if _, hasValue := config["value"]; !hasValue {
		return cfg, errors.New("'value' is required")
	}
	cfg.Value = config["value"]

	return cfg, nil
}

// Verify interface compliance at compile time
var _ Module = (*SetModule)(nil)
