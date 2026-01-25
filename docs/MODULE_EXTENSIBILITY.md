# Module Extensibility

This document explains how to add new module types to the Canectors runtime without modifying the core codebase.

## Overview

The Canectors runtime uses a **module registry** pattern that enables extensibility:

1. **Implement the interface** (`input.Module`, `filter.Module`, or `output.Module`)
2. **Register your constructor** via `registry.RegisterInput`, `RegisterFilter`, or `RegisterOutput`
3. **Use your type in config** - the runtime will instantiate it automatically

You do **not** need to edit `internal/factory/modules.go`, `cmd/canectors/main.go`, or any core files.

## Adding a New Input Module

### Step 1: Implement the Interface

Create your module in a new package (e.g., `internal/modules/input/kafka/kafka.go`):

```go
package kafka

import (
    "context"
    "fmt"
    
    "github.com/canectors/runtime/internal/modules/input"
    "github.com/canectors/runtime/internal/registry"
    "github.com/canectors/runtime/pkg/connector"
)

// Register at init time so the module is available as soon as the package is imported
func init() {
    registry.RegisterInput("kafka", NewKafkaModule)
}

// KafkaModule implements input.Module for Kafka message consumption
type KafkaModule struct {
    brokers []string
    topic   string
    // ... other fields
}

// NewKafkaModule creates a new Kafka input module from configuration
func NewKafkaModule(cfg *connector.ModuleConfig) (input.Module, error) {
    if cfg == nil {
        return nil, nil
    }
    
    brokers, _ := cfg.Config["brokers"].([]string)
    topic, _ := cfg.Config["topic"].(string)
    
    if topic == "" {
        return nil, fmt.Errorf("topic is required")
    }
    
    return &KafkaModule{
        brokers: brokers,
        topic:   topic,
    }, nil
}

// Fetch implements input.Module
func (m *KafkaModule) Fetch(ctx context.Context) ([]map[string]interface{}, error) {
    // Your Kafka consumer logic here
    return nil, nil
}

// Close implements input.Module
func (m *KafkaModule) Close() error {
    // Cleanup resources
    return nil
}

// Ensure interface compliance at compile time
var _ input.Module = (*KafkaModule)(nil)
```

### Step 2: Import Your Package

In your application or a dedicated `imports.go` file, import the package so `init()` runs:

```go
import (
    _ "github.com/canectors/runtime/internal/modules/input/kafka"
)
```

### Step 3: Use in Configuration

```yaml
input:
  type: kafka
  config:
    brokers:
      - "localhost:9092"
    topic: "events"
```

## Adding a New Filter Module

Filter modules transform data between input and output.

```go
package enrich

import (
    "context"
    
    "github.com/canectors/runtime/internal/modules/filter"
    "github.com/canectors/runtime/internal/registry"
    "github.com/canectors/runtime/pkg/connector"
)

func init() {
    registry.RegisterFilter("enrich", NewEnrichModule)
}

type EnrichModule struct {
    lookupTable map[string]interface{}
}

func NewEnrichModule(cfg connector.ModuleConfig, index int) (filter.Module, error) {
    // Parse configuration
    return &EnrichModule{}, nil
}

func (m *EnrichModule) Process(ctx context.Context, records []map[string]interface{}) ([]map[string]interface{}, error) {
    // Transform records
    return records, nil
}

var _ filter.Module = (*EnrichModule)(nil)
```

### Nested Filter Modules

Custom filter modules registered via the registry **automatically work** in nested configurations, such as condition `then`/`else` blocks. The runtime uses the registry to resolve nested filter types, so your custom filter types will be instantiated correctly even when nested inside condition modules.

Example nested configuration:

```yaml
filters:
  - type: condition
    config:
      expression: "status == 'active'"
      then:
        type: enrich  # Your custom filter type works here!
        config:
          lookupTable: {...}
```

The registry is used for all nested filter module creation, ensuring consistent behavior across the pipeline.

## Adding a New Output Module

Output modules send data to destinations.

```go
package s3

import (
    "context"
    
    "github.com/canectors/runtime/internal/modules/output"
    "github.com/canectors/runtime/internal/registry"
    "github.com/canectors/runtime/pkg/connector"
)

func init() {
    registry.RegisterOutput("s3", NewS3Module)
}

type S3Module struct {
    bucket string
    region string
}

func NewS3Module(cfg *connector.ModuleConfig) (output.Module, error) {
    if cfg == nil {
        return nil, nil
    }
    
    bucket, _ := cfg.Config["bucket"].(string)
    region, _ := cfg.Config["region"].(string)
    
    return &S3Module{bucket: bucket, region: region}, nil
}

func (m *S3Module) Send(ctx context.Context, records []map[string]interface{}) (int, error) {
    // Upload to S3
    return len(records), nil
}

func (m *S3Module) Close() error {
    return nil
}

var _ output.Module = (*S3Module)(nil)
```

## Module Interfaces

### input.Module

```go
type Module interface {
    // Fetch retrieves data from the source system
    Fetch(ctx context.Context) ([]map[string]interface{}, error)
    // Close releases resources
    Close() error
}
```

### filter.Module

```go
type Module interface {
    // Process transforms input records
    Process(ctx context.Context, records []map[string]interface{}) ([]map[string]interface{}, error)
}
```

### output.Module

```go
type Module interface {
    // Send transmits records to the destination
    Send(ctx context.Context, records []map[string]interface{}) (int, error)
    // Close releases resources
    Close() error
}
```

## Registry API Reference

See [godoc for internal/registry](../internal/registry/registry.go) for the complete API.

### Registration Functions

- `RegisterInput(moduleType string, constructor InputConstructor)` - Register an input module constructor
- `RegisterFilter(moduleType string, constructor FilterConstructor)` - Register a filter module constructor
- `RegisterOutput(moduleType string, constructor OutputConstructor)` - Register an output module constructor

### Constructor Signatures

```go
// InputConstructor creates an input module from configuration
// Returns an error if the configuration is invalid.
// Return (nil, nil) if the configuration is nil.
type InputConstructor func(cfg *connector.ModuleConfig) (input.Module, error)

// FilterConstructor creates a filter module from configuration
type FilterConstructor func(cfg connector.ModuleConfig, index int) (filter.Module, error)

// OutputConstructor creates an output module from configuration
type OutputConstructor func(cfg *connector.ModuleConfig) (output.Module, error)
```

### Utility Functions

- `GetInputConstructor(moduleType string) InputConstructor` - Look up a registered input constructor
- `GetFilterConstructor(moduleType string) FilterConstructor` - Look up a registered filter constructor
- `GetOutputConstructor(moduleType string) OutputConstructor` - Look up a registered output constructor
- `ListInputTypes() []string` - List all registered input module types
- `ListFilterTypes() []string` - List all registered filter module types
- `ListOutputTypes() []string` - List all registered output module types

## Stub Fallback

If a module type is not registered, the factory creates a **stub module** that:

- Logs when invoked
- Returns sample data (input) or passes through data unchanged (filter)
- Succeeds without errors (output)

This allows pipelines to run even with unimplemented module types, which is useful for:

- Testing pipeline structure without all implementations
- Gradual development of module implementations
- Validating configuration before all modules are ready

## Built-in Modules

The following modules are registered by default:

### Input Modules

| Type | Description |
|------|-------------|
| `httpPolling` | HTTP polling with pagination and authentication |
| `webhook` | HTTP webhook server for push-based data |

### Filter Modules

| Type | Description |
|------|-------------|
| `mapping` | Field mapping and transformation |
| `condition` | Conditional routing based on expressions |

### Output Modules

| Type | Description |
|------|-------------|
| `httpRequest` | HTTP POST/PUT/PATCH to REST APIs |

## Module Boundaries

Understanding module boundaries is crucial for implementing correct modules. Each module type has clear responsibilities and should not perform tasks outside its scope.

**For comprehensive boundary documentation, see [MODULE_BOUNDARIES.md](./MODULE_BOUNDARIES.md).**

### Quick Reference

The pipeline follows a clear data flow: **Input → Filter → Output**

- **Input modules**: Fetch data from sources (don't transform or send)
- **Filter modules**: Transform data (don't fetch or send)
- **Output modules**: Send data to destinations (don't fetch or transform)

### Interface Stability Guarantee

The module interfaces (`input.Module`, `filter.Module`, `output.Module`) are designed to remain stable across versions:

- New methods will NOT be added to these interfaces
- Optional capabilities use separate interfaces (e.g., `output.PreviewableModule`)
- Modules implemented today will work in future versions

### Runtime Boundary Enforcement

The runtime interacts with modules **only** through public interfaces:

- Runtime does NOT access module internals
- Modules do NOT import runtime packages (`internal/runtime`, `internal/factory`)
- All interactions use interface methods (`Fetch()`, `Process()`, `Send()`, `Close()`)

**See [MODULE_BOUNDARIES.md](./MODULE_BOUNDARIES.md) for:**
- Detailed responsibilities and anti-patterns for each module type
- Complete examples of correct and incorrect implementations
- Common pitfalls and how to avoid them
- Architectural principles and enforcement mechanisms

## Best Practices

1. **Use init() for registration** - This ensures modules are available when the package is imported
2. **Handle nil configs gracefully** - Return nil or a default when config is nil
3. **Return meaningful errors** - Especially for invalid configuration
4. **Implement interface compliance checks** - Use `var _ Interface = (*Type)(nil)` to verify your module implements the interface correctly (this catches implementation errors at compile time)
5. **Document your module** - Include godoc comments explaining configuration options
6. **Write tests** - Test your constructor and module behavior
7. **Respect module boundaries** - Don't perform tasks outside your module's responsibility
8. **Keep modules stateless** - Avoid storing state between pipeline executions (or manage it internally)
