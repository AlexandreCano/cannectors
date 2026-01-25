# PACKAGE MAP — Canectors Runtime

**Date**: 2026-01-23
**Public cible**: Ingénieur Java Senior (10+ ans) apprenant Go
**Niveau**: Zoom par package (responsabilités, types, dépendances)

---

## Vue d'Ensemble — Arborescence Packages

```
canectors-runtime/
├── cmd/canectors/          → CLI entry point
├── pkg/connector/          → API publique (types exportés)
└── internal/               → Code privé (interne au projet)
    ├── auth/               → Authentification HTTP
    ├── cli/                → Formatage sortie CLI
    ├── config/             → Parsing/validation configuration
    ├── errhandling/        → Classification erreurs + retry
    ├── factory/            → Factory pattern (création modules)
    ├── logger/             → Logging structuré
    ├── modules/
    │   ├── input/          → Modules input (fetch data)
    │   ├── filter/         → Modules filter (transform data)
    │   └── output/         → Modules output (send data)
    ├── runtime/            → Orchestrateur pipeline
    └── scheduler/          → Planificateur CRON
```

---

## Package `cmd/canectors`

### Responsabilité

**CLI entry point** — Point d'entrée de l'application

**Équivalent Java**: `public static void main(String[] args)`

---

### Ce qu'il fait

✅ Parse arguments CLI via Cobra framework
✅ Charge configuration depuis fichier
✅ Valide configuration
✅ Crée executor et modules
✅ Exécute pipeline (one-shot ou scheduled)
✅ Gère signals système (SIGINT, SIGTERM)
✅ Détermine exit code approprié

---

### Ce qu'il ne fait PAS

❌ Parse/validate config (délègue à `internal/config`)
❌ Crée modules (délègue à `internal/factory`)
❌ Exécute pipeline logic (délègue à `internal/runtime`)
❌ Logging métier (utilise `internal/logger`)

---

### Types Exposés

**Aucun type exporté** — Package `main` (non-importable)

**Types internes**:
- `PipelineExecutorAdapter`: Adapte `runtime.Executor` pour `scheduler.Executor`

---

### Fonctions Publiques (Commands)

| Command | Signature | Description |
|---------|-----------|-------------|
| `validate <config-file>` | CLI command | Parse + validate config, affiche erreurs |
| `run <config-file>` | CLI command | Exécute pipeline (one-shot ou scheduled) |
| `version` | CLI command | Affiche version, commit, build date |

---

### Dépendances

**Entrantes** (packages utilisant `cmd/canectors`):
- Aucune (entry point)

**Sortantes** (packages utilisés par `cmd/canectors`):
- `pkg/connector` → Types Pipeline, ExecutionResult
- `internal/config` → Parse/validate config
- `internal/factory` → Create modules
- `internal/runtime` → Executor
- `internal/scheduler` → Scheduler
- `internal/logger` → Logging
- `internal/cli` → Output formatting
- `github.com/spf13/cobra` → CLI framework

---

### Exemples Usage

**Validate config**:
```bash
canectors validate config.yaml
# Exit 0 si valide, 1 si validation errors, 2 si parse errors
```

**Run pipeline once**:
```bash
canectors run config.yaml
# Exit 0 si success, 3 si runtime error
```

**Run scheduled pipeline**:
```bash
canectors run config.yaml  # Si config contient schedule CRON
# Tourne indéfiniment jusqu'à Ctrl+C
```

**Dry-run mode**:
```bash
canectors run config.yaml --dry-run
# Exécute pipeline sans envoyer data
```

**Verbose logging**:
```bash
canectors run config.yaml --verbose
# Log level DEBUG + format human-readable
```

---

## Package `pkg/connector`

### Responsabilité

**API publique** — Types exportés pour clients externes

**Équivalent Java**: Package `com.canectors.api` avec classes DTO publiques

---

### Ce qu'il fait

✅ Définit types configuration (Pipeline, ModuleConfig, AuthConfig)
✅ Définit types résultat (ExecutionResult, ExecutionError, RetryInfo)
✅ Définit interfaces publiques (RetryInfoProvider)
✅ Expose contrats stable pour consommateurs externes

---

### Ce qu'il ne fait PAS

❌ Parse config (délégué à `internal/config`)
❌ Valide config (délégué à `internal/config`)
❌ Exécute logic (délégué à `internal/runtime`, `internal/modules`)

---

### Types Exposés

**Configuration Types**:

| Type | Description | Champs Principaux |
|------|-------------|-------------------|
| `Pipeline` | Configuration pipeline complète | ID, Name, Version, Schedule, Input, Filters, Output, Defaults |
| `ModuleConfig` | Configuration module générique | Type, Config (map), Authentication |
| `AuthConfig` | Configuration authentification | Type, Credentials (map) |
| `ModuleDefaults` | Valeurs par défaut modules | OnError, TimeoutMs, Retry |
| `ErrorHandling` | Legacy error handling config | RetryCount, RetryDelayMs, OnError |
| `DryRunOptions` | Options dry-run | ShowCredentials |

**Execution Types**:

| Type | Description | Champs Principaux |
|------|-------------|-------------------|
| `ExecutionResult` | Résultat exécution pipeline | PipelineID, Status, RecordsProcessed, RecordsFailed, StartedAt, CompletedAt, Error, RetryInfo, DryRunPreview |
| `ExecutionError` | Erreur exécution détaillée | Code, Message, Module, ErrorCategory, ErrorType, Details |
| `RetryInfo` | Info retry tentatives | TotalAttempts, RetryCount, RetryDelaysMs |
| `RequestPreview` | Preview requête HTTP (dry-run) | Endpoint, Method, Headers, BodyPreview, RecordCount |

**Interfaces**:

| Interface | Description | Méthodes |
|-----------|-------------|----------|
| `RetryInfoProvider` | Modules supportant retry info | `GetRetryInfo() *RetryInfo` |

---

### Équivalent Java

```java
// pkg/connector types.go
package com.canectors.api;

public class Pipeline {
    private String id;
    private String name;
    private String version;
    private String schedule;
    private ModuleConfig input;
    private List<ModuleConfig> filters;
    private ModuleConfig output;
    private ModuleDefaults defaults;
    // getters/setters
}

public class ExecutionResult {
    private String pipelineId;
    private String status;
    private int recordsProcessed;
    private int recordsFailed;
    private LocalDateTime startedAt;
    private LocalDateTime completedAt;
    private ExecutionError error;
    private RetryInfo retryInfo;
    // getters/setters
}

public interface RetryInfoProvider {
    RetryInfo getRetryInfo();
}
```

---

### Dépendances

**Entrantes**:
- Tous packages internes utilisent ces types

**Sortantes**:
- Aucune (types purs, pas de dépendances)

---

## Package `internal/config`

### Responsabilité

**Parsing & Validation** — Charge et valide fichiers configuration

**Équivalent Java**: `@ConfigurationProperties` + Bean Validation

---

### Ce qu'il fait

✅ Parse JSON/YAML files → `map[string]interface{}`
✅ Valide data contre schéma JSON
✅ Convertit map → struct `Pipeline`
✅ Applique résolution priorités defaults/error handling
✅ Retourne erreurs détaillées (parse, validation)

---

### Ce qu'il ne fait PAS

❌ Exécute pipeline
❌ Crée modules
❌ Log execution

---

### Types Exposés

| Type | Description | Champs Principaux |
|------|-------------|-------------------|
| `Result` | Résultat combiné parsing+validation | Data, Format, ParseErrors, ValidationErrors, IsValid |
| `ParseError` | Erreur parsing | Line, Column, Message |
| `ValidationError` | Erreur validation schema | Field, Message, Value |

---

### Fonctions Publiques

| Fonction | Signature | Description |
|----------|-----------|-------------|
| `ParseConfig` | `(filepath string) Result` | Parse + valide fichier (auto-détecte JSON/YAML) |
| `ParseJSONFile` | `(filepath string) (map, error)` | Parse JSON uniquement |
| `ParseYAMLFile` | `(filepath string) (map, error)` | Parse YAML uniquement |
| `ParseConfigString` | `(content, format string) Result` | Parse depuis string (testing) |
| `ValidateConfig` | `(data map) error` | Valide contre schéma JSON |
| `ConvertToPipeline` | `(data map) (Pipeline, error)` | Convertit map → Pipeline struct |

**Helpers Internes**:
- `extractPipelineMetadata()`: Extract ID, Name, Version, etc.
- `extractModules()`: Extract Input, Filters, Output
- `applyErrorHandling()`: Applique defaults/error handling inheritance

---

### Exemple Usage

```go
// Parse config file
result := config.ParseConfig("config.yaml")

if !result.IsValid() {
    // Print validation errors
    for _, err := range result.ValidationErrors {
        fmt.Printf("Error: %s at %s\n", err.Message, err.Field)
    }
    os.Exit(1)
}

// Convert to Pipeline struct
pipeline, err := config.ConvertToPipeline(result.Data)
if err != nil {
    log.Fatal(err)
}

// Use pipeline
executor.Execute(pipeline)
```

---

### Schéma JSON

**Fichier**: `internal/config/schema/pipeline-schema.json`

**Responsabilités**:
- Définit structure configuration attendue
- Enforce required fields
- Validate types (string, int, bool, array, object)
- Validate patterns (regex pour ID, version, etc.)
- Validate enums (onError, method, lang, etc.)

**Exemples contraintes**:
- `connector.name`: pattern `^[a-z][a-z0-9_-]*$`, max 128 chars
- `connector.version`: pattern `^\d+\.\d+\.\d+$` (semver)
- `input.type="httpPolling"` → required fields `endpoint`, `schedule`
- `output.method`: enum `POST`, `PUT`, `PATCH`
- `defaults.onError`: enum `fail`, `skip`, `log`

---

### Dépendances

**Entrantes**:
- `cmd/canectors` → Parse/validate config

**Sortantes**:
- `pkg/connector` → Types Pipeline, ModuleConfig
- `internal/errhandling` → For defaults conversion
- `github.com/santhosh-tekuri/jsonschema` → Schema validation
- `gopkg.in/yaml.v3` → YAML parsing

---

## Package `internal/runtime`

### Responsabilité

**Orchestrateur Pipeline** — Coordonne exécution Input → Filters → Output

**Équivalent Java**: `@Service` orchestrateur type Spring Batch `JobExecutor`

---

### Ce qu'il fait

✅ Reçoit Pipeline + Modules (dependency injection)
✅ Exécute stages séquentiellement: Input → Filters → Output
✅ Gère erreurs à chaque stage
✅ Collecte métriques (duration, records/sec)
✅ Log execution start/end
✅ Supporte dry-run mode
✅ Supporte context propagation (cancellation/timeout)

---

### Ce qu'il ne fait PAS

❌ Parse config
❌ Crée modules (reçoit modules via DI)
❌ Retry logic (délégué aux modules)

---

### Types Exposés

| Type | Description | Champs Principaux |
|------|-------------|-------------------|
| `Executor` | Orchestrateur pipeline | inputModule, filterModules, outputModule, dryRun |

**Erreurs**:
- `ErrNilPipeline`: Pipeline nil
- `ErrNilInputModule`: Input module nil
- `ErrNilOutputModule`: Output module nil (sauf si dryRun)

**Codes erreur**:
- `ErrCodeInputFailed`: "INPUT_FAILED"
- `ErrCodeFilterFailed`: "FILTER_FAILED"
- `ErrCodeOutputFailed`: "OUTPUT_FAILED"
- `ErrCodeInvalidInput`: "INVALID_INPUT"

---

### Fonctions Publiques

| Fonction | Signature | Description |
|----------|-----------|-------------|
| `NewExecutor` | `(dryRun bool) *Executor` | Constructeur minimal |
| `NewExecutorWithModules` | `(input, filters, output, dryRun) *Executor` | Constructeur avec DI |
| `Execute` | `(pipeline) (ExecutionResult, error)` | Exécute avec context.Background() |
| `ExecuteWithContext` | `(ctx, pipeline) (ExecutionResult, error)` | Exécute avec context custom |
| `ExecuteWithRecords` | `(pipeline, records) (ExecutionResult, error)` | Exécute avec records pre-fetched (webhooks) |
| `ExecuteWithRecordsContext` | `(ctx, pipeline, records) (ExecutionResult, error)` | Exécute pre-fetched + context |

---

### Flow Interne

**Phase 1**: Validation
```go
func (e *Executor) validateExecution(pipeline, result) error {
    if pipeline == nil { return ErrNilPipeline }
    if e.inputModule == nil { return ErrNilInputModule }
    if e.outputModule == nil && !e.dryRun { return ErrNilOutputModule }
    return nil
}
```

**Phase 2**: Input Execution
```go
func (e *Executor) executeInput(ctx, pipeline, result) ([]Record, time.Duration, error) {
    logger.LogStageStart(ctx)
    startTime := time.Now()

    records, err := e.inputModule.Fetch(ctx)
    duration := time.Since(startTime)

    if err != nil {
        result.Error = buildExecutionError(ErrCodeInputFailed, "input", err)
        logger.LogStageEnd(ctx, 0, duration, err)
        return nil, duration, err
    }

    logger.LogStageEnd(ctx, len(records), duration, nil)
    return records, duration, nil
}
```

**Phase 3**: Filters Execution (Sequential)
```go
func (e *Executor) executeFilters(ctx, pipelineID, records) filterResult {
    currentRecords := records
    for i, filter := range e.filterModules {
        logger.LogStageStart(ctx)
        startTime := time.Now()

        currentRecords, err = filter.Process(ctx, currentRecords)
        duration := time.Since(startTime)

        if err != nil {
            logger.LogStageEnd(ctx, len(currentRecords), duration, err)
            return filterResult{err: err, errIdx: i}
        }

        logger.LogStageEnd(ctx, len(currentRecords), duration, nil)
    }
    return filterResult{records: currentRecords}
}
```

**Phase 4**: Output Execution
```go
func (e *Executor) executeOutput(ctx, pipelineID, records) outputResult {
    if e.dryRun {
        logger.Debug("dry-run mode: skipping output")
        return outputResult{recordsSent: len(records)}
    }

    logger.LogStageStart(ctx)
    startTime := time.Now()

    recordsSent, err := e.outputModule.Send(ctx, records)
    duration := time.Since(startTime)

    if err != nil {
        logger.LogStageEnd(ctx, recordsSent, duration, err)
        return outputResult{recordsSent: recordsSent, recordsFailed: len(records)-recordsSent, err: err}
    }

    logger.LogStageEnd(ctx, recordsSent, duration, nil)
    return outputResult{recordsSent: recordsSent}
}
```

**Phase 5**: Finalize & Return
```go
func (e *Executor) finalizeSuccess(result, startedAt, pipeline, timings) {
    result.Status = "success"
    result.CompletedAt = time.Now()
    totalDuration := time.Since(startedAt)

    // Calculate metrics
    recordsPerSecond := float64(result.RecordsProcessed) / totalDuration.Seconds()
    avgRecordTime := totalDuration / time.Duration(result.RecordsProcessed)

    // Log metrics
    logger.LogExecutionEnd(ctx, "success", result.RecordsProcessed, totalDuration)
    logger.LogMetrics(ctx, ExecutionMetrics{...})
}
```

---

### Dépendances

**Entrantes**:
- `cmd/canectors` → Execute pipeline
- `scheduler` → Execute pipeline (via adapter)

**Sortantes**:
- `pkg/connector` → Pipeline, ExecutionResult types
- `internal/modules/input` → input.Module interface
- `internal/modules/filter` → filter.Module interface
- `internal/modules/output` → output.Module interface
- `internal/errhandling` → Error classification
- `internal/logger` → Logging

---

## Package `internal/factory`

### Responsabilité

**Factory Pattern** — Création instances modules depuis configuration

**Équivalent Java**: `@Configuration` class avec `@Bean` methods

---

### Ce qu'il fait

✅ Parse config module → Crée instance module
✅ Switch sur type module → Return impl concrète
✅ Return Stub si type non implémenté
✅ Parse nested modules configs (conditions)

---

### Ce qu'il ne fait PAS

❌ Valide config (assume déjà validé)
❌ Execute logic métier

---

### Fonctions Publiques

| Fonction | Signature | Description |
|----------|-----------|-------------|
| `CreateInputModule` | `(config *ModuleConfig) input.Module` | Crée module input |
| `CreateFilterModules` | `(configs []ModuleConfig) ([]filter.Module, error)` | Crée array filters |
| `CreateOutputModule` | `(config *ModuleConfig) (output.Module, error)` | Crée module output |
| `ParseConditionConfig` | `(config map) (ConditionConfig, error)` | Parse config condition |

**Helpers Internes**:
- `parseNestedConfig()`: Parse nested modules (condition then/else)
- `getStringFromMaps()`, `getMapFromMaps()`, `getFromMaps()`: Helpers extraction config

---

### Création Input Module

```go
func CreateInputModule(cfg *ModuleConfig) input.Module {
    if cfg == nil {
        return nil
    }

    switch cfg.Type {
    case "httpPolling":
        endpoint, _ := cfg.Config["endpoint"].(string)
        return input.NewStub(cfg.Type, endpoint)  // Note: Stub utilisé pour MVP
    default:
        return input.NewStub(cfg.Type, "")
    }
}
```

**Note**: Actuellement retourne Stub car HTTP polling pas encore implémenté complètement dans factory.

---

### Création Filter Modules

```go
func CreateFilterModules(cfgs []ModuleConfig) ([]filter.Module, error) {
    modules := make([]filter.Module, 0, len(cfgs))

    for i, cfg := range cfgs {
        switch cfg.Type {
        case "mapping":
            mappings, err := filter.ParseFieldMappings(cfg.Config["mappings"])
            if err != nil {
                return nil, fmt.Errorf("invalid mapping config: %w", err)
            }
            onError, _ := cfg.Config["onError"].(string)
            module, err := filter.NewMappingFromConfig(mappings, onError)
            if err != nil {
                return nil, err
            }
            modules = append(modules, module)

        case "condition":
            condConfig, err := ParseConditionConfig(cfg.Config)
            if err != nil {
                return nil, err
            }
            module, err := filter.NewConditionFromConfig(condConfig)
            if err != nil {
                return nil, err
            }
            modules = append(modules, module)

        default:
            modules = append(modules, filter.NewStub(cfg.Type, i))
        }
    }

    return modules, nil
}
```

---

### Création Output Module

```go
func CreateOutputModule(cfg *ModuleConfig) (output.Module, error) {
    if cfg == nil {
        return nil, nil
    }

    switch cfg.Type {
    case "httpRequest":
        module, err := output.NewHTTPRequestFromConfig(cfg)
        if err != nil {
            return nil, fmt.Errorf("creating httpRequest module: %w", err)
        }
        return module, nil

    default:
        endpoint, _ := cfg.Config["endpoint"].(string)
        method, _ := cfg.Config["method"].(string)
        return output.NewStub(cfg.Type, endpoint, method), nil
    }
}
```

---

### Parsing Condition Config

**Supporte nested modules** (then/else):

```go
func ParseConditionConfig(cfg map[string]interface{}) (filter.ConditionConfig, error) {
    condConfig := filter.ConditionConfig{}

    // Required field
    expr, ok := cfg["expression"].(string)
    if !ok || expr == "" {
        return condConfig, fmt.Errorf("required field 'expression' missing")
    }
    condConfig.Expression = expr

    // Optional fields
    if lang, ok := cfg["lang"].(string); ok {
        condConfig.Lang = lang
    }
    if onTrue, ok := cfg["onTrue"].(string); ok {
        condConfig.OnTrue = onTrue
    }
    if onFalse, ok := cfg["onFalse"].(string); ok {
        condConfig.OnFalse = onFalse
    }

    // Nested modules
    if thenCfg, ok := cfg["then"].(map[string]interface{}); ok {
        nestedModule, err := parseNestedConfig(thenCfg)
        if err != nil {
            return condConfig, fmt.Errorf("invalid 'then' config: %w", err)
        }
        condConfig.Then = nestedModule
    }

    if elseCfg, ok := cfg["else"].(map[string]interface{}); ok {
        nestedModule, err := parseNestedConfig(elseCfg)
        if err != nil {
            return condConfig, fmt.Errorf("invalid 'else' config: %w", err)
        }
        condConfig.Else = nestedModule
    }

    return condConfig, nil
}
```

**Limite profondeur**: `maxNestingDepth = 50` (prévient stack overflow)

---

### Dépendances

**Entrantes**:
- `cmd/canectors` → Create modules

**Sortantes**:
- `pkg/connector` → ModuleConfig type
- `internal/modules/input` → input.Module, constructeurs
- `internal/modules/filter` → filter.Module, constructeurs
- `internal/modules/output` → output.Module, constructeurs

---

## Package `internal/modules/input`

### Responsabilité

**Input Modules** — Fetch data depuis sources externes

**Équivalent Java**: `@Component` implementations de `DataSource` interface

---

### Ce qu'il fait

✅ Définit interface `Module`
✅ Implémente HTTP Polling (fetch via GET)
✅ Implémente Webhook (push-based)
✅ Supporte authentification, retry, pagination

---

### Ce qu'il ne fait PAS

❌ Transform data (rôle des filters)
❌ Send data (rôle output modules)

---

### Interface Principale

```go
type Module interface {
    Fetch(ctx context.Context) ([]map[string]interface{}, error)
    Close() error
}
```

**Équivalent Java**:
```java
public interface InputModule {
    List<Map<String, Object>> fetch() throws Exception;
    void close() throws Exception;
}
```

---

### Implémentations

#### 1. HTTPPolling

**Fichier**: `http_polling.go`

**Responsabilités**:
- Fetch data via HTTP GET
- Support authentication (API Key, Bearer, Basic, OAuth2)
- Support retry avec backoff
- Support pagination (page, offset, cursor)
- Extract data depuis champ nested (dataField)

**Configuration**:
```yaml
input:
  type: httpPolling
  endpoint: "https://api.example.com/users"
  method: GET
  headers:
    X-Custom-Header: "value"
  timeoutMs: 30000
  dataField: "data.items"  # Optional: extract nested field
  pagination:
    type: "page"           # or "offset" or "cursor"
    limitParam: "limit"
    limit: 100
  authentication:
    type: "bearer"
    credentials:
      token: "my-token"
```

**Méthode Fetch()**:
```go
func (h *HTTPPolling) Fetch(ctx context.Context) ([]map[string]interface{}, error) {
    // 1. Build HTTP request
    req, err := http.NewRequestWithContext(ctx, "GET", h.endpoint, nil)

    // 2. Apply authentication
    if err := h.authHandler.ApplyAuth(ctx, req); err != nil {
        return nil, err
    }

    // 3. Execute with retry
    resp, err := h.executeWithRetry(ctx, req)

    // 4. Parse response
    var data interface{}
    json.Unmarshal(body, &data)

    // 5. Extract records (dataField)
    records := extractRecords(data, h.dataField)

    // 6. Handle pagination (loop si needed)
    // ...

    return records, nil
}
```

**Implements**: `connector.RetryInfoProvider`

---

#### 2. Webhook

**Fichier**: `webhook.go`

**Responsabilités**:
- Reçoit data via webhook HTTP
- Mode push-based (pas de fetch actif)
- Utilisé avec `ExecuteWithRecords()` au lieu de `Fetch()`

**Configuration**:
```yaml
input:
  type: webhook
  path: "/webhook/data"
```

**Note**: Webhook ne implements pas vraiment `Fetch()` car mode push. Utilisé différemment dans runtime.

---

#### 3. Stub

**Fichier**: `stub.go`

**Responsabilités**:
- Placeholder pour types non implémentés
- Retourne error "not implemented" lors Fetch()

**Usage**:
```go
func NewStub(moduleType, endpoint string) *StubModule {
    return &StubModule{
        ModuleType: moduleType,
        Endpoint:   endpoint,
    }
}

func (m *StubModule) Fetch(ctx context.Context) ([]map[string]interface{}, error) {
    return nil, fmt.Errorf("input type %s not implemented", m.ModuleType)
}
```

---

### Dépendances

**Entrantes**:
- `factory` → Create instances
- `runtime.Executor` → Appelle Fetch()

**Sortantes**:
- `internal/auth` → Authentication handlers
- `internal/errhandling` → Error classification, retry
- `internal/logger` → Logging
- `pkg/connector` → RetryInfoProvider interface

---

## Package `internal/modules/filter`

### Responsabilité

**Filter Modules** — Transform/filter data

**Équivalent Java**: Stream operations type `map()`, `filter()`, `flatMap()`

---

### Ce qu'il fait

✅ Définit interface `Module`
✅ Implémente Mapping (field-to-field transformations)
✅ Implémente Condition (expression-based filtering)
✅ Supporte nested modules (condition then/else)

---

### Ce qu'il ne fait PAS

❌ Fetch data (rôle input)
❌ Send data (rôle output)

---

### Interface Principale

```go
type Module interface {
    Process(ctx context.Context, records []map[string]interface{}) ([]map[string]interface{}, error)
}
```

**Équivalent Java**:
```java
public interface FilterModule {
    List<Map<String, Object>> process(List<Map<String, Object>> records);
}
```

---

### Implémentations

#### 1. MappingModule

**Fichier**: `mapping.go`

**Responsabilités**:
- Field-to-field mappings
- Transformations (toString, trim, uppercase, replace, etc.)
- Default values si champ source manquant
- OnMissing strategies (setNull, skipField, useDefault, fail)

**Configuration**:
```yaml
filters:
  - type: mapping
    onError: skip
    mappings:
      - source: "id"
        target: "user_id"
      - source: "name"
        target: "full_name"
        transforms:
          - op: "uppercase"
          - op: "trim"
        defaultValue: "Unknown"
        onMissing: "useDefault"
```

**Méthode Process()**:
```go
func (m *MappingModule) Process(ctx context.Context, records []map[string]interface{}) ([]map[string]interface{}, error) {
    results := make([]map[string]interface{}, 0, len(records))

    for _, record := range records {
        // Apply all mappings
        newRecord := make(map[string]interface{})
        for _, mapping := range m.mappings {
            // 1. Extract source value
            value, exists := getFieldValue(record, mapping.Source)

            // 2. Handle missing
            if !exists {
                switch mapping.OnMissing {
                case "setNull":
                    newRecord[mapping.Target] = nil
                case "skipField":
                    continue
                case "useDefault":
                    newRecord[mapping.Target] = mapping.DefaultValue
                case "fail":
                    return nil, fmt.Errorf("missing field: %s", mapping.Source)
                }
                continue
            }

            // 3. Apply transforms
            transformedValue := value
            for _, transform := range mapping.Transforms {
                transformedValue = applyTransform(transformedValue, transform)
            }

            // 4. Set target
            setFieldValue(newRecord, mapping.Target, transformedValue)
        }

        results = append(results, newRecord)
    }

    return results, nil
}
```

**Transformations supportées**:
- **Type conversions**: `toString`, `toInt`, `toFloat`, `toBool`, `toArray`, `toObject`
- **String ops**: `trim`, `uppercase`, `lowercase`, `replace`, `split`, `join`
- **Date ops**: `formatDate`

---

#### 2. ConditionModule

**Fichier**: `condition.go`

**Responsabilités**:
- Evaluate expressions sur records
- Filter records based on condition (keep/skip)
- Nested modules (then/else branches)
- Langages: "simple" (via expr-lang/expr), "cel", "jsonata", "jmespath" (TODO)

**Configuration**:
```yaml
filters:
  - type: condition
    lang: simple
    expression: "status == 'active' && age >= 18"
    onTrue: continue   # keep record
    onFalse: skip      # remove record
    onError: fail
    then:              # Optional nested module si true
      type: mapping
      mappings: [...]
    else:              # Optional nested module si false
      type: mapping
      mappings: [...]
```

**Méthode Process()**:
```go
func (c *ConditionModule) Process(ctx context.Context, records []map[string]interface{}) ([]map[string]interface{}, error) {
    results := make([]map[string]interface{}, 0, len(records))

    for _, record := range records {
        // 1. Evaluate expression
        result, err := expr.Run(c.program, record)

        if err != nil {
            // Handle error selon onError
            switch c.onError {
            case "fail":
                return nil, err
            case "skip", "log":
                logger.Error("expression eval failed", err)
                continue
            }
        }

        // 2. Cast result to bool
        keep, ok := result.(bool)
        if !ok {
            return nil, fmt.Errorf("expression must return bool")
        }

        // 3. Apply behavior
        if keep {
            // onTrue behavior
            if c.onTrue == "skip" {
                continue  // Skip record
            }
            // onTrue = "continue" → keep record
            if c.thenModule != nil {
                // Process through nested module
                processed, err := c.thenModule.Process(ctx, []map[string]interface{}{record})
                if err != nil {
                    return nil, err
                }
                results = append(results, processed...)
            } else {
                results = append(results, record)
            }
        } else {
            // onFalse behavior
            if c.onFalse == "skip" {
                continue  // Skip record
            }
            // onFalse = "continue" → keep record
            if c.elseModule != nil {
                processed, err := c.elseModule.Process(ctx, []map[string]interface{}{record})
                if err != nil {
                    return nil, err
                }
                results = append(results, processed...)
            } else {
                results = append(results, record)
            }
        }
    }

    return results, nil
}
```

**Limite profondeur**: `MaxNestingDepth = 50`

**Expression examples**:
- `status == "active"`
- `age >= 18 && country == "US"`
- `price > 100.0 || discount > 0.2`
- `len(items) > 0`
- `name startsWith "Admin"`

---

#### 3. Stub

**Fichier**: `stub.go`

**Responsabilités**:
- Placeholder pour types non implémentés
- Retourne records inchangés (passthrough)

---

### Dépendances

**Entrantes**:
- `factory` → Create instances
- `runtime.Executor` → Appelle Process()

**Sortantes**:
- `internal/logger` → Logging
- `github.com/expr-lang/expr` → Expression evaluation

---

## Package `internal/modules/output`

### Responsabilité

**Output Modules** — Send data vers destinations externes

**Équivalent Java**: `@Component` implementations de `DataSink` interface

---

### Ce qu'il fait

✅ Définit interfaces `Module` et `PreviewableModule`
✅ Implémente HTTP Request (send via POST/PUT/PATCH)
✅ Supporte authentification, retry, error handling
✅ Supporte dry-run preview

---

### Ce qu'il ne fait PAS

❌ Fetch data (rôle input)
❌ Transform data (rôle filters)

---

### Interfaces Principales

```go
type Module interface {
    Send(ctx context.Context, records []map[string]interface{}) (int, error)
    Close() error
}

type PreviewableModule interface {
    Module
    PreviewRequest(records []map[string]interface{}, opts PreviewOptions) ([]RequestPreview, error)
}
```

**Équivalent Java**:
```java
public interface OutputModule {
    int send(List<Map<String, Object>> records) throws Exception;
    void close() throws Exception;
}

public interface PreviewableOutputModule extends OutputModule {
    List<RequestPreview> previewRequest(List<Map<String, Object>> records, PreviewOptions opts);
}
```

---

### Implémentations

#### 1. HTTPRequestModule

**Fichier**: `http_request.go`

**Responsabilités**:
- Send data via HTTP POST/PUT/PATCH
- Support authentication (API Key, Bearer, Basic, OAuth2)
- Support retry avec backoff
- Support batch mode (tous records en 1 requête) ou single mode (1 req/record)
- Support dynamic endpoints (path params depuis records)
- Support dry-run preview

**Configuration**:
```yaml
output:
  type: httpRequest
  endpoint: "https://api.example.com/data"
  method: POST
  headers:
    X-Custom: "value"
  timeoutMs: 30000
  onError: skip
  request:
    bodyFrom: records    # "records" (batch) or "record" (single)
    pathParams:
      userId: "user.id"  # Extract from record
    queryParams:
      source: "pipeline"
    headersFromRecord:
      X-Trace-Id: "trace_id"
  success:
    statusCodes: [200, 201, 202]
  authentication:
    type: oauth2_client_credentials
    credentials:
      tokenUrl: "https://auth.example.com/token"
      clientId: "${CLIENT_ID}"
      clientSecret: "${CLIENT_SECRET}"
```

**Méthode Send()** (Batch Mode):
```go
func (h *HTTPRequestModule) Send(ctx context.Context, records []map[string]interface{}) (int, error) {
    // Batch mode: tous records en 1 requête
    if h.request.BodyFrom == "records" {
        // 1. Marshal records to JSON array
        body, err := json.Marshal(records)

        // 2. Build HTTP request
        req, err := http.NewRequestWithContext(ctx, h.method, h.endpoint, bytes.NewReader(body))

        // 3. Set headers
        req.Header.Set("Content-Type", "application/json")
        for k, v := range h.headers {
            req.Header.Set(k, v)
        }

        // 4. Apply authentication
        if err := h.authHandler.ApplyAuth(ctx, req); err != nil {
            return 0, err
        }

        // 5. Execute with retry
        err = h.executeWithRetry(ctx, req)
        if err != nil {
            return 0, err
        }

        return len(records), nil
    }

    // Single mode: 1 requête per record
    sent := 0
    for _, record := range records {
        // Marshal single record
        body, err := json.Marshal(record)

        // Resolve endpoint (path params from record)
        endpoint := h.resolveEndpointForRecord(record)

        // Execute
        err = h.doRequestWithHeaders(ctx, endpoint, body, nil)
        if err != nil {
            if h.onError == "fail" {
                return sent, err
            }
            // skip or log: continue
            continue
        }
        sent++
    }

    return sent, nil
}
```

**Méthode PreviewRequest()**:
```go
func (h *HTTPRequestModule) PreviewRequest(records []map[string]interface{}, opts PreviewOptions) ([]RequestPreview, error) {
    previews := []RequestPreview{}

    if h.request.BodyFrom == "records" {
        // Batch mode: 1 preview
        bodyPreview, _ := json.MarshalIndent(records, "", "  ")
        headers := h.buildPreviewHeaders(nil, opts)  // Mask auth

        previews = append(previews, RequestPreview{
            Endpoint:    h.endpoint,
            Method:      h.method,
            Headers:     headers,
            BodyPreview: string(bodyPreview),
            RecordCount: len(records),
        })
    } else {
        // Single mode: N previews
        for _, record := range records {
            endpoint := h.resolveEndpointForRecord(record)
            bodyPreview, _ := json.MarshalIndent(record, "", "  ")
            headers := h.buildPreviewHeaders(extractHeadersFromRecord(record), opts)

            previews = append(previews, RequestPreview{
                Endpoint:    endpoint,
                Method:      h.method,
                Headers:     headers,
                BodyPreview: string(bodyPreview),
                RecordCount: 1,
            })
        }
    }

    return previews, nil
}
```

**Implements**: `connector.RetryInfoProvider`, `PreviewableModule`

---

#### 2. Stub

**Fichier**: `stub.go`

**Responsabilités**:
- Placeholder pour types non implémentés
- Retourne count = len(records) sans envoyer

**Usage**:
```go
func NewStub(moduleType, endpoint, method string) *StubModule {
    return &StubModule{
        ModuleType: moduleType,
        Endpoint:   endpoint,
        Method:     method,
    }
}

func (m *StubModule) Send(ctx context.Context, records []map[string]interface{}) (int, error) {
    logger.Info("Output stub called",
        slog.String("type", m.ModuleType),
        slog.Int("records", len(records)))
    return len(records), nil  // Pretend all sent
}
```

---

### Dépendances

**Entrantes**:
- `factory` → Create instances
- `runtime.Executor` → Appelle Send()

**Sortantes**:
- `internal/auth` → Authentication handlers
- `internal/errhandling` → Error classification, retry
- `internal/logger` → Logging
- `pkg/connector` → RetryInfoProvider, PreviewableModule interfaces

---

## Package `internal/auth`

### Responsabilité

**Gestion Authentification HTTP** — Applique authentification sur requêtes HTTP

**Équivalent Java**: Intercepteur HTTP type Spring `ClientHttpRequestInterceptor`

---

### Ce qu'il fait

✅ Définit interface `Handler` (abstraction auth)
✅ Implémente API Key (header ou query param)
✅ Implémente Bearer token
✅ Implémente HTTP Basic auth
✅ Implémente OAuth2 client credentials flow
✅ Cache tokens OAuth2 avec expiry
✅ Refresh automatique tokens
✅ Thread-safe token caching (sync.RWMutex)
✅ Invalidate token on 401 response

---

### Ce qu'il ne fait PAS

❌ Execute HTTP requests (applique juste auth headers)
❌ Retry logic (rôle modules)
❌ Store credentials sur disque (cache memory only)

---

### Interface Principale

```go
type Handler interface {
    ApplyAuth(ctx context.Context, req *http.Request) error
    Type() string
}

type OAuth2Invalidator interface {
    InvalidateToken()
}
```

**Équivalent Java**:
```java
public interface AuthHandler {
    void applyAuth(HttpRequest req) throws Exception;
    String getType();
}

public interface OAuth2Invalidator {
    void invalidateToken();
}
```

---

### Types Exposés

| Type | Description | Méthodes |
|------|-------------|----------|
| `Handler` | Interface auth générique | `ApplyAuth()`, `Type()` |
| `OAuth2Invalidator` | Interface invalidation token | `InvalidateToken()` |

**Implémentations** (types internes non-exportés):
- `apiKeyHandler`: API key auth
- `bearerHandler`: Bearer token auth
- `basicHandler`: HTTP Basic auth
- `oauth2Handler`: OAuth2 client credentials

---

### Fonctions Publiques

| Fonction | Signature | Description |
|----------|-----------|-------------|
| `NewHandler` | `(config *AuthConfig, httpClient *http.Client) (Handler, error)` | Factory créant handler depuis config |

---

### Implémentation API Key

**Configuration**:
```yaml
authentication:
  type: api-key
  credentials:
    key: "${API_KEY}"
    location: header          # ou "query"
    headerName: X-API-Key     # custom header name
    paramName: api_key        # custom query param name
```

**Comportement**:
- Location `header`: Ajoute header `X-API-Key: <key>`
- Location `query`: Ajoute query param `?api_key=<key>`

---

### Implémentation Bearer

**Configuration**:
```yaml
authentication:
  type: bearer
  credentials:
    token: "${BEARER_TOKEN}"
```

**Comportement**:
- Ajoute header: `Authorization: Bearer <token>`

---

### Implémentation Basic

**Configuration**:
```yaml
authentication:
  type: basic
  credentials:
    username: "${USERNAME}"
    password: "${PASSWORD}"
```

**Comportement**:
- Ajoute header: `Authorization: Basic <base64(username:password)>`

---

### Implémentation OAuth2

**Fichier**: `oauth2.go`

**Configuration**:
```yaml
authentication:
  type: oauth2
  credentials:
    tokenUrl: "https://auth.example.com/token"
    clientId: "${CLIENT_ID}"
    clientSecret: "${CLIENT_SECRET}"
    scopes: "read,write"  # Optional, comma-separated
```

**Flow OAuth2 Client Credentials**:
```go
func (h *oauth2Handler) ApplyAuth(ctx context.Context, req *http.Request) error {
    // 1. Get valid token (use cached if not expired)
    token, err := h.getValidToken(ctx)

    // 2. Apply to request
    req.Header.Set("Authorization", "Bearer "+token)
    return nil
}

func (h *oauth2Handler) getValidToken(ctx context.Context) (string, error) {
    // Read lock: check cached token
    h.mu.RLock()
    if h.cachedToken != "" && time.Now().Before(h.tokenExpiry) {
        token := h.cachedToken
        h.mu.RUnlock()
        return token, nil
    }
    h.mu.RUnlock()

    // Write lock: refresh token
    h.mu.Lock()
    defer h.mu.Unlock()

    // Double-check (another goroutine may have refreshed)
    if h.cachedToken != "" && time.Now().Before(h.tokenExpiry) {
        return h.cachedToken, nil
    }

    // Fetch new token
    token, expiry, err := h.fetchToken(ctx)
    if err != nil {
        return "", err
    }

    h.cachedToken = token
    h.tokenExpiry = expiry
    return token, nil
}
```

**Token Expiry Buffer**: 60 secondes avant expiry réelle (refresh proactif)

**Thread-Safety**:
- `sync.RWMutex` protège `cachedToken` et `tokenExpiry`
- Read lock pour lecture cache (fast path, multiple goroutines)
- Write lock pour refresh (slow path, exclusive)
- Double-check pattern évite race conditions

**Invalidation Token**:
```go
func (h *oauth2Handler) InvalidateToken() {
    h.mu.Lock()
    defer h.mu.Unlock()
    h.cachedToken = ""
    h.tokenExpiry = time.Time{}
}
```

**Utilisé par modules** quand 401 Unauthorized reçu:
```go
// Dans modules/output/http_request.go
if resp.StatusCode == 401 {
    if invalidator, ok := h.authHandler.(auth.OAuth2Invalidator); ok {
        invalidator.InvalidateToken()
        // Retry request → new token fetched
    }
}
```

**Sécurité**:
- Credentials JAMAIS loggés
- Token response body JAMAIS loggé
- Tokens stockés en mémoire uniquement (pas de persistence disque)
- Error messages sanitized (pas de leak credentials)

---

### Erreurs

| Erreur | Description |
|--------|-------------|
| `ErrNilConfig` | Config auth nil |
| `ErrUnknownType` | Type auth inconnu |
| `ErrMissingAPIKey` | API key manquante |
| `ErrMissingBearerToken` | Bearer token manquant |
| `ErrMissingBasicAuth` | Username/password manquants |
| `ErrMissingOAuth2Creds` | OAuth2 creds manquantes |
| `ErrOAuth2TokenFailed` | Échec obtention token |

---

### Dépendances

**Entrantes**:
- `modules/input` → Applique auth sur HTTP GET
- `modules/output` → Applique auth sur HTTP POST/PUT/PATCH

**Sortantes**:
- `pkg/connector` → Type AuthConfig
- `internal/logger` → Logging
- `net/http` → HTTP requests

---

## Package `internal/errhandling`

### Responsabilité

**Classification Erreurs + Retry Logic** — Classifie erreurs et détermine retry stratégie

**Équivalent Java**: Exception hierarchy + `@Retryable` annotation (Spring Retry)

---

### Ce qu'il fait

✅ Définit catégories erreurs (Network, Auth, Validation, etc.)
✅ Classifie HTTP status codes → catégories
✅ Classifie network errors (timeout, DNS, connection refused)
✅ Détermine si erreur retryable ou fatal
✅ Calcule délais retry avec exponential backoff
✅ Execute fonctions avec retry automatique
✅ Track retry info (attempts, delays, errors)

---

### Ce qu'il ne fait PAS

❌ Execute HTTP requests (rôle modules)
❌ Log errors (rôle logger)
❌ Parse config (rôle config package)

---

### Types Exposés

**Error Categories**:

| Catégorie | Description | Retryable |
|-----------|-------------|-----------|
| `CategoryNetwork` | Network errors (timeout, DNS, connection refused) | ✅ Oui |
| `CategoryAuthentication` | Auth errors (401, 403) | ❌ Non (fatal) |
| `CategoryValidation` | Validation errors (400, 422) | ❌ Non (fatal) |
| `CategoryRateLimit` | Rate limit (429) | ✅ Oui |
| `CategoryServer` | Server errors (5xx) | ✅ Oui |
| `CategoryNotFound` | Not found (404) | ❌ Non (fatal) |
| `CategoryUnknown` | Unclassified | ❌ Non |

**Configuration Types**:

| Type | Description | Champs Principaux |
|------|-------------|-------------------|
| `RetryConfig` | Config retry | MaxAttempts, DelayMs, BackoffMultiplier, MaxDelayMs, RetryableStatusCodes |
| `ErrorHandlingConfig` | Config error handling | OnError, TimeoutMs, Retry |
| `ClassifiedError` | Erreur classifiée | Category, Retryable, StatusCode, Message, OriginalErr |
| `RetryInfo` | Info tentatives retry | TotalAttempts, RetryCount, Delays, Errors |
| `OnErrorStrategy` | Stratégie erreur | "fail", "skip", "log" |

---

### Fonctions Publiques

**Classification**:

| Fonction | Signature | Description |
|----------|-----------|-------------|
| `ClassifyHTTPStatus` | `(statusCode int, message string) *ClassifiedError` | Classifie HTTP status code |
| `ClassifyNetworkError` | `(err error) *ClassifiedError` | Classifie network error |
| `ClassifyError` | `(err error) *ClassifiedError` | Classifie erreur générique |
| `IsRetryable` | `(err error) bool` | Détermine si retryable |
| `IsFatal` | `(err error) bool` | Détermine si fatal |
| `GetErrorCategory` | `(err error) ErrorCategory` | Retourne catégorie |

**Retry Config**:

| Fonction | Signature | Description |
|----------|-----------|-------------|
| `DefaultRetryConfig` | `() RetryConfig` | Config retry par défaut |
| `ParseRetryConfig` | `(map[string]interface{}) RetryConfig` | Parse depuis map |
| `ResolveRetryConfig` | `(module, defaults *RetryConfig) RetryConfig` | Résout priorités |
| `(c RetryConfig) Validate` | `() error` | Valide config |
| `(c RetryConfig) CalculateDelay` | `(attempt int) time.Duration` | Calcule délai retry |
| `(c RetryConfig) ShouldRetry` | `(attempt int, err error) bool` | Détermine si retry |

**Retry Executor**:

| Fonction | Signature | Description |
|----------|-----------|-------------|
| `NewRetryExecutor` | `(config RetryConfig) *RetryExecutor` | Créer executor |
| `(e *RetryExecutor) Execute` | `(ctx, fn RetryFunc) (interface{}, error)` | Execute avec retry |
| `(e *RetryExecutor) GetRetryInfo` | `() RetryInfo` | Retourne info tentatives |

---

### Classification HTTP Status

**Règles**:
```go
401, 403           → CategoryAuthentication (fatal)
400, 422           → CategoryValidation (fatal)
404                → CategoryNotFound (fatal)
429                → CategoryRateLimit (retryable)
500, 502, 503, 504 → CategoryServer (retryable)
Other 5xx          → CategoryServer (retryable)
Other 4xx          → CategoryValidation (fatal)
```

**Exemple Usage**:
```go
classified := errhandling.ClassifyHTTPStatus(503, "Service Unavailable")
// classified.Category = CategoryServer
// classified.Retryable = true
// classified.StatusCode = 503
```

---

### Classification Network Errors

**Détection**:
```go
// Timeout
errors.Is(err, context.DeadlineExceeded) → CategoryNetwork (retryable)

// Context canceled (user-initiated)
errors.Is(err, context.Canceled) → CategoryNetwork (non-retryable)

// Network operation error
errors.As(err, &net.OpError{}) → CategoryNetwork (retryable)

// DNS error
errors.As(err, &net.DNSError{}) → CategoryNetwork (retryable)

// URL error
errors.As(err, &url.Error{}) → CategoryNetwork (retryable)
```

**Pattern matching** avec `errors.As()` (Go 1.13+ error wrapping)

---

### Retry Config

**Defaults**:
```go
MaxAttempts:          3
DelayMs:              1000        // 1 second
BackoffMultiplier:    2.0         // Double delay each retry
MaxDelayMs:           30000       // 30 seconds max
RetryableStatusCodes: [429, 500, 502, 503, 504]
```

**Exponential Backoff Formula**:
```
delay = min(delayMs * (backoffMultiplier ^ attempt), maxDelayMs)

Exemple (DelayMs=1000, BackoffMultiplier=2.0):
- Attempt 0: 1000ms * 2^0 = 1000ms  (1 second)
- Attempt 1: 1000ms * 2^1 = 2000ms  (2 seconds)
- Attempt 2: 1000ms * 2^2 = 4000ms  (4 seconds)
- Attempt 3: 1000ms * 2^3 = 8000ms  (8 seconds)
```

**Résolution Priorités**:
```
Module retry config > Defaults retry config > Default values
```

---

### Retry Executor

**Exemple Usage**:
```go
config := errhandling.DefaultRetryConfig()
executor := errhandling.NewRetryExecutor(config)

result, err := executor.Execute(ctx, func(ctx context.Context) (interface{}, error) {
    // Function to retry
    resp, err := http.Get("https://api.example.com/data")
    if err != nil {
        return nil, err
    }
    if resp.StatusCode >= 500 {
        return nil, errhandling.ClassifyHTTPStatus(resp.StatusCode, "Server error")
    }
    return resp, nil
})

info := executor.GetRetryInfo()
// info.TotalAttempts = 3
// info.RetryCount = 2
// info.Delays = [1s, 2s]
// info.Errors = [err1, err2]
```

**Comportement**:
- Initial attempt + retries = MaxAttempts + 1 total attempts
- Fatal errors → pas de retry (authentication, validation)
- Transient errors → retry avec backoff
- Context cancellation → arrêt immédiat
- Collect info sur tentatives pour logging/metrics

---

### OnError Strategies

| Stratégie | Comportement | Usage |
|-----------|--------------|-------|
| `fail` | Arrête execution, retourne erreur | Strict mode (default) |
| `skip` | Skip record/module, continue | Tolérance partielle |
| `log` | Log erreur, continue | Best-effort mode |

**Exemple Config**:
```yaml
output:
  type: httpRequest
  onError: skip  # Continue même si certains records échouent
  retry:
    maxAttempts: 3
    delayMs: 1000
```

---

### Dépendances

**Entrantes**:
- Tous modules (input, filter, output) → Classifie erreurs, retry
- `internal/config` → Parse retry config

**Sortantes**:
- Standard library: `context`, `net`, `net/url`, `errors`

---

## Package `internal/logger`

### Responsabilité

**Logging Structuré** — Logging centralisé avec structured logging (slog)

**Équivalent Java**: SLF4J + Logback avec MDC (Mapped Diagnostic Context)

---

### Ce qu'il fait

✅ Wrapper autour de `log/slog` (Go 1.21+)
✅ Logging structuré (JSON par défaut)
✅ Format human-readable optionnel (console)
✅ Helpers execution context (pipeline, stage, module)
✅ Helpers métriques performance
✅ Log rotation simple (10MB max)
✅ Dual output (console + fichier)
✅ Levels: DEBUG, INFO, WARN, ERROR

---

### Ce qu'il ne fait PAS

❌ Parse logs (rôle outils externes: jq, grep, etc.)
❌ Envoie logs vers services externes (Datadog, Splunk, etc.)
❌ Advanced log rotation (utiliser logrotate en prod)

---

### Types Exposés

**Context Types**:

| Type | Description | Champs Principaux |
|------|-------------|-------------------|
| `ExecutionContext` | Contexte execution pipeline | PipelineID, PipelineName, Stage, ModuleType, ModuleName, DryRun, FilterIndex |
| `ExecutionError` | Erreur execution structurée | Code, Message, Details |
| `ErrorContext` | Contexte erreur détaillé | PipelineID, Stage, ErrorCode, ErrorMessage, Err, RecordIndex, Endpoint, HTTPStatus, Duration, Extra |
| `ExecutionMetrics` | Métriques performance | TotalDuration, InputDuration, FilterDuration, OutputDuration, RecordsProcessed, RecordsFailed, RecordsPerSecond, AvgRecordTime |

**Format Types**:

| Type | Description |
|------|-------------|
| `OutputFormat` | Format output (JSON ou Human) |
| `HumanHandler` | Handler human-readable avec colors |

---

### Fonctions Publiques

**Basic Logging**:

| Fonction | Signature | Description |
|----------|-----------|-------------|
| `Info` | `(msg string, args ...any)` | Log info |
| `Debug` | `(msg string, args ...any)` | Log debug |
| `Warn` | `(msg string, args ...any)` | Log warning |
| `Error` | `(msg string, args ...any)` | Log error |
| `SetLevel` | `(level slog.Level)` | Configure level |
| `SetFormat` | `(format OutputFormat)` | Configure format (JSON/Human) |

**Execution Context Helpers**:

| Fonction | Signature | Description |
|----------|-----------|-------------|
| `WithExecution` | `(ctx ExecutionContext) *slog.Logger` | Logger avec contexte attaché |
| `LogExecutionStart` | `(ctx ExecutionContext)` | Log début execution |
| `LogExecutionEnd` | `(ctx, status, records, duration)` | Log fin execution |
| `LogStageStart` | `(ctx ExecutionContext)` | Log début stage |
| `LogStageEnd` | `(ctx, recordCount, duration, err)` | Log fin stage |
| `LogMetrics` | `(ctx ExecutionContext, metrics ExecutionMetrics)` | Log métriques |
| `LogError` | `(message string, errCtx ErrorContext)` | Log erreur avec contexte complet |

**File Output**:

| Fonction | Signature | Description |
|----------|-----------|-------------|
| `SetLogFile` | `(path, level, consoleFormat)` | Output vers fichier + console |
| `CloseLogFile` | `()` | Ferme fichier log |

---

### Format JSON (Default)

**Exemple Output**:
```json
{
  "time": "2026-01-23T14:30:00.123Z",
  "level": "INFO",
  "msg": "execution started",
  "pipeline_id": "users-sync",
  "pipeline_name": "Users Sync Pipeline",
  "stage": "input",
  "module_type": "httpPolling"
}

{
  "time": "2026-01-23T14:30:01.456Z",
  "level": "INFO",
  "msg": "stage completed",
  "pipeline_id": "users-sync",
  "stage": "input",
  "record_count": 150,
  "duration": 1333000000
}

{
  "time": "2026-01-23T14:30:05.789Z",
  "level": "ERROR",
  "msg": "HTTP request failed",
  "pipeline_id": "users-sync",
  "stage": "output",
  "error_code": "HTTP_ERROR",
  "error": "503 Service Unavailable",
  "http_status": 503,
  "endpoint": "https://api.example.com/users"
}
```

**Avantages JSON**:
- Machine-readable (parsing facile)
- Structured queries (jq, grep, awk)
- Log aggregation (ELK, Datadog, Splunk)

---

### Format Human-Readable

**Activation**:
```go
logger.SetFormat(logger.FormatHuman)
```

**Exemple Output**:
```
14:30:00 ℹ execution started pipeline_id=users-sync stage=input
14:30:01 ✓ stage completed pipeline_id=users-sync record_count=150 duration=1.33s
14:30:05 ✗ HTTP request failed pipeline_id=users-sync http_status=503
```

**Features**:
- ANSI colors (auto-détecté si terminal)
- Symboles: ✓ (success), ✗ (error), ⚠ (warn), ℹ (info)
- Timestamp human-readable
- Truncation (5 attrs max inline, "+N more")
- Duration formatting (µs, ms, s, m)

---

### Execution Context Helpers

**Exemple Usage**:
```go
// Runtime executor
ctx := logger.ExecutionContext{
    PipelineID:   pipeline.ID,
    PipelineName: pipeline.Name,
    Stage:        "input",
    ModuleType:   "httpPolling",
}

logger.LogExecutionStart(ctx)

// Execute input stage
startTime := time.Now()
records, err := inputModule.Fetch(ctx)
duration := time.Since(startTime)

if err != nil {
    execErr := &logger.ExecutionError{
        Code:    "INPUT_FAILED",
        Message: err.Error(),
    }
    logger.LogStageEnd(ctx, 0, duration, execErr)
} else {
    logger.LogStageEnd(ctx, len(records), duration, nil)
}

// Log metrics
metrics := logger.ExecutionMetrics{
    TotalDuration:    time.Since(execStartTime),
    InputDuration:    inputDuration,
    FilterDuration:   filterDuration,
    OutputDuration:   outputDuration,
    RecordsProcessed: 150,
    RecordsFailed:    0,
    RecordsPerSecond: 112.5,
    AvgRecordTime:    8 * time.Millisecond,
}
logger.LogMetrics(ctx, metrics)
```

**Structured Output** (JSON):
```json
{
  "time": "...",
  "level": "INFO",
  "msg": "execution metrics",
  "pipeline_id": "users-sync",
  "total_duration": 1333000000,
  "input_duration": 500000000,
  "filter_duration": 333000000,
  "output_duration": 500000000,
  "records_processed": 150,
  "records_per_second": 112.5,
  "avg_record_time": 8000000
}
```

---

### Error Context Helper

**Exemple Usage**:
```go
// Module output HTTP request failed
errCtx := logger.ErrorContext{
    PipelineID:   pipeline.ID,
    PipelineName: pipeline.Name,
    Stage:        "output",
    ModuleType:   "httpRequest",
    ErrorCode:    "HTTP_ERROR",
    ErrorMessage: "503 Service Unavailable",
    Err:          originalErr,  // Original error for chain
    RecordIndex:  42,
    RecordCount:  150,
    Endpoint:     "https://api.example.com/users",
    HTTPStatus:   503,
    Duration:     2 * time.Second,
    Extra: map[string]interface{}{
        "retry_attempt": 2,
        "method":        "POST",
    },
}

logger.LogError("HTTP request failed", errCtx)
```

**Output** (JSON):
```json
{
  "time": "...",
  "level": "ERROR",
  "msg": "HTTP request failed",
  "pipeline_id": "users-sync",
  "pipeline_name": "Users Sync Pipeline",
  "stage": "output",
  "module_type": "httpRequest",
  "error_code": "HTTP_ERROR",
  "error": "503 Service Unavailable",
  "error_type": "*errhandling.ClassifiedError",
  "error_chain": "503 Service Unavailable -> HTTP error",
  "record_index": 42,
  "record_count": 150,
  "endpoint": "https://api.example.com/users",
  "http_status": 503,
  "duration": 2000000000,
  "retry_attempt": 2,
  "method": "POST"
}
```

**Avantages**:
- **Actionable errors** (NFR32): Tous les contextes nécessaires pour debug
- Error chain inclus (`error_chain`) pour tracer root cause
- Fields structurés (parseable, filterable)

---

### Log File Output

**Configuration**:
```go
// Dual output: console (human) + file (JSON)
err := logger.SetLogFile(
    "/var/log/canectors/pipeline.log",
    slog.LevelInfo,
    logger.FormatHuman,  // Console format
)
// File toujours en JSON (machine-readable)
```

**Log Rotation**:
- Limite: 10MB par fichier
- Rotation: Rename avec timestamp `pipeline.log.20260123-143000`
- Simple rotation (production: utiliser logrotate externe)

**Fermeture**:
```go
defer logger.CloseLogFile()
```

---

### Structured Logging Best Practices

**Snake_case fields** (convention slog):
```go
logger.Info("execution started",
    slog.String("pipeline_id", "users-sync"),     // ✓
    slog.Int("record_count", 150),                // ✓
    slog.Duration("duration", 1*time.Second),     // ✓
)
```

**Consistent field names**:
- `pipeline_id`, `pipeline_name`
- `stage`, `module_type`, `module_name`
- `record_count`, `record_index`
- `error_code`, `error`, `error_type`, `error_chain`
- `http_status`, `endpoint`, `method`
- `duration`, `total_duration`, `avg_record_time`
- `records_processed`, `records_failed`, `records_per_second`

---

### Dépendances

**Entrantes**:
- Tous packages internes → Logging

**Sortantes**:
- `log/slog` → Structured logging (Go 1.21+)
- `os` → File I/O

---

## Package `internal/scheduler`

### Responsabilité

**Planificateur CRON** — Execute pipelines sur schedule CRON

**Équivalent Java**: Spring `@Scheduled` + Quartz Scheduler

---

### Ce qu'il fait

✅ Parse expressions CRON (5-field ou 6-field)
✅ Enregistre pipelines avec schedules
✅ Execute pipelines automatiquement selon CRON
✅ Gère overlap detection (skip si execution en cours)
✅ Graceful shutdown (attend executions en cours)
✅ Thread-safe (sync.RWMutex)
✅ Dynamic registration/unregistration
✅ Context propagation (cancellation)

---

### Ce qu'il ne fait PAS

❌ Execute pipeline logic (délègue à Executor)
❌ Parse config (reçoit Pipeline struct)
❌ Store schedules sur disque

---

### Types Exposés

| Type | Description | Champs Principaux |
|------|-------------|-------------------|
| `Scheduler` | Planificateur CRON | cron, pipelines, executor, started, ctx, wg |
| `Executor` | Interface execution pipeline | `Execute(pipeline) (result, error)` |

**Erreurs**:

| Erreur | Description |
|--------|-------------|
| `ErrNilPipeline` | Pipeline nil |
| `ErrPipelineDisabled` | Pipeline disabled |
| `ErrEmptySchedule` | Schedule vide |
| `ErrInvalidCronExpression` | Expression CRON invalide |
| `ErrSchedulerAlreadyRunning` | Scheduler déjà started |
| `ErrPipelineNotFound` | Pipeline non enregistré |
| `ErrPipelineRunning` | Pipeline en cours execution |
| `ErrSchedulerNotStarted` | Scheduler pas started |

---

### Fonctions Publiques

**Lifecycle**:

| Fonction | Signature | Description |
|----------|-----------|-------------|
| `New` | `() *Scheduler` | Créer scheduler (stub executor) |
| `NewWithExecutor` | `(executor Executor) *Scheduler` | Créer avec executor |
| `Start` | `(ctx context.Context) error` | Démarre scheduler |
| `Stop` | `(ctx context.Context) error` | Arrête gracefully |

**Pipeline Management**:

| Fonction | Signature | Description |
|----------|-----------|-------------|
| `Register` | `(pipeline *Pipeline) error` | Enregistre pipeline |
| `Unregister` | `(pipelineID string) error` | Désenregistre pipeline |
| `HasPipeline` | `(pipelineID string) bool` | Vérifie si enregistré |
| `IsRunning` | `(pipelineID string) bool` | Vérifie si en execution |
| `GetNextRun` | `(pipelineID string) (time.Time, error)` | Prochain run time |

**Validation**:

| Fonction | Signature | Description |
|----------|-----------|-------------|
| `ValidateCronExpression` | `(expr string) error` | Valide expression CRON |

---

### CRON Expression Format

**Standard 5-field** (minute precision):
```
┌─────── minute (0 - 59)
│ ┌───── hour (0 - 23)
│ │ ┌─── day of month (1 - 31)
│ │ │ ┌─ month (1 - 12)
│ │ │ │ ┌ day of week (0 - 6) (Sunday=0)
│ │ │ │ │
* * * * *
```

**Extended 6-field** (second precision):
```
┌──────── second (0 - 59) [OPTIONAL]
│ ┌────── minute (0 - 59)
│ │ ┌──── hour (0 - 23)
│ │ │ ┌── day of month (1 - 31)
│ │ │ │ ┌ month (1 - 12)
│ │ │ │ │ ┌ day of week (0 - 6)
│ │ │ │ │ │
* * * * * *
```

**Exemples**:
```
"0 */5 * * * *"      → Every 5 minutes (6-field)
"*/30 * * * * *"     → Every 30 seconds (6-field)
"0 0 * * *"          → Every day at midnight (5-field)
"0 9 * * 1-5"        → Every weekday at 9 AM (5-field)
"0 */2 * * *"        → Every 2 hours (5-field)
"@hourly"            → Descriptor: every hour
"@daily"             → Descriptor: every day at midnight
```

**Descriptors supportés**: `@yearly`, `@monthly`, `@weekly`, `@daily`, `@hourly`

---

### Workflow Complet

**1. Création + Enregistrement**:
```go
// Create scheduler avec executor
executor := runtime.NewExecutor(false)
scheduler := scheduler.NewWithExecutor(&PipelineExecutorAdapter{executor})

// Register pipeline avec schedule
pipeline := &connector.Pipeline{
    ID:       "users-sync",
    Name:     "Users Sync",
    Schedule: "0 */5 * * * *",  // Every 5 minutes
    Enabled:  true,
    // ... input, filters, output
}

err := scheduler.Register(pipeline)
// Valide CRON expression
// Crée CRON job
// Store dans scheduler.pipelines map
```

**2. Start Scheduler**:
```go
ctx := context.Background()
err := scheduler.Start(ctx)
// Start CRON scheduler
// CRON commence à trigger jobs selon schedule
```

**3. Execution Automatique** (CRON trigger):
```go
// CRON appelle executePipeline() automatiquement
func (s *Scheduler) executePipeline(reg *registeredPipeline) {
    // 1. Overlap detection
    if !s.tryStartExecution(reg) {
        logger.Warn("skipping overlapping execution")
        return
    }
    defer s.finishExecution(reg)

    // 2. Execute
    result, err := s.executor.Execute(reg.pipeline)

    // 3. Log result
    logger.Info("scheduled pipeline execution completed",
        slog.String("status", result.Status),
        slog.Int("records_processed", result.RecordsProcessed),
    )
}
```

**4. Graceful Shutdown**:
```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

err := scheduler.Stop(ctx)
// 1. Stop CRON (pas de nouveaux triggers)
// 2. Set started=false (bloque nouvelles executions)
// 3. Cancel context (signal executions en cours)
// 4. Wait for WaitGroup (attend executions)
// 5. Clear pipelines
```

---

### Overlap Detection

**Problème**: Pipeline schedule = "*/1 * * * *" (every minute), mais execution prend 2 minutes

**Solution**: Skip si pipeline déjà en cours
```go
type registeredPipeline struct {
    pipeline *connector.Pipeline
    entryID  cron.EntryID
    running  bool       // Protected by mu
    mu       sync.Mutex
}

func (s *Scheduler) tryStartExecution(reg *registeredPipeline) bool {
    reg.mu.Lock()
    defer reg.mu.Unlock()

    if reg.running {
        // Skip overlapping execution
        return false
    }
    reg.running = true
    return true
}

func (s *Scheduler) finishExecution(reg *registeredPipeline) {
    reg.mu.Lock()
    reg.running = false
    reg.mu.Unlock()
}
```

**Timeline**:
```
Time: 00:00 → Trigger → Start execution (running=true)
Time: 00:01 → Trigger → Skip (running=true)
Time: 00:02 → Finish → Set running=false
Time: 00:02 → Trigger → Start execution (running=true)
```

---

### Thread-Safety

**Multiple Locks**:
1. **Scheduler-level lock** (`s.mu`): Protège `pipelines` map, `started` flag
2. **Pipeline-level lock** (`reg.mu`): Protège `running` flag par pipeline

**Pourquoi 2 locks?**
- Scheduler lock → Coarse-grained (register, unregister, start, stop)
- Pipeline lock → Fine-grained (overlap detection per pipeline)

**Permet**:
- Multiple pipelines exécutent en parallèle
- Pas de contention entre pipelines différents
- Lock granularity optimale

**WaitGroup**:
```go
s.wg.Add(1)              // Avant execution
defer s.wg.Done()        // Après execution

// Stop() waits for all executions
s.wg.Wait()
```

---

### Dynamic Update Pipeline

**Register pipeline déjà enregistré** = Update:
```go
func (s *Scheduler) Register(pipeline *connector.Pipeline) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    if existing, ok := s.pipelines[pipeline.ID]; ok {
        // Check if pipeline running → ERROR
        existing.mu.Lock()
        isRunning := existing.running
        existing.mu.Unlock()

        if isRunning {
            return ErrPipelineRunning
        }

        // Remove old CRON job
        s.cron.Remove(existing.entryID)
        delete(s.pipelines, pipeline.ID)
    }

    // Add new CRON job with updated schedule
    entryID, err := s.cron.AddFunc(pipeline.Schedule, ...)
    s.pipelines[pipeline.ID] = &registeredPipeline{...}

    return nil
}
```

**Protections**:
- Empêche update si pipeline en cours execution
- Evite race condition sur references pipeline

---

### Dépendances

**Entrantes**:
- `cmd/canectors` → Start scheduler pour pipelines scheduled

**Sortantes**:
- `pkg/connector` → Pipeline type
- `internal/runtime` → Executor interface
- `internal/logger` → Logging
- `github.com/robfig/cron/v3` → CRON parsing & scheduling

---

## Package `internal/cli`

### Responsabilité

**Formatage Output CLI** — Affiche résultats validation/execution dans CLI

**Équivalent Java**: Console output utilities (pas d'équivalent direct)

---

### Ce qu'il fait

✅ Formate erreurs parsing (parse errors)
✅ Formate erreurs validation (validation errors)
✅ Formate résultats execution (success/failure)
✅ Formate dry-run preview (HTTP requests)
✅ Affiche config summary
✅ Support mode verbose/quiet
✅ Pretty-print JSON bodies
✅ Truncation intelligente (long outputs)

---

### Ce qu'il ne fait PAS

❌ Parse config (rôle config package)
❌ Execute pipeline (rôle runtime)
❌ Logging structuré (rôle logger)

---

### Types Exposés

| Type | Description | Champs |
|------|-------------|--------|
| `OutputOptions` | Options affichage | Verbose, Quiet, DryRun |

---

### Fonctions Publiques

**Error Display**:

| Fonction | Signature | Description |
|----------|-----------|-------------|
| `PrintParseErrors` | `(errors []ParseError, verbose bool)` | Affiche erreurs parsing |
| `PrintValidationErrors` | `(errors []ValidationError, verbose, quiet bool)` | Affiche erreurs validation |

**Execution Display**:

| Fonction | Signature | Description |
|----------|-----------|-------------|
| `PrintExecutionResult` | `(result, err, opts)` | Affiche résultat execution |
| `PrintDryRunPreview` | `(previews []RequestPreview, verbose bool)` | Affiche preview dry-run |
| `PrintConfigSummary` | `(data map[string]interface{})` | Affiche nom/version connector |

---

### Parse Errors Output

**Exemple**:
```
✗ Parse errors:
  config.yaml:12:5: invalid YAML syntax
  config.yaml:24:10: unexpected token
```

**Verbose Mode**:
```
✗ Parse errors:
  config.yaml:12:5: invalid YAML syntax
    Type: yaml_syntax_error
  config.yaml:24:10: unexpected token
    Type: yaml_parse_error
```

**Format**: `path:line:column: message`

---

### Validation Errors Output

**Compact Mode** (default):
```
✗ Validation errors:
  /connector/name: must match pattern ^[a-z][a-z0-9_-]*$
  /input/endpoint: is required
  /output/method: must be one of: POST, PUT, PATCH

Hint: Use --verbose for detailed error information
```

**Verbose Mode**:
```
✗ Validation errors:
  /connector/name:
    Message: must match pattern ^[a-z][a-z0-9_-]*$
    Type: pattern
    Expected: ^[a-z][a-z0-9_-]*$
  /input/endpoint:
    Message: is required
    Type: required
  /output/method:
    Message: must be one of: POST, PUT, PATCH
    Type: enum
    Expected: POST, PUT, PATCH
```

**Truncation** (compact): Messages >80 chars → "...truncated..."

---

### Execution Result Output

**Success**:
```
✓ Pipeline executed successfully
  Status: success
  Records processed: 150
```

**Success (verbose)**:
```
✓ Pipeline executed successfully
  Status: success
  Records processed: 150
  Duration: 3.45s
```

**Failure**:
```
✗ Pipeline execution failed
  Module: output
  Error: HTTP 503 Service Unavailable
```

**Quiet Mode**: Pas d'output (seulement exit code)

---

### Dry-Run Preview Output

**Single Request**:
```
📋 Dry-Run Preview (what would have been sent):

  Endpoint: POST https://api.example.com/users
  Records: 150
  Headers:
    Authorization: ***REDACTED***
    Content-Type: application/json
    X-Custom-Header: value
  Body:
    [
      {
        "id": "user-001",
        "name": "Alice Smith",
        "email": "alice@example.com"
      },
      {
        "id": "user-002",
        "name": "Bob Johnson",
        "email": "bob@example.com"
      }
    ]

ℹ️  No data was sent to the target system (dry-run mode)
```

**Multiple Requests** (single mode):
```
📋 Dry-Run Preview (what would have been sent):

─── Request 1 of 150 ───
  Endpoint: POST https://api.example.com/users/user-001
  Records: 1
  Headers:
    Authorization: ***REDACTED***
  Body:
    {
      "id": "user-001",
      "name": "Alice Smith"
    }

─── Request 2 of 150 ───
  Endpoint: POST https://api.example.com/users/user-002
  Records: 1
  ...
```

**Truncation** (compact mode):
- Body >10 lines → Affiche 10 premières + "... (N more lines)"
- Verbose mode → Full body

**Redaction**:
- Authorization headers: `***REDACTED***`
- Configurable via `DryRunOptions.ShowCredentials`

---

### Config Summary Output

**Exemple**:
```
  Connector: users-sync
  Version: 1.0.0
```

**Utilisé par**: `canectors validate` command

---

### Dépendances

**Entrantes**:
- `cmd/canectors` → Affiche outputs CLI

**Sortantes**:
- `internal/config` → Types ParseError, ValidationError
- `pkg/connector` → Types ExecutionResult, RequestPreview
- `os`, `fmt` → Output vers stdout/stderr

---

## Fin PACKAGE_MAP

**Tous les packages documentés**.

**Utilisation**: Naviguer par package pour comprendre responsabilités, types, fonctions, flows détaillés.

**Prochaines étapes**:
- Lire **TYPE_AND_METHOD_MAP.md** pour call graphs complets
- Lire **EXECUTION_FLOW.md** pour flows end-to-end
- Lire **INVARIANTS_AND_ASSUMPTIONS.md** pour contraintes système
