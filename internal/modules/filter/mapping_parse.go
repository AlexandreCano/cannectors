package filter

import (
	"fmt"
	"regexp"
)

// parseMappingConfig validates and normalizes a single field mapping.
func parseMappingConfig(m FieldMapping, index int) (MappingConfig, error) {
	config := MappingConfig{
		DefaultValue: m.DefaultValue,
		OnMissing:    m.OnMissing,
	}

	// Validate target is provided
	if m.Target == "" {
		return config, fmt.Errorf("%w at index %d: mapping must have a target field", ErrInvalidMapping, index)
	}

	// Handle source field:
	// - nil (not declared) = delete the target field
	// - empty string = error (invalid)
	// - non-empty string = normal mapping
	if m.Source == nil {
		config.Source = "" // Empty string internally means "delete"
	} else if *m.Source == "" {
		return config, fmt.Errorf("%w at index %d: source cannot be empty string, omit it to delete the field", ErrInvalidMapping, index)
	} else {
		config.Source = *m.Source
	}
	config.Target = m.Target

	// Set default onMissing behavior
	if config.OnMissing == "" {
		config.OnMissing = OnMissingSetNull
	}

	// Parse transforms array and pre-compile regex patterns
	if m.Transforms != nil {
		config.Transforms = make([]TransformConfig, len(m.Transforms))
		for i, t := range m.Transforms {
			tc := TransformConfig{
				Op:          t.Op,
				Format:      t.Format,
				Pattern:     t.Pattern,
				Replacement: t.Replacement,
				Separator:   t.Separator,
			}
			// Pre-compile regex pattern for replace operations
			if t.Op == "replace" && t.Pattern != "" {
				re, err := regexp.Compile(t.Pattern)
				if err != nil {
					return config, fmt.Errorf("%w at index %d: invalid regex pattern in transform %d: %v", ErrInvalidMapping, index, i, err)
				}
				tc.CompiledPattern = re
			}
			config.Transforms[i] = tc
		}
	}

	return config, nil
}

// ParseFieldMappings parses raw mapping configuration into FieldMapping structs.
// Accepts []FieldMapping, []map[string]interface{}, or []interface{} decoded from JSON/YAML.
func ParseFieldMappings(raw interface{}) ([]FieldMapping, error) {
	if raw == nil {
		return []FieldMapping{}, nil
	}

	switch v := raw.(type) {
	case []FieldMapping:
		return v, nil
	case []map[string]interface{}:
		return parseFieldMappingList(v)
	case []interface{}:
		mappings := make([]map[string]interface{}, 0, len(v))
		for i, item := range v {
			switch m := item.(type) {
			case map[string]interface{}:
				mappings = append(mappings, m)
			case FieldMapping:
				mappings = append(mappings, map[string]interface{}{
					"source":       m.Source,
					"target":       m.Target,
					"defaultValue": m.DefaultValue,
					"onMissing":    m.OnMissing,
					"transforms":   m.Transforms,
				})
			default:
				return nil, fmt.Errorf("mapping at index %d must be an object", i)
			}
		}
		return parseFieldMappingList(mappings)
	default:
		return nil, fmt.Errorf("mappings must be an array")
	}
}

func parseFieldMappingList(raw []map[string]interface{}) ([]FieldMapping, error) {
	mappings := make([]FieldMapping, 0, len(raw))
	for i, item := range raw {
		mapping, err := parseFieldMappingMap(item, i)
		if err != nil {
			return nil, fmt.Errorf("parsing field mapping at index %d: %w", i, err)
		}
		mappings = append(mappings, mapping)
	}
	return mappings, nil
}

func parseFieldMappingMap(data map[string]interface{}, index int) (FieldMapping, error) {
	mapping := FieldMapping{}

	// Source is a pointer: nil means "not declared" (delete), non-nil is the value
	if source, ok := data["source"].(string); ok {
		mapping.Source = &source
	}
	if target, ok := data["target"].(string); ok {
		mapping.Target = target
	}
	if defaultValue, ok := data["defaultValue"]; ok {
		mapping.DefaultValue = defaultValue
	}
	if onMissing, ok := data["onMissing"].(string); ok {
		mapping.OnMissing = onMissing
	}
	if rawTransforms, ok := data["transforms"]; ok {
		transforms, err := parseTransformOps(rawTransforms, index)
		if err != nil {
			return mapping, err
		}
		mapping.Transforms = transforms
	}

	return mapping, nil
}

func parseTransformOps(raw interface{}, mappingIndex int) ([]TransformOp, error) {
	if raw == nil {
		return nil, nil
	}

	list, ok := raw.([]interface{})
	if !ok {
		if typed, okTyped := raw.([]TransformOp); okTyped {
			return typed, nil
		}
		return nil, fmt.Errorf("transforms for mapping %d must be an array", mappingIndex)
	}

	ops := make([]TransformOp, 0, len(list))
	for i, item := range list {
		switch v := item.(type) {
		case string:
			ops = append(ops, TransformOp{Op: v})
		case map[string]interface{}:
			op, _ := v["op"].(string)
			if op == "" {
				return nil, fmt.Errorf("transform op missing at mapping %d index %d", mappingIndex, i)
			}
			ops = append(ops, TransformOp{
				Op:          op,
				Format:      stringValue(v["format"]),
				Pattern:     stringValue(v["pattern"]),
				Replacement: stringValue(v["replacement"]),
				Separator:   stringValue(v["separator"]),
			})
		default:
			return nil, fmt.Errorf("transform op at mapping %d index %d must be an object or string", mappingIndex, i)
		}
	}

	return ops, nil
}

func stringValue(value interface{}) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", value)
}
