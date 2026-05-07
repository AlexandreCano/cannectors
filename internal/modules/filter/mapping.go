// Package filter provides implementations for filter modules.
// Filter modules transform, map, and conditionally process data.
package filter

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"github.com/cannectors/runtime/internal/errhandling"
	"github.com/cannectors/runtime/internal/logger"
	"github.com/cannectors/runtime/internal/recordpath"
	"github.com/cannectors/runtime/pkg/connector"
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
// Source is a pointer to distinguish "not declared" (nil = delete field) from "empty string".
type FieldMapping struct {
	Source       *string       `json:"source,omitempty"`
	Target       string        `json:"target"`
	DefaultValue interface{}   `json:"defaultValue,omitempty"`
	OnMissing    string        `json:"onMissing,omitempty"`
	Transforms   []TransformOp `json:"transforms,omitempty"`
}

// TransformOp represents a transform operation configuration.
type TransformOp struct {
	Op          string `json:"op"`
	Format      string `json:"format,omitempty"`
	Pattern     string `json:"pattern,omitempty"`
	Replacement string `json:"replacement,omitempty"`
	Separator   string `json:"separator,omitempty"`
}

// MappingModuleConfig is the top-level config struct for the mapping filter module.
// Used by moduleconfig.ParseModuleConfig to deserialize from JSON.
type MappingModuleConfig struct {
	connector.ModuleBase
	Mappings []FieldMapping `json:"mappings"`
}

// MappingConfig represents the parsed and validated mapping configuration.
type MappingConfig struct {
	Source       string
	Target       string
	DefaultValue interface{}
	OnMissing    string
	Transforms   []TransformConfig
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
	onError  errhandling.OnErrorStrategy
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

// ErrorCode implements errhandling.ModuleError.
func (e *MappingError) ErrorCode() string { return e.Code }

// ErrorModule implements errhandling.ModuleError.
func (e *MappingError) ErrorModule() string { return "mapping" }

// ErrorRecordIndex implements errhandling.ModuleError.
func (e *MappingError) ErrorRecordIndex() int { return e.RecordIndex }

// ErrorDetails implements errhandling.ModuleError. Surfaces structured
// mapping context (target/source field, mapping index, transform op, source
// value) as a flat map for ExecutionError.Details.
func (e *MappingError) ErrorDetails() map[string]interface{} {
	d := map[string]interface{}{
		"source_field":  e.SourceField,
		"target_field":  e.TargetField,
		"mapping_index": e.MappingIndex,
	}
	if e.TransformOp != "" {
		d["transform_op"] = e.TransformOp
	}
	if e.SourceValue != nil {
		d["source_value"] = e.SourceValue
	}
	return d
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

// NewMappingFromConfig creates a new mapping filter module from configuration.
// It validates the mappings and returns an error if any mapping is invalid.
//
// Parameters:
//   - mappings: Array of field mappings (source/target format)
//   - onError: Error handling mode ("fail", "skip", "log")
func NewMappingFromConfig(mappings []FieldMapping, onError string) (*MappingModule, error) {
	strategy := errhandling.ParseOnErrorStrategy(onError)

	// Parse and validate mappings
	configs := make([]MappingConfig, 0, len(mappings))
	for i, m := range mappings {
		config, err := parseMappingConfig(m, i)
		if err != nil {
			return nil, fmt.Errorf("parsing mapping config: %w", err)
		}
		configs = append(configs, config)
	}

	logger.Debug("mapping module initialized",
		slog.Int("mapping_count", len(configs)),
		slog.String("on_error", string(strategy)),
	)

	return &MappingModule{
		mappings: configs,
		onError:  strategy,
	}, nil
}

// Process applies field mappings to the input records.
// It returns the transformed records with fields mapped from source to target paths.
//
// The context can be used to cancel long-running operations.
//
// For each input record:
//  1. Creates a new target record
//  2. For each mapping, extracts source value and sets target value
//  3. Handles missing fields according to onMissing configuration
//  4. Applies transforms if configured
//
// Returns the transformed records and any error that occurred.
func (m *MappingModule) Process(_ context.Context, records []map[string]interface{}) ([]map[string]interface{}, error) {
	if records == nil {
		return []map[string]interface{}{}, nil
	}

	startTime := time.Now()
	inputCount := len(records)

	logger.Debug("filter processing started",
		slog.String("module_type", "mapping"),
		slog.Int("mapping_count", len(m.mappings)),
		slog.Int("input_records", inputCount),
		slog.String("on_error", string(m.onError)),
	)

	result := make([]map[string]interface{}, 0, len(records))
	skippedCount := 0

	for recordIdx, record := range records {
		targetRecord, err := m.processRecord(record, recordIdx)
		if err != nil {
			var mappingErr *MappingError
			hasContext := errors.As(err, &mappingErr)
			switch m.onError {
			case errhandling.OnErrorFail:
				duration := time.Since(startTime)
				logger.Error("filter processing failed",
					slog.String("module_type", "mapping"),
					slog.Int("record_index", recordIdx),
					slog.Duration("duration", duration),
					slog.String("error", err.Error()),
				)
				return nil, err
			case errhandling.OnErrorSkip:
				skippedCount++
				if hasContext {
					logger.Warn("skipping record due to mapping error",
						slog.String("module_type", "mapping"),
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
						slog.String("module_type", "mapping"),
						slog.Int("record_index", recordIdx),
						slog.String("error", err.Error()),
					)
				}
				continue
			case errhandling.OnErrorLog:
				if hasContext {
					logger.Error("mapping error (continuing)",
						slog.String("module_type", "mapping"),
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
						slog.String("module_type", "mapping"),
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

	duration := time.Since(startTime)
	outputCount := len(result)

	logger.Info("filter processing completed",
		slog.String("module_type", "mapping"),
		slog.Int("input_records", inputCount),
		slog.Int("output_records", outputCount),
		slog.Int("skipped_records", skippedCount),
		slog.Duration("duration", duration),
	)

	return result, nil
}

// processRecord applies all mappings to a single record in-place.
func (m *MappingModule) processRecord(record map[string]interface{}, recordIdx int) (map[string]interface{}, error) {
	for mappingIdx, mapping := range m.mappings {
		// Empty source means delete the target field
		if mapping.Source == "" {
			recordpath.Delete(record, mapping.Target)
			continue
		}

		value, found, err := m.getSourceValue(record, mapping, recordIdx, mappingIdx)
		if err != nil {
			return record, err
		}
		if !found {
			continue
		}

		transformedValue, err := m.applyTransforms(value, mapping)
		if err != nil {
			return m.handleTransformError(err, mapping, recordIdx, mappingIdx, value, record)
		}

		if err := recordpath.Set(record, mapping.Target, transformedValue); err != nil {
			return m.handleSetValueError(err, mapping, recordIdx, mappingIdx, transformedValue, record)
		}
	}

	return record, nil
}

// getSourceValue retrieves the source value for a mapping, handling missing field cases.
func (m *MappingModule) getSourceValue(record map[string]interface{}, mapping MappingConfig, recordIdx, mappingIdx int) (interface{}, bool, error) {
	value, found := recordpath.Get(record, mapping.Source)

	if !found {
		switch mapping.OnMissing {
		case OnMissingSetNull:
			return nil, true, nil
		case OnMissingSkipField:
			return nil, false, nil
		case OnMissingUseDefault:
			return mapping.DefaultValue, true, nil
		case OnMissingFail:
			message := fmt.Sprintf("missing required field %q for target %q at record %d, mapping %d",
				mapping.Source, mapping.Target, recordIdx, mappingIdx)
			return nil, false, newMappingError(ErrCodeMissingField, message, mapping, recordIdx, mappingIdx, nil, "")
		}
	}

	return value, found, nil
}

// handleTransformError creates an appropriate error for transform failures.
func (m *MappingModule) handleTransformError(err error, mapping MappingConfig, recordIdx, mappingIdx int, value interface{}, target map[string]interface{}) (map[string]interface{}, error) {
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

// handleSetValueError creates an appropriate error for set value failures.
func (m *MappingModule) handleSetValueError(err error, mapping MappingConfig, recordIdx, mappingIdx int, value interface{}, target map[string]interface{}) (map[string]interface{}, error) {
	message := fmt.Sprintf("failed to set target field %q at record %d, mapping %d: %v",
		mapping.Target, recordIdx, mappingIdx, err)
	return target, newMappingError(ErrCodeNestedPathFailed, message, mapping, recordIdx, mappingIdx, value, "")
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
// Helpers (applyTrim, applyToInt, etc.) live in mapping_transforms.go.
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
