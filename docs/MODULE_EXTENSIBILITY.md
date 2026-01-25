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

## Output Module Retry Configuration

The `httpRequest` output module supports advanced retry configuration to handle transient errors intelligently.

### Retry Configuration Options

```yaml
output:
  type: httpRequest
  config:
    endpoint: "https://api.example.com/data"
    method: POST
    retry:
      # Basic retry settings
      maxAttempts: 3                    # Max retry attempts (0 = disabled, default: 3)
      delayMs: 1000                     # Initial delay in ms (default: 1000)
      backoffMultiplier: 2.0            # Exponential backoff multiplier (default: 2.0)
      maxDelayMs: 30000                 # Max delay cap in ms (default: 30000)
      
      # Custom retryable status codes (overrides defaults)
      retryableStatusCodes: [429, 500, 502, 503, 504]  # Default
      # Example: Make 408 retryable, exclude 500
      # retryableStatusCodes: [408, 429, 502, 503, 504]
      
      # Retry-After header support
      useRetryAfterHeader: true         # Use Retry-After header for delay (default: false)
      
      # Body hint for retry decision (expr expression)
      retryHintFromBody: "body.retryable == true"  # expr expression to evaluate (default: "")
```

### Configuration Details

#### retryableStatusCodes

Defines which HTTP status codes should trigger a retry. This configuration takes precedence over defaults.

| Default Codes | Description |
|---------------|-------------|
| 429 | Too Many Requests (rate limiting) |
| 500 | Internal Server Error |
| 502 | Bad Gateway |
| 503 | Service Unavailable |
| 504 | Gateway Timeout |

**Example:** Make 408 (Request Timeout) retryable, exclude 500:
```yaml
retry:
  retryableStatusCodes: [408, 429, 502, 503, 504]
```

**Note:** Status codes NOT in the list will not be retried, even if they were previously retryable by default.

#### useRetryAfterHeader

When enabled, the module uses the `Retry-After` HTTP header (if present) to determine the delay before retrying. Supports:
- Seconds format: `Retry-After: 120` (wait 120 seconds)
- HTTP-date format: `Retry-After: Fri, 31 Dec 2025 23:59:59 GMT`

The delay is capped by `maxDelayMs`. If the header is invalid or absent, falls back to exponential backoff.

```yaml
retry:
  useRetryAfterHeader: true
  maxDelayMs: 60000  # Cap at 60 seconds even if server says more
```

#### retryHintFromBody

An [expr](https://github.com/expr-lang/expr) expression to evaluate against the JSON response body. The parsed JSON body is available as the `body` variable. When configured:
- **Expression returns `true`** → Error is retryable (if status code is in `retryableStatusCodes`)
- **Expression returns `false`** → Error is NOT retryable (overrides status code)
- **Body not valid JSON** → Falls back to status code only

**Note on naming:** In YAML/JSON configuration files, use `retryHintFromBody` (camelCase). In Go code, the field is `RetryHintFromBody` (PascalCase). This is standard Go naming convention - exported fields use PascalCase, while configuration files typically use camelCase.

**Example expressions:**
- `body.retryable == true` → Check boolean field
- `body.error.code == "TEMPORARY"` → Check string field
- `body.error.code == "RATE_LIMIT" || body.error.code == "TEMPORARY"` → Complex conditions

**Example:** API returns `{"error": {"code": "PERMANENT"}}` for non-retryable errors:
```yaml
retry:
  retryableStatusCodes: [500, 503]
  retryHintFromBody: 'body.error.code != "PERMANENT"'
```

With this config:
- 500 with `{"error": {"code": "TEMPORARY"}}` → Retried (expression true)
- 500 with `{"error": {"code": "PERMANENT"}}` → NOT retried (expression false)
- 500 with non-JSON body → Retried (fallback to status code)

### Configuration Precedence

Retry configuration follows this precedence order:
1. **Module config** (e.g., output.config.retry)
2. **Defaults config** (if specified at pipeline level)
3. **Built-in defaults** (maxAttempts=3, delayMs=1000, etc.)

### Special Cases

#### OAuth2 401 Handling

For OAuth2 authentication, 401 Unauthorized triggers token invalidation and a single retry attempt regardless of `retryableStatusCodes`. This is handled specially to support token refresh flows.

#### Network Errors

Network errors (timeout, connection refused, DNS failures) are always retried based on `errhandling.IsRetryable()`, independent of `retryableStatusCodes`. The status code configuration only affects HTTP response errors.

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
