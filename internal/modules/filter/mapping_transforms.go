package filter

// Transform operations for the mapping filter module.
//
// To add a new op:
//  1. Add a case in MappingModule.applyTransformOp (in mapping.go) that calls the new helper.
//  2. Implement the helper here, returning (interface{}, error).
//  3. If the op performs type conversion, also add it to typeConversionOps in mapping.go
//     for proper error code classification.
//  4. Add a unit test in mapping_test.go.

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func applyTrim(value interface{}) (interface{}, error) {
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s), nil
	}
	return value, nil
}

func applyLowercase(value interface{}) (interface{}, error) {
	if s, ok := value.(string); ok {
		return strings.ToLower(s), nil
	}
	return value, nil
}

func applyUppercase(value interface{}) (interface{}, error) {
	if s, ok := value.(string); ok {
		return strings.ToUpper(s), nil
	}
	return value, nil
}

func applyDateFormat(value interface{}, format string) (interface{}, error) {
	if format == "" {
		format = "2006-01-02T15:04:05" // Default: YYYY-MM-DDTHH:mm:ss
	}

	// Convert common format strings to Go layout
	goFormat := convertDateFormat(format)

	// Parse the input value
	var t time.Time
	var err error

	switch v := value.(type) {
	case string:
		// Try common input formats
		inputFormats := []string{
			time.RFC3339,
			"2006-01-02T15:04:05Z07:00",
			"2006-01-02T15:04:05",
			"2006-01-02",
			time.RFC1123,
		}
		for _, inputFmt := range inputFormats {
			t, err = time.Parse(inputFmt, v)
			if err == nil {
				break
			}
		}
		if err != nil {
			return value, fmt.Errorf("could not parse date: %s", v)
		}
	case time.Time:
		t = v
	default:
		return value, nil
	}

	return t.Format(goFormat), nil
}

// convertDateFormat converts common date format strings to Go layout.
// The current patterns are designed not to overlap, so replacement order does not affect behavior.
// If overlapping patterns are added in the future (e.g., "M" and "MM"), ensure longer patterns are replaced first.
func convertDateFormat(format string) string {
	replacements := []struct {
		pattern     string
		replacement string
	}{
		{"YYYY", "2006"},
		{"YY", "06"},
		{"MM", "01"},
		{"DD", "02"},
		{"HH", "15"},
		{"mm", "04"},
		{"ss", "05"},
		{"SSS", "000"},
	}

	result := format
	for _, r := range replacements {
		result = strings.ReplaceAll(result, r.pattern, r.replacement)
	}
	return result
}

func applyReplace(value interface{}, compiledPattern *regexp.Regexp, replacement string) (interface{}, error) {
	if s, ok := value.(string); ok {
		if compiledPattern == nil {
			return s, nil
		}
		return compiledPattern.ReplaceAllString(s, replacement), nil
	}
	return value, nil
}

func applySplit(value interface{}, separator string) (interface{}, error) {
	if s, ok := value.(string); ok {
		if separator == "" {
			separator = ","
		}
		parts := strings.Split(s, separator)
		result := make([]interface{}, len(parts))
		for i, p := range parts {
			// Note: each split part is trimmed of surrounding whitespace.
			result[i] = strings.TrimSpace(p)
		}
		return result, nil
	}
	return value, nil
}

func applyJoin(value interface{}, separator string) (interface{}, error) {
	if separator == "" {
		separator = ","
	}

	switch v := value.(type) {
	case []interface{}:
		parts := make([]string, len(v))
		for i, item := range v {
			parts[i] = fmt.Sprintf("%v", item)
		}
		return strings.Join(parts, separator), nil
	case []string:
		return strings.Join(v, separator), nil
	}
	return value, nil
}

func applyToString(value interface{}) (interface{}, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	case time.Time:
		return v.Format(time.RFC3339), nil
	default:
		return fmt.Sprintf("%v", value), nil
	}
}

func applyToInt(value interface{}) (interface{}, error) {
	switch v := value.(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case int32:
		return int(v), nil
	case uint:
		return int(v), nil
	case uint64:
		return int(v), nil
	case uint32:
		return int(v), nil
	case float64:
		if math.Trunc(v) != v {
			return nil, fmt.Errorf("cannot convert float with fractional part to int: %v", v)
		}
		return int(v), nil
	case float32:
		if math.Trunc(float64(v)) != float64(v) {
			return nil, fmt.Errorf("cannot convert float with fractional part to int: %v", v)
		}
		return int(v), nil
	case json.Number:
		i, err := v.Int64()
		if err != nil {
			return nil, fmt.Errorf("cannot convert number to int: %w", err)
		}
		return int(i), nil
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return nil, fmt.Errorf("cannot parse int from string %q: %w", v, err)
		}
		return i, nil
	case bool:
		if v {
			return 1, nil
		}
		return 0, nil
	default:
		return nil, fmt.Errorf("cannot convert %T to int", value)
	}
}

func applyToFloat(value interface{}) (interface{}, error) {
	switch v := value.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case uint:
		return float64(v), nil
	case uint64:
		return float64(v), nil
	case uint32:
		return float64(v), nil
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return nil, fmt.Errorf("cannot convert number to float: %w", err)
		}
		return f, nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return nil, fmt.Errorf("cannot parse float from string %q: %w", v, err)
		}
		return f, nil
	case bool:
		if v {
			return 1.0, nil
		}
		return 0.0, nil
	default:
		return nil, fmt.Errorf("cannot convert %T to float", value)
	}
}

func applyToBool(value interface{}) (interface{}, error) {
	switch v := value.(type) {
	case bool:
		return v, nil
	case string:
		b, err := strconv.ParseBool(strings.TrimSpace(v))
		if err != nil {
			return nil, fmt.Errorf("cannot parse bool from string %q: %w", v, err)
		}
		return b, nil
	case int:
		return v != 0, nil
	case int64:
		return v != 0, nil
	case int32:
		return v != 0, nil
	case uint:
		return v != 0, nil
	case uint64:
		return v != 0, nil
	case uint32:
		return v != 0, nil
	case float64:
		return v != 0, nil
	case float32:
		return v != 0, nil
	default:
		return nil, fmt.Errorf("cannot convert %T to bool", value)
	}
}

func applyToArray(value interface{}) (interface{}, error) {
	switch v := value.(type) {
	case []interface{}:
		return v, nil
	case []string:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = item
		}
		return result, nil
	default:
		return []interface{}{value}, nil
	}
}

func applyToObject(value interface{}) (interface{}, error) {
	switch v := value.(type) {
	case map[string]interface{}:
		return v, nil
	case map[string]string:
		result := make(map[string]interface{}, len(v))
		for key, item := range v {
			result[key] = item
		}
		return result, nil
	default:
		return nil, fmt.Errorf("cannot convert %T to object", value)
	}
}
