// Package filter provides implementations for filter modules.
// Filter modules transform, map, and conditionally process data.
package filter

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/canectors/runtime/internal/logger"
)

// Error codes for mapping module
const (
	ErrCodeInvalidMapping   = "INVALID_MAPPING"
	ErrCodeMissingField     = "MISSING_FIELD"
	ErrCodeTypeConversion   = "TYPE_CONVERSION"
	ErrCodeTransformFailed  = "TRANSFORM_FAILED"
	ErrCodeNestedPathFailed = "NESTED_PATH_FAILED"
)

// OnMissing behavior constants
const (
	OnMissingSetNull    = "setNull"
	OnMissingSkipField  = "skipField"
	OnMissingUseDefault = "useDefault"
	OnMissingFail       = "fail"
)

// OnError behavior constants
const (
	OnErrorFail = "fail"
	OnErrorSkip = "skip"
	OnErrorLog  = "log"
)

// typeConversionOps is the set of transform operations that perform type conversion.
// Used for error code classification.
var typeConversionOps = map[string]bool{
	"toString": true,
	"toInt":    true,
	"toFloat":  true,
	"toBool":   true,
	"toArray":  true,
	"toObject": true,
}

// Common errors
var (
	// ErrInvalidMapping is returned when a mapping configuration is invalid
	ErrInvalidMapping = errors.New("invalid mapping configuration")
)

// FieldMapping represents a field mapping configuration.
// This is the input format from the pipeline configuration.
type FieldMapping struct {
	Source       string        `json:"source"`
	Target       string        `json:"target"`
	DefaultValue interface{}   `json:"defaultValue,omitempty"`
	OnMissing    string        `json:"onMissing,omitempty"`
	Transforms   []TransformOp `json:"transforms,omitempty"`
	Confidence   float64       `json:"confidence,omitempty"`
}

// TransformOp represents a transform operation configuration.
type TransformOp struct {
	Op          string `json:"op"`
	Format      string `json:"format,omitempty"`
	Pattern     string `json:"pattern,omitempty"`
	Replacement string `json:"replacement,omitempty"`
	Separator   string `json:"separator,omitempty"`
}

// MappingConfig represents the parsed and validated mapping configuration.
type MappingConfig struct {
	Source       string
	Target       string
	DefaultValue interface{}
	OnMissing    string
	Transforms   []TransformConfig
	Confidence   float64
}

// TransformConfig represents a transform operation configuration.
type TransformConfig struct {
	Op              string
	Format          string
	Pattern         string
	Replacement     string
	Separator       string
	CompiledPattern *regexp.Regexp // Pre-compiled regex for replace operations
}

// MappingModule implements field mapping transformations.
// It transforms input records by applying field-to-field mappings.
type MappingModule struct {
	mappings []MappingConfig
	onError  string
}

// NewMappingFromConfig creates a new mapping filter module from configuration.
// It validates the mappings and returns an error if any mapping is invalid.
//
// Parameters:
//   - mappings: Array of field mappings (source/target format)
//   - onError: Error handling mode ("fail", "skip", "log")
func NewMappingFromConfig(mappings []FieldMapping, onError string) (*MappingModule, error) {
	// Validate onError mode
	if onError == "" {
		onError = OnErrorFail
	}
	if onError != OnErrorFail && onError != OnErrorSkip && onError != OnErrorLog {
		onError = OnErrorFail
	}

	// Parse and validate mappings
	configs := make([]MappingConfig, 0, len(mappings))
	for i, m := range mappings {
		config, err := parseMappingConfig(m, i)
		if err != nil {
			return nil, err
		}
		configs = append(configs, config)
	}

	logger.Debug("mapping module initialized",
		slog.Int("mapping_count", len(configs)),
		slog.String("on_error", onError),
	)

	return &MappingModule{
		mappings: configs,
		onError:  onError,
	}, nil
}

// MappingError carries structured context for mapping failures.
type MappingError struct {
	Code         string
	Message      string
	SourceField  string
	TargetField  string
	RecordIndex  int
	MappingIndex int
	TransformOp  string
	SourceValue  interface{}
}

func (e *MappingError) Error() string {
	return e.Message
}

// TransformError carries context for transform failures.
type TransformError struct {
	Op  string
	Err error
}

func (e TransformError) Error() string {
	return fmt.Sprintf("transform %q failed: %v", e.Op, e.Err)
}

func newMappingError(code, message string, mapping MappingConfig, recordIdx, mappingIdx int, value interface{}, transformOp string) *MappingError {
	return &MappingError{
		Code:         code,
		Message:      message,
		SourceField:  mapping.Source,
		TargetField:  mapping.Target,
		RecordIndex:  recordIdx,
		MappingIndex: mappingIdx,
		TransformOp:  transformOp,
		SourceValue:  value,
	}
}

// parseMappingConfig validates and normalizes a single field mapping.
func parseMappingConfig(m FieldMapping, index int) (MappingConfig, error) {
	config := MappingConfig{
		DefaultValue: m.DefaultValue,
		OnMissing:    m.OnMissing,
		Confidence:   m.Confidence,
	}

	// Validate source and target are provided
	if m.Source == "" || m.Target == "" {
		return config, fmt.Errorf("%w at index %d: mapping must have both source and target fields", ErrInvalidMapping, index)
	}

	config.Source = m.Source
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
					"confidence":   m.Confidence,
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
			return nil, err
		}
		mappings = append(mappings, mapping)
	}
	return mappings, nil
}

func parseFieldMappingMap(data map[string]interface{}, index int) (FieldMapping, error) {
	mapping := FieldMapping{}

	if source, ok := data["source"].(string); ok {
		mapping.Source = source
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
	if confidence, ok := parseFloatValue(data["confidence"]); ok {
		mapping.Confidence = confidence
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

func parseFloatValue(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case int32:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint64:
		return float64(v), true
	case uint32:
		return float64(v), true
	case json.Number:
		if f, err := v.Float64(); err == nil {
			return f, true
		}
	}
	return 0, false
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

// Process applies field mappings to the input records.
// It returns the transformed records with fields mapped from source to target paths.
//
// For each input record:
//  1. Creates a new target record
//  2. For each mapping, extracts source value and sets target value
//  3. Handles missing fields according to onMissing configuration
//  4. Applies transforms if configured
//
// Returns the transformed records and any error that occurred.
func (m *MappingModule) Process(records []map[string]interface{}) ([]map[string]interface{}, error) {
	if records == nil {
		return []map[string]interface{}{}, nil
	}

	result := make([]map[string]interface{}, 0, len(records))

	for recordIdx, record := range records {
		targetRecord, err := m.processRecord(record, recordIdx)
		if err != nil {
			var mappingErr *MappingError
			hasContext := errors.As(err, &mappingErr)
			switch m.onError {
			case OnErrorFail:
				return nil, err
			case OnErrorSkip:
				if hasContext {
					logger.Warn("skipping record due to mapping error",
						slog.Int("record_index", mappingErr.RecordIndex),
						slog.Int("mapping_index", mappingErr.MappingIndex),
						slog.String("source_field", mappingErr.SourceField),
						slog.String("target_field", mappingErr.TargetField),
						slog.String("transform_op", mappingErr.TransformOp),
						slog.String("error", mappingErr.Error()),
						slog.Any("source_value", mappingErr.SourceValue),
						slog.String("error_code", mappingErr.Code),
					)
				} else {
					logger.Warn("skipping record due to mapping error",
						slog.Int("record_index", recordIdx),
						slog.String("error", err.Error()),
					)
				}
				continue
			case OnErrorLog:
				if hasContext {
					logger.Error("mapping error (continuing)",
						slog.Int("record_index", mappingErr.RecordIndex),
						slog.Int("mapping_index", mappingErr.MappingIndex),
						slog.String("source_field", mappingErr.SourceField),
						slog.String("target_field", mappingErr.TargetField),
						slog.String("transform_op", mappingErr.TransformOp),
						slog.String("error", mappingErr.Error()),
						slog.Any("source_value", mappingErr.SourceValue),
						slog.String("error_code", mappingErr.Code),
					)
				} else {
					logger.Error("mapping error (continuing)",
						slog.Int("record_index", recordIdx),
						slog.String("error", err.Error()),
					)
				}
				// Continue but add partial result
				result = append(result, targetRecord)
				continue
			}
		}
		result = append(result, targetRecord)
	}

	return result, nil
}

// processRecord applies all mappings to a single record.
func (m *MappingModule) processRecord(record map[string]interface{}, recordIdx int) (map[string]interface{}, error) {
	target := make(map[string]interface{})

	for mappingIdx, mapping := range m.mappings {
		// Get source value
		value, found := getNestedValue(record, mapping.Source)

		if !found {
			// Handle missing source field
			switch mapping.OnMissing {
			case OnMissingSetNull:
				value = nil
				found = true
			case OnMissingSkipField:
				continue
			case OnMissingUseDefault:
				value = mapping.DefaultValue
				found = true
			case OnMissingFail:
				message := fmt.Sprintf("missing required field %q for target %q at record %d, mapping %d",
					mapping.Source, mapping.Target, recordIdx, mappingIdx)
				return target, newMappingError(ErrCodeMissingField, message, mapping, recordIdx, mappingIdx, nil, "")
			}
		}

		if found {
			// Apply transforms if configured
			transformedValue, err := m.applyTransforms(value, mapping)
			if err != nil {
				transformOp := ""
				if transformErr, ok := err.(TransformError); ok {
					transformOp = transformErr.Op
				}
				message := fmt.Sprintf("transform failed for field %q -> %q at record %d, mapping %d: %v",
					mapping.Source, mapping.Target, recordIdx, mappingIdx, err)
				code := ErrCodeTransformFailed
				if typeConversionOps[transformOp] {
					code = ErrCodeTypeConversion
				}
				return target, newMappingError(code, message, mapping, recordIdx, mappingIdx, value, transformOp)
			}

			// Set target value
			if err := setNestedValue(target, mapping.Target, transformedValue); err != nil {
				message := fmt.Sprintf("failed to set target field %q at record %d, mapping %d: %v",
					mapping.Target, recordIdx, mappingIdx, err)
				return target, newMappingError(ErrCodeNestedPathFailed, message, mapping, recordIdx, mappingIdx, transformedValue, "")
			}
		}
	}

	return target, nil
}

// getNestedValue extracts a value from a nested object using dot notation.
// Supports paths like "user.profile.name".
func getNestedValue(obj map[string]interface{}, path string) (interface{}, bool) {
	if path == "" {
		return nil, false
	}

	parts := strings.Split(path, ".")
	current := interface{}(obj)

	for _, part := range parts {
		// Handle array indexing (e.g., "items[0]")
		arrayIdx := -1
		key, index, hasIndex, err := parsePathPart(part)
		if err != nil {
			return nil, false
		}
		part = key
		if hasIndex {
			arrayIdx = index
		}

		// Navigate to the field
		switch v := current.(type) {
		case map[string]interface{}:
			val, ok := v[part]
			if !ok {
				return nil, false
			}
			current = val
		default:
			return nil, false
		}

		// Handle array indexing
		if arrayIdx >= 0 {
			switch arr := current.(type) {
			case []interface{}:
				if arrayIdx >= len(arr) {
					return nil, false
				}
				current = arr[arrayIdx]
			default:
				return nil, false
			}
		}
	}

	return current, true
}

func parsePathPart(part string) (string, int, bool, error) {
	idx := strings.Index(part, "[")
	if idx == -1 {
		return part, -1, false, nil
	}
	endIdx := strings.Index(part, "]")
	if endIdx == -1 || endIdx < idx+1 {
		return "", -1, false, fmt.Errorf("invalid array index in path segment %q", part)
	}
	if endIdx != len(part)-1 {
		return "", -1, false, fmt.Errorf("invalid array index in path segment %q", part)
	}
	index, err := strconv.Atoi(part[idx+1 : endIdx])
	if err != nil || index < 0 {
		return "", -1, false, fmt.Errorf("invalid array index in path segment %q", part)
	}
	return part[:idx], index, true, nil
}

// setNestedValue sets a value in a nested object using dot notation.
// Creates intermediate objects as needed.
func setNestedValue(obj map[string]interface{}, path string, value interface{}) error {
	if path == "" {
		return errors.New("empty path")
	}

	parts := strings.Split(path, ".")
	current := obj

	// Navigate/create intermediate objects
	for i := 0; i < len(parts); i++ {
		part, index, hasIndex, err := parsePathPart(parts[i])
		if err != nil {
			return err
		}
		isLast := i == len(parts)-1

		if !hasIndex {
			if isLast {
				current[part] = value
				return nil
			}

			next, ok := current[part].(map[string]interface{})
			if !ok {
				next = make(map[string]interface{})
				current[part] = next
			}
			current = next
			continue
		}

		arr, ok := current[part].([]interface{})
		if !ok {
			arr = make([]interface{}, index+1)
			current[part] = arr
		}
		if len(arr) <= index {
			arr = append(arr, make([]interface{}, index+1-len(arr))...)
			current[part] = arr
		}

		if isLast {
			arr[index] = value
			return nil
		}

		if arr[index] == nil {
			arr[index] = make(map[string]interface{})
		}
		next, ok := arr[index].(map[string]interface{})
		if !ok {
			next = make(map[string]interface{})
			arr[index] = next
		}
		current = next
	}

	return nil
}

// applyTransforms applies transform operations to a value.
// Transforms are applied in order from the Transforms array.
func (m *MappingModule) applyTransforms(value interface{}, mapping MappingConfig) (interface{}, error) {
	for _, transform := range mapping.Transforms {
		transformedValue, err := m.applyTransformOp(value, transform)
		if err != nil {
			return value, TransformError{Op: transform.Op, Err: err}
		}
		value = transformedValue
	}

	return value, nil
}

// applyTransformOp applies a specific transform operation.
func (m *MappingModule) applyTransformOp(value interface{}, config TransformConfig) (interface{}, error) {
	if value == nil {
		return nil, nil
	}

	switch config.Op {
	case "trim":
		return applyTrim(value)
	case "lowercase":
		return applyLowercase(value)
	case "uppercase":
		return applyUppercase(value)
	case "toString":
		return applyToString(value)
	case "toInt":
		return applyToInt(value)
	case "toFloat":
		return applyToFloat(value)
	case "toBool":
		return applyToBool(value)
	case "toArray":
		return applyToArray(value)
	case "toObject":
		return applyToObject(value)
	case "dateFormat":
		return applyDateFormat(value, config.Format)
	case "replace":
		return applyReplace(value, config.CompiledPattern, config.Replacement)
	case "split":
		return applySplit(value, config.Separator)
	case "join":
		return applyJoin(value, config.Separator)
	default:
		// Unknown transform - return value unchanged
		logger.Debug("unknown transform operation, passing value through",
			slog.String("op", config.Op),
		)
		return value, nil
	}
}

// Transform operations

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
// Uses ordered replacement to avoid shorter patterns matching inside longer ones.
func convertDateFormat(format string) string {
	// Order matters! Longer patterns must be replaced first.
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
