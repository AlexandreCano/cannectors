# Story 14.3: JavaScript Logging Integration

Status: in-progress

<!-- Note: Validation is optional. Run validate-create-story for quality check before dev-story. -->

## Story

As a developer,
I want JavaScript console.log() calls in script filter modules to appear in the Go runtime logs,
so that I can debug and monitor script execution using familiar JavaScript logging patterns.

## Acceptance Criteria

1. **Given** I have a script filter module with console.log() calls
   **When** The script executes
   **Then** console.log() output appears in the Go runtime logs
   **And** Log messages include appropriate log level (info/debug)
   **And** Log messages include context (module type, script location)

2. **Given** I have a script with console.error(), console.warn(), console.info()
   **When** The script executes
   **Then** console.error() appears as error level in Go logs
   **And** console.warn() appears as warning level in Go logs
   **And** console.info() appears as info level in Go logs
   **And** console.debug() appears as debug level in Go logs (if debug logging enabled)

3. **Given** I have a script with console.log() calls
   **When** The script processes multiple records
   **Then** Each log call appears in the logs with record context when available
   **And** Log messages are properly formatted and readable
   **And** Log messages do not interfere with script execution

4. **Given** I have a script that logs complex objects
   **When** The script executes console.log() with objects
   **Then** Objects are serialized to JSON in log messages
   **And** Log messages remain readable and not truncated (within limits)
   **And** Circular references in objects are handled gracefully

5. **Given** I have a script filter module
   **When** The module is initialized
   **Then** console.* methods are automatically available in the script
   **And** No additional configuration is required to enable logging
   **And** Logging works out of the box

## Tasks / Subtasks

- [ ] Task 1: Implement console object in Goja runtime (AC: #1, #2, #5)
  - [ ] Create Go functions for console.log, console.error, console.warn, console.info, console.debug
  - [ ] Register console object in Goja runtime during module initialization
  - [ ] Map JavaScript console methods to Go logger calls
  - [ ] Ensure console is available before script compilation

- [ ] Task 2: Implement log level mapping (AC: #2)
  - [ ] Map console.error() → logger.Error()
  - [ ] Map console.warn() → logger.Warn()
  - [ ] Map console.info() → logger.Info()
  - [ ] Map console.log() → logger.Info() (default)
  - [ ] Map console.debug() → logger.Debug()

- [ ] Task 3: Implement log message formatting (AC: #1, #3)
  - [ ] Format log messages with module context (module_type: "script")
  - [ ] Include script location/identifier in log context
  - [ ] Format multiple arguments passed to console methods
  - [ ] Handle string interpolation and formatting

- [ ] Task 4: Implement object serialization for logs (AC: #4)
  - [ ] Convert JavaScript objects to JSON for logging
  - [ ] Handle circular references in objects (prevent infinite loops)
  - [ ] Limit log message length to prevent log flooding
  - [ ] Format arrays and nested objects readably

- [ ] Task 5: Add record context to logs (AC: #3)
  - [ ] Include record index in log context when processing records
  - [ ] Optionally include record ID or key fields in log context
  - [ ] Ensure context doesn't leak sensitive data

- [ ] Task 6: Add tests for JavaScript logging (AC: #1, #2, #3, #4)
  - [ ] Test console.log() appears in Go logs
  - [ ] Test console.error() appears as error level
  - [ ] Test console.warn() appears as warning level
  - [ ] Test console.info() appears as info level
  - [ ] Test console.debug() appears as debug level
  - [ ] Test multiple arguments in console methods
  - [ ] Test object logging (JSON serialization)
  - [ ] Test log message formatting and context
  - [ ] Test log messages during record processing

- [ ] Task 7: Update documentation (AC: #5)
  - [ ] Document console.* methods availability in script modules
  - [ ] Add examples of using console.log() in scripts
  - [ ] Document log levels and formatting
  - [ ] Document log context and record information

## Dev Notes

### Relevant Architecture Patterns and Constraints

**Goja Console Implementation:**
- Goja allows registering Go functions as JavaScript functions
- Use `runtime.Set("console", consoleObject)` to register console
- Console methods should accept variadic arguments (like JavaScript console)
- Console methods should format arguments appropriately

**Logger Integration:**
- Project uses `internal/logger` package with structured logging (slog)
- Logger supports levels: Debug, Info, Warn, Error
- Logger uses slog for structured logging with context fields
- Log messages should include module context for filtering

**Log Message Formatting:**
- JavaScript console methods accept multiple arguments
- Need to format: `console.log("Hello", name, obj)` → "Hello <name> <obj_json>"
- Handle different argument types: strings, numbers, objects, arrays
- Format objects as JSON for readability

**Object Serialization:**
- Use Goja's Export() to convert JavaScript objects to Go values
- Use json.Marshal() to serialize Go values to JSON strings
- Handle circular references (detect and replace with "[Circular]")
- Limit serialization depth to prevent extremely long logs

**Performance Considerations:**
- Logging should not significantly impact script execution performance
- Object serialization can be expensive for large objects
- Consider limiting log message length or depth
- Consider rate limiting if script logs excessively

**Security Considerations:**
- Log messages should not expose sensitive data
- Consider sanitizing log output (remove passwords, tokens, etc.)
- Log length limits prevent log flooding attacks
- Log context should not leak internal implementation details

### Project Structure Notes

**Files to Modify:**
- `internal/modules/filter/script.go` - Add console object registration
- `internal/modules/filter/script_test.go` - Add logging tests

**Console Object Implementation:**
```go
// Create console object with methods
consoleObj := vm.NewObject()
consoleObj.Set("log", func(call goja.FunctionCall) goja.Value {
    // Format arguments and log at info level
    msg := formatConsoleArgs(call.Arguments)
    logger.Info(msg, slog.String("module_type", "script"))
    return goja.Undefined()
})

consoleObj.Set("error", func(call goja.FunctionCall) goja.Value {
    msg := formatConsoleArgs(call.Arguments)
    logger.Error(msg, slog.String("module_type", "script"))
    return goja.Undefined()
})

// ... similar for warn, info, debug

vm.Set("console", consoleObj)
```

**Argument Formatting:**
```go
func formatConsoleArgs(args []goja.Value) string {
    parts := make([]string, 0, len(args))
    for _, arg := range args {
        if goja.IsUndefined(arg) {
            parts = append(parts, "undefined")
        } else if goja.IsNull(arg) {
            parts = append(parts, "null")
        } else {
            // Export to Go value, then serialize to JSON
            goVal := arg.Export()
            jsonBytes, _ := json.Marshal(goVal)
            parts = append(parts, string(jsonBytes))
        }
    }
    return strings.Join(parts, " ")
}
```

**Object Serialization with Circular Reference Handling:**
```go
func serializeForLog(obj interface{}, maxDepth int) string {
    // Use a visited map to detect circular references
    visited := make(map[uintptr]bool)
    return serializeWithVisited(obj, visited, 0, maxDepth)
}
```

**Log Context:**
```go
// When processing records, include record index
logger.Info("Processing record",
    slog.String("module_type", "script"),
    slog.Int("record_index", recordIndex),
    slog.String("script_id", scriptID), // optional identifier
)
```

**Configuration:**
- No additional configuration needed
- Console object is automatically available in all script modules
- Logging respects runtime log level settings

**Testing:**
- Capture log output during tests
- Verify log messages appear with correct levels
- Verify log formatting and context
- Test with various argument types and objects

### References

- [Source: internal/modules/filter/script.go] - Script module implementation (Story 14-2)
- [Source: internal/logger] - Logger package and structured logging
- [Source: docs/ARCHITECTURE.md] - Project architecture and logging patterns
- [Goja FunctionCall documentation](https://pkg.go.dev/github.com/dop251/goja#FunctionCall) - How to implement JavaScript functions in Go

## Dev Agent Record

### Agent Model Used

Claude Sonnet 4.5 (via Cursor)

### Debug Log References

### Code Review Fixes (2026-01-25)

**Review target:** 14-3 JavaScript Logging Integration. Fixes applied automatically (option 1).

- [HIGH] **golangci-lint**: errcheck (console.Set / runtime.Set), gofmt (struct alignment), unparam (formatObject), unused (testLogHandler). Fixed: newJSConsole returns error and checks all Set calls; gofmt alignment; formatObject uses formatMapSafe with depth/seen; testLogHandler used in TestJSConsole_LogLevels.
- [HIGH] **AC2 / Task 6**: Log levels not asserted. Added TestJSConsole_LogLevels using testLogHandler to capture slog records and assert console.error→Error, warn→Warn, info→Info, log→Info, debug→Debug.
- [HIGH] **AC4 circular refs**: formatObject used json.Marshal only; formatArray used length as pseudo-ptr. Fixed: formatMapSafe / formatSliceSafe with reflect.ValueOf(.).Pointer() for seen; formatArray uses uintptr(unsafe.Pointer(obj)); added TestJSConsole_CircularReference.
- [MEDIUM] **AC1 script location**: moduleID always "". Fixed: NewScriptFromConfig passes moduleID = config.ScriptFile or "inline".
- [MEDIUM] **exportToGoMap dead branch**: Removed *goja.Object branch (Export returns Go types, not *goja.Object).

### Completion Notes List

### File List

**Created:**
- internal/modules/filter/console.go
- internal/modules/filter/console_test.go

**Modified:**
- internal/modules/filter/script.go (console init, moduleID, exportToGoMap cleanup)
- go.sum
