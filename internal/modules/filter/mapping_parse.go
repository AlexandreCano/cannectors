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

	// Validate onMissing strategy: empty defaults to setNull, otherwise must be
	// one of the supported strategies. useDefault additionally requires
	// defaultValue to be set.
	switch config.OnMissing {
	case "":
		config.OnMissing = OnMissingSetNull
	case OnMissingSetNull, OnMissingSkipField, OnMissingFail:
	case OnMissingUseDefault:
		if !m.HasDefaultValue {
			return config, fmt.Errorf("%w at index %d: onMissing %q requires defaultValue", ErrInvalidMapping, index, config.OnMissing)
		}
	default:
		return config, fmt.Errorf("%w at index %d: unknown onMissing strategy %q (expected setNull|skipField|useDefault|fail)", ErrInvalidMapping, index, config.OnMissing)
	}

	// Parse transforms array and pre-compile regex patterns
	if m.Transforms != nil {
		config.Transforms = make([]TransformConfig, len(m.Transforms))
		for i, t := range m.Transforms {
			if !isSupportedTransformOp(t.Op) {
				return config, fmt.Errorf("%w at index %d: unknown transform op %q in transform %d", ErrInvalidMapping, index, t.Op, i)
			}
			tc := TransformConfig{
				Op:          t.Op,
				Format:      t.Format,
				Pattern:     t.Pattern,
				Replacement: t.Replacement,
				Separator:   t.Separator,
			}
			if t.Op == "replace" {
				if t.Pattern == "" {
					return config, fmt.Errorf("%w at index %d: transform %d (replace) requires non-empty pattern", ErrInvalidMapping, index, i)
				}
				re, err := regexp.Compile(t.Pattern)
				if err != nil {
					return config, fmt.Errorf("%w at index %d: invalid regex pattern in transform %d: %w", ErrInvalidMapping, index, i, err)
				}
				tc.CompiledPattern = re
			}
			config.Transforms[i] = tc
		}
	}

	return config, nil
}

// supportedTransformOps mirrors the schema enum on transformOp.op.
var supportedTransformOps = map[string]struct{}{
	"trim":       {},
	"lowercase":  {},
	"uppercase":  {},
	"dateFormat": {},
	"replace":    {},
	"split":      {},
	"join":       {},
	"toString":   {},
	"toInt":      {},
	"toFloat":    {},
	"toBool":     {},
	"toArray":    {},
	"toObject":   {},
}

func isSupportedTransformOp(op string) bool {
	_, ok := supportedTransformOps[op]
	return ok
}

// ParseFieldMappings parses raw mapping configuration into FieldMapping structs.
// Accepts []FieldMapping, []map[string]any, or []any decoded from JSON/YAML.
func ParseFieldMappings(raw any) ([]FieldMapping, error) {
	if raw == nil {
		return []FieldMapping{}, nil
	}

	switch v := raw.(type) {
	case []FieldMapping:
		return v, nil
	case []map[string]any:
		return parseFieldMappingList(v)
	case []any:
		mappings := make([]map[string]any, 0, len(v))
		for i, item := range v {
			switch m := item.(type) {
			case map[string]any:
				mappings = append(mappings, m)
			case FieldMapping:
				converted := map[string]any{
					"source":     m.Source,
					"target":     m.Target,
					"onMissing":  m.OnMissing,
					"transforms": m.Transforms,
				}
				if m.HasDefaultValue {
					converted["defaultValue"] = m.DefaultValue
				}
				mappings = append(mappings, converted)
			default:
				return nil, fmt.Errorf("mapping at index %d must be an object", i)
			}
		}
		return parseFieldMappingList(mappings)
	default:
		return nil, fmt.Errorf("mappings must be an array")
	}
}

func parseFieldMappingList(raw []map[string]any) ([]FieldMapping, error) {
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

func parseFieldMappingMap(data map[string]any, index int) (FieldMapping, error) {
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
		mapping.HasDefaultValue = true
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

func parseTransformOps(raw any, mappingIndex int) ([]TransformOp, error) {
	if raw == nil {
		return nil, nil
	}

	list, ok := raw.([]any)
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
		case map[string]any:
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

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", value)
}
