# Audit Schema-driven (Cannectors) — Plan d'amélioration

## 0) TL;DR

| Problème | Impact | Priorité |
|----------|--------|----------|
| **Schema `auth-schema.json` contient `oauth2` mais code lit `scopes` comme comma-separated alors que schema attend `scope` (singulier)** | OAuth2 scopes ignorés silencieusement | P0 |
| **Champ `webhook.path` du schema non mappé vers code qui attend `endpoint`** | Config webhook invalide rejetée malgré schema valide | P0 |
| **Defaults `moduleDefaults` partiellement appliqués** — retry hérité mais pas backoffMultiplier individuel | Retry config incohérente | P1 |
| **Champ `timeout` legacy (seconds) vs `timeoutMs` — conversion dispersée, non centralisée** | Confusion, bugs silencieux si legacy timeout utilisé | P1 |
| **Filter `then`/`else` nested attendu comme array dans schema mais code attend object** | Nested filters cassés | P0 |
| **Schema `pagination.oneOf` non validé côté runtime** — champs cursor vs offset non vérifiés | Crash tardif / erreur confuse | P1 |
| **Missing validation runtime des champs `required` du schema** (ex: `databaseConnectionConfig.oneOf`) | Config invalide acceptée | P2 |
| **Logs sans contexte structuré suffisant pour debug** — pas de trace ID, pas de record sampling | Debug production difficile | P2 |
| **Code mort: `DynamicParamsConfig` deprecated mais encore présent** | Dette technique | P3 |
| **Templates `{{record.field}}` non validés avant exécution** | Erreur runtime tardive | P2 |

---

## 1) Cartographie du code (où vit quoi)

### 1.1 Packages/Modules impliqués

```
cmd/cannectors/main.go          → CLI entry point, commandes run/validate/version
internal/config/
  ├── types.go                   → ParseResult, ValidationResult, ValidationError
  ├── parser.go                  → ParseConfig, ParseJSONFile, ParseYAMLFile
  ├── validator.go               → ValidateConfig, embedded schemas, jsonschema/v6
  └── converter.go               → ConvertToPipeline, applyErrorHandling
internal/factory/modules.go     → CreateInputModule, CreateFilterModules, CreateOutputModule
internal/registry/
  ├── registry.go                → RegisterInput/Filter/Output, GetXxxConstructor
  └── builtins.go                → Enregistrement des modules built-in
internal/runtime/
  ├── pipeline.go                → Executor, Execute, executeFilters, executeOutput
  ├── retry.go                   → Re-export errhandling types
  └── errors.go / metadata.go    → Error codes, metadata
internal/modules/
  ├── input/
  │   ├── http_polling.go        → HTTPPolling module
  │   ├── webhook.go             → Webhook module
  │   └── database.go            → DatabaseInput module
  ├── filter/
  │   ├── filter.go              → Module interface
  │   ├── mapping.go             → MappingModule, FieldMapping
  │   ├── condition.go           → ConditionModule, NestedModuleConfig
  │   ├── http_call.go           → HTTPCallModule
  │   ├── sql_call.go            → SQLCallModule
  │   ├── script.go              → ScriptModule (Goja)
  │   ├── set.go / remove.go     → SetModule, RemoveModule
  │   └── path.go                → GetNestedValue, SetNestedValue
  └── output/
      ├── http_request.go        → HTTPRequestModule
      ├── database.go            → DatabaseOutput
      └── template.go            → TemplateEvaluator wrapper
internal/auth/
  ├── auth.go                    → NewHandler, apiKeyHandler, bearerHandler, basicHandler
  └── oauth2.go                  → oauth2Handler, token caching
internal/httpconfig/
  ├── config.go                  → BaseConfig, KeyConfig, types
  └── extraction.go              → ExtractBaseConfig, ExtractKeysConfig
internal/template/template.go   → Evaluator, ParseVariables, Evaluate
internal/persistence/
  ├── config.go                  → StatePersistenceConfig
  └── state.go                   → StateStore, Save, Load
internal/errhandling/
  ├── retry.go                   → RetryConfig, RetryExecutor
  └── errors.go                  → ClassifyError, IsRetryable
internal/database/database.go   → Open, Config, FormatPlaceholder
internal/cache/lru.go           → LRUCache
```

### 1.2 Diagramme: Config → Validation → Compilation → Exécution

```
[config.yaml/json]
        │
        ▼
┌─────────────────────────────────────────────────────────┐
│  config.ParseConfig(filepath)                           │
│    ├── ParseJSONFile / ParseYAMLFile                    │
│    └── ValidateConfig(data) ← jsonschema/v6            │
│           └── Uses embedded pipeline-schema.json        │
│               + common/input/filter/output/auth-schema  │
└─────────────────────────────────────────────────────────┘
        │ ParseResult + ValidationResult
        ▼
┌─────────────────────────────────────────────────────────┐
│  config.ConvertToPipeline(data)                         │
│    ├── extractPipelineMetadata()                        │
│    ├── extractModules() → convertModuleConfig()         │
│    ├── extractDefaultsAndErrorHandling()                │
│    └── applyErrorHandling() ← resolve per-module        │
└─────────────────────────────────────────────────────────┘
        │ *connector.Pipeline
        ▼
┌─────────────────────────────────────────────────────────┐
│  factory.CreateInputModule(pipeline.Input)              │
│  factory.CreateFilterModules(pipeline.Filters)          │
│  factory.CreateOutputModule(pipeline.Output)            │
│    └── Uses registry.GetXxxConstructor(type)            │
└─────────────────────────────────────────────────────────┘
        │ input.Module, []filter.Module, output.Module
        ▼
┌─────────────────────────────────────────────────────────┐
│  runtime.NewExecutorWithModules(...)                    │
│  executor.Execute(pipeline)                             │
│    ├── executeInput() → input.Fetch(ctx)                │
│    ├── executeFilters() → filter.Process(ctx, records)  │
│    └── executeOutput() → output.Send(ctx, records)      │
└─────────────────────────────────────────────────────────┘
```

---

## 2) Conformité Schema ↔ Runtime

### 2.1 Champs supportés vs ignorés

| Champ (Schema) | Fichier Schema | Support Runtime ? | Fichier Code | Commentaire | Risque |
|----------------|----------------|-------------------|--------------|-------------|--------|
| `webhook.path` | input-schema.json | ❌ **NON** | input/webhook.go | Code attend `endpoint`, schema définit `path` | **CRITIQUE** — webhook cassé |
| `auth.oauth2.credentials.scope` | auth-schema.json | ⚠️ Partiel | auth/oauth2.go:L52 | Code lit `scopes` (pluriel, comma-separated) mais schema définit `scope` (singulier) | **MAJEUR** — scopes ignorés |
| `filter.condition.then/else` | filter-schema.json | ⚠️ Partiel | filter/condition.go | Schema définit comme `array` d'items, code attend `object` (NestedModuleConfig) | **MAJEUR** — parsing échoue |
| `input.httpPolling.retry` | common-schema.json | ✅ Oui | input/http_polling.go:L192 | Extrait via extractRetryConfig | OK |
| `defaults.retry.useRetryAfterHeader` | common-schema.json | ✅ Oui | errhandling/retry.go:L206 | Parsé dans ParseRetryConfig | OK |
| `defaults.retry.retryHintFromBody` | common-schema.json | ✅ Oui | errhandling/retry.go:L210 | Parsé dans ParseRetryConfig | OK |
| `output.httpRequest.timeout` (legacy) | output-schema.json | ✅ Oui (deprecated) | output/http_request.go:L248 | Convertit seconds→ms | OK mais deprecated |
| `output.httpRequest.success.expression` | output-schema.json | ❌ **NON** | output/http_request.go | `success.statusCodes` supporté, pas `expression` ni `lang` | Silencieux |
| `filter.mapping.transforms.op` | filter-schema.json | ⚠️ Partiel | filter/mapping.go:L516-565 | Ops supportés: trim/lowercase/uppercase/toString/toInt/toFloat/toBool/toArray/toObject/dateFormat/replace/split/join. Schema liste `examples` seulement | À documenter |
| `input.database.incremental.enabled` | common-schema.json | ✅ Oui | input/database.go:L255 | Via parseIncrementalConfig | OK |
| `filter.httpCallFilterConfig.retry` | N/A (manquant) | ❌ Schema absent | filter/http_call.go | http_call filter n'a pas de retry dans schema | Incohérence |
| `common.statePersistenceConfig.id.field` | common-schema.json | ✅ Oui | persistence/config.go:L77 | Parsé correctement | OK |
| `output.databaseOutputConfig.transaction` | output-schema.json | ✅ Oui | output/database.go:L77 | Supporté | OK |

### 2.2 Discriminateurs `type` + oneOf

**Problème 1: Validation runtime absente pour oneOf complexes**

Fichier: `internal/config/converter.go`  
Fonction: `convertModuleConfig()`

Le code extrait simplement `type` et copie tout dans `Config` map sans valider que les champs requis par le discriminateur sont présents. Par exemple:
- Schema `pagination.oneOf` requiert `cursorParam`+`nextCursorField` si `type: "cursor"`
- Runtime ne valide pas cette contrainte — échec tardif dans `fetchWithPagination()`

**Problème 2: Fallback silencieux vers stub**

Fichier: `internal/factory/modules.go:L53-55`  
Fonction: `CreateInputModule()`

Si un `type` inconnu est fourni, le code retourne un stub au lieu d'erreur. Le pipeline s'exécute mais ne fait rien d'utile:
```go
return input.NewStub(cfg.Type, endpoint), nil
```

**Recommandation:** Logger un warning et potentiellement échouer si strict mode.

### 2.3 Defaults / Héritage

**Flow de résolution actuel:**
```
Module config > pipeline.Defaults > pipeline.ErrorHandling > hardcoded defaults
```

Fichier: `internal/config/converter.go`  
Fonctions: `resolveOnError()`, `resolveTimeout()`, `resolveRetry()`

**Problèmes identifiés:**

1. **Retry partiel:** `resolveRetry()` remplace entièrement le retry config si présent au niveau module. Pas de merge granulaire (ex: module override juste `maxAttempts` mais hérite `delayMs` de defaults).

2. **timeoutMs non propagé aux filters:** `applyErrorHandling()` ne traite que Input/Output, les filters ne reçoivent pas le timeout par défaut.

3. **onError par défaut "fail" hardcodé** dans chaque module (mapping.go:L76, condition.go:L164) au lieu de lire depuis defaults.

### 2.4 Validation & messages d'erreur

**Qualité actuelle:**

Fichier: `internal/config/validator.go`  
Fonction: `convertValidationErrors()`

Les erreurs jsonschema sont converties mais perdent du contexte:
- Le type d'erreur est extrait par string matching (`strings.Contains(msg, "required")`)
- Le path est formaté mais pas toujours lisible pour l'utilisateur
- Pas de suggestion de correction

**Exemple d'erreur actuelle:**
```
/filters/0: allOf failed: [oneOf failed]
```

**Erreur souhaitée:**
```
filters[0]: Filter type "unknown" is not valid. 
Supported types: mapping, condition, script, http_call, sql_call, set, remove
```

---

## 3) Bugs possibles & Edge cases (priorisés)

### Bug 1: Webhook `path` vs `endpoint` mismatch
- **Symptom:** Webhook config valide selon schema mais rejetée par runtime
- **Root cause:** Schema définit `webhookInputConfig.path` (required), code cherche `endpoint` dans `cfg["endpoint"]`
- **Fichier:** `internal/modules/input/webhook.go:L145`
- **Impact:** Webhook ne fonctionne jamais avec config conforme au schema
- **Repro:** Config avec `input.type: webhook` et `input.path: "/hook"` — code échoue avec `ErrMissingEndpoint`
- **Correctif:** Modifier extractWebhookEndpoint pour lire `path` OU `endpoint`

### Bug 2: OAuth2 scope ignoré
- **Symptom:** Token obtenu sans scopes même si configurés
- **Root cause:** `oauth2.go:L52` lit `config.Credentials["scopes"]` mais schema définit `scope` (singulier)
- **Fichier:** `internal/auth/oauth2.go:L52`
- **Impact:** API calls peuvent échouer par manque de permissions
- **Repro:** Config avec `authentication.credentials.scope: "read write"` — scope non envoyé
- **Correctif:** Lire `scope` (avec fallback `scopes` pour compat)

### Bug 3: Condition filter `then`/`else` parsing failure
- **Symptom:** Nested filters dans condition ne fonctionnent pas
- **Root cause:** Schema définit `then` comme `array` d'items, code attend `map[string]interface{}`
- **Fichier:** `internal/factory/modules.go:L108-114`
- **Impact:** Condition routing cassé
- **Repro:** Config avec `condition.then: [{type: mapping, ...}]` — parsing échoue
- **Correctif:** Modifier parseNestedModuleConfig pour gérer array

### Bug 4: State persistence timestamp format incompatible
- **Symptom:** Incremental queries retournent toutes les données
- **Root cause:** `replaceLastRunTimestamp()` utilise `time.Time` directement, mais certains DB attendent string ISO8601
- **Fichier:** `internal/modules/input/database.go:L343`
- **Impact:** Pagination inefficace, surcharge
- **Repro:** PostgreSQL avec `WHERE updated_at > $1` et timestamp comme time.Time
- **Correctif:** Formater selon driver (PostgreSQL accepte time.Time, MySQL préfère string)

### Bug 5: Cache key non déterministe pour sql_call
- **Symptom:** Cache misses inattendus
- **Root cause:** `buildCacheKey()` utilise `json.Marshal(record)` mais Go maps n'ont pas d'ordre garanti
- **Fichier:** `internal/modules/filter/sql_call.go:L351`
- **Impact:** Cache inefficace, performances dégradées
- **Repro:** Même record avec champs dans ordre différent = clés différentes
- **Correctif:** Utiliser orderedmap ou trier les clés avant marshal

### Bug 6: Script filter context cancellation race
- **Symptom:** Script continue après context cancel, ou panic
- **Root cause:** `runtime.Interrupt()` peut être appelé pendant que script est déjà terminé
- **Fichier:** `internal/modules/filter/script.go:L274`
- **Impact:** Resources leak, comportement imprévisible
- **Repro:** Context cancel pendant exécution script rapide
- **Correctif:** Vérifier état avant interrupt, utiliser atomic flag

### Bug 7: HTTP 401 retry infinite loop potential
- **Symptom:** Module HTTP retry indéfiniment sur 401
- **Root cause:** OAuth2 token invalidé puis re-fetché, mais si credentials invalides, boucle
- **Fichier:** `internal/modules/input/http_polling.go:L430-438`
- **Impact:** Hang de pipeline, rate limiting
- **Repro:** OAuth2 avec credentials expirés/révoqués
- **Correctif:** Compter invalidations consécutives, fail après N

### Bug 8: Template injection via record data
- **Symptom:** Requêtes SQL malformées ou injection
- **Root cause:** Templates `{{record.field}}` dans queryFile non échappés pour caractères spéciaux
- **Fichier:** `internal/modules/output/database.go:L282`
- **Impact:** SQL injection si données non contrôlées
- **Repro:** Record avec `field: "'; DROP TABLE users; --"`
- **Correctif:** Utiliser parameterized queries (déjà fait) mais valider que TOUTES les substitutions sont paramétrisées

---

## 4) Code mort / Fonctionnalités fantômes

| Élément | Fichier | Ligne | Raison |
|---------|---------|-------|--------|
| `DynamicParamsConfig` struct | httpconfig/config.go | L49 | Deprecated, remplacé par KeyConfig |
| `console` filter type | N/A | N/A | Mentionné dans code mais pas implémenté/registered |
| `filter.Stub` usage | filter/stub.go | Entier | Utilisé en fallback mais devrait logger warning |
| `retryDelayMs` legacy field | config/converter.go | L190 | Doublon avec `retry.delayMs` |
| `backoffMultiplier` at root level | config/converter.go | L196 | Legacy, devrait être dans `retry` block |
| `maxRetryDelayMs` legacy | config/converter.go | L199 | Legacy, devrait être `retry.maxDelayMs` |
| `retryableStatusCodes` at root | config/converter.go | L202 | Legacy, devrait être dans `retry` block |

---

## 5) Refactorisations proposées

### 5.1 Quick wins (≤ 1 jour)

#### QW-1: Corriger webhook path/endpoint
- **Objectif:** Aligner code avec schema
- **Scope:** `internal/modules/input/webhook.go` (extractWebhookEndpoint)
- **Risques:** Aucun si backward compatible
- **Bénéfices:** Webhook fonctionne avec configs conformes

#### QW-2: Corriger OAuth2 scope/scopes
- **Objectif:** Lire `scope` (schema) avec fallback `scopes`
- **Scope:** `internal/auth/oauth2.go:L52`
- **Risques:** Aucun
- **Bénéfices:** OAuth2 scopes fonctionnels

#### QW-3: Logger warning pour types inconnus
- **Objectif:** Alerter quand stub utilisé
- **Scope:** `factory/modules.go` (3 endroits)
- **Risques:** Aucun
- **Bénéfices:** Debug facilité

#### QW-4: Valider templates au load time
- **Objectif:** Échouer tôt si template malformé
- **Scope:** `output/http_request.go`, `filter/http_call.go`, `filter/sql_call.go`
- **Risques:** Breaking change si configs invalides existent
- **Bénéfices:** Erreurs claires à la validation

### 5.2 Refactors moyens (1–3 jours)

#### RM-1: Centraliser extraction timeout/retry
- **Objectif:** Un seul lieu de résolution timeout (ms vs s)
- **Scope:** `httpconfig/extraction.go`, tous les modules
- **Risques:** Régression si pas testé
- **Bénéfices:** Consistance, moins de bugs legacy

#### RM-2: Améliorer messages validation schema
- **Objectif:** Messages actionnables pour utilisateurs
- **Scope:** `config/validator.go`
- **Risques:** Faible
- **Bénéfices:** UX CLI améliorée

#### RM-3: Merge granulaire retry config
- **Objectif:** Module peut override un seul champ de retry
- **Scope:** `config/converter.go`, `errhandling/retry.go`
- **Risques:** Changement comportement
- **Bénéfices:** Configuration plus flexible

#### RM-4: Supprimer code legacy
- **Objectif:** Nettoyer DynamicParamsConfig et champs retry legacy
- **Scope:** `httpconfig/config.go`, `config/converter.go`
- **Risques:** Breaking change pour vieilles configs
- **Bénéfices:** Moins de confusion, code plus simple

### 5.3 Chantiers structurants (≥ 1 semaine)

#### CS-1: Validation runtime des contraintes schema
- **Objectif:** Valider oneOf/allOf/required au-delà de jsonschema (double check)
- **Scope:** Nouveau package `internal/validation/`
- **Risques:** Complexité, duplication avec jsonschema
- **Bénéfices:** Erreurs plus claires, fail-fast

#### CS-2: Refactor condition filter pour arrays
- **Objectif:** Supporter `then: [...]` comme array de filters
- **Scope:** `filter/condition.go`, `factory/modules.go`
- **Risques:** Breaking change si ancien format utilisé
- **Bénéfices:** Conformité schema, pipelines plus expressifs

#### CS-3: Observabilité structurée
- **Objectif:** Trace IDs, metrics, structured errors
- **Scope:** `internal/logger/`, tous les modules
- **Risques:** Changements API logging
- **Bénéfices:** Debug production, monitoring

#### CS-4: Schema-to-struct generation
- **Objectif:** Générer structs Go depuis schemas JSON
- **Scope:** Nouvel outil build, refonte types
- **Risques:** Effort important, migration
- **Bénéfices:** Conformité garantie, moins de divergence

---

## 6) Tests & Qualité

### 6.1 Matrice de tests recommandée

| Catégorie | Couverture actuelle | Objectif | Priorité |
|-----------|---------------------|----------|----------|
| Unit tests modules | Bonne (mappings, filters) | 90%+ | P1 |
| Integration tests DB | Présents | Ajouter MySQL/SQLite | P2 |
| E2E tests pipeline | Limités | 1 test par input/output type | P1 |
| Schema compliance | Absent | Nouveau | P0 |
| Error messages | Absent | Snapshot tests | P2 |
| Performance (pagination) | Absent | Benchmark 10k+ records | P3 |

### 6.2 Tests "schema compliance" à ajouter

```
internal/config/schema_compliance_test.go (nouveau)
- TestAllSchemaFieldsMappedToCode()
- TestDefaultValuesMatchSchema()
- TestOneOfConstraintsValidatedAtRuntime()
- TestRequiredFieldsProduceHelpfulErrors()
```

### 6.3 Tests de non-régression

Utiliser `configs/examples/` comme fixtures:
- Chaque exemple doit:
  1. Valider sans erreur
  2. Parser en Pipeline
  3. Créer tous modules sans panic
- Ajouter CI step: `go test ./... -run TestExampleConfigs`

---

## 7) Observabilité & ergonomie

### 7.1 Logs

**Problèmes actuels:**
- Logs sans trace ID / correlation ID
- Pas de sampling pour high-volume records
- Secrets parfois loggés (OAuth2 token URL loggé)

**Améliorations:**
1. Ajouter `trace_id` à tous les logs d'exécution
2. Logger record sample (1%) au niveau DEBUG
3. Auditer tous les logs pour secrets (`clientId`, `token`, `password`)

### 7.2 Metrics (futures)

Si prometheus/metrics ajoutés:
- `cannectors_records_processed_total{pipeline, status}`
- `cannectors_execution_duration_seconds{pipeline, stage}`
- `cannectors_retry_attempts_total{module, status_code}`
- `cannectors_cache_hits_total{module}`

### 7.3 UX CLI

**Améliorations:**
1. `--strict` mode: fail sur unknown types au lieu de stub
2. `--validate-only`: comme validate mais aussi test connexions
3. Colorized output pour erreurs (déjà ✓/✗)
4. Exit codes documentés (actuellement 0-3, bon)

---

## 8) Plan d'exécution recommandé

### Sprint 1 (S1) — Bugs critiques
| Tâche | Priorité | Effort | Dépendances |
|-------|----------|--------|-------------|
| Fix webhook path/endpoint | P0 | 2h | Aucune |
| Fix OAuth2 scope | P0 | 1h | Aucune |
| Fix condition then/else parsing | P0 | 4h | Aucune |
| Ajouter tests schema compliance | P0 | 1j | Aucune |

### Sprint 2 (S2) — Stabilité
| Tâche | Priorité | Effort | Dépendances |
|-------|----------|--------|-------------|
| Centraliser timeout extraction | P1 | 1j | S1 |
| Logger warning stub modules | P1 | 2h | Aucune |
| Valider templates au load | P1 | 0.5j | Aucune |
| Fix cache key determinism | P1 | 2h | Aucune |
| Améliorer messages validation | P1 | 1j | Aucune |

### Sprint 3 (S3) — Clean-up & Observabilité
| Tâche | Priorité | Effort | Dépendances |
|-------|----------|--------|-------------|
| Supprimer code legacy | P2 | 1j | S2 |
| Trace IDs dans logs | P2 | 1j | Aucune |
| Audit secrets dans logs | P2 | 0.5j | Aucune |
| E2E tests par module type | P2 | 2j | S1 |

---

## 9) Annexe

### 9.1 Checklist de conformité schema/runtime

- [ ] Tous les champs `required` du schema validés côté runtime
- [ ] Tous les `oneOf` discriminateurs validés (pas juste via jsonschema)
- [ ] Tous les `default` du schema appliqués côté runtime
- [ ] Noms de champs identiques (snake_case vs camelCase alignés)
- [ ] Types de champs correspondants (integer vs float64 JSON)
- [ ] Messages d'erreur incluent path schema

### 9.2 Liste des fichiers analysés

**Schemas:**
- internal/config/schema/pipeline-schema.json
- internal/config/schema/common-schema.json
- internal/config/schema/input-schema.json
- internal/config/schema/filter-schema.json
- internal/config/schema/output-schema.json
- internal/config/schema/auth-schema.json

**Code principal:**
- cmd/cannectors/main.go
- internal/config/types.go, parser.go, validator.go, converter.go
- internal/factory/modules.go
- internal/registry/registry.go, builtins.go
- internal/runtime/pipeline.go, retry.go
- internal/modules/input/http_polling.go, webhook.go, database.go
- internal/modules/filter/filter.go, mapping.go, condition.go, http_call.go, sql_call.go, script.go, set.go, remove.go, path.go
- internal/modules/output/http_request.go, database.go, template.go
- internal/auth/auth.go, oauth2.go
- internal/httpconfig/config.go, extraction.go
- internal/template/template.go
- internal/persistence/config.go, state.go
- internal/errhandling/retry.go, errors.go
- internal/database/database.go
- internal/cache/lru.go
- pkg/connector/types.go

### 9.3 Éléments non analysés

| Élément | Raison | Impact sur audit |
|---------|--------|------------------|
| Tests (*_test.go) | Hors scope (audit code prod) | Tests peuvent révéler plus de bugs |
| Logger package | Pas chargé en détail | Peut manquer issues logging |
| CLI output formatting | Superficiel | UX secondaire |
| Scheduler package | Superficiel | CRON pas critique pour schema |

---

*Fin du rapport d'audit — Généré le 2026-02-12*
