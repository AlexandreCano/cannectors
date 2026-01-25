# Module Boundaries

This document explains the clear boundaries between module types and the runtime, ensuring developers can implement modules confidently without over-engineering or breaking changes.

## Overview

The Canectors runtime follows a strict separation of concerns with well-defined boundaries:

- **Input modules**: Fetch data from sources
- **Filter modules**: Transform data
- **Output modules**: Send data to destinations
- **Runtime**: Orchestrates execution and manages resources

These boundaries are enforced at compile time through Go interfaces and are designed to remain stable across versions.

## Module Responsibilities

### Input Modules

**What Input Modules Do:**
- Fetch data from external source systems (APIs, databases, files, message queues, etc.)
- Return data in a standardized format: `[]map[string]interface{}`
- Manage resources (connections, file handles) and release them via `Close()`
- Handle source-specific protocols and authentication
- Respect context cancellation and timeouts

**What Input Modules Should NOT Do:**
- ❌ Transform or filter data (that's the filter module's responsibility)
- ❌ Send data to destinations (that's the output module's responsibility)
- ❌ Perform business logic beyond data retrieval
- ❌ Store state between `Fetch()` calls (modules should be stateless or manage their own state)

**Example Correct Usage:**
```go
// ✅ Correct: Input module fetches data
func (m *HTTPPollingModule) Fetch(ctx context.Context) ([]map[string]interface{}, error) {
    resp, err := m.client.Get(ctx, m.endpoint)
    // ... parse response into []map[string]interface{}
    return records, nil
}
```

**Example Anti-Pattern:**
```go
// ❌ Wrong: Input module should not transform data
func (m *HTTPPollingModule) Fetch(ctx context.Context) ([]map[string]interface{}, error) {
    records, _ := fetchFromAPI(ctx)
    // ❌ Don't transform here - that's the filter's job
    transformed := transformRecords(records)  // WRONG
    return transformed, nil
}
```

### Filter Modules

**What Filter Modules Do:**
- Transform records (field mapping, value conversion, enrichment)
- Filter records (conditional processing, routing)
- Validate and sanitize data
- Aggregate or split records
- Process data in sequence (filters are chained)

**What Filter Modules Should NOT Do:**
- ❌ Fetch data from external sources (that's the input module's responsibility)
- ❌ Send data to destinations (that's the output module's responsibility)
- ❌ Store state between pipeline executions (filters should be stateless)
- ❌ Perform side effects outside of data transformation (e.g., logging to external systems)

**Example Correct Usage:**
```go
// ✅ Correct: Filter module transforms data
func (m *MappingModule) Process(ctx context.Context, records []map[string]interface{}) ([]map[string]interface{}, error) {
    transformed := make([]map[string]interface{}, len(records))
    for i, record := range records {
        transformed[i] = map[string]interface{}{
            "userId": record["id"],
            "fullName": record["name"],
        }
    }
    return transformed, nil
}
```

**Example Anti-Pattern:**
```go
// ❌ Wrong: Filter module should not fetch external data
func (m *EnrichModule) Process(ctx context.Context, records []map[string]interface{}) ([]map[string]interface{}, error) {
    for _, record := range records {
        // ❌ Don't fetch external data here - that's the input's job
        extraData, _ := fetchFromExternalAPI(record["id"])  // WRONG
        record["extra"] = extraData
    }
    return records, nil
}
```

### Output Modules

**What Output Modules Do:**
- Send data to external destination systems (APIs, databases, message queues, etc.)
- Format data appropriately for the destination (JSON, XML, etc.)
- Handle destination-specific protocols and authentication
- Manage resources (connections, file handles) and release them via `Close()`
- Return the count of successfully sent records

**What Output Modules Should NOT Do:**
- ❌ Fetch data from sources (that's the input module's responsibility)
- ❌ Transform or filter data (that's the filter module's responsibility)
- ❌ Perform business logic beyond data transmission
- ❌ Store state between `Send()` calls (modules should be stateless or manage their own state)

**Example Correct Usage:**
```go
// ✅ Correct: Output module sends data
func (m *HTTPRequestModule) Send(ctx context.Context, records []map[string]interface{}) (int, error) {
    body, _ := json.Marshal(records)
    resp, err := m.client.Post(ctx, m.endpoint, body)
    // ... handle response
    return len(records), nil
}
```

**Example Anti-Pattern:**
```go
// ❌ Wrong: Output module should not transform data
func (m *HTTPRequestModule) Send(ctx context.Context, records []map[string]interface{}) (int, error) {
    // ❌ Don't transform here - that's the filter's job
    transformed := transformRecords(records)  // WRONG
    body, _ := json.Marshal(transformed)
    // ... send
    return len(transformed), nil
}
```

## Runtime Boundaries

### How the Runtime Interacts with Modules

The runtime interacts with modules **only** through their public interfaces:

1. **Module Creation**: Uses factory functions that return interface types
2. **Module Execution**: Calls interface methods (`Fetch()`, `Process()`, `Send()`)
3. **Resource Management**: Calls `Close()` to release resources
4. **Type Assertions**: Uses type assertions for optional interfaces (e.g., `PreviewableModule`)

**The runtime does NOT:**
- Access module internals (private fields, unexported methods)
- Require modules to import runtime internals
- Use reflection to access private members
- Depend on concrete module types

### How Modules Interact with the Runtime

Modules interact with the runtime through:

1. **Interface Implementation**: Implement the required interface methods
2. **Context Usage**: Use `context.Context` for cancellation and timeouts
3. **Error Returns**: Return errors that the runtime handles
4. **Resource Management**: Implement `Close()` for cleanup

**Modules should NOT:**
- Import runtime internals (e.g., `internal/runtime`, `internal/factory`)
- Access runtime private types or functions
- Depend on runtime implementation details
- Store references to runtime objects

### Boundary Enforcement

Boundaries are enforced at compile time through:

1. **Interface Type Declarations**: The `Executor` struct declares fields as interface types, not concrete types. Go's type system enforces this at compile time - the runtime cannot access concrete module types or their internals.

2. **Module Implementation Checks**: `var _ Interface = (*Type)(nil)` in module implementations verifies that concrete types implement the required interfaces. This is useful for module developers to catch implementation errors early.

3. **Package Boundaries**: Modules in `internal/modules/` don't import runtime internals (`internal/runtime`, `internal/factory`).

4. **Factory Pattern**: Module creation goes through factory functions that return interfaces, ensuring the runtime only receives interface types.

**Example:**
```go
// ✅ Correct: Runtime uses interface type (enforced by Go's type system)
type Executor struct {
    inputModule   input.Module      // Interface type - Go enforces this at compile time
    filterModules []filter.Module   // Interface type - cannot be changed to concrete type
    outputModule  output.Module    // Interface type - runtime cannot access internals
}

// ✅ Correct: Module implementation check (verifies module implements interface)
type HTTPPollingModule struct { /* ... */ }
func (m *HTTPPollingModule) Fetch(ctx context.Context) ([]map[string]interface{}, error) { /* ... */ }
func (m *HTTPPollingModule) Close() error { /* ... */ }
var _ input.Module = (*HTTPPollingModule)(nil)  // Verifies HTTPPollingModule implements input.Module
```

**Note:** The runtime's use of interfaces is guaranteed by Go's type system through the field declarations in `Executor`. The `var _ Interface = (*Type)(nil)` declarations verify that module implementations correctly implement the interfaces, but they don't verify runtime behavior - that's already enforced by the type system.

## Interface Stability

### Stable Interfaces

The core module interfaces are designed to remain stable:

- `input.Module`: `Fetch()`, `Close()`
- `filter.Module`: `Process()`
- `output.Module`: `Send()`, `Close()`

**Stability guarantees:**
- New methods will NOT be added to these interfaces
- Existing methods will NOT be removed or changed
- Modules implemented today will work in future versions

### Optional Extensions

Optional capabilities are provided via separate interfaces:

- `output.PreviewableModule`: Extends `Module` with `PreviewRequest()` for dry-run mode

**Extension pattern:**
```go
// Base interface (stable)
type Module interface {
    Send(ctx context.Context, records []map[string]interface{}) (int, error)
    Close() error
}

// Optional extension (also stable)
type PreviewableModule interface {
    Module  // Composition
    PreviewRequest(records []map[string]interface{}, opts PreviewOptions) ([]RequestPreview, error)
}
```

This pattern ensures:
- Base interfaces remain minimal and stable
- Optional features don't break existing modules
- New capabilities can be added without changing core interfaces

## Common Pitfalls

### Pitfall 1: Mixing Responsibilities

**Problem:** A module performs tasks outside its responsibility.

```go
// ❌ Wrong: Input module transforms data
func (m *InputModule) Fetch(ctx context.Context) ([]map[string]interface{}, error) {
    records := fetchFromSource(ctx)
    return transformRecords(records), nil  // Should be done by filter
}
```

**Solution:** Keep modules focused on their single responsibility.

```go
// ✅ Correct: Input module only fetches
func (m *InputModule) Fetch(ctx context.Context) ([]map[string]interface{}, error) {
    return fetchFromSource(ctx), nil
}
```

### Pitfall 2: Accessing Runtime Internals

**Problem:** A module imports or depends on runtime internals.

```go
// ❌ Wrong: Module imports runtime internals
import "github.com/canectors/runtime/internal/runtime"

func (m *MyModule) Process(ctx context.Context, records []map[string]interface{}) ([]map[string]interface{}, error) {
    // Accessing runtime internals - breaks boundary
    executor := runtime.NewExecutor(...)  // WRONG
}
```

**Solution:** Modules should only use their interface and standard library.

```go
// ✅ Correct: Module only uses its interface
func (m *MyModule) Process(ctx context.Context, records []map[string]interface{}) ([]map[string]interface{}, error) {
    // Only use context and records - no runtime dependencies
    return transform(records), nil
}
```

### Pitfall 3: Storing State Between Executions

**Problem:** A filter module stores state that persists between pipeline executions.

```go
// ❌ Wrong: Filter stores state between executions
type MyFilter struct {
    processedCount int  // State persists between executions
}

func (m *MyFilter) Process(ctx context.Context, records []map[string]interface{}) ([]map[string]interface{}, error) {
    m.processedCount++  // State persists - may cause issues
    // ...
}
```

**Solution:** Keep filters stateless or reset state appropriately.

```go
// ✅ Correct: Filter is stateless
type MyFilter struct {
    config map[string]interface{}  // Configuration only, not execution state
}

func (m *MyFilter) Process(ctx context.Context, records []map[string]interface{}) ([]map[string]interface{}, error) {
    // Process records without storing execution state
    return transform(records), nil
}
```

### Pitfall 4: Breaking Interface Stability

**Problem:** Expecting new methods to be added to core interfaces.

```go
// ❌ Wrong: Expecting new methods
type MyModule struct {}
func (m *MyModule) Fetch(ctx context.Context) ([]map[string]interface{}, error) { /* ... */ }
func (m *MyModule) Close() error { /* ... */ }
func (m *MyModule) NewMethod() error { /* ... */ }  // Won't be called by runtime
```

**Solution:** Use interface composition for extensions.

```go
// ✅ Correct: Use extension interface
type ExtendedModule interface {
    input.Module
    NewMethod() error
}
```

## Examples

### Example 1: Correct Module Implementation

```go
package kafka

import (
    "context"
    "github.com/canectors/runtime/internal/modules/input"
    "github.com/canectors/runtime/internal/registry"
    "github.com/canectors/runtime/pkg/connector"
)

func init() {
    registry.RegisterInput("kafka", NewKafkaModule)
}

type KafkaModule struct {
    brokers []string
    topic   string
    consumer *kafka.Consumer
}

func NewKafkaModule(cfg *connector.ModuleConfig) (input.Module, error) {
    // Parse config and create consumer
    return &KafkaModule{...}, nil
}

// ✅ Correct: Only fetches data, doesn't transform
func (m *KafkaModule) Fetch(ctx context.Context) ([]map[string]interface{}, error) {
    messages, err := m.consumer.ReadMessages(ctx)
    // Convert messages to []map[string]interface{}
    return records, err
}

// ✅ Correct: Releases resources
func (m *KafkaModule) Close() error {
    return m.consumer.Close()
}

var _ input.Module = (*KafkaModule)(nil)
```

### Example 2: Correct Boundary Usage

```go
// ✅ Runtime uses interfaces only
executor := runtime.NewExecutorWithModules(
    inputModule,    // input.Module interface
    filterModules,  // []filter.Module interface
    outputModule,   // output.Module interface
    dryRun,
)

// ✅ Modules don't know about runtime
type MyFilter struct {
    config map[string]interface{}
}

func (m *MyFilter) Process(ctx context.Context, records []map[string]interface{}) ([]map[string]interface{}, error) {
    // Only uses context and records - no runtime dependencies
    return transform(records), nil
}
```

## Summary

**Key Principles:**
1. **Single Responsibility**: Each module type has one clear responsibility
2. **Interface-Based**: All interactions use interfaces, not concrete types
3. **Stable Contracts**: Core interfaces remain stable across versions
4. **Clear Boundaries**: Runtime and modules interact only through public interfaces
5. **No Dependencies**: Modules don't depend on runtime internals

**Benefits:**
- ✅ Easy to implement new modules
- ✅ No breaking changes when runtime evolves
- ✅ Modules can be developed independently
- ✅ Clear separation of concerns
- ✅ Testable in isolation

For more information on implementing modules, see [MODULE_EXTENSIBILITY.md](./MODULE_EXTENSIBILITY.md).
