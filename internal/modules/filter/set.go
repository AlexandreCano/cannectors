// Package filter provides implementations for filter modules.
// This file implements the "set" filter module for setting or modifying record fields.
//
// The set filter mutates each record in place (same map reference is returned).
// Other filter modules (e.g. mapping) may create a copy; this design choice is intentional
// so that the pipeline receives the same record with the field set or updated.
package filter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/cannectors/runtime/internal/errhandling"
	"github.com/cannectors/runtime/internal/logger"
	"github.com/cannectors/runtime/internal/recordpath"
	"github.com/cannectors/runtime/pkg/connector"
)

// SetConfig represents the configuration for a set filter module.
type SetConfig struct {
	connector.ModuleBase
	// Target is the field path to set (supports dot notation for nested paths)
	Target string `json:"target"`
	// Value is the literal value to set
	Value any `json:"value"`
	// HasValue tracks whether the JSON `value` key was present (including
	// explicit null) so the runtime can distinguish "absent" — which the
	// schema rejects — from "present and null" — which the runtime accepts
	// and stores as a real nil.
	HasValue bool `json:"-"`
}

// UnmarshalJSON tracks whether the `value` key was provided. Without this
// probe a payload like {"target":"x"} would be indistinguishable from
// {"target":"x","value":null}, both leaving Value at its zero value (nil).
func (c *SetConfig) UnmarshalJSON(data []byte) error {
	type alias SetConfig
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*c = SetConfig(a)
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return err
	}
	_, c.HasValue = probe["value"]
	return nil
}

// SetModule implements the set filter that sets or modifies a single field on each record.
type SetModule struct {
	config SetConfig
}

// NewSetFromConfig creates a new set filter module from configuration.
// Validates Target and the explicit presence of Value (null is accepted).
func NewSetFromConfig(config SetConfig) (*SetModule, error) {
	if config.Target == "" {
		return nil, errors.New("target field path is required")
	}
	if !config.HasValue {
		return nil, errors.New("'value' is required (use null to set the field to null)")
	}

	if _, err := errhandling.ParseOnErrorStrategy(config.OnError); err != nil {
		return nil, err
	}

	logger.Debug("set filter module initialized", "target", config.Target)

	return &SetModule{config: config}, nil
}

// Process implements the filter.Module interface.
// It sets or modifies a single field on each record.
func (m *SetModule) Process(ctx context.Context, records []map[string]any) ([]map[string]any, error) {
	// Respect context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if len(records) == 0 {
		return records, nil
	}

	result := make([]map[string]any, 0, len(records))

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
func (m *SetModule) processRecord(record map[string]any) (map[string]any, error) {
	target := m.config.Target
	value := m.config.Value

	// Simple case: no dot notation (flat field)
	if !recordpath.IsNested(target) {
		record[target] = value
		return record, nil
	}

	// Nested path - always create intermediate objects
	if err := recordpath.Set(record, target, value); err != nil {
		return nil, err
	}
	return record, nil
}

// ParseSetConfig parses a raw configuration map into SetConfig.
func ParseSetConfig(config map[string]any) (SetConfig, error) {
	var cfg SetConfig

	target, ok := config["target"].(string)
	if !ok || target == "" {
		return cfg, errors.New("'target' is required and must be a non-empty string")
	}
	cfg.Target = target

	value, hasValue := config["value"]
	if !hasValue {
		return cfg, errors.New("'value' is required (use null to set the field to null)")
	}
	cfg.Value = value
	cfg.HasValue = true

	return cfg, nil
}

// Verify interface compliance at compile time
var _ Module = (*SetModule)(nil)
