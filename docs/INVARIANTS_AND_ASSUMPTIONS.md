# INVARIANTS AND ASSUMPTIONS — Canectors Runtime

**Date**: 2026-01-23
**Public cible**: Ingénieur Java Senior (10+ ans) apprenant Go
**Niveau**: Invariants système, hypothèses implicites, règles non violables

---

## 1. Invariants Architecturaux

### Invariant 1: Pipeline Configuration Immutabilité

**Règle**: Une fois `Pipeline` struct créée via `config.ConvertToPipeline()`, **elle ne doit JAMAIS être mutée**.

**Raison**:
- Configuration = contrat fixe entre user et runtime
- Mutation → comportement imprévisible, race conditions
- Pas de locking nécessaire si immutable

**Code Critique**:
```go
// ✅ CORRECT: Create new Pipeline, don't mutate existing
pipeline, err := config.ConvertToPipeline(data)
executor.Execute(pipeline)  // Read-only

// ❌ WRONG: Mutate pipeline après création
pipeline.Name = "Modified"  // ⚠️ Ne JAMAIS faire
pipeline.Filters = append(pipeline.Filters, newFilter)  // ⚠️ Ne JAMAIS faire
```

**Conséquence violation**:
- Race conditions si pipeline partagée entre goroutines
- Comportement inattendu lors exécutions multiples
- Debugging difficile (state muté)

**Équivalent Java**: `@Immutable` class ou tous fields `final`

---

### Invariant 2: Module Independence

**Règle**: Modules **ne doivent PAS se connaître** entre eux. Communication uniquement via `Executor`.

**Architecture**:
```
Input Module
    ↓
    | (via Executor)
    ↓
Filter Modules (séquentiels)
    ↓
    | (via Executor)
    ↓
Output Module
```

**Code Interdit**:
```go
// ❌ WRONG: Module dépend directement d'autre module
type MappingFilter struct {
    outputModule *HTTPRequestModule  // ⚠️ Couplage direct
}

// ✅ CORRECT: Module isolé, Executor orchestre
type MappingFilter struct {
    mappings []FieldMapping
    onError  string
}
// Executor possède tous modules et orchestre
```

**Raison**:
- **Testabilité**: Modules testables isolément
- **Réutilisabilité**: Modules composables différemment
- **Dependency Inversion**: Depend on interfaces, not concrete types

**Conséquence violation**:
- Tight coupling → changements cassent multiple modules
- Testing difficile (mocking complexe)
- Impossibilité réutiliser module dans autre contexte

---

### Invariant 3: Error Classification Exhaustive

**Règle**: Toute erreur **doit être classifiée** dans une des catégories:
- `network`
- `authentication`
- `validation`
- `rate_limit`
- `server`
- `not_found`
- `unknown` (fallback)

**Code Obligatoire**:
```go
// ✅ CORRECT: Classify before retry logic
err := module.Fetch(ctx)
if err != nil {
    classified := errhandling.ClassifyError(err)
    if classified.Retryable {
        // Retry with backoff
    } else {
        // Fail immediately
    }
}

// ❌ WRONG: Retry sans classification
err := module.Fetch(ctx)
if err != nil {
    // Retry aveugle → peut retry auth error indéfiniment
    retry(func() { module.Fetch(ctx) })
}
```

**Raison**:
- Retry logic dépend classification
- Auth errors → pas retry (credentials invalides)
- Network errors → retry avec backoff
- Server errors (5xx) → retry
- Client errors (4xx) → fail (sauf 429 rate limit)

**Conséquence violation**:
- Retry loop infini sur auth errors
- Gaspillage ressources (retry sur erreurs non-retryables)
- Logs pollués (tentatives inutiles)
- User experience dégradée (attente inutile)

---

### Invariant 4: Context Propagation Obligatoire

**Règle**: Toutes opérations **asynchrones/IO-bound** doivent accepter `context.Context` comme **premier paramètre**.

**Signature Mandatory**:
```go
// ✅ CORRECT
func Fetch(ctx context.Context) ([]Record, error)
func Process(ctx context.Context, records []Record) ([]Record, error)
func Send(ctx context.Context, records []Record) (int, error)

// ❌ WRONG
func Fetch() ([]Record, error)  // Pas de context → pas de cancellation
```

**Raison**:
- **Cancellation**: User peut annuler opération (Ctrl+C)
- **Timeout**: Éviter hang indéfini
- **Tracing**: Propagation request ID, trace spans
- **Resource cleanup**: Connections fermées proprement

**Conséquence violation**:
- Opérations non-cancellables (hang indéfini)
- HTTP requests sans timeout (leak connections)
- Graceful shutdown impossible
- Memory leaks (goroutines zombies)

**Code Pattern**:
```go
// Executor pass context to all modules
func (e *Executor) ExecuteWithContext(ctx context.Context, pipeline *Pipeline) (*ExecutionResult, error) {
    records, err := e.inputModule.Fetch(ctx)  // Propagate
    // ...
    for _, filter := range e.filterModules {
        records, err = filter.Process(ctx, records)  // Propagate
    }
    // ...
    sent, err := e.outputModule.Send(ctx, records)  // Propagate
}

// HTTP request with context
req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
resp, err := client.Do(req)  // Cancelled if ctx cancelled
```

---

### Invariant 5: Sequential Module Execution

**Règle**: Modules exécutés **séquentiellement** (pas en parallèle). Ordre garanti:
1. Input.Fetch()
2. Filters[0].Process()
3. Filters[1].Process()
4. ...
5. Output.Send()

**Code Critical**:
```go
// ✅ CORRECT: Sequential execution
records, err := inputModule.Fetch(ctx)
for _, filter := range filterModules {
    records, err = filter.Process(ctx, records)  // Sequential loop
}
sent, err := outputModule.Send(ctx, records)

// ❌ WRONG: Parallel execution (undefined order)
var wg sync.WaitGroup
for _, filter := range filterModules {
    wg.Add(1)
    go func(f filter.Module) {
        defer wg.Done()
        f.Process(ctx, records)  // ⚠️ Race condition!
    }(filter)
}
wg.Wait()
```

**Raison**:
- **Order preservation**: Filters dépendent order (mapping puis condition)
- **Data dependency**: Filter N peut dépendre output Filter N-1
- **Simplicity**: Pas de synchronization complexity

**Conséquence violation**:
- Race conditions sur records slice
- Order non-garanti (condition avant mapping → bug)
- Unpredictable results

---

## 2. Hypothèses Implicites du Code

### Hypothèse 1: Records = `map[string]interface{}`

**Hypothèse**: Tous records représentés comme `map[string]interface{}` (schemaless).

**Code Assumption**:
```go
type Record = map[string]interface{}

// Assume keys existent ou manquent gracefully
value, exists := record["field_name"]
if !exists {
    // Handle missing field selon onMissing strategy
}

// Assume values peuvent être n'importe quel type
switch v := value.(type) {
case string:
    // Handle string
case float64:  // JSON numbers = float64
    // Handle number
case map[string]interface{}:
    // Handle nested object
default:
    // Handle unknown
}
```

**Implications**:
- **Pas de compile-time type safety** (trade-off flexibilité vs safety)
- **Type assertions** obligatoires partout
- **Nil/missing fields** doivent être handled
- **Nested access** nécessite chaining with nil checks

**Alternative rejetée**: Struct typée par domaine (rigide, nécessite recompilation)

---

### Hypothèse 2: Configuration Valide à Runtime

**Hypothèse**: Si config passe validation schema, **elle est correcte**.

**Code Assumption**:
```go
// Après ValidateConfig(), assume fields existent
config, _ := cfg.Config["endpoint"].(string)  // Assume existe (validated)
method, _ := cfg.Config["method"].(string)    // Assume existe (validated)

// ⚠️ Si validation manquée, panic ou behavior undefined
```

**Dépendance Critique**:
- Schema JSON **doit être exhaustif**
- Schema validation **doit être appelée** avant ConvertToPipeline()
- Tout required field **doit être dans schema**

**Conséquence violation**:
- Runtime panics (nil pointer dereference)
- Silent failures (empty strings, zero values)
- Unexpected behavior

**Mitigation**:
```go
// Defensive programming pour fields critiques
endpoint, ok := cfg.Config["endpoint"].(string)
if !ok || endpoint == "" {
    return fmt.Errorf("missing required field 'endpoint'")
}
```

---

### Hypothèse 3: Single Pipeline Instance per Execution

**Hypothèse**: Chaque exécution pipeline = **nouvelle instance Executor** avec modules frais.

**Code Pattern**:
```go
// cmd/canectors/main.go - runPipelineOnce()
func runPipelineOnce(pipeline *Pipeline) error {
    // Create fresh modules for THIS execution
    input := factory.CreateInputModule(pipeline.Input)
    filters, _ := factory.CreateFilterModules(pipeline.Filters)
    output, _ := factory.CreateOutputModule(pipeline.Output)

    // Create fresh executor
    executor := runtime.NewExecutorWithModules(input, filters, output, dryRun)

    // Execute ONCE
    result, err := executor.Execute(pipeline)

    // Cleanup
    input.Close()
    output.Close()

    return err
}
```

**Implication**:
- **Stateless modules**: Pas de state conservé entre exécutions
- **No connection pooling** (chaque exec = new HTTP client)
- **No cursor persistence** (HTTP polling restart from beginning)

**Conséquence**:
- Overhead création modules (acceptable pour one-shot)
- Pas de resume après crash (pas de checkpointing)
- Scheduler mode: multiple exécutions = multiple instances

---

### Hypothèse 4: HTTP Polling Stateless

**Hypothèse**: HTTP polling **ne conserve PAS** de state entre fetches (pas de last-fetch timestamp, pas de cursor persistence).

**Code Reality**:
```go
// input/http_polling.go
func (h *HTTPPolling) Fetch(ctx context.Context) ([]Record, error) {
    // Fetch depuis endpoint (toujours même URL)
    resp, err := h.client.Get(h.endpoint)

    // Parse response
    records := parseRecords(resp.Body)

    // ⚠️ Pas de persistence de "last fetch timestamp"
    // ⚠️ Pas de "only new records since last fetch"
    return records, nil
}
```

**Implication**:
- **Duplicate data** si API retourne toujours mêmes records
- **No incremental sync** (full fetch à chaque fois)
- **API must support filtering** (query params avec timestamp) pour éviter duplicates

**Workaround User-Side**:
- Use query params: `endpoint?since={{lastFetchTimestamp}}`
- Use external state management (database de last sync)

**Alternative rejetée**: State persistence (nécessite storage, complexité)

---

### Hypothèse 5: Scheduler Single-Node

**Hypothèse**: Scheduler **local** (1 instance runtime = 1 scheduler). **Pas de coordination distribuée**.

**Code Reality**:
```go
// scheduler/scheduler.go
type Scheduler struct {
    cron      *cron.Cron              // Local CRON
    pipelines map[string]*registered  // Local map (in-memory)
    // ⚠️ Pas de distributed lock
    // ⚠️ Pas de leader election
}
```

**Implication**:
- **Pas de HA**: Si instance crash, scheduler stop
- **Pas de load balancing**: 1 pipeline = 1 node
- **No horizontal scaling**: Multiple instances = duplicate executions
- **Overlap prevention** local seulement (pas cross-node)

**Conséquence**:
- Production deployment: 1 instance max par pipeline
- Si besoin HA: external scheduler (Kubernetes CronJob, Airflow, etc.)

---

## 3. Ordres d'Appel Obligatoires

### Ordre 1: Parse → Validate → Convert

**Séquence Mandatory**:
```go
// 1. Parse fichier
result := config.ParseConfig(filepath)

// 2. Valider schema
if !result.IsValid() {
    // STOP - ne PAS continuer
    return result.ValidationErrors
}

// 3. Convertir en Pipeline struct
pipeline, err := config.ConvertToPipeline(result.Data)

// ❌ WRONG: Convert sans validate
result := config.ParseConfig(filepath)
pipeline, _ := config.ConvertToPipeline(result.Data)  // ⚠️ Peut crasher
```

**Raison**:
- ConvertToPipeline **assume** data validée
- Validation manquée → panic ou silent errors

---

### Ordre 2: Create Modules → Create Executor → Execute

**Séquence Mandatory**:
```go
// 1. Create modules
input := factory.CreateInputModule(pipeline.Input)
filters, _ := factory.CreateFilterModules(pipeline.Filters)
output, _ := factory.CreateOutputModule(pipeline.Output)

// 2. Create executor avec modules
executor := runtime.NewExecutorWithModules(input, filters, output, dryRun)

// 3. Execute pipeline
result, err := executor.Execute(pipeline)

// 4. Cleanup
defer input.Close()
defer output.Close()

// ❌ WRONG: Execute sans modules
executor := runtime.NewExecutor(dryRun)  // No modules
executor.Execute(pipeline)  // ⚠️ Panic (nil pointer)
```

**Raison**:
- Executor **assume** modules déjà créés et injectés
- Modules nil → panic lors Fetch/Process/Send

---

### Ordre 3: Lock → Check → Modify → Unlock

**Pattern Concurrent Accès**:
```go
// ✅ CORRECT: Lock-Check-Modify-Unlock
func (s *Scheduler) executePipeline(reg *registeredPipeline) {
    reg.mu.Lock()
    if reg.running {
        reg.mu.Unlock()
        return  // Skip
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

// ❌ WRONG: Check sans lock (race condition)
if reg.running {  // ⚠️ Race condition
    return
}
reg.mu.Lock()
reg.running = true
reg.mu.Unlock()
```

**Raison**:
- Check + Modify doivent être **atomiques**
- Sans lock → race condition (two goroutines pass check simultaneously)

---

### Ordre 4: WaitGroup Add → Launch Goroutine → Done

**Pattern Goroutine Synchronization**:
```go
// ✅ CORRECT: Add AVANT goroutine
wg.Add(1)
go func() {
    defer wg.Done()
    // Work
}()

// ❌ WRONG: Add APRÈS goroutine launch
go func() {
    wg.Add(1)  // ⚠️ Race: wg.Wait() peut return avant Add()
    defer wg.Done()
}()

wg.Wait()  // Peut return prematurely
```

**Raison**:
- `wg.Add(1)` **doit précéder** `go func()`
- Sinon race: `wg.Wait()` peut return avant goroutine lancée

---

## 4. Ce Qui Provoquerait Bugs Subtils

### Bug 1: Mutate Records Slice During Iteration

**Code Dangereux**:
```go
// ❌ WRONG: Mutate slice pendant iteration
for i, record := range records {
    if shouldSkip(record) {
        records = append(records[:i], records[i+1:]...)  // ⚠️ Modifie slice
    }
}
```

**Problème**:
- Index décalage après delete
- Iteration skip elements
- Panic si out of bounds

**Solution**:
```go
// ✅ CORRECT: Build new slice
filtered := make([]Record, 0, len(records))
for _, record := range records {
    if !shouldSkip(record) {
        filtered = append(filtered, record)
    }
}
records = filtered
```

---

### Bug 2: Forget defer cancel() After context.WithTimeout()

**Code Dangereux**:
```go
// ❌ WRONG: Pas de defer cancel
ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
result, err := module.Fetch(ctx)
// ⚠️ Cancel jamais appelé → goroutine leak

// ✅ CORRECT: Always defer cancel
ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
defer cancel()  // MANDATORY
result, err := module.Fetch(ctx)
```

**Problème**:
- Cancel pas appelé → timer goroutine leak
- Memory leak si repeated calls
- Performance degradation

---

### Bug 3: Close HTTP Response Body Sans defer

**Code Dangereux**:
```go
// ❌ WRONG: Close conditionnel
resp, err := client.Do(req)
if err != nil {
    return err  // ⚠️ Pas de Close si error early
}
body, _ := io.ReadAll(resp.Body)
resp.Body.Close()

// ✅ CORRECT: defer Close immédiatement
resp, err := client.Do(req)
if err != nil {
    return err
}
defer resp.Body.Close()  // MANDATORY
body, _ := io.ReadAll(resp.Body)
```

**Problème**:
- Early return → body pas closed → connection leak
- Connection pool exhaustion
- HTTP client hang (max connections reached)

---

### Bug 4: Type Assertion Sans Check

**Code Dangereux**:
```go
// ❌ WRONG: Type assertion sans check
endpoint := config["endpoint"].(string)  // ⚠️ Panic si pas string

// ✅ CORRECT: Check avant assertion
endpoint, ok := config["endpoint"].(string)
if !ok || endpoint == "" {
    return fmt.Errorf("invalid endpoint config")
}
```

**Problème**:
- Panic runtime si type incorrect
- Crash application
- Difficult debugging (stack trace pas clair)

---

### Bug 5: Goroutine Closure Capture Loop Variable

**Code Dangereux**:
```go
// ❌ WRONG: Closure capture loop variable
for _, filter := range filters {
    go func() {
        filter.Process(records)  // ⚠️ Capture variable 'filter' (last value)
    }()
}

// ✅ CORRECT: Pass variable as parameter
for _, filter := range filters {
    go func(f filter.Module) {
        f.Process(records)
    }(filter)  // Pass explicit parameter
}
```

**Problème**:
- Toutes goroutines voient **dernière valeur** de `filter`
- Race condition
- Unpredictable behavior

---

### Bug 6: Map Concurrent Access Sans Lock

**Code Dangereux**:
```go
// ❌ WRONG: Concurrent read/write sans lock
type Cache struct {
    data map[string]int
}

func (c *Cache) Get(key string) int {
    return c.data[key]  // ⚠️ Race si concurrent write
}

func (c *Cache) Set(key string, val int) {
    c.data[key] = val  // ⚠️ Race si concurrent read/write
}

// ✅ CORRECT: Protect avec mutex
type Cache struct {
    mu   sync.RWMutex
    data map[string]int
}

func (c *Cache) Get(key string) int {
    c.mu.RLock()
    defer c.mu.RUnlock()
    return c.data[key]
}

func (c *Cache) Set(key string, val int) {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.data[key] = val
}
```

**Problème**:
- Fatal error: "concurrent map read and map write"
- Immediate crash
- Non-recoverable

---

## 5. Assumptions Runtime à Ne Jamais Casser

### Assumption 1: Zero Values Are Safe

**Assumption**: Zero values Go doivent toujours être **safe à utiliser**.

**Exemples**:
```go
var s string       // "" (empty string, safe)
var i int          // 0 (safe)
var b bool         // false (safe)
var slice []int    // nil slice (safe for len/append, pas for indexing)
var m map[K]V      // nil map (safe for read, pas for write)
var ptr *T         // nil pointer (⚠️ NOT safe for dereference)
```

**Code Pattern**:
```go
// ✅ CORRECT: Check nil avant dereference
if pipeline != nil {
    executor.Execute(pipeline)
}

// ✅ CORRECT: nil slice safe pour append
var records []Record  // nil
records = append(records, newRecord)  // Works (allocate backing array)

// ❌ WRONG: nil map write panic
var m map[string]int  // nil
m["key"] = 123  // ⚠️ PANIC: assignment to entry in nil map

// ✅ CORRECT: Initialize map avant write
m := make(map[string]int)
m["key"] = 123
```

---

### Assumption 2: Exported Fields Are Public Contract

**Assumption**: Si field est **Exported** (uppercase), c'est **public API** → **ne PAS changer**.

**Code Contract**:
```go
// pkg/connector/types.go
type Pipeline struct {
    ID      string  // ✅ Public field (exported)
    Name    string  // ✅ Public field
    version string  // ❌ Private field (non-exported)
}

// External code depends on exported fields
pipeline.ID = "123"    // ✅ OK (exported)
pipeline.Name = "Test" // ✅ OK (exported)
pipeline.version = "1.0.0"  // ❌ Compile error (non-exported)
```

**Breaking Change**:
```go
// ⚠️ BREAKING CHANGE: Rename exported field
type Pipeline struct {
    PipelineID string  // Was "ID" → breaks external code
}
```

**Conséquence**:
- External code compile errors
- Breaking change pour consumers

**Mitigation**:
- Add new field, deprecate old
- Version API (v2 package)

---

### Assumption 3: Logger Thread-Safe

**Assumption**: `internal/logger` **thread-safe** → peut être appelé depuis n'importe quel goroutine sans synchronization.

**Code Reality**:
```go
// logger/logger.go
var logger *slog.Logger  // Package-level variable

func init() {
    logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))
    // slog.Logger is thread-safe
}

// Multiple goroutines peuvent log simultaneously
go func() {
    logger.Info("goroutine 1")
}()

go func() {
    logger.Info("goroutine 2")
}()
```

**Implication**:
- Pas besoin mutex pour logger calls
- Pas de race conditions

**⚠️ Si broken**:
- Logs interleaved (corrupted)
- Race conditions
- Panic

---

### Assumption 4: HTTP Client Reusable

**Assumption**: `http.Client` **réutilisable** et **thread-safe** pour multiple requests.

**Code Pattern**:
```go
// ✅ CORRECT: Reuse client
type HTTPPolling struct {
    client *http.Client  // Created once, reused
}

func NewHTTPPolling() *HTTPPolling {
    return &HTTPPolling{
        client: &http.Client{Timeout: 30 * time.Second},
    }
}

func (h *HTTPPolling) Fetch(ctx context.Context) ([]Record, error) {
    // Reuse client (thread-safe)
    req, _ := http.NewRequestWithContext(ctx, "GET", h.endpoint, nil)
    resp, err := h.client.Do(req)
    // ...
}

// ❌ WRONG: Create new client every request
func (h *HTTPPolling) Fetch(ctx context.Context) ([]Record, error) {
    client := &http.Client{}  // ⚠️ Overhead, no connection pooling
    resp, err := client.Do(req)
}
```

**Raison**:
- HTTP client **pools connections** (performance)
- Thread-safe (Go stdlib guarantee)
- Creation coûteux (transport setup)

---

### Assumption 5: Context Cancel = Immediate Stop

**Assumption**: Si `ctx.Done()` closed, opération **doit arrêter immédiatement**.

**Code Pattern**:
```go
// ✅ CORRECT: Check context régulièrement
for _, record := range records {
    select {
    case <-ctx.Done():
        return ctx.Err()  // Stop immediately
    default:
        // Continue processing
    }
    processRecord(record)
}

// ❌ WRONG: Pas de context check (ignore cancellation)
for _, record := range records {
    processRecord(record)  // ⚠️ Continue même si ctx cancelled
}
```

**Conséquence**:
- User presse Ctrl+C → application continue processing
- Graceful shutdown impossible
- Resource leaks

---

## 6. Résumé — Checklist Critique

### ✅ Avant Lancer Prod

- [ ] Config **validée** via schema avant ConvertToPipeline
- [ ] Modules **isolés** (pas de cross-references)
- [ ] Toutes erreurs **classifiées** avant retry
- [ ] Context **propagé** à toutes opérations async/IO
- [ ] Pipeline **immutable** après création
- [ ] Modules **stateless** (pas de state entre exécutions)
- [ ] HTTP response body **defer closed**
- [ ] Context cancel **defer appelé** après WithTimeout/WithCancel
- [ ] Maps **protégées** par mutex si concurrent access
- [ ] Type assertions **checked** (`value, ok := ...`)
- [ ] Goroutines **synchronisées** via WaitGroup si needed
- [ ] Loop variables **passées** en paramètre si closure goroutine
- [ ] Zero values **safe** à utiliser
- [ ] Logger **thread-safe** (slog.Logger OK)
- [ ] HTTP client **réutilisé** (pas créé per-request)

---

**Fichier suivant**: TYPE_AND_METHOD_MAP.md (CŒUR DU TRAVAIL)
