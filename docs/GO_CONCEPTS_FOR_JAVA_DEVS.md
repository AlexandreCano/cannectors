# GO CONCEPTS FOR JAVA DEVS — Canectors Runtime

**Date**: 2026-01-23
**Public cible**: Ingénieur Java Senior (10+ ans) apprenant Go
**Niveau**: Explications concepts Go avec comparaisons Java

---

## 1. Struct vs Class

### Java Class

```java
public class Pipeline {
    private String id;
    private String name;
    private ModuleConfig input;

    public Pipeline(String id, String name, ModuleConfig input) {
        this.id = id;
        this.name = name;
        this.input = input;
    }

    public String getId() {
        return id;
    }

    public void setId(String id) {
        this.id = id;
    }

    public void execute() {
        // Business logic
    }
}
```

### Go Struct

```go
type Pipeline struct {
    ID    string
    Name  string
    Input *ModuleConfig
}

// Constructor function (convention)
func NewPipeline(id, name string, input *ModuleConfig) *Pipeline {
    return &Pipeline{
        ID:    id,
        Name:  name,
        Input: input,
    }
}

// Method (receiver function)
func (p *Pipeline) Execute() error {
    // Business logic
}
```

### Différences Clés

| Aspect | Java | Go |
|--------|------|-----|
| **Data** | Fields privés + getters/setters | Fields publics (si capitalized) |
| **Encapsulation** | `private` + méthodes public | Exported (Uppercase) / non-exported (lowercase) |
| **Héritage** | `extends` | **PAS D'HÉRITAGE** (composition uniquement) |
| **Constructeur** | `new Pipeline(...)` (keyword) | `NewPipeline(...)` (convention function) |
| **Méthodes** | Déclarées dans class | Déclarées via **receiver** (hors struct) |
| **Comportement** | Class = data + behavior | Struct = data seulement, methods séparés |

### Composition vs Héritage

**Java (Héritage)**:
```java
public abstract class BaseModule {
    protected String type;
    public abstract void execute();
}

public class HTTPModule extends BaseModule {
    public void execute() { ... }
}
```

**Go (Composition)**:
```go
// Embedding (composition, pas héritage)
type BaseModule struct {
    Type string
}

type HTTPModule struct {
    BaseModule  // Embedded field (composition)
    Endpoint string
}

func (h *HTTPModule) Execute() error {
    // Can access h.Type directly (promoted field)
    fmt.Println(h.Type)
}
```

**Note**: Go embedding != héritage. Pas de polymorphisme via embedding. Utiliser **interfaces** pour polymorphisme.

---

## 2. Interface Go vs Interface Java

### Java Interface

```java
public interface InputModule {
    List<Record> fetch() throws Exception;
    void close() throws Exception;
    String getType();
}

public class HTTPInput implements InputModule {
    @Override
    public List<Record> fetch() { ... }

    @Override
    public void close() { ... }

    @Override
    public String getType() { return "http"; }
}
```

### Go Interface

```go
// Interface (implicit implementation)
type InputModule interface {
    Fetch(ctx context.Context) ([]Record, error)
    Close() error
}

// Struct (no "implements" keyword)
type HTTPInput struct {
    endpoint string
}

// Methods (automatic interface satisfaction)
func (h *HTTPInput) Fetch(ctx context.Context) ([]Record, error) {
    // ...
}

func (h *HTTPInput) Close() error {
    // ...
}

// HTTPInput satisfies InputModule interface (implicit)
```

### Différences Clés

| Aspect | Java | Go |
|--------|------|-----|
| **Déclaration** | `implements` keyword | **Implicite** (pas de keyword) |
| **Vérification** | Compile-time | Compile-time (mais implicite) |
| **Taille** | Peut avoir beaucoup de méthodes | **Small interfaces** (1-3 méthodes idiomatiques) |
| **Composition** | Interface extends interface | Interface embeds interface |

### Idiom Go: Small Interfaces

**Java (Large Interface)**:
```java
public interface Repository {
    void save(Entity e);
    Entity findById(Long id);
    List<Entity> findAll();
    void delete(Long id);
    void update(Entity e);
    long count();
    boolean exists(Long id);
}
```

**Go (Small Interfaces)**:
```go
type Reader interface {
    Read(p []byte) (n int, err error)
}

type Writer interface {
    Write(p []byte) (n int, err error)
}

type ReadWriter interface {
    Reader  // Embed
    Writer  // Embed
}
```

**Canectors Example**:
```go
// Small interface (2 methods)
type Module interface {
    Fetch(ctx context.Context) ([]Record, error)
    Close() error
}

// Composable extension (optional interface)
type RetryInfoProvider interface {
    GetRetryInfo() *RetryInfo
}

// Module peut optionellement implement RetryInfoProvider
var module Module = createModule()
if provider, ok := module.(RetryInfoProvider); ok {
    info := provider.GetRetryInfo()
}
```

---

## 3. Error Handling — error vs exceptions

### Java Exceptions

```java
public List<Record> fetch() throws IOException, AuthException {
    try {
        HttpResponse resp = client.get(endpoint);
        if (resp.status() != 200) {
            throw new HTTPException("Request failed");
        }
        return parseRecords(resp.body());
    } catch (IOException e) {
        logger.error("Network error", e);
        throw e;
    } catch (ParseException e) {
        throw new RuntimeException("Parse failed", e);
    }
}

// Caller
try {
    List<Record> records = module.fetch();
    process(records);
} catch (IOException e) {
    handleNetworkError(e);
} catch (AuthException e) {
    handleAuthError(e);
}
```

### Go Errors

```go
func (m *HTTPInput) Fetch(ctx context.Context) ([]Record, error) {
    resp, err := m.client.Get(m.endpoint)
    if err != nil {
        logger.Error("network error", slog.String("error", err.Error()))
        return nil, fmt.Errorf("fetching data: %w", err)  // Wrap error
    }

    if resp.StatusCode != 200 {
        return nil, fmt.Errorf("http error %d", resp.StatusCode)
    }

    records, err := parseRecords(resp.Body)
    if err != nil {
        return nil, fmt.Errorf("parsing records: %w", err)
    }

    return records, nil
}

// Caller
records, err := module.Fetch(ctx)
if err != nil {
    // Check error type
    if errors.Is(err, ErrNetwork) {
        handleNetworkError(err)
    } else if errors.Is(err, ErrAuth) {
        handleAuthError(err)
    } else {
        handleGenericError(err)
    }
    return err
}
process(records)
```

### Différences Clés

| Aspect | Java | Go |
|--------|------|-----|
| **Mécanisme** | `throw`/`catch` (stack unwinding) | `return error` (explicit) |
| **Checked exceptions** | Forcé par compilateur (`throws`) | **Pas de checked errors** |
| **Error propagation** | Automatique (stack unwind) | **Manuel** (if err != nil) |
| **Error wrapping** | `new Exception("...", cause)` | `fmt.Errorf("...: %w", err)` |
| **Error inspection** | `instanceof`, `catch` specific | `errors.Is()`, `errors.As()` |
| **Performance** | Stack unwinding coûteux | Return error = cheap |

### Go Error Patterns

**1. Error Wrapping** (`%w` format):
```go
// Wrap error (preserve original)
return fmt.Errorf("connecting to database: %w", err)

// Check wrapped error
if errors.Is(err, sql.ErrNoRows) {
    // Handle not found
}
```

**2. Error As** (type assertion):
```go
var httpErr *HTTPError
if errors.As(err, &httpErr) {
    fmt.Println("HTTP status:", httpErr.StatusCode)
}
```

**3. Custom Errors**:
```go
var ErrNotFound = errors.New("not found")
var ErrUnauthorized = errors.New("unauthorized")

// Check sentinel error
if errors.Is(err, ErrNotFound) {
    // Handle not found
}
```

**4. Multi-Return** (value + error):
```go
func fetch() ([]Record, error) {
    // Happy path
    return records, nil

    // Error path
    return nil, fmt.Errorf("fetch failed: %w", err)
}
```

**Canectors Pattern**:
```go
// errhandling/errors.go
type ClassifiedError struct {
    Category   ErrorCategory  // network, auth, validation, server
    Retryable  bool
    StatusCode int
    OriginalErr error
}

func ClassifyError(err error) *ClassifiedError {
    // Classify error for retry logic
}

// Usage
err := module.Fetch(ctx)
if err != nil {
    classified := errhandling.ClassifyError(err)
    if classified.Retryable {
        // Retry with backoff
    } else {
        // Fail immediately
    }
}
```

---

## 4. context.Context vs ThreadLocal / ExecutorService

### Java ThreadLocal / Future

```java
// ThreadLocal (context propagation)
public class RequestContext {
    private static ThreadLocal<String> requestId = new ThreadLocal<>();

    public static void setRequestId(String id) {
        requestId.set(id);
    }

    public static String getRequestId() {
        return requestId.get();
    }
}

// ExecutorService (timeout/cancellation)
ExecutorService executor = Executors.newSingleThreadExecutor();
Future<List<Record>> future = executor.submit(() -> {
    return fetchRecords();
});

try {
    List<Record> records = future.get(30, TimeUnit.SECONDS);
} catch (TimeoutException e) {
    future.cancel(true);  // Cancel task
}
```

### Go context.Context

```go
// Context creation
ctx := context.Background()                           // Root context
ctx, cancel := context.WithCancel(ctx)               // Cancellable
ctx, cancel := context.WithTimeout(ctx, 30*time.Second)  // With timeout
ctx, cancel := context.WithDeadline(ctx, time.Now().Add(1*time.Hour))  // With deadline
defer cancel()  // Always defer cancel

// Context propagation (pass as first parameter)
func Fetch(ctx context.Context) ([]Record, error) {
    // Check cancellation
    select {
    case <-ctx.Done():
        return nil, ctx.Err()  // context.Canceled or context.DeadlineExceeded
    default:
        // Continue
    }

    // Pass context to sub-functions
    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    resp, err := client.Do(req)  // Request cancelled if ctx cancelled

    return records, nil
}

// Context values (like ThreadLocal, but immutable)
type contextKey string
const requestIDKey contextKey = "requestID"

ctx = context.WithValue(ctx, requestIDKey, "req-123")
requestID := ctx.Value(requestIDKey).(string)
```

### Différences Clés

| Aspect | Java ThreadLocal | Go context.Context |
|--------|------------------|---------------------|
| **Mutability** | Mutable (set/get) | **Immutable** (WithValue returns new context) |
| **Cancellation** | `Future.cancel()` | `ctx.Done()` channel |
| **Timeout** | `future.get(timeout)` | `context.WithTimeout()` |
| **Propagation** | Thread-bound (automatic) | **Manual** (pass as parameter) |
| **Cleanup** | Manual (`remove()`) | **Automatic** (defer cancel) |
| **Type safety** | Generic `<T>` | `interface{}` (need type assertion) |

### Canectors Usage

```go
// runtime/pipeline.go
func (e *Executor) ExecuteWithContext(ctx context.Context, pipeline *Pipeline) (*ExecutionResult, error) {
    // Pass context to all operations
    records, err := e.inputModule.Fetch(ctx)
    if err != nil {
        return nil, err
    }

    for _, filter := range e.filterModules {
        records, err = filter.Process(ctx, records)
        if err != nil {
            return nil, err
        }
    }

    sent, err := e.outputModule.Send(ctx, records)
    return result, err
}

// input/http_polling.go
func (h *HTTPPolling) Fetch(ctx context.Context) ([]Record, error) {
    // HTTP request with context (auto-cancelled if ctx cancelled)
    req, err := http.NewRequestWithContext(ctx, "GET", h.endpoint, nil)
    resp, err := h.client.Do(req)
    // ...
}
```

---

## 5. Composition vs Héritage

### Java Héritage

```java
public abstract class BaseModule {
    protected String type;
    protected Logger logger;

    public BaseModule(String type) {
        this.type = type;
        this.logger = LoggerFactory.getLogger(getClass());
    }

    public String getType() {
        return type;
    }

    protected void log(String message) {
        logger.info(message);
    }
}

public class HTTPModule extends BaseModule {
    public HTTPModule() {
        super("http");
    }

    public void execute() {
        log("Executing HTTP module");  // Inherited method
    }
}
```

### Go Composition (Embedding)

```go
type BaseModule struct {
    Type string
}

func (b *BaseModule) Log(message string) {
    logger.Info(message, slog.String("type", b.Type))
}

type HTTPModule struct {
    BaseModule  // Embedded (composition, pas héritage)
    Endpoint string
}

func NewHTTPModule(endpoint string) *HTTPModule {
    return &HTTPModule{
        BaseModule: BaseModule{Type: "http"},
        Endpoint:   endpoint,
    }
}

func (h *HTTPModule) Execute() error {
    h.Log("Executing HTTP module")  // Promoted method from BaseModule
    // ...
}
```

### Différences Clés

| Aspect | Java Héritage | Go Embedding |
|--------|---------------|--------------|
| **Polymorphisme** | ✅ Oui (`BaseModule ref = new HTTPModule()`) | ❌ Non (embedding != héritage) |
| **Override** | ✅ Oui (`@Override`) | ❌ Non (shadowing, pas override) |
| **Type hierarchy** | ✅ Oui (is-a relationship) | ❌ Non (has-a relationship) |
| **Interface polymorphism** | Via interface | Via interface (seul moyen) |

### Go Polymorphisme via Interface

```go
// Polymorphisme en Go = via interfaces uniquement
type Module interface {
    Execute(ctx context.Context) error
}

type HTTPModule struct { ... }
func (h *HTTPModule) Execute(ctx context.Context) error { ... }

type WebhookModule struct { ... }
func (w *WebhookModule) Execute(ctx context.Context) error { ... }

// Polymorphic usage
var modules []Module = []Module{
    &HTTPModule{},
    &WebhookModule{},
}

for _, module := range modules {
    module.Execute(ctx)  // Polymorphic call
}
```

**Canectors Pattern**:
```go
// No base class, only interfaces
type InputModule interface {
    Fetch(ctx context.Context) ([]Record, error)
    Close() error
}

// Concrete implementations
type HTTPPolling struct { ... }
type Webhook struct { ... }

// Both satisfy interface (polymorphism)
var input InputModule
if config.Type == "httpPolling" {
    input = NewHTTPPolling(config)
} else {
    input = NewWebhook(config)
}

records, err := input.Fetch(ctx)  // Polymorphic call
```

---

## 6. Visibilité — Exported vs Non-Exported

### Java Access Modifiers

```java
public class Pipeline {
    public String id;           // Public (accessible partout)
    protected String name;      // Protected (package + subclasses)
    String version;             // Package-private (package seulement)
    private ModuleConfig input; // Private (class seulement)

    public void execute() { ... }       // Public method
    protected void validate() { ... }   // Protected method
    void log() { ... }                  // Package-private method
    private void cleanup() { ... }      // Private method
}
```

### Go Exported / Non-Exported

```go
// Visibilité = première lettre (case-sensitive)
type Pipeline struct {
    ID      string        // Exported (public) - Uppercase
    Name    string        // Exported
    version string        // non-exported (private) - lowercase
    input   *ModuleConfig // non-exported
}

func (p *Pipeline) Execute() error { ... }  // Exported method
func (p *Pipeline) validate() error { ... } // non-exported method

// Package-level
var GlobalConfig Config  // Exported variable
var internalState State  // non-exported variable

func NewPipeline() *Pipeline { ... }  // Exported function
func parseConfig() Config { ... }      // non-exported function
```

### Règles Go Visibility

| Identifier | Visibilité | Accessible Depuis |
|------------|-----------|-------------------|
| `Pipeline` | Exported | Tous packages |
| `pipeline` | non-exported | Package actuel seulement |
| `Execute()` | Exported | Tous packages |
| `execute()` | non-exported | Package actuel seulement |

### Package `internal/` Special

**Convention Go**:
- Packages sous `internal/` ne peuvent PAS être importés depuis autre repo
- Plus strict que package-private Java

```
myproject/
├── pkg/connector/      → Importable depuis autre repo (public API)
└── internal/
    ├── config/         → Importable dans myproject seulement
    ├── runtime/        → Importable dans myproject seulement
    └── ...
```

**Équivalent Java**:
- `pkg/` = `public` packages
- `internal/` = `package-private` (mais strictement enforced)

---

## 7. init() & main() — Initialization

### Java Static Initialization

```java
public class Application {
    // Static initializer block (runs once at class load)
    static {
        System.out.println("Static init");
        configureLogging();
    }

    // Instance initializer block
    {
        System.out.println("Instance init");
    }

    public static void main(String[] args) {
        Application app = new Application();
        app.run();
    }
}
```

### Go init() & main()

```go
package main

import "fmt"

// init() runs automatically before main() (package initialization)
// Can have multiple init() functions in same package
func init() {
    fmt.Println("Init 1")
    setupLogging()
}

func init() {
    fmt.Println("Init 2")
    loadConfig()
}

// main() is entry point (only in package main)
func main() {
    fmt.Println("Main")
    run()
}

// Execution order:
// 1. Import all dependencies (their init() functions)
// 2. init() in current package (in order defined)
// 3. main()
```

### Ordre Initialization

**Java**:
1. Static initializers (parent → child)
2. Instance initializers
3. Constructor

**Go**:
1. Package-level variables initialization
2. `init()` functions (ordre: imports → current package)
3. `main()` (si package main)

**Exemple Canectors**:
```go
// internal/logger/logger.go
package logger

import "log/slog"

var logger *slog.Logger  // Package-level variable

func init() {
    // Runs automatically at import
    logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))
}

// cmd/canectors/main.go
package main

import (
    "github.com/canectors/runtime/internal/logger"  // Triggers logger.init()
)

func init() {
    // Custom initialization
}

func main() {
    // Entry point
}
```

---

## 8. Goroutines vs Threads

### Java Threads

```java
// Create thread
Thread thread = new Thread(() -> {
    System.out.println("Running in thread");
});
thread.start();

// ExecutorService (thread pool)
ExecutorService executor = Executors.newFixedThreadPool(10);
executor.submit(() -> {
    fetchData();
});

// CompletableFuture (async)
CompletableFuture.supplyAsync(() -> {
    return fetchData();
}).thenAccept(data -> {
    processData(data);
});

executor.shutdown();
```

### Go Goroutines

```go
// Goroutine (lightweight thread)
go func() {
    fmt.Println("Running in goroutine")
}()

// Channel communication
ch := make(chan string)

go func() {
    result := fetchData()
    ch <- result  // Send to channel
}()

result := <-ch  // Receive from channel
processData(result)

// WaitGroup (wait for multiple goroutines)
var wg sync.WaitGroup

for i := 0; i < 10; i++ {
    wg.Add(1)
    go func(id int) {
        defer wg.Done()
        fetchData(id)
    }(i)
}

wg.Wait()  // Wait all goroutines finish
```

### Différences Clés

| Aspect | Java Threads | Go Goroutines |
|--------|--------------|---------------|
| **Création** | `new Thread()` | `go func()` |
| **Poids** | ~1MB stack | ~2KB stack (growable) |
| **Scheduler** | OS threads (1:1) | Go runtime (M:N) |
| **Cost** | Création coûteuse | Création cheap (millions possible) |
| **Communication** | Shared memory + locks | **Channels** (CSP model) |

### Canectors Usage

```go
// scheduler/scheduler.go
func (s *Scheduler) executePipeline(reg *registeredPipeline) {
    s.wg.Add(1)
    go func() {
        defer s.wg.Done()

        // Execute pipeline in goroutine
        result, err := s.executor.Execute(reg.pipeline)

        // Log result
        logger.LogResult(result, err)
    }()
}

// Graceful shutdown
func (s *Scheduler) Stop(ctx context.Context) error {
    s.cron.Stop()  // Stop scheduler

    // Wait for all goroutines with timeout
    done := make(chan struct{})
    go func() {
        s.wg.Wait()
        close(done)
    }()

    select {
    case <-done:
        return nil
    case <-ctx.Done():
        return ctx.Err()  // Timeout
    }
}
```

---

## 9. Synchronization — sync.Mutex vs synchronized

### Java synchronized

```java
public class Counter {
    private int count = 0;

    // Synchronized method
    public synchronized void increment() {
        count++;
    }

    // Synchronized block
    public void decrement() {
        synchronized(this) {
            count--;
        }
    }

    // Read lock (manual)
    private final ReadWriteLock lock = new ReentrantReadWriteLock();

    public int getCount() {
        lock.readLock().lock();
        try {
            return count;
        } finally {
            lock.readLock().unlock();
        }
    }
}
```

### Go sync.Mutex

```go
type Counter struct {
    mu    sync.Mutex  // Mutex for exclusive access
    count int
}

func (c *Counter) Increment() {
    c.mu.Lock()
    defer c.mu.Unlock()  // Always defer unlock (like finally)
    c.count++
}

func (c *Counter) Decrement() {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.count--
}

// RWMutex (read-write lock)
type SafeMap struct {
    mu   sync.RWMutex
    data map[string]int
}

func (m *SafeMap) Get(key string) int {
    m.mu.RLock()         // Read lock (multiple readers OK)
    defer m.mu.RUnlock()
    return m.data[key]
}

func (m *SafeMap) Set(key string, value int) {
    m.mu.Lock()          // Write lock (exclusive)
    defer m.mu.Unlock()
    m.data[key] = value
}
```

### Différences Clés

| Aspect | Java synchronized | Go sync.Mutex |
|--------|-------------------|---------------|
| **Syntax** | Keyword `synchronized` | Explicit `Lock()`/`Unlock()` |
| **Cleanup** | Automatic unlock | **Manual** (defer recommended) |
| **Reentrant** | ✅ Oui (`synchronized` on same object) | ❌ Non (deadlock si re-lock) |
| **Read-Write** | `ReadWriteLock` (manual) | `sync.RWMutex` |

### Canectors Usage

```go
// scheduler/scheduler.go
type Scheduler struct {
    mu        sync.RWMutex                      // Protects pipelines map
    pipelines map[string]*registeredPipeline
}

func (s *Scheduler) Register(pipeline *Pipeline) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    s.pipelines[pipeline.ID] = &registeredPipeline{
        pipeline: pipeline,
    }
    return nil
}

func (s *Scheduler) IsRunning(pipelineID string) (bool, error) {
    s.mu.RLock()  // Read lock
    defer s.mu.RUnlock()

    reg, exists := s.pipelines[pipelineID]
    if !exists {
        return false, ErrPipelineNotFound
    }
    return reg.running, nil
}

// Per-pipeline mutex (overlap prevention)
type registeredPipeline struct {
    mu      sync.Mutex
    running bool
}

func executePipeline(reg *registeredPipeline) {
    reg.mu.Lock()
    if reg.running {
        reg.mu.Unlock()
        return  // Skip (overlap)
    }
    reg.running = true
    reg.mu.Unlock()

    defer func() {
        reg.mu.Lock()
        reg.running = false
        reg.mu.Unlock()
    }()

    // Execute pipeline
}
```

---

## 10. Collections — map, slice vs Java Collections

### Java Collections

```java
// List (dynamic array)
List<String> list = new ArrayList<>();
list.add("item1");
list.add("item2");
String first = list.get(0);

// Map (hash map)
Map<String, Integer> map = new HashMap<>();
map.put("key1", 100);
Integer value = map.get("key1");
boolean exists = map.containsKey("key1");

// Set
Set<String> set = new HashSet<>();
set.add("item");
boolean contains = set.contains("item");
```

### Go Collections

```go
// Slice (dynamic array)
var slice []string
slice = append(slice, "item1")
slice = append(slice, "item2")
first := slice[0]

// Make with capacity
slice := make([]string, 0, 10)  // len=0, cap=10

// Map (hash map)
m := make(map[string]int)
m["key1"] = 100
value, exists := m["key1"]  // value=100, exists=true
value2, exists2 := m["key2"] // value2=0, exists2=false

// Delete
delete(m, "key1")

// Iteration
for key, value := range m {
    fmt.Println(key, value)
}

// Set (simulate avec map[T]bool)
set := make(map[string]bool)
set["item"] = true
exists := set["item"]
```

### Différences Clés

| Aspect | Java | Go |
|--------|------|-----|
| **Génériques** | `List<T>`, `Map<K,V>` | `[]T`, `map[K]V` (built-in) |
| **Initialisation** | `new ArrayList<>()` | `make([]T, len, cap)` |
| **Append** | `list.add(item)` | `slice = append(slice, item)` |
| **Map access** | `map.get(key)` (null si absent) | `value, ok := m[key]` (zero value si absent) |
| **Null values** | ✅ Possible (`null`) | ❌ Zero value (0, "", false, nil) |
| **Thread safety** | `ConcurrentHashMap`, `Collections.synchronizedList()` | Manual (sync.Mutex) ou `sync.Map` |

### Canectors Usage

```go
// Slices
records := make([]map[string]interface{}, 0, 100)  // Capacity hint
records = append(records, record)

// Maps (config data)
config := map[string]interface{}{
    "endpoint": "https://api.example.com",
    "method":   "POST",
    "headers": map[string]string{
        "X-Custom": "value",
    },
}

// Type assertion
endpoint, ok := config["endpoint"].(string)
if !ok {
    return ErrMissingEndpoint
}

// Iterate
for key, value := range config {
    fmt.Println(key, value)
}
```

---

## 11. JSON Marshaling — struct tags

### Java Jackson

```java
@JsonIgnoreProperties(ignoreUnknown = true)
public class Pipeline {
    @JsonProperty("id")
    private String id;

    @JsonProperty("pipeline_name")
    private String name;

    @JsonIgnore
    private String internalState;

    @JsonFormat(pattern = "yyyy-MM-dd")
    private LocalDate createdAt;
}

// Serialize
ObjectMapper mapper = new ObjectMapper();
String json = mapper.writeValueAsString(pipeline);

// Deserialize
Pipeline pipeline = mapper.readValue(json, Pipeline.class);
```

### Go struct tags

```go
type Pipeline struct {
    ID            string    `json:"id"`
    Name          string    `json:"pipeline_name"`
    InternalState string    `json:"-"`              // Ignored
    CreatedAt     time.Time `json:"created_at,omitempty"`  // Omit if zero value
}

// Serialize
data, err := json.Marshal(pipeline)
// data = []byte(`{"id":"123","pipeline_name":"My Pipeline"}`)

// Deserialize
var pipeline Pipeline
err := json.Unmarshal(data, &pipeline)
```

### Struct Tags Options

| Tag | Description |
|-----|-------------|
| `json:"name"` | Field name in JSON |
| `json:"-"` | Ignore field |
| `json:",omitempty"` | Omit if zero value |
| `json:"name,string"` | Force string encoding |

**Canectors Usage**:
```go
// pkg/connector/types.go
type ExecutionResult struct {
    PipelineID       string           `json:"pipelineId"`
    Status           string           `json:"status"`
    RecordsProcessed int              `json:"recordsProcessed"`
    RecordsFailed    int              `json:"recordsFailed,omitempty"`
    StartedAt        time.Time        `json:"startedAt"`
    CompletedAt      time.Time        `json:"completedAt,omitempty"`
    Error            *ExecutionError  `json:"error,omitempty"`
}
```

---

## Résumé — Mental Model Go pour Java Dev

| Concept | Java | Go | Idiom Go |
|---------|------|-----|----------|
| **Classes** | class avec fields + methods | struct (data) + methods (behavior séparé) | Composition over inheritance |
| **Héritage** | `extends` | Embedding (composition) | Use interfaces for polymorphism |
| **Interfaces** | Explicit `implements` | Implicit satisfaction | Small interfaces (1-3 methods) |
| **Exceptions** | `try/catch/throw` | `return error` | Check every error |
| **Null** | `null` values | Zero values (0, "", false, nil) | No null pointer exceptions |
| **Concurrency** | Threads + locks | Goroutines + channels | "Don't communicate by sharing memory; share memory by communicating" |
| **Generics** | `<T>` | `interface{}` + type assertion (Go 1.18+ generics) | Avoid `interface{}` si possible |
| **Package visibility** | `public`/`private`/`protected` | Uppercase=Exported, lowercase=non-exported | Simple binary |
| **Initialization** | Constructors | `NewT()` functions (convention) | Return pointer for mutable |
| **Collections** | `ArrayList`, `HashMap` | `[]T`, `map[K]V` (built-in) | No generics before 1.18 |
| **Synchronization** | `synchronized` keyword | `sync.Mutex` (explicit) | Use channels where possible |

**Philosophie Go**: **Simplicité, explicitness, composition**

---

**Fichier suivant**: INVARIANTS_AND_ASSUMPTIONS.md
