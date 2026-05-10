// Package filter provides implementations for filter modules.
// This file implements the "remove" filter module for removing fields from records.
//
// The remove filter mutates each record in place (same map reference is returned).
// If the target field does not exist, the record is left unchanged (no error).
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

// RemoveConfig represents the configuration for a remove filter module.
//
// Target accepts either a single field path (string) or a list of field paths
// ([]string). The runtime normalizes both shapes into the internal
// `targetList` slice. The legacy `targets` field has been removed (Story 24.10).
type RemoveConfig struct {
	connector.ModuleBase
	// Target is either a non-empty string or a non-empty list of non-empty strings.
	Target any `json:"target"`
	// targetList is the normalized internal representation populated by
	// UnmarshalJSON / ParseRemoveConfig.
	targetList []string
}

// UnmarshalJSON normalizes the polymorphic Target field into targetList.
func (c *RemoveConfig) UnmarshalJSON(data []byte) error {
	type alias RemoveConfig
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*c = RemoveConfig(a)
	targets, err := normalizeRemoveTarget(c.Target)
	if err != nil {
		return err
	}
	c.targetList = targets
	return nil
}

// normalizeRemoveTarget accepts a string or a slice of strings and returns
// the normalized []string. It rejects empty entries; the schema is the first
// line of defense, this is the runtime safety net.
func normalizeRemoveTarget(raw any) ([]string, error) {
	switch v := raw.(type) {
	case nil:
		return nil, errors.New("'target' is required")
	case string:
		if v == "" {
			return nil, errors.New("'target' must be a non-empty string")
		}
		return []string{v}, nil
	case []string:
		if len(v) == 0 {
			return nil, errors.New("'target' list must contain at least one entry")
		}
		for i, s := range v {
			if s == "" {
				return nil, fmt.Errorf("'target[%d]' must be a non-empty string", i)
			}
		}
		return v, nil
	case []any:
		if len(v) == 0 {
			return nil, errors.New("'target' list must contain at least one entry")
		}
		out := make([]string, 0, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("'target[%d]' must be a string", i)
			}
			if s == "" {
				return nil, fmt.Errorf("'target[%d]' must be a non-empty string", i)
			}
			out = append(out, s)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("'target' must be a string or list of strings, got %T", raw)
	}
}

// RemoveModule implements the remove filter that removes fields from each record.
type RemoveModule struct {
	targets []string
}

// NewRemoveFromConfig creates a new remove filter module from configuration.
// It validates that at least one target is provided.
func NewRemoveFromConfig(config RemoveConfig) (*RemoveModule, error) {
	if _, err := errhandling.ParseOnErrorStrategy(config.OnError); err != nil {
		return nil, err
	}

	targets := config.targetList
	if len(targets) == 0 {
		// Caller built the struct directly (programmatic API) and bypassed
		// UnmarshalJSON; normalize here as a safety net.
		var err error
		targets, err = normalizeRemoveTarget(config.Target)
		if err != nil {
			return nil, err
		}
	}

	// De-duplicate while preserving order so identical targets do not trigger
	// repeated work / log noise.
	seen := make(map[string]bool, len(targets))
	uniqueTargets := make([]string, 0, len(targets))
	for _, t := range targets {
		if !seen[t] {
			seen[t] = true
			uniqueTargets = append(uniqueTargets, t)
		}
	}

	logger.Debug("remove filter module initialized", "targets", uniqueTargets)

	return &RemoveModule{targets: uniqueTargets}, nil
}

// Process implements the filter.Module interface.
// It removes the configured fields from each record.
func (m *RemoveModule) Process(ctx context.Context, records []map[string]any) ([]map[string]any, error) {
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
func (m *RemoveModule) processRecord(record map[string]any) map[string]any {
	if record == nil {
		return record
	}
	for _, target := range m.targets {
		if !recordpath.IsNested(target) {
			delete(record, target)
		} else {
			recordpath.Delete(record, target)
		}
	}
	return record
}

// ParseRemoveConfig parses a raw configuration map into RemoveConfig.
func ParseRemoveConfig(config map[string]any) (RemoveConfig, error) {
	var cfg RemoveConfig

	raw, ok := config["target"]
	if !ok {
		return cfg, errors.New("'target' is required")
	}
	targets, err := normalizeRemoveTarget(raw)
	if err != nil {
		return cfg, err
	}
	cfg.Target = raw
	cfg.targetList = targets
	return cfg, nil
}

// Verify interface compliance at compile time
var _ Module = (*RemoveModule)(nil)
