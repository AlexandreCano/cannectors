# Plan : Centralisation de la gestion d'erreur `onError`

## 1. Constat actuel

La logique `onError` (fail/skip/log) est **dupliquée dans chaque module** qui la supporte. Voici l'inventaire :

### Modules avec `onError` intégré (code dupliqué)

| Module | Fichier | Pattern |
|--------|---------|---------|
| **mapping** (filter) | `filter/mapping.go:392-451` | Switch `m.onError` dans `Process()` – gère fail/skip/log par record |
| **condition** (filter) | `filter/condition.go:408-452` | Deux méthodes `handleError()` + `handleNestedModuleError()` + switch dans `runNestedModules()` |
| **http_call** (filter) | `filter/http_call.go:558-587` | Switch `m.onError` dans `Process()` – identique au pattern de mapping |
| **sql_call** (filter) | `filter/sql_call.go:392-420` | Switch `m.onError` dans `Process()` – identique au pattern de mapping |
| **database** (output) | `output/database.go:305-346` | Deux méthodes `handleQueryBuildError()` + `handleDatabaseError()` |

### Modules SANS gestion `onError`

| Module | Fichier | Comportement |
|--------|---------|--------------|
| **set** (filter) | `filter/set.go` | Retourne directement l'erreur (fail implicite) |
| **remove** (filter) | `filter/remove.go` | Ne peut pas échouer (delete silencieux) |
| **console** (filter) | `filter/console.go` | Module de logging JS, pas de gestion d'erreur record-level |
| **script** (filter) | `filter/script.go` | Non examiné |
| **http_polling** (input) | `input/http_polling.go` | Pas d'`onError` – utilise retry + fail direct |
| **webhook** (input) | `input/webhook.go` | Pas d'`onError` – push-based |
| **http_request** (output) | `output/http_request.go` | Pas d'`onError` visible – utilise retry |

### Executor actuel (`internal/runtime/pipeline.go`)

L'executor **ne gère PAS** `onError`. Il appelle simplement :
- `inputModule.Fetch()` → erreur = échec pipeline
- `filterModule.Process()` → erreur = échec pipeline
- `outputModule.Send()` → erreur = échec pipeline (ou partial)

L'`onError` est actuellement géré **à l'intérieur** de chaque module, ce qui crée :
1. **Duplication de code** : le même switch fail/skip/log est copié dans 5+ modules
2. **Incohérence** : certains modules (set, script) ne le supportent pas
3. **Difficulté de maintenance** : ajouter un nouveau comportement = modifier chaque module

## 2. Architecture proposée

### Principe : les modules retournent leurs erreurs, l'executor décide quoi en faire

```
Module.Process(ctx, records)
    → retourne (records, error)     // Le module ne gère plus onError

Executor (boucle sur chaque record pour les filters)
    → appelle module.Process(ctx, [record])
    → si error != nil → applique la stratégie onError
        → fail: arrête le pipeline
        → skip: ignore le record, continue
        → log: log l'erreur, garde le record original, continue
```

### 2.1 Nouveau type `ModuleError`

Dans `internal/errhandling/` ou `pkg/connector/`, ajouter :

```go
// ModuleError wraps an error with module context for centralized error handling.
type ModuleError struct {
    Err         error
    ModuleType  string // "input", "filter", "output"
    ModuleName  string // "mapping", "http_call", "condition", etc.
    RecordIndex int    // -1 si pas lié à un record
}
```

### 2.2 Résolution de la config `onError` par module

Le `onError` d'un module est déterminé avec la précédence existante :
**module config > defaults > "fail"** (par défaut).

Cela existe déjà via `errhandling.ResolveErrorHandlingConfig()` et `ModuleDefaults` dans le pipeline.

Le changement : cette config sera **lue par l'executor**, pas par le module.

### 2.3 Modification de `ModuleConfig` (si nécessaire)

`ModuleConfig` a déjà un champ `Config map[string]interface{}` qui contient `onError`. On peut soit :
- **Option A** : extraire `onError` depuis `Config` dans l'executor (pas de changement de struct)
- **Option B** : ajouter un champ `OnError string` à `ModuleConfig` directement

**Recommandation** : Option A pour minimiser les changements de schema JSON.

### 2.4 Modifications de l'Executor (`internal/runtime/pipeline.go`)

#### Pour les Filter modules (traitement record par record)

```go
func (e *Executor) executeFiltersWithOnError(
    ctx context.Context,
    pipelineID string,
    records []map[string]interface{},
    filterConfigs []connector.ModuleConfig, // pour accéder au onError de chaque filter
    defaults *connector.ModuleDefaults,
) filterResult {
    currentRecords := records

    for i, filterModule := range e.filterModules {
        onError := resolveOnError(filterConfigs, i, defaults) // "fail", "skip", "log"

        if onError == "fail" {
            // Mode fail : comportement actuel, on passe tout le batch
            processed, err := filterModule.Process(ctx, currentRecords)
            if err != nil {
                return filterResult{err: err, errIdx: i}
            }
            currentRecords = processed
        } else {
            // Mode skip/log : traitement record par record
            var result []map[string]interface{}
            for recordIdx, record := range currentRecords {
                processed, err := filterModule.Process(ctx, []map[string]interface{}{record})
                if err != nil {
                    if onError == "skip" {
                        logger.Warn("skipping record due to filter error", ...)
                        continue
                    }
                    // onError == "log"
                    logger.Error("filter error (continuing)", ...)
                    result = append(result, record) // garde le record original
                    continue
                }
                result = append(result, processed...)
            }
            currentRecords = result
        }
    }
    return filterResult{records: currentRecords}
}
```

#### Pour les Output modules (traitement record par record)

Même logique : si `onError` != "fail", on envoie record par record et on gère les erreurs.

#### Pour les Input modules

L'input est un fetch unique (pas record par record). Le `onError` au niveau input signifie :
- `fail` : arrête le pipeline
- `log` : log l'erreur, retourne un tableau vide
- `skip` : même comportement que `log` pour l'input

### 2.5 Simplification des modules

Chaque module est **simplifié** en supprimant :
- Le champ `onError` de la struct du module
- La logique switch fail/skip/log dans `Process()` / `Send()`
- Les méthodes `handleError()` spécifiques

Les modules retournent simplement l'erreur quand il y en a une.

**Avant** (mapping) :
```go
func (m *MappingModule) Process(_ context.Context, records []map[string]interface{}) ([]map[string]interface{}, error) {
    for recordIdx, record := range records {
        targetRecord, err := m.processRecord(record, recordIdx)
        if err != nil {
            switch m.onError {
            case OnErrorFail:
                return nil, err
            case OnErrorSkip:
                // log + continue
            case OnErrorLog:
                // log + append partial
            }
        }
    }
}
```

**Après** (mapping) :
```go
func (m *MappingModule) Process(_ context.Context, records []map[string]interface{}) ([]map[string]interface{}, error) {
    for recordIdx, record := range records {
        targetRecord, err := m.processRecord(record, recordIdx)
        if err != nil {
            return nil, err  // L'executor gère la stratégie
        }
        result = append(result, targetRecord)
    }
    return result, nil
}
```

## 3. Cas particulier : Condition module

Le condition module a une complexité supplémentaire car :
1. Il a son propre `onError` pour **l'évaluation de l'expression** (pas juste le traitement de records)
2. Il a des **nested modules** qui peuvent aussi échouer

**Approche** :
- L'`onError` de l'évaluation d'expression reste dans le condition module (c'est un comportement interne spécifique)
- L'`onError` des nested modules est délégué au même mécanisme centralisé (chaque nested module a sa propre config `onError`)
- L'`onError` du condition module lui-même (en tant que filter dans le pipeline) est géré par l'executor

Alternativement, on peut aussi extraire la logique d'évaluation d'expression : le condition module retourne une erreur si l'expression échoue, et l'executor applique le `onError` du condition module. C'est plus simple et plus cohérent.

## 4. Impact sur les tests

- Les tests unitaires des modules doivent être adaptés pour ne plus tester la logique onError **dans** le module
- De nouveaux tests unitaires doivent être ajoutés pour la logique onError dans l'executor
- Les tests d'intégration existants dans `internal/runtime/` couvriront le comportement end-to-end

## 5. Plan d'exécution (étapes)

### Étape 1 : Créer le handler centralisé dans l'executor
- Ajouter une fonction `resolveOnError(filterConfigs, index, defaults)` dans `internal/runtime/`
- Ajouter la logique record-by-record dans `executeFilters()` quand `onError != "fail"`
- Ajouter la logique dans `executeOutput()` pour le même pattern

### Étape 2 : Propager la config des modules à l'executor
- L'executor doit recevoir les `ModuleConfig` (ou au minimum les `onError` résolus) pour chaque module
- Modifier `NewExecutorWithModules()` ou ajouter un setter pour les configs

### Étape 3 : Simplifier les modules (un par un)
1. `filter/mapping.go` – supprimer `onError` et la logique switch
2. `filter/http_call.go` – supprimer `onError` et la logique switch
3. `filter/sql_call.go` – supprimer `onError` et la logique switch
4. `filter/condition.go` – supprimer `handleError()` et `handleNestedModuleError()`, adapter `Process()`
5. `output/database.go` – supprimer `handleQueryBuildError()`, `handleDatabaseError()`, simplifier `Send()`

### Étape 4 : Adapter les tests
- Pour chaque module simplifié, adapter les tests unitaires
- Ajouter les tests de la logique onError centralisée dans `internal/runtime/pipeline_test.go`

### Étape 5 : Ajouter `onError` aux modules qui ne l'avaient pas
- `filter/set.go` – bénéficiera automatiquement du support onError via l'executor
- `filter/script.go` – idem
- `output/http_request.go` – idem

## 6. Risques et points d'attention

| Risque | Mitigation |
|--------|-----------|
| **Performance** : traitement record-by-record au lieu de batch quand onError != fail | Acceptable car c'est le même coût qu'actuellement (les modules font déjà du record-by-record en interne) |
| **Comportement du condition module** | L'`onError` de l'évaluation d'expression peut rester interne si nécessaire, seul le `onError` du module en tant que filter est centralisé |
| **Backward compatibility** | La config JSON ne change pas. Le comportement observable reste identique. |
| **Nested modules dans condition** | Les nested modules bénéficient automatiquement du pattern si on les exécute via le même executor helper |
| **Output modules** : `database` a un mode transaction | En mode transaction avec onError=skip/log, il faut décider si on rollback le record en erreur ou si on continue la transaction. Recommandation : en mode transaction, forcer `onError=fail` (une erreur dans une transaction doit la faire échouer). |

## 7. Résumé

- **Objectif** : un seul endroit pour la logique fail/skip/log → l'executor
- **Modules** : retournent simplement `error` sans gérer la politique d'erreur
- **Config** : `onError` est lu depuis `ModuleConfig.Config["onError"]` et résolu avec la précédence existante
- **Gain** : suppression de ~200 lignes de code dupliqué, cohérence, facilité d'ajout de nouveaux modules
