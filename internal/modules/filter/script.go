// Package filter provides implementations for filter modules.
// Script module executes JavaScript transformations using the Goja engine.
package filter

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"

	"github.com/canectors/runtime/internal/logger"
)

// Error codes for script module
const (
	ErrCodeScriptEmpty          = "SCRIPT_EMPTY"
	ErrCodeScriptTooLong        = "SCRIPT_TOO_LONG"
	ErrCodeCompilationFailed    = "COMPILATION_FAILED"
	ErrCodeMissingTransform     = "MISSING_TRANSFORM"
	ErrCodeNotFunction          = "NOT_FUNCTION"
	ErrCodeExecutionFailed      = "EXECUTION_FAILED"
	ErrCodeInvalidScriptFile    = "INVALID_SCRIPT_FILE"
	ErrCodeScriptFileReadFailed = "SCRIPT_FILE_READ_FAILED"
)

// Security limits for script validation
const (
	// MaxScriptLength is the maximum allowed script length in bytes (100KB)
	MaxScriptLength = 100 * 1024
)

// Common errors for script module
var (
	// ErrScriptEmpty is returned when the script is empty or whitespace-only
	ErrScriptEmpty = fmt.Errorf("script cannot be empty")
	// ErrScriptTooLong is returned when the script exceeds MaxScriptLength
	ErrScriptTooLong = fmt.Errorf("script exceeds maximum length")
	// ErrMissingTransformFunc is returned when the script doesn't define a transform function
	ErrMissingTransformFunc = fmt.Errorf("transform function not found in script")
	// ErrTransformNotFunction is returned when transform is defined but is not a function
	ErrTransformNotFunction = fmt.Errorf("transform is not a function")
)

// ScriptConfig represents the configuration for a script filter module.
// Either Script or ScriptFile must be provided (but not both).
type ScriptConfig struct {
	// Script is the inline JavaScript source code containing a transform(record) function
	Script string `json:"script,omitempty"`
	// ScriptFile is the path to a JavaScript file containing the transform(record) function
	ScriptFile string `json:"scriptFile,omitempty"`
	// OnError specifies error handling mode: "fail" (default), "skip", "log"
	OnError string `json:"onError,omitempty"`
}

// ScriptModule implements a filter that executes JavaScript transformations using Goja.
// It transforms input records by executing a user-defined transform(record) function.
//
// Thread Safety:
//   - Goja runtime instances are NOT goroutine-safe
//   - Each ScriptModule instance has its own runtime
//   - Process() should not be called concurrently on the same instance
//
// Context Cancellation:
//   - JavaScript execution can be interrupted via runtime.Interrupt() when context is canceled
//   - A goroutine monitors context cancellation and interrupts JavaScript execution
type ScriptModule struct {
	scriptSource string
	onError      string
	runtime      *goja.Runtime // Not goroutine-safe - one runtime per module instance
	transformFn  goja.Callable
	interruptMu  sync.Mutex // Protects interrupt state
}

// ScriptError carries structured context for script execution failures.
type ScriptError struct {
	Code        string
	Message     string
	RecordIndex int
	StackTrace  string
	Details     map[string]interface{}
}

func (e *ScriptError) Error() string {
	return e.Message
}

// newScriptError creates a ScriptError with optional details.
func newScriptError(code, message string, recordIdx int, stackTrace string, err error) *ScriptError {
	details := make(map[string]interface{})
	if err != nil {
		details["underlying_error"] = err.Error()
	}
	if stackTrace != "" {
		details["stack_trace"] = stackTrace
	}

	return &ScriptError{
		Code:        code,
		Message:     message,
		RecordIndex: recordIdx,
		StackTrace:  stackTrace,
		Details:     details,
	}
}

// NewScriptFromConfig creates a new script filter module from configuration.
// It validates the script, compiles it, and verifies the transform function exists.
// Supports both inline script (config.Script) and file-based script (config.ScriptFile).
//
// Security considerations:
//   - Scripts are validated for length (max 100KB) to prevent DoS
//   - Goja provides sandboxed JavaScript execution (no file system, network access)
//   - Scripts cannot access Go runtime internals directly
//   - Script is compiled once during initialization for efficiency
func NewScriptFromConfig(config ScriptConfig) (*ScriptModule, error) {
	// Get the script source (either inline or from file)
	scriptSource, err := resolveScriptSource(config)
	if err != nil {
		return nil, err
	}

	// Validate script is non-empty and within limits
	if validateErr := validateScript(scriptSource); validateErr != nil {
		return nil, validateErr
	}

	// Normalize onError
	onError := normalizeScriptOnError(config.OnError)

	// Create Goja runtime
	vm := goja.New()

	// Compile and run the script
	_, err = vm.RunString(scriptSource)
	if err != nil {
		return nil, newScriptError(ErrCodeCompilationFailed, fmt.Sprintf("script compilation failed: %v", err), -1, "", err)
	}

	// Get and validate the transform function
	transformFn, err := getTransformFunction(vm)
	if err != nil {
		return nil, err
	}

	logger.Debug("script module initialized",
		slog.Int("script_length", len(scriptSource)),
		slog.String("on_error", onError),
		slog.Bool("from_file", config.ScriptFile != ""),
	)

	return &ScriptModule{
		scriptSource: scriptSource,
		onError:      onError,
		runtime:      vm,
		transformFn:  transformFn,
	}, nil
}

// resolveScriptSource returns the script source code, either from inline config or from file.
// Validates script file path to prevent path traversal attacks.
func resolveScriptSource(config ScriptConfig) (string, error) {
	if config.Script != "" && config.ScriptFile != "" {
		return "", newScriptError(ErrCodeInvalidScriptFile, "cannot specify both 'script' and 'scriptFile' - use only one", -1, "", nil)
	}

	// If inline script is provided, use it
	if config.Script != "" {
		return config.Script, nil
	}

	// If script file is provided, validate and read it
	if config.ScriptFile != "" {
		// Validate path format and security
		if err := validateScriptFilePath(config.ScriptFile); err != nil {
			return "", err
		}

		// Check file size before reading to prevent DoS via memory exhaustion
		fileInfo, err := os.Stat(config.ScriptFile)
		if err != nil {
			return "", newScriptError(ErrCodeScriptFileReadFailed, fmt.Sprintf("failed to stat script file %q: %v", config.ScriptFile, err), -1, "", err)
		}

		// Reject files that exceed MaxScriptLength before reading
		if fileInfo.Size() > MaxScriptLength {
			return "", newScriptError(ErrCodeScriptTooLong, fmt.Sprintf("script file %q exceeds maximum length: %d bytes exceeds maximum %d bytes", config.ScriptFile, fileInfo.Size(), MaxScriptLength), -1, "", nil)
		}

		// Read file with size limit to prevent DoS even if file grows between Stat and Read
		file, err := os.Open(config.ScriptFile)
		if err != nil {
			return "", newScriptError(ErrCodeScriptFileReadFailed, fmt.Sprintf("failed to open script file %q: %v", config.ScriptFile, err), -1, "", err)
		}
		defer func() {
			if closeErr := file.Close(); closeErr != nil {
				logger.Warn("failed to close script file",
					slog.String("file", config.ScriptFile),
					slog.String("error", closeErr.Error()),
				)
			}
		}()

		// Use LimitReader to cap reading at MaxScriptLength+1 bytes
		// If we read more than MaxScriptLength, the file is too large
		limitedReader := io.LimitReader(file, MaxScriptLength+1)
		content, err := io.ReadAll(limitedReader)
		if err != nil {
			return "", newScriptError(ErrCodeScriptFileReadFailed, fmt.Sprintf("failed to read script file %q: %v", config.ScriptFile, err), -1, "", err)
		}

		// Check if we hit the limit (file was larger than MaxScriptLength)
		if len(content) > MaxScriptLength {
			return "", newScriptError(ErrCodeScriptTooLong, fmt.Sprintf("script file %q exceeds maximum length: file is larger than %d bytes", config.ScriptFile, MaxScriptLength), -1, "", nil)
		}

		return string(content), nil
	}

	// Neither provided - this should have been caught by ParseScriptConfig
	return "", newScriptError(ErrCodeScriptEmpty, "either 'script' or 'scriptFile' must be provided", -1, "", nil)
}

// validateScriptFilePath validates the script file path for security and format.
// Prevents path traversal attacks and validates path format.
func validateScriptFilePath(filePath string) error {
	if filePath == "" {
		return newScriptError(ErrCodeInvalidScriptFile, "scriptFile path cannot be empty", -1, "", nil)
	}

	// Validate path doesn't contain null bytes or other invalid characters
	if strings.Contains(filePath, "\x00") {
		return newScriptError(ErrCodeInvalidScriptFile, "scriptFile path contains invalid characters", -1, "", nil)
	}

	// Clean the path to resolve any . or .. components
	cleaned := filepath.Clean(filePath)

	// Convert to forward slashes for consistent checking across platforms
	normalized := filepath.ToSlash(cleaned)

	// Check for path traversal by examining path segments
	// A path with ".." as a segment indicates traversal attempt
	// This avoids false positives like "scripts/..hidden/transform.js"
	segments := strings.Split(normalized, "/")
	for _, segment := range segments {
		if segment == ".." {
			return newScriptError(ErrCodeInvalidScriptFile, fmt.Sprintf("scriptFile path contains path traversal: %q", filePath), -1, "", nil)
		}
	}

	// Also check for leading ".." which is a common traversal pattern
	if strings.HasPrefix(normalized, "../") || normalized == ".." {
		return newScriptError(ErrCodeInvalidScriptFile, fmt.Sprintf("scriptFile path contains path traversal: %q", filePath), -1, "", nil)
	}

	// Check for absolute paths (optional - can be allowed if needed)
	// For now, we allow absolute paths but log a warning
	if filepath.IsAbs(cleaned) {
		logger.Warn("scriptFile uses absolute path",
			slog.String("path", cleaned),
		)
	}

	return nil
}

// validateScript validates the script is non-empty and within length limits.
func validateScript(script string) error {
	if len(script) == 0 || isScriptWhitespaceOnly(script) {
		return newScriptError(ErrCodeScriptEmpty, "script cannot be empty", -1, "", ErrScriptEmpty)
	}
	if len(script) > MaxScriptLength {
		return newScriptError(ErrCodeScriptTooLong, fmt.Sprintf("script exceeds maximum length: %d bytes exceeds maximum %d bytes", len(script), MaxScriptLength), -1, "", ErrScriptTooLong)
	}
	return nil
}

// isScriptWhitespaceOnly checks if a string contains only whitespace.
func isScriptWhitespaceOnly(s string) bool {
	for _, r := range s {
		if r != ' ' && r != '\t' && r != '\n' && r != '\r' {
			return false
		}
	}
	return true
}

// normalizeScriptOnError normalizes the onError configuration value.
func normalizeScriptOnError(onError string) string {
	if onError == "" {
		return OnErrorFail
	}
	if onError != OnErrorFail && onError != OnErrorSkip && onError != OnErrorLog {
		logger.Warn("invalid onError value for script module; defaulting to fail",
			slog.String("on_error", onError),
		)
		return OnErrorFail
	}
	return onError
}

// getTransformFunction retrieves and validates the transform function from the runtime.
func getTransformFunction(vm *goja.Runtime) (goja.Callable, error) {
	transformVal := vm.Get("transform")
	if transformVal == nil || goja.IsUndefined(transformVal) {
		return nil, newScriptError(ErrCodeMissingTransform, "transform function not found in script", -1, "", ErrMissingTransformFunc)
	}

	transformFn, ok := goja.AssertFunction(transformVal)
	if !ok {
		return nil, newScriptError(ErrCodeNotFunction, "transform is not a function", -1, "", ErrTransformNotFunction)
	}

	return transformFn, nil
}

// ParseScriptConfig parses a script filter configuration from raw config.
// Supports both inline script and script file path.
func ParseScriptConfig(cfg map[string]interface{}) (ScriptConfig, error) {
	config := ScriptConfig{}

	script, hasScript := cfg["script"].(string)
	scriptFile, hasScriptFile := cfg["scriptFile"].(string)

	// Validate that exactly one of script or scriptFile is provided
	if hasScript && hasScriptFile {
		return config, fmt.Errorf("cannot specify both 'script' and 'scriptFile' - use only one")
	}

	if !hasScript && !hasScriptFile {
		// Check if they provided wrong types
		if cfg["script"] != nil {
			return config, fmt.Errorf("field 'script' must be a string")
		}
		if cfg["scriptFile"] != nil {
			return config, fmt.Errorf("field 'scriptFile' must be a string")
		}
		return config, fmt.Errorf("either 'script' or 'scriptFile' is required in script config")
	}

	if hasScript {
		config.Script = script
	}
	if hasScriptFile {
		config.ScriptFile = scriptFile
	}

	if onError, ok := cfg["onError"].(string); ok {
		config.OnError = onError
	}

	return config, nil
}

// Process applies the JavaScript transform function to each input record.
//
// For each record:
//  1. Converts Go map to JavaScript object
//  2. Calls the transform(record) function
//  3. Converts the result back to Go map
//  4. Handles errors according to onError configuration
//
// The context is checked before processing to respect cancellation.
func (m *ScriptModule) Process(ctx context.Context, records []map[string]interface{}) ([]map[string]interface{}, error) {
	// Check context cancellation before processing
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	if records == nil {
		return []map[string]interface{}{}, nil
	}

	startTime := time.Now()
	inputCount := len(records)

	logger.Debug("filter processing started",
		slog.String("module_type", "script"),
		slog.Int("input_records", inputCount),
		slog.String("on_error", m.onError),
	)

	result := make([]map[string]interface{}, 0, len(records))
	skippedCount := 0
	errorCount := 0

	for recordIdx, record := range records {
		// Check context cancellation periodically
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		transformedRecord, err := m.processRecord(ctx, record, recordIdx)
		if err != nil {
			errorCount++
			switch m.onError {
			case OnErrorFail:
				duration := time.Since(startTime)
				logger.Error("filter processing failed",
					slog.String("module_type", "script"),
					slog.Int("record_index", recordIdx),
					slog.Duration("duration", duration),
					slog.String("error", err.Error()),
				)
				return nil, err
			case OnErrorSkip:
				skippedCount++
				logger.Warn("skipping record due to script error",
					slog.String("module_type", "script"),
					slog.Int("record_index", recordIdx),
					slog.String("error", err.Error()),
				)
				continue
			case OnErrorLog:
				logger.Error("script error (continuing)",
					slog.String("module_type", "script"),
					slog.Int("record_index", recordIdx),
					slog.String("error", err.Error()),
				)
				// For log mode, add the original record (not transformed)
				result = append(result, record)
				continue
			}
		}
		result = append(result, transformedRecord)
	}

	duration := time.Since(startTime)
	outputCount := len(result)

	logger.Info("filter processing completed",
		slog.String("module_type", "script"),
		slog.Int("input_records", inputCount),
		slog.Int("output_records", outputCount),
		slog.Int("skipped_records", skippedCount),
		slog.Int("error_count", errorCount),
		slog.Duration("duration", duration),
	)

	return result, nil
}

// processRecord transforms a single record using the JavaScript function.
// The context is used to interrupt JavaScript execution if canceled.
func (m *ScriptModule) processRecord(ctx context.Context, record map[string]interface{}, recordIdx int) (map[string]interface{}, error) {
	// Set up interruption goroutine to monitor context cancellation
	interruptDone := make(chan struct{})
	defer close(interruptDone)

	// Start goroutine to interrupt JavaScript execution if context is canceled
	go func() {
		select {
		case <-ctx.Done():
			// Context was canceled - interrupt JavaScript execution
			m.interruptMu.Lock()
			m.runtime.Interrupt(ctx.Err().Error())
			m.interruptMu.Unlock()
		case <-interruptDone:
			// Function completed normally - clear any pending interrupt
			m.interruptMu.Lock()
			m.runtime.ClearInterrupt()
			m.interruptMu.Unlock()
		}
	}()

	// Convert Go map to JavaScript object
	jsRecord := m.runtime.ToValue(record)

	// Call the transform function
	// If context is canceled, runtime.Interrupt() will cause this to return an error
	result, err := m.transformFn(goja.Undefined(), jsRecord)
	if err != nil {
		// Check if error is due to context cancellation
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, m.handleJSError(err, recordIdx)
	}

	// Clear interrupt after successful execution
	m.interruptMu.Lock()
	m.runtime.ClearInterrupt()
	m.interruptMu.Unlock()

	// Convert result back to Go map
	goResult, err := m.exportToGoMap(result, recordIdx)
	if err != nil {
		return nil, err
	}

	return goResult, nil
}

// handleJSError converts a JavaScript error to a Go error with context.
func (m *ScriptModule) handleJSError(err error, recordIdx int) error {
	// Check if it's a Goja exception
	if jsErr, ok := err.(*goja.Exception); ok {
		// Extract stack trace if available
		stackTrace := ""
		if jsErr.Value() != nil {
			if obj, ok := jsErr.Value().(*goja.Object); ok {
				if stack := obj.Get("stack"); stack != nil && !goja.IsUndefined(stack) {
					stackTrace = stack.String()
				}
			}
		}

		message := fmt.Sprintf("script execution failed at record %d: %v", recordIdx, jsErr.Value())
		return newScriptError(ErrCodeExecutionFailed, message, recordIdx, stackTrace, err)
	}

	// Handle other types of errors
	message := fmt.Sprintf("script execution failed at record %d: %v", recordIdx, err)
	return newScriptError(ErrCodeExecutionFailed, message, recordIdx, "", err)
}

// exportToGoMap converts a JavaScript value back to a Go map.
// The transform function must return an object (map), not a primitive or array.
func (m *ScriptModule) exportToGoMap(value goja.Value, recordIdx int) (map[string]interface{}, error) {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return nil, newScriptError(ErrCodeExecutionFailed, fmt.Sprintf("script at record %d returned null or undefined - transform function must return an object", recordIdx), recordIdx, "", nil)
	}

	exported := value.Export()

	// Explicitly detect and reject arrays before attempting ExportTo
	// Arrays should not be accepted as valid return types
	if arr, ok := exported.([]interface{}); ok {
		return nil, newScriptError(ErrCodeExecutionFailed, fmt.Sprintf("script at record %d returned an array (length %d) - transform function must return an object, not an array", recordIdx, len(arr)), recordIdx, "", nil)
	}
	// Also check for []any (Go 1.18+ type alias)
	if arr, ok := exported.([]any); ok {
		return nil, newScriptError(ErrCodeExecutionFailed, fmt.Sprintf("script at record %d returned an array (length %d) - transform function must return an object, not an array", recordIdx, len(arr)), recordIdx, "", nil)
	}

	// If already a Go map, return it directly
	if result, ok := exported.(map[string]interface{}); ok {
		return result, nil
	}

	// If it's a Goja Object, check if it's actually an array by checking the class name
	if obj, ok := exported.(*goja.Object); ok {
		// Check if the object is actually an array
		if obj.Get("length") != nil {
			// Has a length property - likely an array
			className := obj.ClassName()
			if className == "Array" {
				return nil, newScriptError(ErrCodeExecutionFailed, fmt.Sprintf("script at record %d returned an array - transform function must return an object, not an array", recordIdx), recordIdx, "", nil)
			}
		}

		// Try to export it to a Go map
		var result map[string]interface{}
		if err := m.runtime.ExportTo(value, &result); err != nil {
			return nil, newScriptError(ErrCodeExecutionFailed, fmt.Sprintf("failed to convert script result at record %d: %v", recordIdx, err), recordIdx, "", err)
		}
		return result, nil
	}

	// For primitives and other types, reject explicitly
	return nil, newScriptError(ErrCodeExecutionFailed, fmt.Sprintf("script at record %d returned invalid type %T - transform function must return an object, got %T", recordIdx, exported, exported), recordIdx, "", nil)
}
