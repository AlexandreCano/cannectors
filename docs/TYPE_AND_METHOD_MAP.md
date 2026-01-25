# TYPE AND METHOD MAP — Canectors Runtime

**Date**: 2026-01-23
**Public cible**: Ingénieur Java Senior (10+ ans) apprenant Go
**Niveau**: Détail exhaustif types, méthodes, call graphs

---

## Navigation Rapide

1. [Package cmd/canectors](#1-package-cmdcanectors)
2. [Package pkg/connector](#2-package-pkgconnector)
3. [Package internal/config](#3-package-internalconfig)
4. [Package internal/runtime](#4-package-internalruntime)
5. [Package internal/factory](#5-package-internalfactory)
6. [Package internal/modules/input](#6-package-internalmodulesinput)
7. [Package internal/modules/filter](#7-package-internalmodulesfilter)
8. [Package internal/modules/output](#8-package-internalmodulesoutput)
9. [Package internal/auth](#9-package-internalauth)
10. [Package internal/errhandling](#10-package-internalerrhandling)
11. [Package internal/scheduler](#11-package-internalscheduler)
12. [Package internal/logger](#12-package-internallogger)

---

## 1. Package `cmd/canectors`

### Type: `PipelineExecutorAdapter`

**Rôle fonctionnel**: Adapte `runtime.Executor` pour interface `scheduler.Executor`

**Cycle de vie**: Créé lors setup scheduler, détruit après scheduler.Stop()

**Champs**:
```go
type PipelineExecutorAdapter struct {
    inputModule   input.Module
    filterModules []filter.Module
    outputModule  output.Module
    dryRun        bool
}
```

**Mutabilité**: Immutable après création

---

#### Méthode: `Execute(pipeline *connector.Pipeline)`

**Signature**:
```go
func (a *PipelineExecutorAdapter) Execute(pipeline *connector.Pipeline) (*connector.ExecutionResult, error)
```

**Responsabilité**: Exécute pipeline via runtime.Executor

**Appelle**:
- `runtime.NewExecutorWithModules(input, filters, output, dryRun)`
- `executor.Execute(pipeline)`

**Appelée par**:
- `scheduler.executePipeline()` (à chaque tick CRON)

**Effets de bord**: Aucun (stateless, crée executor frais)

**Erreurs possibles**:
- Erreurs propagées depuis `executor.Execute()`

---

### Function: `main()`

**Signature**:
```go
func main()
```

**Responsabilité**: Entry point CLI, setup Cobra commands

**Appelle**:
- `rootCmd.Execute()`

**Call chain**:
```
main()
  → rootCmd.Execute()
    → validateCmd.Run() OU runCmd.Run() OU versionCmd.Run()
```

**Effets de bord**:
- Parse CLI args
- Exit process avec code approprié

---

### Function: `validateCmd.Run`

**Responsabilité**: Validate config command handler

**Call chain**:
```
validateCmd.Run(cmd, args)
  → config.ParseConfig(filepath)
    → config.ValidateConfig(data)
  → cli.PrintValidationErrors(result) OU cli.PrintSuccess()
  → os.Exit(exitCode)
```

**Effets de bord**:
- Print à stdout/stderr
- Exit process

---

### Function: `runCmd.Run`

**Responsabilité**: Run pipeline command handler

**Call chain détaillé**:
```
runCmd.Run(cmd, args)
  ├─ config.ParseConfig(filepath)
  │   ├─ config.DetectFormat(filepath)
  │   ├─ config.ParseJSONFile() OU config.ParseYAMLFile()
  │   └─ config.ValidateConfig(data)
  │
  ├─ config.ConvertToPipeline(result.Data)
  │   ├─ extractPipelineMetadata()
  │   ├─ extractModules()
  │   └─ applyErrorHandling()
  │
  ├─ Si pipeline.Schedule != "" (scheduled mode):
  │   ├─ scheduler.ValidateCronExpression(schedule)
  │   ├─ factory.CreateInputModule(pipeline.Input)
  │   ├─ factory.CreateFilterModules(pipeline.Filters)
  │   ├─ factory.CreateOutputModule(pipeline.Output)
  │   ├─ NewPipelineExecutorAdapter(input, filters, output, dryRun)
  │   ├─ scheduler.NewWithExecutor(adapter)
  │   ├─ scheduler.Register(pipeline)
  │   ├─ scheduler.Start(ctx)
  │   ├─ [Wait for signals]
  │   └─ scheduler.Stop(ctx)
  │
  └─ Sinon (one-shot mode):
      ├─ factory.CreateInputModule(pipeline.Input)
      ├─ factory.CreateFilterModules(pipeline.Filters)
      ├─ factory.CreateOutputModule(pipeline.Output)
      ├─ runtime.NewExecutorWithModules(input, filters, output, dryRun)
      ├─ executor.Execute(pipeline)
      ├─ cli.PrintExecutionResult(result)
      ├─ input.Close()
      ├─ output.Close()
      └─ os.Exit(exitCode)
```

**Effets de bord**:
- Crée modules
- Exécute pipeline
- Print résultats
- Exit process

---

## 2. Package `pkg/connector`

### Type: `Pipeline`

**Rôle fonctionnel**: Configuration pipeline complète (DTO)

**Cycle de vie**: Créé via `config.ConvertToPipeline()`, immutable après

**Champs principaux**:
```go
type Pipeline struct {
    ID              string
    Name            string
    Description     string
    Version         string
    Enabled         bool
    Schedule        string           // CRON expression (optionnel)
    Input           *ModuleConfig
    Filters         []ModuleConfig
    Output          *ModuleConfig
    Defaults        *ModuleDefaults
    DryRunOptions   *DryRunOptions
    ErrorHandling   *ErrorHandling   // Legacy
}
```

**Mutabilité**: **Immutable** (ne jamais modifier après création)

**Pas de méthodes** (struct pure data)

---

### Type: `ModuleConfig`

**Rôle fonctionnel**: Configuration module générique

**Champs**:
```go
type ModuleConfig struct {
    Type           string
    Config         map[string]interface{}  // Configuration flexible
    Authentication *AuthConfig
}
```

**Usage**: Passé à factory pour créer module concret

---

### Type: `ExecutionResult`

**Rôle fonctionnel**: Résultat exécution pipeline

**Champs**:
```go
type ExecutionResult struct {
    PipelineID       string
    Status           string  // "success" ou "error"
    RecordsProcessed int
    RecordsFailed    int
    StartedAt        time.Time
    CompletedAt      time.Time
    Error            *ExecutionError
    RetryInfo        *RetryInfo
    DryRunPreview    []RequestPreview
}
```

**Créé par**: `runtime.Executor.Execute()`

**Consommé par**: `cmd/canectors` pour print résultats

---

### Interface: `RetryInfoProvider`

**Rôle fonctionnel**: Modules exposant info retry

**Méthodes**:
```go
type RetryInfoProvider interface {
    GetRetryInfo() *RetryInfo
}
```

**Implémentations**:
- `input.HTTPPolling`
- `output.HTTPRequestModule`

**Usage pattern**:
```go
// Type assertion pour récupérer retry info
if provider, ok := module.(connector.RetryInfoProvider); ok {
    retryInfo := provider.GetRetryInfo()
    result.RetryInfo = retryInfo
}
```

---

## 3. Package `internal/config`

### Type: `Result`

**Rôle fonctionnel**: Résultat parsing + validation combinés

**Champs**:
```go
type Result struct {
    Data             map[string]interface{}
    Format           string  // "json" ou "yaml"
    ParseErrors      []ParseError
    ValidationErrors []ValidationError
}
```

**Méthodes**:

#### `IsValid() bool`

**Responsabilité**: Vérifie si config valide (pas d'erreurs)

**Implémentation**:
```go
func (r *Result) IsValid() bool {
    return len(r.ParseErrors) == 0 && len(r.ValidationErrors) == 0
}
```

**Appelée par**: `cmd/canectors` pour déterminer exit code

---

### Function: `ParseConfig(filepath string)`

**Signature**:
```go
func ParseConfig(filepath string) Result
```

**Responsabilité**: Parse + valide fichier config (auto-détecte format)

**Call chain**:
```
ParseConfig(filepath)
  ├─ DetectFormat(filepath)
  │   ├─ IsJSON(filepath) → bool
  │   └─ IsYAML(filepath) → bool
  │
  ├─ Si JSON:
  │   └─ ParseJSONFile(filepath)
  │       ├─ os.ReadFile(filepath)
  │       ├─ json.Unmarshal(data, &result)
  │       └─ return map[string]interface{}
  │
  ├─ Si YAML:
  │   └─ ParseYAMLFile(filepath)
  │       ├─ os.ReadFile(filepath)
  │       ├─ yaml.Unmarshal(data, &result)
  │       └─ return map[string]interface{}
  │
  └─ ValidateConfig(data)
      ├─ Lazy load schema (sync.Once)
      ├─ schema.Validate(data)
      └─ Convert errors → ValidationError[]
```

**Erreurs possibles**:
- File not found → ParseError
- Invalid JSON/YAML syntax → ParseError
- Schema validation failed → ValidationError[]

**Effets de bord**: Lecture fichier

---

### Function: `ConvertToPipeline(data map[string]interface{})`

**Signature**:
```go
func ConvertToPipeline(data map[string]interface{}) (*connector.Pipeline, error)
```

**Responsabilité**: Convertit map validée → struct Pipeline

**Call chain**:
```
ConvertToPipeline(data)
  ├─ extractPipelineMetadata(pipeline, connectorData)
  │   ├─ Extract: ID, Name, Version, Enabled, Schedule
  │   └─ Set defaults (Enabled=true si absent)
  │
  ├─ extractModules(pipeline, connectorData)
  │   ├─ Extract Input config → ModuleConfig
  │   ├─ Extract Filters configs → []ModuleConfig
  │   └─ Extract Output config → ModuleConfig
  │
  └─ applyErrorHandling(pipeline, connectorData)
      ├─ Extract defaults → ModuleDefaults
      ├─ Extract legacy errorHandling → ErrorHandling
      ├─ For each module:
      │   ├─ Merge: module.OnError || defaults.OnError || "fail"
      │   ├─ Merge: module.TimeoutMs || defaults.TimeoutMs || 30000
      │   └─ Merge: module.Retry || defaults.Retry || DefaultRetryConfig
      └─ Return Pipeline
```

**Assume**: Data déjà validée via schema

**Erreurs possibles**:
- Required fields manquants (shouldn't happen si validated)
- Type assertions échouent

---

## 4. Package `internal/runtime`

### Type: `Executor`

**Rôle fonctionnel**: Orchestrateur pipeline (Input → Filters → Output)

**Cycle de vie**:
1. Créé via `NewExecutorWithModules()`
2. Appelé `Execute()` ou `ExecuteWithContext()`
3. Détruit après exécution

**Champs**:
```go
type Executor struct {
    inputModule   input.Module
    filterModules []filter.Module
    outputModule  output.Module
    dryRun        bool
}
```

**Mutabilité**: Immutable après création

---

#### Méthode: `Execute(pipeline *connector.Pipeline)`

**Signature**:
```go
func (e *Executor) Execute(pipeline *connector.Pipeline) (*connector.ExecutionResult, error)
```

**Responsabilité**: Exécute pipeline avec context.Background()

**Appelle**:
```go
return e.ExecuteWithContext(context.Background(), pipeline)
```

**Appelée par**:
- `cmd/canectors` (one-shot mode)
- `PipelineExecutorAdapter` (scheduled mode)

---

#### Méthode: `ExecuteWithContext(ctx context.Context, pipeline *connector.Pipeline)`

**Signature**:
```go
func (e *Executor) ExecuteWithContext(ctx context.Context, pipeline *connector.Pipeline) (*connector.ExecutionResult, error)
```

**Responsabilité**: Orchestre exécution complète pipeline

**Call chain détaillé**:
```
ExecuteWithContext(ctx, pipeline)
  ├─ validateExecution(pipeline, result)
  │   ├─ Check pipeline != nil
  │   ├─ Check inputModule != nil
  │   └─ Check outputModule != nil (sauf si dryRun)
  │
  ├─ defer inputModule.Close()
  ├─ defer outputModule.Close()
  │
  ├─ logger.LogExecutionStart(execCtx)
  │
  ├─ executeInput(ctx, pipeline, result)
  │   ├─ logger.LogStageStart(execCtx)
  │   ├─ inputModule.Fetch(ctx)
  │   │   └─ [Dépend impl: HTTP GET, webhook, etc.]
  │   ├─ Si RetryInfoProvider:
  │   │   └─ result.RetryInfo = module.GetRetryInfo()
  │   └─ logger.LogStageEnd(execCtx, len(records), duration, err)
  │
  ├─ executeFiltersWithResult(ctx, pipeline, records, result)
  │   ├─ logger.LogStageStart(execCtx)
  │   ├─ executeFilters(ctx, pipelineID, records)
  │   │   └─ For each filter (sequential):
  │   │       ├─ logger.LogStageStart(filterCtx)
  │   │       ├─ filterModule.Process(ctx, currentRecords)
  │   │       │   └─ [Dépend impl: mapping, condition, etc.]
  │   │       └─ logger.LogStageEnd(filterCtx, len(records), duration, err)
  │   └─ logger.LogStageEnd(execCtx, len(records), duration, err)
  │
  ├─ Si dryRun:
  │   └─ executeDryRunPreview(pipelineID, records, opts)
  │       └─ Si PreviewableModule:
  │           └─ outputModule.PreviewRequest(records, opts)
  │
  ├─ executeOutputWithResult(ctx, pipeline, records, result)
  │   ├─ logger.LogStageStart(execCtx)
  │   ├─ executeOutput(ctx, pipelineID, records)
  │   │   ├─ Si dryRun:
  │   │   │   └─ return len(records) (skip send)
  │   │   └─ Sinon:
  │   │       └─ outputModule.Send(ctx, records)
  │   │           └─ [Dépend impl: HTTP POST, etc.]
  │   ├─ Si RetryInfoProvider:
  │   │   └─ result.RetryInfo = module.GetRetryInfo()
  │   └─ logger.LogStageEnd(execCtx, sent, duration, err)
  │
  ├─ finalizeSuccessWithMetrics(result, startedAt, pipeline, timings)
  │   ├─ Calculate metrics (records/sec, avg time)
  │   ├─ logger.LogExecutionEnd(execCtx, "success", recordsProcessed, duration)
  │   └─ logger.LogMetrics(execCtx, metrics)
  │
  └─ return ExecutionResult
```

**Appelée par**:
- `Execute()` (wrapper)
- Tests

**Erreurs possibles**:
- `ErrNilPipeline`
- `ErrNilInputModule`
- `ErrNilOutputModule`
- Erreurs propagées depuis modules

**Effets de bord**:
- Execute modules (fetch, transform, send data)
- Log execution
- Close modules (defer)

---

#### Méthode: `ExecuteWithRecords(pipeline, records []map[string]interface{})`

**Signature**:
```go
func (e *Executor) ExecuteWithRecords(pipeline *connector.Pipeline, records []map[string]interface{}) (*connector.ExecutionResult, error)
```

**Responsabilité**: Exécute pipeline avec records pré-fetchés (mode webhook)

**Différence vs Execute**:
- Skip input.Fetch() (records déjà fournis)
- Directement à filters → output

**Appelle**:
```go
return e.ExecuteWithRecordsContext(context.Background(), pipeline, records)
```

**Usage**: Webhooks (data pushed, pas pulled)

---

## 5. Package `internal/factory`

### Function: `CreateInputModule(config *connector.ModuleConfig)`

**Signature**:
```go
func CreateInputModule(config *connector.ModuleConfig) input.Module
```

**Responsabilité**: Factory input modules

**Call chain**:
```
CreateInputModule(config)
  └─ Switch config.Type:
      ├─ "httpPolling" → input.NewStub(type, endpoint)  // Note: Stub pour MVP
      └─ default → input.NewStub(type, "")
```

**Note**: Actuellement retourne Stub, pas impl réelle (à compléter)

**Appelée par**: `cmd/canectors` lors setup pipeline

**Erreurs**: Aucune (retourne toujours module, même stub)

---

### Function: `CreateFilterModules(configs []connector.ModuleConfig)`

**Signature**:
```go
func CreateFilterModules(configs []connector.ModuleConfig) ([]filter.Module, error)
```

**Responsabilité**: Factory array filter modules

**Call chain détaillé**:
```
CreateFilterModules(configs)
  └─ For each config:
      ├─ Switch config.Type:
      │
      ├─ "mapping":
      │   ├─ filter.ParseFieldMappings(config.Config["mappings"])
      │   │   ├─ Type assert to []map[string]interface{}
      │   │   └─ For each mapping:
      │   │       ├─ parseFieldMappingMap(item, index)
      │   │       │   ├─ Extract: source, target
      │   │       │   ├─ Extract: defaultValue, onMissing
      │   │       │   ├─ Extract: transforms[]
      │   │       │   └─ Validate required fields
      │   │       └─ return FieldMapping
      │   │
      │   ├─ Extract onError strategy
      │   └─ filter.NewMappingFromConfig(mappings, onError)
      │       ├─ Validate mappings not empty
      │       ├─ Normalize onError (default "fail")
      │       └─ return MappingModule
      │
      ├─ "condition":
      │   ├─ ParseConditionConfig(config.Config)
      │   │   ├─ Extract: expression (required)
      │   │   ├─ Extract: lang, onTrue, onFalse, onError
      │   │   ├─ Si then config:
      │   │   │   └─ parseNestedConfig(thenCfg)  // Recursive
      │   │   └─ Si else config:
      │   │       └─ parseNestedConfig(elseCfg)  // Recursive
      │   │
      │   └─ filter.NewConditionFromConfig(condConfig)
      │       ├─ validateExpression(expression)
      │       ├─ normalizeLang(lang)
      │       ├─ compileExpression(expression)  // expr.Compile()
      │       ├─ createNestedModules(then, else, depth+1)
      │       └─ return ConditionModule
      │
      └─ default:
          └─ filter.NewStub(type, index)
```

**Appelée par**: `cmd/canectors`

**Erreurs possibles**:
- Invalid mapping config (missing source/target)
- Invalid condition config (missing expression)
- Expression compilation failed
- Nesting depth exceeded (>50)

---

### Function: `CreateOutputModule(config *connector.ModuleConfig)`

**Signature**:
```go
func CreateOutputModule(config *connector.ModuleConfig) (output.Module, error)
```

**Responsabilité**: Factory output modules

**Call chain**:
```
CreateOutputModule(config)
  └─ Switch config.Type:
      ├─ "httpRequest":
      │   └─ output.NewHTTPRequestFromConfig(config)
      │       ├─ extractBasicConfig(config.Config)
      │       │   ├─ Extract: endpoint, method, timeout
      │       │   └─ Validate: endpoint not empty, method in [POST,PUT,PATCH]
      │       ├─ extractHeaders(config.Config)
      │       ├─ extractRequestConfig(config.Config)
      │       │   ├─ Extract: bodyFrom, pathParams, queryParams
      │       │   └─ Extract: queryFromRecord, headersFromRecord
      │       ├─ extractErrorHandling(config.Config)
      │       ├─ extractSuccessCodes(config.Config)
      │       ├─ extractRetryConfig(config.Config)
      │       ├─ createHTTPClient(timeout)
      │       └─ auth.NewHandler(config.Authentication, client)
      │           ├─ Si type="bearer":
      │           │   └─ newBearerHandler(credentials)
      │           ├─ Si type="api-key":
      │           │   └─ newAPIKeyHandler(credentials)
      │           ├─ Si type="basic":
      │           │   └─ newBasicHandler(credentials)
      │           ├─ Si type="oauth2*":
      │           │   └─ newOAuth2Handler(credentials, client)
      │           └─ default: nil
      │
      └─ default:
          └─ output.NewStub(type, endpoint, method)
```

**Appelée par**: `cmd/canectors`

**Erreurs possibles**:
- Missing required fields (endpoint, method)
- Invalid method (not POST/PUT/PATCH)
- Invalid auth config

---

## 6. Package `internal/modules/input`

### Interface: `Module`

**Méthodes**:
```go
type Module interface {
    Fetch(ctx context.Context) ([]map[string]interface{}, error)
    Close() error
}
```

**Implémenté par**:
- `HTTPPolling`
- `Webhook`
- `StubModule`

---

### Type: `HTTPPolling`

**Rôle fonctionnel**: Fetch data via HTTP GET avec retry

**Cycle de vie**:
1. Créé via factory
2. Appelé Fetch() (potentiellement plusieurs fois si scheduled)
3. Close() appelé pour cleanup

**Champs principaux**:
```go
type HTTPPolling struct {
    endpoint    string
    method      string
    headers     map[string]string
    timeout     time.Duration
    dataField   string  // Nested field extraction
    authHandler auth.Handler
    client      *http.Client
    retry       errhandling.RetryConfig
    lastRetryInfo *connector.RetryInfo
}
```

**Implements**:
- `input.Module`
- `connector.RetryInfoProvider`

---

#### Méthode: `Fetch(ctx context.Context)`

**Signature**:
```go
func (h *HTTPPolling) Fetch(ctx context.Context) ([]map[string]interface{}, error)
```

**Responsabilité**: Fetch data depuis endpoint HTTP

**Call chain détaillé**:
```
Fetch(ctx)
  ├─ buildRequest(ctx)
  │   ├─ http.NewRequestWithContext(ctx, method, endpoint, nil)
  │   ├─ Set headers
  │   └─ authHandler.ApplyAuth(ctx, req)
  │       └─ [Dépend type: Bearer, API Key, OAuth2, etc.]
  │
  ├─ executeWithRetry(ctx, req)
  │   └─ Retry loop (maxAttempts):
  │       ├─ client.Do(req)
  │       ├─ Si error:
  │       │   ├─ errhandling.ClassifyNetworkError(err)
  │       │   ├─ Si retryable:
  │       │   │   ├─ Calculate backoff delay
  │       │   │   ├─ time.Sleep(delay)
  │       │   │   └─ Continue loop
  │       │   └─ Si fatal: return error
  │       └─ Si success: break loop
  │
  ├─ io.ReadAll(resp.Body)
  ├─ defer resp.Body.Close()
  │
  ├─ Check HTTP status:
  │   └─ Si not 2xx:
  │       ├─ errhandling.ClassifyHTTPStatus(statusCode)
  │       └─ return error
  │
  ├─ json.Unmarshal(body, &data)
  │
  ├─ extractRecords(data, dataField)
  │   ├─ Si dataField empty: return root
  │   └─ Sinon: navigate path (e.g., "data.items")
  │       ├─ Split by "."
  │       └─ Navigate nested maps
  │
  ├─ handlePagination(records)  // Si pagination configuré
  │   ├─ Si type="page": increment page number
  │   ├─ Si type="offset": increment offset
  │   └─ Si type="cursor": extract next cursor
  │
  └─ return records, nil
```

**Appelée par**: `runtime.Executor.executeInput()`

**Erreurs possibles**:
- Network error → retryable
- Auth error (401) → fatal
- Server error (5xx) → retryable
- Parse error → fatal

**Effets de bord**:
- HTTP request externe
- Update lastRetryInfo

---

#### Méthode: `GetRetryInfo()`

**Signature**:
```go
func (h *HTTPPolling) GetRetryInfo() *connector.RetryInfo
```

**Responsabilité**: Expose retry info (RetryInfoProvider interface)

**Implémentation simple**:
```go
return h.lastRetryInfo
```

**Appelée par**: `runtime.Executor` après Fetch()

---

### Type: `StubModule`

**Rôle fonctionnel**: Placeholder pour types non implémentés

**Méthodes**:

#### `Fetch(ctx context.Context)`

**Implémentation**:
```go
func (m *StubModule) Fetch(ctx context.Context) ([]map[string]interface{}, error) {
    logger.Info("Input stub called", slog.String("type", m.ModuleType))
    return nil, fmt.Errorf("input type %s not implemented", m.ModuleType)
}
```

**Usage**: Permet config valide même si impl manquante

---

## 7. Package `internal/modules/filter`

### Interface: `Module`

**Méthodes**:
```go
type Module interface {
    Process(ctx context.Context, records []map[string]interface{}) ([]map[string]interface{}, error)
}
```

**Implémenté par**:
- `MappingModule`
- `ConditionModule`
- `StubModule`

---

### Type: `MappingModule`

**Rôle fonctionnel**: Field-to-field transformations

**Champs principaux**:
```go
type MappingModule struct {
    mappings []FieldMapping
    onError  OnErrorStrategy  // "fail", "skip", "log"
}

type FieldMapping struct {
    Source       string
    Target       string
    DefaultValue interface{}
    OnMissing    string  // "setNull", "skipField", "useDefault", "fail"
    Transforms   []TransformOp
}
```

---

#### Méthode: `Process(ctx context.Context, records []map[string]interface{})`

**Signature**:
```go
func (m *MappingModule) Process(ctx context.Context, records []map[string]interface{}) ([]map[string]interface{}, error)
```

**Responsabilité**: Transform records via mappings

**Call chain détaillé**:
```
Process(ctx, records)
  └─ For each record:
      ├─ newRecord := make(map[string]interface{})
      │
      └─ For each mapping:
          ├─ getFieldValue(record, mapping.Source)
          │   ├─ Split source path by "."
          │   └─ Navigate nested maps:
          │       ├─ record["user"]["name"]
          │       └─ return value, exists
          │
          ├─ Si !exists:
          │   └─ Handle onMissing:
          │       ├─ "setNull": newRecord[target] = nil
          │       ├─ "skipField": continue
          │       ├─ "useDefault": newRecord[target] = defaultValue
          │       └─ "fail": return error
          │
          ├─ applyTransforms(value, transforms)
          │   └─ For each transform:
          │       ├─ Switch transform.Op:
          │       │   ├─ "toString": fmt.Sprintf("%v", value)
          │       │   ├─ "toInt": strconv.Atoi(value)
          │       │   ├─ "uppercase": strings.ToUpper(value)
          │       │   ├─ "trim": strings.TrimSpace(value)
          │       │   ├─ "replace": regexp.ReplaceAllString(value, pattern, replacement)
          │       │   └─ etc.
          │       └─ value = transformedValue
          │
          └─ setFieldValue(newRecord, mapping.Target, value)
              ├─ Split target path by "."
              └─ Create nested maps si needed:
                  └─ newRecord["user"]["full_name"] = value
```

**Appelée par**: `runtime.Executor.executeFilters()`

**Erreurs possibles**:
- Missing required field (si onMissing="fail")
- Transform error (si onError="fail")
- Type conversion error

**Effets de bord**: Aucun (pure transformation)

---

### Type: `ConditionModule`

**Rôle fonctionnel**: Expression-based filtering + nested modules

**Champs principaux**:
```go
type ConditionModule struct {
    expression string
    program    *vm.Program  // Compiled expression (expr-lang)
    lang       string       // "simple" (default), "cel", etc.
    onTrue     OnCondition  // "continue" ou "skip"
    onFalse    OnCondition  // "continue" ou "skip"
    onError    OnErrorStrategy
    thenModule Module       // Nested module si true
    elseModule Module       // Nested module si false
}
```

---

#### Méthode: `Process(ctx context.Context, records []map[string]interface{})`

**Signature**:
```go
func (c *ConditionModule) Process(ctx context.Context, records []map[string]interface{}) ([]map[string]interface{}, error)
```

**Responsabilité**: Filter/route records based on expression

**Call chain détaillé**:
```
Process(ctx, records)
  └─ For each record:
      ├─ expr.Run(program, record)
      │   └─ Evaluate expression:
      │       ├─ Exemple: "status == 'active' && age >= 18"
      │       └─ return bool (true/false)
      │
      ├─ Si error evaluating:
      │   └─ Handle onError:
      │       ├─ "fail": return error
      │       ├─ "skip": logger.Error(), continue
      │       └─ "log": logger.Warn(), continue
      │
      ├─ Si result == true:
      │   ├─ Si onTrue == "skip": continue (remove record)
      │   └─ Si onTrue == "continue":
      │       ├─ Si thenModule != nil:
      │       │   └─ thenModule.Process(ctx, [record])
      │       └─ Append to results
      │
      └─ Si result == false:
          ├─ Si onFalse == "skip": continue (remove record)
          └─ Si onFalse == "continue":
              ├─ Si elseModule != nil:
              │   └─ elseModule.Process(ctx, [record])
              └─ Append to results
```

**Appelée par**: `runtime.Executor.executeFilters()`

**Erreurs possibles**:
- Expression evaluation error
- Nested module error (propagated)

**Effets de bord**: Aucun (pure filtering)

---

## 8. Package `internal/modules/output`

### Interface: `Module`

**Méthodes**:
```go
type Module interface {
    Send(ctx context.Context, records []map[string]interface{}) (int, error)
    Close() error
}
```

**Implémenté par**:
- `HTTPRequestModule`
- `StubModule`

---

### Interface: `PreviewableModule`

**Méthodes**:
```go
type PreviewableModule interface {
    Module
    PreviewRequest(records []map[string]interface{}, opts PreviewOptions) ([]RequestPreview, error)
}
```

**Implémenté par**:
- `HTTPRequestModule`

---

### Type: `HTTPRequestModule`

**Rôle fonctionnel**: Send data via HTTP POST/PUT/PATCH

**Champs principaux**:
```go
type HTTPRequestModule struct {
    endpoint      string
    method        string
    headers       map[string]string
    timeout       time.Duration
    request       RequestConfig  // bodyFrom, pathParams, etc.
    retry         RetryConfig
    authHandler   auth.Handler
    client        *http.Client
    onError       OnErrorStrategy
    successCodes  []int
    lastRetryInfo *connector.RetryInfo
}

type RequestConfig struct {
    BodyFrom          string  // "records" (batch) ou "record" (single)
    PathParams        map[string]string
    QueryParams       map[string]string
    QueryFromRecord   map[string]string
    HeadersFromRecord map[string]string
}
```

**Implements**:
- `output.Module`
- `output.PreviewableModule`
- `connector.RetryInfoProvider`

---

#### Méthode: `Send(ctx context.Context, records []map[string]interface{})`

**Signature**:
```go
func (h *HTTPRequestModule) Send(ctx context.Context, records []map[string]interface{}) (int, error)
```

**Responsabilité**: Send records via HTTP

**Call chain détaillé**:
```
Send(ctx, records)
  ├─ Si len(records) == 0: return 0, nil
  │
  ├─ logger.LogInfo("output send started")
  │
  ├─ Si request.BodyFrom == "record" (single mode):
  │   └─ sendSingleRecordMode(ctx, records)
  │       └─ For each record:
  │           ├─ json.Marshal(record)  // Single object
  │           ├─ resolveEndpointForRecord(record)
  │           │   ├─ Replace path params: {userId} → record["user"]["id"]
  │           │   ├─ Add query params from config
  │           │   └─ Add query params from record
  │           ├─ extractHeadersFromRecord(record)
  │           │   └─ Extract headers from record data
  │           ├─ doRequestWithHeaders(ctx, endpoint, body, headers)
  │           │   └─ [Voir ci-dessous]
  │           ├─ Si error && onError="fail": return sent, error
  │           ├─ Si error && onError="skip": continue
  │           └─ sent++
  │
  └─ Sinon (batch mode, default):
      └─ sendBatchMode(ctx, records)
          ├─ json.Marshal(records)  // Array
          ├─ resolveEndpointWithStaticQuery(endpoint)
          └─ doRequestWithHeaders(ctx, endpoint, body, nil)
```

**doRequestWithHeaders() détaillé**:
```
doRequestWithHeaders(ctx, endpoint, body, recordHeaders)
  └─ retryLoop(ctx, endpoint, body, recordHeaders, startTime, delaysMs, oauth2Retried)
      └─ For attempt := 0; attempt <= maxAttempts; attempt++:
          ├─ Si attempt > 0:
          │   ├─ backoff = retry.CalculateDelay(attempt-1)
          │   │   └─ min(delayMs * (backoffMultiplier^attempt), maxDelayMs)
          │   ├─ logger.Info("retrying request")
          │   └─ time.Sleep(backoff)
          │
          ├─ executeHTTPRequest(ctx, endpoint, body, recordHeaders)
          │   ├─ http.NewRequestWithContext(ctx, method, endpoint, body)
          │   ├─ Set headers (defaults + custom + record)
          │   ├─ authHandler.ApplyAuth(ctx, req)
          │   │   └─ [Dépend type auth]
          │   ├─ client.Do(req)
          │   ├─ defer resp.Body.Close()
          │   ├─ io.ReadAll(resp.Body)  // Limited to 1MB
          │   ├─ Si statusCode not in successCodes:
          │   │   ├─ errhandling.ClassifyHTTPStatus(statusCode)
          │   │   └─ return HTTPError
          │   └─ return nil (success)
          │
          ├─ Si err == nil: return nil (success)
          │
          ├─ Si OAuth2 401 && !oauth2Retried:
          │   ├─ authHandler.InvalidateToken()
          │   ├─ oauth2Retried = true
          │   └─ continue (retry with fresh token)
          │
          ├─ Si !errhandling.IsRetryable(err):
          │   ├─ logger.Debug("non-retryable error")
          │   └─ return err (fatal)
          │
          └─ logger.Warn("transient error, will retry")
```

**Appelée par**: `runtime.Executor.executeOutput()`

**Erreurs possibles**:
- Network error → retry avec backoff
- Auth error → fail (ou invalidate token si OAuth2)
- Server error (5xx) → retry
- Client error (4xx) → fail (sauf 429)

**Effets de bord**:
- HTTP requests externes
- Update lastRetryInfo

---

#### Méthode: `PreviewRequest(records []map[string]interface{}, opts PreviewOptions)`

**Signature**:
```go
func (h *HTTPRequestModule) PreviewRequest(records []map[string]interface{}, opts PreviewOptions) ([]RequestPreview, error)
```

**Responsabilité**: Generate request previews (dry-run mode)

**Call chain**:
```
PreviewRequest(records, opts)
  ├─ Si bodyFrom == "records" (batch):
  │   └─ previewBatchMode(records, opts)
  │       ├─ resolveEndpointWithStaticQuery(endpoint)
  │       ├─ json.MarshalIndent(records, "", "  ")
  │       ├─ buildPreviewHeaders(nil, opts)
  │       │   ├─ Set default headers
  │       │   ├─ Set custom headers
  │       │   └─ Si opts.ShowCredentials:
  │       │       └─ addUnmaskedAuthHeaders(headers)
  │       │   └─ Sinon:
  │       │       └─ addMaskedAuthHeaders(headers)
  │       │           └─ "Authorization: Bearer [MASKED-TOKEN]"
  │       └─ return []RequestPreview{preview}
  │
  └─ Sinon (single mode):
      └─ previewSingleRecordMode(records, opts)
          └─ For each record:
              ├─ resolveEndpointForRecord(record)
              ├─ json.MarshalIndent(record, "", "  ")
              ├─ extractHeadersFromRecord(record)
              ├─ buildPreviewHeaders(recordHeaders, opts)
              └─ previews = append(previews, preview)
```

**Appelée par**: `runtime.Executor.executeDryRunPreview()`

**Usage**: Dry-run mode (validation sans side effects)

---

## 9. Package `internal/auth`

### Interface: `Handler`

**Méthodes**:
```go
type Handler interface {
    ApplyAuth(ctx context.Context, req *http.Request) error
    Type() string
}
```

**Implémenté par**:
- `bearerHandler`
- `apiKeyHandler`
- `basicHandler`
- `oauth2Handler`

---

### Type: `oauth2Handler`

**Rôle fonctionnel**: OAuth2 Client Credentials flow + token caching

**Champs**:
```go
type oauth2Handler struct {
    tokenURL     string
    clientID     string
    clientSecret string
    scopes       []string
    client       *http.Client
    mu           sync.RWMutex
    token        *oauth2Token
}

type oauth2Token struct {
    accessToken string
    expiresAt   time.Time
}
```

**Implements**:
- `auth.Handler`
- Interface inline `OAuth2Invalidator` (InvalidateToken())

---

#### Méthode: `ApplyAuth(ctx context.Context, req *http.Request)`

**Signature**:
```go
func (h *oauth2Handler) ApplyAuth(ctx context.Context, req *http.Request) error
```

**Responsabilité**: Apply OAuth2 Bearer token to request

**Call chain**:
```
ApplyAuth(ctx, req)
  ├─ getValidToken(ctx)
  │   ├─ h.mu.RLock()
  │   ├─ Si token != nil && !expired:
  │   │   ├─ h.mu.RUnlock()
  │   │   └─ return token (cached)
  │   ├─ h.mu.RUnlock()
  │   │
  │   ├─ h.mu.Lock()  // Upgrade to write lock
  │   ├─ Double-check (si autre goroutine a refresh)
  │   ├─ Si toujours expired:
  │   │   └─ fetchToken(ctx)
  │   │       ├─ Build token request (POST tokenURL)
  │   │       │   ├─ Form data: grant_type, client_id, client_secret, scope
  │   │       │   └─ Content-Type: application/x-www-form-urlencoded
  │   │       ├─ client.Do(req)
  │   │       ├─ json.Unmarshal(resp.Body, &tokenResp)
  │   │       ├─ Calculate expiresAt (now + expires_in - 60s buffer)
  │   │       └─ h.token = &oauth2Token{access_token, expiresAt}
  │   ├─ h.mu.Unlock()
  │   └─ return token
  │
  └─ req.Header.Set("Authorization", "Bearer " + token)
```

**Appelée par**:
- `input.HTTPPolling.Fetch()` (via buildRequest)
- `output.HTTPRequestModule.executeHTTPRequest()`

**Erreurs possibles**:
- Token fetch failed (network, auth invalid)

**Effets de bord**:
- HTTP request to token URL
- Cache token (thread-safe)

---

#### Méthode: `InvalidateToken()`

**Signature**:
```go
func (h *oauth2Handler) InvalidateToken()
```

**Responsabilité**: Invalidate cached token (force refresh on next request)

**Implémentation**:
```go
func (h *oauth2Handler) InvalidateToken() {
    h.mu.Lock()
    defer h.mu.Unlock()
    h.token = nil
}
```

**Appelée par**: `output.HTTPRequestModule.handleOAuth2Unauthorized()` (si 401)

**Usage pattern**:
```
Si HTTP 401 && OAuth2:
  1. InvalidateToken() → force cached token null
  2. Retry request → getValidToken() fetch fresh token
  3. Si 401 persiste → auth credentials invalides (fail)
```

---

## 10. Package `internal/errhandling`

### Type: `ClassifiedError`

**Rôle fonctionnel**: Error wrapper avec classification

**Champs**:
```go
type ClassifiedError struct {
    Category    ErrorCategory  // network, auth, validation, server, etc.
    Retryable   bool
    StatusCode  int
    Message     string
    OriginalErr error
}
```

**Implements**: `error` interface

---

### Function: `ClassifyError(err error)`

**Signature**:
```go
func ClassifyError(err error) *ClassifiedError
```

**Responsabilité**: Classify any error

**Call chain**:
```
ClassifyError(err)
  ├─ Si err == nil: return nil
  │
  ├─ Si déjà ClassifiedError:
  │   └─ return err (déjà classifié)
  │
  ├─ Si HTTPError (custom type):
  │   └─ ClassifyHTTPStatus(statusCode, message)
  │
  ├─ Si network error (url.Error, net.Error, context errors):
  │   └─ ClassifyNetworkError(err)
  │       ├─ Si context.Canceled: Category=network, Retryable=false
  │       ├─ Si context.DeadlineExceeded: Category=network, Retryable=true
  │       ├─ Si connection refused: Category=network, Retryable=true
  │       └─ default: Category=network, Retryable=true
  │
  └─ default:
      └─ return ClassifiedError{Category: unknown, Retryable: false}
```

**Appelée par**:
- `runtime.Executor.buildExecutionError()`
- Module implementations (retry logic)

---

### Function: `ClassifyHTTPStatus(statusCode int, message string)`

**Signature**:
```go
func ClassifyHTTPStatus(statusCode int, message string) *ClassifiedError
```

**Responsabilité**: Classify HTTP status codes

**Logic**:
```
ClassifyHTTPStatus(statusCode, message)
  └─ Switch statusCode:
      ├─ 401: Category=authentication, Retryable=false
      ├─ 403: Category=authentication, Retryable=false
      ├─ 404: Category=not_found, Retryable=false
      ├─ 429: Category=rate_limit, Retryable=true
      ├─ 400-499: Category=validation, Retryable=false
      ├─ 500: Category=server, Retryable=true
      ├─ 502-504: Category=server, Retryable=true
      └─ default: Category=unknown, Retryable=false
```

**Appelée par**:
- `input.HTTPPolling.Fetch()` (check response status)
- `output.HTTPRequestModule.executeHTTPRequest()`

---

### Function: `IsRetryable(err error)`

**Signature**:
```go
func IsRetryable(err error) bool
```

**Responsabilité**: Détermine si erreur retryable

**Implémentation**:
```go
func IsRetryable(err error) bool {
    classified := ClassifyError(err)
    return classified != nil && classified.Retryable
}
```

**Appelée par**: Retry loops dans modules

---

### Type: `RetryConfig`

**Rôle fonctionnel**: Configuration retry logic

**Champs**:
```go
type RetryConfig struct {
    MaxAttempts          int    // Max retry attempts (default 3)
    DelayMs              int    // Initial delay (default 1000)
    BackoffMultiplier    float64 // Exponential multiplier (default 2.0)
    MaxDelayMs           int    // Max delay cap (default 30000)
    RetryableStatusCodes []int  // HTTP codes to retry (default 429,500,502,503,504)
}
```

---

#### Méthode: `CalculateDelay(attempt int)`

**Signature**:
```go
func (r *RetryConfig) CalculateDelay(attempt int) time.Duration
```

**Responsabilité**: Calculate exponential backoff delay

**Implémentation**:
```go
func (r *RetryConfig) CalculateDelay(attempt int) time.Duration {
    delay := float64(r.DelayMs) * math.Pow(r.BackoffMultiplier, float64(attempt))
    if delay > float64(r.MaxDelayMs) {
        delay = float64(r.MaxDelayMs)
    }
    return time.Duration(delay) * time.Millisecond
}
```

**Exemple**:
```
Config: DelayMs=1000, BackoffMultiplier=2.0, MaxDelayMs=30000

Attempt 0: 1000 * (2^0) = 1000ms
Attempt 1: 1000 * (2^1) = 2000ms
Attempt 2: 1000 * (2^2) = 4000ms
Attempt 3: 1000 * (2^3) = 8000ms
Attempt 4: 1000 * (2^4) = 16000ms
Attempt 5: 1000 * (2^5) = 32000ms → capped to 30000ms
```

**Appelée par**: Retry loops

---

## 11. Package `internal/scheduler`

### Type: `Scheduler`

**Rôle fonctionnel**: CRON-based pipeline scheduling

**Cycle de vie**:
1. Créé via `NewWithExecutor(executor)`
2. Register pipelines
3. Start() → run CRON loop
4. Stop() → graceful shutdown

**Champs**:
```go
type Scheduler struct {
    cron      *cron.Cron
    pipelines map[string]*registeredPipeline
    executor  Executor  // Interface: Execute(pipeline) (result, error)
    mu        sync.RWMutex
    started   bool
    ctx       context.Context
    cancel    context.CancelFunc
    wg        sync.WaitGroup
    stopMu    sync.RWMutex
    stopChan  chan struct{}
}

type registeredPipeline struct {
    pipeline *connector.Pipeline
    entryID  cron.EntryID
    running  bool
    mu       sync.Mutex
}
```

---

#### Méthode: `Register(pipeline *connector.Pipeline)`

**Signature**:
```go
func (s *Scheduler) Register(pipeline *connector.Pipeline) error
```

**Responsabilité**: Register pipeline avec CRON schedule

**Call chain**:
```
Register(pipeline)
  ├─ Validate:
  │   ├─ pipeline != nil
  │   ├─ pipeline.Enabled == true
  │   └─ pipeline.Schedule != ""
  │
  ├─ ValidateCronExpression(pipeline.Schedule)
  │   ├─ cron.Parser.Parse(schedule)
  │   └─ Si invalid: return ErrInvalidCronExpression
  │
  ├─ s.mu.Lock()
  ├─ Check si already registered:
  │   └─ Si exists && running: return ErrPipelineRunning
  │
  ├─ cron.AddFunc(schedule, func() { executePipeline(reg) })
  │   └─ Store entryID
  │
  ├─ pipelines[pipelineID] = &registeredPipeline{
  │       pipeline: pipeline,
  │       entryID:  entryID,
  │       running:  false,
  │   }
  │
  └─ s.mu.Unlock()
```

**Appelée par**: `cmd/canectors` (scheduled mode)

**Erreurs possibles**:
- ErrNilPipeline
- ErrPipelineDisabled
- ErrEmptySchedule
- ErrInvalidCronExpression
- ErrPipelineRunning

---

#### Méthode: `Start(ctx context.Context)`

**Signature**:
```go
func (s *Scheduler) Start(ctx context.Context) error
```

**Responsabilité**: Start CRON scheduler

**Call chain**:
```
Start(ctx)
  ├─ s.mu.Lock()
  ├─ Si already started: return ErrSchedulerAlreadyRunning
  ├─ s.started = true
  ├─ s.ctx, s.cancel = context.WithCancel(ctx)
  ├─ s.mu.Unlock()
  │
  └─ s.cron.Start()  // Start CRON (non-blocking)
```

**Appelée par**: `cmd/canectors`

**Effets de bord**: Start goroutine CRON loop

---

#### Méthode: `Stop(ctx context.Context)`

**Signature**:
```go
func (s *Scheduler) Stop(ctx context.Context) error
```

**Responsabilité**: Graceful shutdown scheduler

**Call chain**:
```
Stop(ctx)
  ├─ s.mu.Lock()
  ├─ Si !started: return ErrSchedulerNotStarted
  ├─ s.mu.Unlock()
  │
  ├─ s.cron.Stop()  // Stop accepting new ticks
  │
  ├─ Wait for in-flight executions (avec timeout):
  │   ├─ done := make(chan struct{})
  │   ├─ go func() {
  │   │       s.wg.Wait()  // Wait all goroutines
  │   │       close(done)
  │   │   }()
  │   │
  │   └─ select {
  │       case <-done:
  │           return nil
  │       case <-ctx.Done():
  │           return ctx.Err()  // Timeout
  │       }
  │
  ├─ s.mu.Lock()
  ├─ Collect all registered pipelines
  ├─ s.started = false
  └─ s.mu.Unlock()
```

**Appelée par**: `cmd/canectors` (on SIGINT/SIGTERM)

**Timeout**: Défini par caller (typiquement 30s)

---

#### Function: `executePipeline(reg *registeredPipeline)`

**Responsabilité**: Execute pipeline (callback CRON)

**Call chain**:
```
executePipeline(reg)
  ├─ reg.mu.Lock()
  ├─ Si reg.running:
  │   ├─ logger.Warn("previous execution still running, skipping")
  │   ├─ reg.mu.Unlock()
  │   └─ return  // Overlap prevention
  │
  ├─ reg.running = true
  ├─ reg.mu.Unlock()
  │
  ├─ s.wg.Add(1)  // Track goroutine
  │
  └─ go func() {
      ├─ defer s.wg.Done()
      ├─ defer func() {
      │       reg.mu.Lock()
      │       reg.running = false
      │       reg.mu.Unlock()
      │   }()
      │
      ├─ result, err := s.executor.Execute(reg.pipeline)
      │   └─ [Via PipelineExecutorAdapter]
      │
      └─ logger.LogResult(result, err)
      }()
```

**Appelée par**: CRON tick callback

**Pattern**: Goroutine pour non-blocking execution

---

## 12. Package `internal/logger`

### Type: `ExecutionContext`

**Rôle fonctionnel**: Context structuré pour logging

**Champs**:
```go
type ExecutionContext struct {
    PipelineID   string
    PipelineName string
    Stage        string  // "input", "filter", "output"
    ModuleType   string
    ModuleName   string
    DryRun       bool
    FilterIndex  int
}
```

**Usage**: Passé à helpers logging pour structured fields

---

### Function: `LogExecutionStart(ctx ExecutionContext)`

**Signature**:
```go
func LogExecutionStart(ctx ExecutionContext)
```

**Responsabilité**: Log début exécution pipeline

**Implémentation**:
```go
func LogExecutionStart(ctx ExecutionContext) {
    logger.Info("pipeline execution started",
        slog.String("pipeline_id", ctx.PipelineID),
        slog.String("pipeline_name", ctx.PipelineName),
        slog.Bool("dry_run", ctx.DryRun),
    )
}
```

**Appelée par**: `runtime.Executor.ExecuteWithContext()` (début)

---

### Function: `LogExecutionEnd(ctx ExecutionContext, status string, recordsProcessed int, totalDuration time.Duration)`

**Signature**:
```go
func LogExecutionEnd(ctx ExecutionContext, status string, recordsProcessed int, totalDuration time.Duration)
```

**Responsabilité**: Log fin exécution pipeline

**Implémentation**:
```go
func LogExecutionEnd(ctx ExecutionContext, status string, recordsProcessed int, totalDuration time.Duration) {
    logger.Info("pipeline execution completed",
        slog.String("pipeline_id", ctx.PipelineID),
        slog.String("status", status),
        slog.Int("records_processed", recordsProcessed),
        slog.Duration("total_duration", totalDuration),
        slog.Bool("dry_run", ctx.DryRun),
    )
}
```

**Appelée par**: `runtime.Executor.ExecuteWithContext()` (fin)

---

### Function: `LogMetrics(ctx ExecutionContext, metrics ExecutionMetrics)`

**Signature**:
```go
func LogMetrics(ctx ExecutionContext, metrics ExecutionMetrics)
```

**Responsabilité**: Log métriques détaillées performance

**Métriques loggées**:
```go
type ExecutionMetrics struct {
    TotalDuration    time.Duration
    InputDuration    time.Duration
    FilterDuration   time.Duration
    OutputDuration   time.Duration
    RecordsProcessed int
    RecordsFailed    int
    RecordsPerSecond float64
    AvgRecordTime    time.Duration
}
```

**Appelée par**: `runtime.Executor.finalizeSuccessWithMetrics()`

---

## Résumé — Call Graph Global

### Flow Principal: One-Shot Execution

```
main()
  → runCmd.Run()
      → config.ParseConfig()
          → ParseJSONFile/ParseYAMLFile()
          → ValidateConfig()
              → schema.Validate()

      → config.ConvertToPipeline()
          → extractPipelineMetadata()
          → extractModules()
          → applyErrorHandling()

      → factory.CreateInputModule()
      → factory.CreateFilterModules()
          → For each filter:
              → ParseFieldMappings() OU ParseConditionConfig()
              → NewMappingFromConfig() OU NewConditionFromConfig()
      → factory.CreateOutputModule()
          → NewHTTPRequestFromConfig()
              → auth.NewHandler()

      → runtime.NewExecutorWithModules()

      → executor.Execute()
          → ExecuteWithContext()
              → executeInput()
                  → inputModule.Fetch()
                      → [HTTP GET, parse, retry, etc.]

              → executeFilters()
                  → For each filter:
                      → filterModule.Process()
                          → [Mapping: transform fields]
                          → [Condition: evaluate expr, filter]

              → executeOutput()
                  → outputModule.Send()
                      → [HTTP POST/PUT, retry, etc.]

              → finalizeSuccess()
                  → logger.LogMetrics()

      → cli.PrintExecutionResult()
      → os.Exit()
```

### Flow Scheduled Execution

```
main()
  → runCmd.Run()
      → [Parse + Convert identique]

      → scheduler.NewWithExecutor(adapter)
      → scheduler.Register(pipeline)
          → cron.AddFunc(schedule, executePipeline)

      → scheduler.Start()
          → cron.Start()

      → [Wait for SIGINT/SIGTERM]

      → À chaque CRON tick:
          → executePipeline()
              → Check overlap (skip si running)
              → go func() {
                      executor.Execute(pipeline)
                  }()

      → scheduler.Stop()
          → cron.Stop()
          → wg.Wait() (avec timeout 30s)

      → os.Exit()
```

---

**FIN DE TYPE_AND_METHOD_MAP.md**

Cette cartographie exhaustive permet de reconstruire mentalement le call graph complet du système.
