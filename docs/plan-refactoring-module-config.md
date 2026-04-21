# Plan de refactoring : Architecture ModuleConfig & parsing typé

## Contexte

Le type `ModuleConfig` utilise un `Config map[string]interface{}` non typé. Chaque module fait ses propres extractions manuelles avec des patterns differents. L'authentification est artificiellement separée des autres champs. Ce plan propose de tout remplacer par des structs Go miroirs des schemas JSON, parsées via `json.Unmarshal` et une unique fonction generique.

**Pas de contrainte de rétro-compatibilité** — le projet n'est pas encore release.

---

## Diagnostic (10 problemes)

| # | Probleme | Impact |
|---|----------|--------|
| P1 | `Config` est un `map[string]interface{}` non typé | Assertions manuelles partout, typos silencieuses |
| P2 | `onError`/`timeoutMs`/`retry` injectés dans le sac par le converter | Indiscernables des champs spécifiques |
| P3 | Deux systemes paralleles de resolution retry (map vs struct) | Double parsing, incohérence |
| P4 | `Authentication` séparé sur `ModuleConfig` sans raison | Asymétrie schema/Go, threading explicite du auth |
| P5 | `httpconfig` à moitié adopté | Duplication entre modules HTTP |
| P6 | `extractRetryConfig` dupliqué 3 fois | Code identique dans 3 packages |
| P7 | `getNestedValue` ré-implémenté 5+ fois | 5 variantes dans 5 fichiers |
| P8 | `defaultTimeout = 30s` dupliqué dans 7+ fichiers | Maintenance fragile |
| P9 | Pas de pattern partagé pour le parsing de config | Chaque module a sa propre approche |
| P10 | Validateur et registre désynchronisés | Listes de types hardcodées séparément |

---

## Principes directeurs

1. **Les structs Go sont le miroir des schemas JSON.** Les noms de champs (via json tags) correspondent exactement aux propriétés du schema.
2. **Une seule fonction generique de parsing** : `ParseConfig[T any](raw map[string]interface{}) (T, error)`.
3. **L'authentification est un champ normal** dans les structs qui en ont besoin (HTTP modules). Elle disparait de `ModuleConfig`.
4. **Les champs de base (`moduleBase`)** sont un struct embarqué (composition Go) dans chaque config de module.
5. **Les defaults (`onError`, `timeoutMs`, `retry`) sont resolus dans la raw map** par le converter avant le parsing — les modules recoivent directement les valeurs finales.
6. **Plus aucun `Extract*`, `parse*Field`, ou assertion manuelle** dans les modules.

---

## Architecture cible

### Fonction generique de parsing (`internal/moduleconfig/parse.go`)

```go
package moduleconfig

import (
    "encoding/json"
    "fmt"
)

// ParseConfig désérialise du JSON brut en struct typée.
// C'est LA seule fonction de parsing utilisée par tous les modules.
func ParseConfig[T any](raw json.RawMessage) (T, error) {
    var cfg T
    if err := json.Unmarshal(raw, &cfg); err != nil {
        return cfg, fmt.Errorf("parsing module config: %w", err)
    }
    return cfg, nil
}
```

### Structs miroirs des schemas

#### Champs communs — miroir de `common-schema.json#/$defs/moduleBase`

```go
// pkg/connector/types.go

// ModuleBase contient les propriétés communes à tous les modules.
// Miroir de common-schema.json#/$defs/moduleBase + moduleDefaults resolus.
type ModuleBase struct {
    ID          string       `json:"id,omitempty"`
    Name        string       `json:"name,omitempty"`
    Description string       `json:"description,omitempty"`
    Enabled     *bool        `json:"enabled,omitempty"`
    Tags        []string     `json:"tags,omitempty"`
    OnError     string       `json:"onError,omitempty"`
    TimeoutMs   int          `json:"timeoutMs,omitempty"`
    Retry       *RetryConfig `json:"retry,omitempty"`
}
```

#### RetryConfig — miroir de `common-schema.json#/$defs/retryConfig`

```go
// pkg/connector/types.go

type RetryConfig struct {
    MaxAttempts          int     `json:"maxAttempts,omitempty"`
    DelayMs              int     `json:"delayMs,omitempty"`
    BackoffMultiplier    float64 `json:"backoffMultiplier,omitempty"`
    MaxDelayMs           int     `json:"maxDelayMs,omitempty"`
    RetryableStatusCodes []int   `json:"retryableStatusCodes,omitempty"`
    UseRetryAfterHeader  bool    `json:"useRetryAfterHeader,omitempty"`
    RetryHintFromBody    string  `json:"retryHintFromBody,omitempty"`
}
```

#### AuthConfig — miroir de `auth-schema.json#/$defs/auth`

L'authentification a un schema polymorphe (`oneOf`). Comme Go n'a pas d'union types, on utilise un struct plat avec le discriminateur `Type` :

```go
// pkg/connector/types.go

// AuthConfig est un struct "flat union" : le champ Type détermine
// quels champs de Credentials sont pertinents.
type AuthConfig struct {
    Type        string          `json:"type"`
    Credentials json.RawMessage `json:"credentials"`
}

// Les structs de credentials typées pour chaque type d'auth.
// Le consommateur (package auth) fait un second Unmarshal sur Credentials
// selon Type.

type CredentialsAPIKey struct {
    Key        string `json:"key"`
    Location   string `json:"location,omitempty"`
    HeaderName string `json:"headerName,omitempty"`
    ParamName  string `json:"paramName,omitempty"`
}

type CredentialsBearer struct {
    Token string `json:"token"`
}

type CredentialsBasic struct {
    Username string `json:"username"`
    Password string `json:"password"`
}

type CredentialsOAuth2 struct {
    TokenURL     string   `json:"tokenUrl"`
    ClientID     string   `json:"clientId"`
    ClientSecret string   `json:"clientSecret"`
    Scopes       []string `json:"scopes,omitempty"`
}
```

**Alternative plus simple** (si tu préfères garder `Credentials map[string]string`) :

Le schema actuel a `additionalProperties: false` sur chaque type de credentials, donc les champs sont finis et connus. `json.RawMessage` + second unmarshal est plus propre et type-safe, mais `map[string]string` reste acceptable si on veut rester simple. A toi de choisir.

#### Configs de modules — miroir des schemas spécifiques

Chaque config **embarque** `ModuleBase` (composition Go = `allOf` en JSON Schema).

```go
// internal/modules/input/config.go

// HTTPPollingConfig — miroir de input-schema.json#/$defs/httpPollingInputConfig
// Compose: moduleBase + httpRequestBase + champs spécifiques
type HTTPPollingConfig struct {
    connector.ModuleBase                          // moduleBase (id, name, onError, ...)

    // httpRequestBase
    Endpoint       string            `json:"endpoint"`
    Method         string            `json:"method,omitempty"`
    Headers        map[string]string `json:"headers,omitempty"`
    QueryParams    map[string]string `json:"queryParams,omitempty"`
    Authentication *connector.AuthConfig `json:"authentication,omitempty"`
    TimeoutMs      int               `json:"timeoutMs,omitempty"`

    // Champs spécifiques httpPolling
    Schedule       string                `json:"schedule"`
    Pagination     *PaginationConfig     `json:"pagination,omitempty"`
    StatePersistence *StatePersistenceConfig `json:"statePersistence,omitempty"`
    DataField      string                `json:"dataField,omitempty"`
    Retry          *connector.RetryConfig `json:"retry,omitempty"`
}

// WebhookConfig — miroir de input-schema.json#/$defs/webhookInputConfig
type WebhookConfig struct {
    connector.ModuleBase

    Endpoint       string                `json:"endpoint"`
    ListenAddress  string                `json:"listenAddress,omitempty"`
    Signature      *WebhookSignature     `json:"signature,omitempty"`
    RateLimit      *WebhookRateLimit     `json:"rateLimit,omitempty"`
    QueueSize      int                   `json:"queueSize,omitempty"`
    MaxConcurrent  int                   `json:"maxConcurrent,omitempty"`
    DataField      string                `json:"dataField,omitempty"`
}

// DatabaseInputConfig — miroir de input-schema.json#/$defs/databaseInputConfig
// Compose: moduleBase + sqlRequestBase + champs spécifiques
type DatabaseInputConfig struct {
    connector.ModuleBase

    // sqlRequestBase (= databaseConnectionConfig + query/queryFile)
    ConnectionString    string `json:"connectionString,omitempty"`
    ConnectionStringRef string `json:"connectionStringRef,omitempty"`
    Driver              string `json:"driver,omitempty"`
    MaxOpenConns        int    `json:"maxOpenConns,omitempty"`
    MaxIdleConns        int    `json:"maxIdleConns,omitempty"`
    ConnMaxLifetimeSeconds int `json:"connMaxLifetimeSeconds,omitempty"`
    ConnMaxIdleTimeSeconds int `json:"connMaxIdleTimeSeconds,omitempty"`
    Query               string `json:"query,omitempty"`
    QueryFile           string `json:"queryFile,omitempty"`

    // Champs spécifiques
    Parameters  map[string]interface{}   `json:"parameters,omitempty"`
    Pagination  *DatabasePaginationConfig `json:"pagination,omitempty"`
    Incremental *IncrementalConfig        `json:"incremental,omitempty"`
}
```

```go
// internal/modules/filter/config.go

// SetConfig — miroir de filter-schema.json#/$defs/setFilterConfig
type SetConfig struct {
    connector.ModuleBase
    Target string      `json:"target"`
    Value  interface{} `json:"value"`
}

// RemoveConfig — miroir de filter-schema.json#/$defs/removeFilterConfig
type RemoveConfig struct {
    connector.ModuleBase
    Target  string   `json:"target,omitempty"`
    Targets []string `json:"targets,omitempty"`
}

// MappingConfig — miroir de filter-schema.json#/$defs/mappingFilterConfig
type MappingConfig struct {
    connector.ModuleBase
    Mappings []FieldMapping `json:"mappings"`
}

// ConditionConfig — miroir de filter-schema.json#/$defs/conditionFilterConfig
type ConditionConfig struct {
    connector.ModuleBase
    Lang       string                   `json:"lang,omitempty"`
    Expression string                   `json:"expression"`
    OnTrue     string                   `json:"onTrue,omitempty"`
    OnFalse    string                   `json:"onFalse,omitempty"`
    Then       []map[string]interface{} `json:"then,omitempty"` // raw — parsing récursif
    Else       []map[string]interface{} `json:"else,omitempty"` // raw — parsing récursif
}

// HTTPCallConfig — miroir de filter-schema.json#/$defs/httpCallFilterConfig
// Compose: moduleBase + httpRequestBase + champs spécifiques
type HTTPCallConfig struct {
    connector.ModuleBase

    // httpRequestBase
    Endpoint       string               `json:"endpoint"`
    Method         string               `json:"method,omitempty"`
    Headers        map[string]string    `json:"headers,omitempty"`
    QueryParams    map[string]string    `json:"queryParams,omitempty"`
    Authentication *connector.AuthConfig `json:"authentication,omitempty"`
    TimeoutMs      int                  `json:"timeoutMs,omitempty"`

    // Champs spécifiques
    Keys              []KeyConfig   `json:"keys,omitempty"`
    Cache             *CacheConfig  `json:"cache,omitempty"`
    MergeStrategy     string        `json:"mergeStrategy,omitempty"`
    DataField         string        `json:"dataField,omitempty"`
    BodyTemplateFile  string        `json:"bodyTemplateFile,omitempty"`
}

// SQLCallConfig — miroir de filter-schema.json#/$defs/sqlCallFilterConfig
type SQLCallConfig struct {
    connector.ModuleBase

    // sqlRequestBase
    ConnectionString    string `json:"connectionString,omitempty"`
    ConnectionStringRef string `json:"connectionStringRef,omitempty"`
    Driver              string `json:"driver,omitempty"`
    MaxOpenConns        int    `json:"maxOpenConns,omitempty"`
    MaxIdleConns        int    `json:"maxIdleConns,omitempty"`
    ConnMaxLifetimeSeconds int `json:"connMaxLifetimeSeconds,omitempty"`
    ConnMaxIdleTimeSeconds int `json:"connMaxIdleTimeSeconds,omitempty"`
    Query               string `json:"query,omitempty"`
    QueryFile           string `json:"queryFile,omitempty"`

    // Champs spécifiques
    MergeStrategy string       `json:"mergeStrategy,omitempty"`
    ResultKey     string       `json:"resultKey,omitempty"`
    Cache         *CacheConfig `json:"cache,omitempty"`
}
```

```go
// internal/modules/output/config.go

// HTTPRequestConfig — miroir de output-schema.json#/$defs/httpRequestOutputConfig
type HTTPRequestConfig struct {
    connector.ModuleBase

    // httpRequestBase
    Endpoint       string               `json:"endpoint"`
    Method         string               `json:"method,omitempty"`
    Headers        map[string]string    `json:"headers,omitempty"`
    QueryParams    map[string]string    `json:"queryParams,omitempty"`
    Authentication *connector.AuthConfig `json:"authentication,omitempty"`
    TimeoutMs      int                  `json:"timeoutMs,omitempty"`

    // Champs spécifiques
    Success          *SuccessCondition      `json:"success,omitempty"`
    Retry            *connector.RetryConfig `json:"retry,omitempty"`
    Keys             []KeyConfig            `json:"keys,omitempty"`
    RequestMode      string                 `json:"requestMode,omitempty"`
    BodyTemplateFile string                 `json:"bodyTemplateFile,omitempty"`
}

// DatabaseOutputConfig — miroir de output-schema.json#/$defs/databaseOutputConfig
type DatabaseOutputConfig struct {
    connector.ModuleBase

    // sqlRequestBase
    ConnectionString    string `json:"connectionString,omitempty"`
    ConnectionStringRef string `json:"connectionStringRef,omitempty"`
    Driver              string `json:"driver,omitempty"`
    MaxOpenConns        int    `json:"maxOpenConns,omitempty"`
    MaxIdleConns        int    `json:"maxIdleConns,omitempty"`
    ConnMaxLifetimeSeconds int `json:"connMaxLifetimeSeconds,omitempty"`
    ConnMaxIdleTimeSeconds int `json:"connMaxIdleTimeSeconds,omitempty"`
    Query               string `json:"query,omitempty"`
    QueryFile           string `json:"queryFile,omitempty"`
    TimeoutMs           int    `json:"timeoutMs,omitempty"`

    // Champs spécifiques
    Transaction bool `json:"transaction,omitempty"`
}
```

### Nouveau `ModuleConfig` simplifié

```go
// pkg/connector/types.go

// ModuleConfig est le conteneur intermédiaire utilisé par le converter.
// Le champ Type sert au routing vers le bon constructeur.
// Le champ Raw contient le JSON brut de TOUT sauf "type" — les defaults y sont déjà résolus.
// Chaque module parse Raw via moduleconfig.ParseConfig[T](Raw).
type ModuleConfig struct {
    Type string          `json:"type"`
    Raw  json.RawMessage `json:"-"`
}
```

**Ce qui disparait :**
- `Config map[string]interface{}` → remplacé par `Raw json.RawMessage` (JSON brut, parsing differé au module)
- `Authentication *AuthConfig` → **supprimé** de `ModuleConfig`. L'auth reste dans `Raw` et sera parsée dans la struct du module qui en a besoin.

### Flow complet : JSON → Pipeline → Modules

```
1. JSON file
   │
   ▼
2. config.ValidateConfig()          ← validation jsonschema (inchangé)
   │
   ▼
3. config.ConvertToPipeline()
   │  Pour chaque module :
   │    a) Extraire "type" depuis la raw map
   │    b) Résoudre defaults dans la raw map :
   │       - onError  : module > defaults > "fail"
   │       - timeoutMs: module > defaults > 30000
   │       - retry    : merge(defaults.retry, module.retry)
   │    c) json.Marshal(map) → json.RawMessage
   │    d) Créer ModuleConfig{Type, Raw}   ← Raw = JSON brut de tout sauf "type"
   │
   ▼
4. factory.CreateXxxModule(ModuleConfig)
   │  Le constructeur fait :
   │    cfg, err := moduleconfig.ParseConfig[HTTPPollingConfig](moduleConfig.Raw)
   │    → json.Unmarshal(Raw, &cfg)   ← un seul unmarshal, pas de marshal
   │    → cfg.Authentication est là si le module en a besoin
   │    → cfg.OnError, cfg.TimeoutMs, cfg.Retry sont là (via ModuleBase embarqué)
   │
   ▼
5. Module utilise cfg typé directement
```

### Sous-structs partagées

Les sous-structures réutilisées entre modules (correspondant aux `$defs` du schema) sont centralisées :

```go
// pkg/connector/types.go (ou internal/moduleconfig/shared.go selon visibilité)

// PaginationConfig — miroir de common-schema.json#/$defs/pagination
type PaginationConfig struct {
    Type            string `json:"type"`
    LimitParam      string `json:"limitParam"`
    Limit           int    `json:"limit,omitempty"`
    CursorParam     string `json:"cursorParam,omitempty"`
    NextCursorField string `json:"nextCursorField,omitempty"`
    PageParam       string `json:"pageParam,omitempty"`
    TotalPagesField string `json:"totalPagesField,omitempty"`
    OffsetParam     string `json:"offsetParam,omitempty"`
    TotalField      string `json:"totalField,omitempty"`
}

// DatabasePaginationConfig — miroir de common-schema.json#/$defs/databasePaginationConfig
type DatabasePaginationConfig struct {
    Type        string `json:"type"`
    Limit       int    `json:"limit,omitempty"`
    OffsetParam string `json:"offsetParam,omitempty"`
    CursorField string `json:"cursorField,omitempty"`
    CursorParam string `json:"cursorParam,omitempty"`
}

// KeyConfig — miroir de common-schema.json#/$defs/httpCallKeyConfig
type KeyConfig struct {
    Field     string `json:"field"`
    ParamType string `json:"paramType"`
    ParamName string `json:"paramName"`
}

// CacheConfig — miroir de filter-schema.json#/$defs/cacheConfig
type CacheConfig struct {
    Enabled    bool   `json:"enabled,omitempty"`
    MaxSize    int    `json:"maxSize,omitempty"`
    DefaultTTL int    `json:"defaultTTL,omitempty"`
    Key        string `json:"key,omitempty"`
}

// SuccessCondition — miroir de output-schema.json#/$defs/successCondition
type SuccessCondition struct {
    Lang        string `json:"lang,omitempty"`
    Expression  string `json:"expression,omitempty"`
    StatusCodes []int  `json:"statusCodes,omitempty"`
}
```

### Réduction du code dupliqué : `httpRequestBase` et `sqlRequestBase`

Le schema utilise la composition (`allOf` avec `$ref`) pour les bases HTTP et SQL. En Go, on peut faire la meme chose avec l'embedding. Deux options :

**Option A — Embedding de structs de base (recommandé)** :

```go
// HTTPRequestBase — miroir de common-schema.json#/$defs/httpRequestBase
type HTTPRequestBase struct {
    Endpoint       string               `json:"endpoint"`
    Method         string               `json:"method,omitempty"`
    Headers        map[string]string    `json:"headers,omitempty"`
    QueryParams    map[string]string    `json:"queryParams,omitempty"`
    Authentication *connector.AuthConfig `json:"authentication,omitempty"`
    TimeoutMs      int                  `json:"timeoutMs,omitempty"`
}

// SQLRequestBase — miroir de common-schema.json#/$defs/sqlRequestBase
type SQLRequestBase struct {
    ConnectionString       string `json:"connectionString,omitempty"`
    ConnectionStringRef    string `json:"connectionStringRef,omitempty"`
    Driver                 string `json:"driver,omitempty"`
    MaxOpenConns           int    `json:"maxOpenConns,omitempty"`
    MaxIdleConns           int    `json:"maxIdleConns,omitempty"`
    ConnMaxLifetimeSeconds int    `json:"connMaxLifetimeSeconds,omitempty"`
    ConnMaxIdleTimeSeconds int    `json:"connMaxIdleTimeSeconds,omitempty"`
    TimeoutMs              int    `json:"timeoutMs,omitempty"`
    Query                  string `json:"query,omitempty"`
    QueryFile              string `json:"queryFile,omitempty"`
}

// Alors HTTPPollingConfig devient :
type HTTPPollingConfig struct {
    connector.ModuleBase
    HTTPRequestBase
    Schedule         string                  `json:"schedule"`
    Pagination       *PaginationConfig       `json:"pagination,omitempty"`
    StatePersistence *StatePersistenceConfig `json:"statePersistence,omitempty"`
    DataField        string                  `json:"dataField,omitempty"`
    Retry            *connector.RetryConfig  `json:"retry,omitempty"`
}

// Et DatabaseInputConfig :
type DatabaseInputConfig struct {
    connector.ModuleBase
    SQLRequestBase
    Parameters  map[string]interface{}    `json:"parameters,omitempty"`
    Pagination  *DatabasePaginationConfig `json:"pagination,omitempty"`
    Incremental *IncrementalConfig        `json:"incremental,omitempty"`
}
```

Ceci élimine la duplication des champs HTTP/SQL entre modules. `json.Unmarshal` gère correctement les champs des structs embarqués.

**Note sur TimeoutMs en doublon** : `ModuleBase` et `HTTPRequestBase`/`SQLRequestBase` ont tous les deux `timeoutMs`. Lors de l'embedding, Go ne duplique pas — il faut choisir un seul endroit. Le plus simple : laisser `timeoutMs` **uniquement** dans les structs de base (`HTTPRequestBase`, `SQLRequestBase`) et le retirer de `ModuleBase`. Les modules sans HTTP/SQL (set, remove, mapping, condition) n'ont de toute facon pas de timeout.

Version ajustée de `ModuleBase` :

```go
type ModuleBase struct {
    ID          string       `json:"id,omitempty"`
    Name        string       `json:"name,omitempty"`
    Description string       `json:"description,omitempty"`
    Enabled     *bool        `json:"enabled,omitempty"`
    Tags        []string     `json:"tags,omitempty"`
    OnError     string       `json:"onError,omitempty"`
}
```

---

## Plan d'execution

### Phase 1 : Fondations

**1.1 — Créer les types partagés dans `pkg/connector/types.go`**
- Ajouter `ModuleBase`, `RetryConfig`, `AuthConfig` (avec `json.RawMessage` pour credentials)
- Ajouter les structs de credentials typées (`CredentialsAPIKey`, etc.)
- Ajouter les sous-structs partagées : `PaginationConfig`, `DatabasePaginationConfig`, `KeyConfig`, `CacheConfig`, `SuccessCondition`
- Simplifier `ModuleConfig` : garder `Type` + `Raw`, supprimer `Config` et `Authentication`
- Simplifier `ModuleDefaults` : `Retry` passe de `map[string]interface{}` a `*RetryConfig`
- Simplifier `ErrorHandling` : idem pour `Retry`
- Mettre a jour `pkg/connector/types_test.go`

**1.2 — Créer `internal/moduleconfig/`**
- `parse.go` : la fonction generique `ParseConfig[T any](raw map[string]interface{}) (T, error)`
- `parse_test.go` : tests unitaires sur plusieurs types
- `nested.go` : migrer `GetNestedValue` / `SetNestedValue` depuis `filter/path.go` (centraliser une fois pour toutes)
- `nested_test.go`

**1.3 — Créer les structs de base HTTP/SQL (`internal/moduleconfig/shared.go`)**
- `HTTPRequestBase` (miroir `httpRequestBase`)
- `SQLRequestBase` (miroir `sqlRequestBase`)
- Ces structs sont dans `internal/moduleconfig` car utilisées par input/filter/output

**1.4 — Adapter le converter**
- `convertModuleConfig()` : ne plus extraire `authentication` séparément. Tout reste dans la map sauf `type`.
- Resolution des defaults (`onError`, `timeoutMs`, `retry`) : meme logique, toujours dans la raw map.
- En fin de conversion : `json.Marshal(map)` → `json.RawMessage` stocké dans `ModuleConfig.Raw`. Le marshal se fait une seule fois ici ; les modules n'ont plus qu'un `json.Unmarshal` a faire.
- `convertModuleDefaults` et `convertErrorHandling` : adapter pour les nouveaux types `RetryConfig`

### Phase 2 : Créer les config structs et migrer les modules

Pour chaque module :
1. Creer la struct `XxxConfig` dans un fichier `config.go` du package du module
2. Remplacer tout le code d'extraction par `moduleconfig.ParseConfig[XxxConfig](moduleConfig.Raw)`
3. Le module utilise directement les champs typés
4. Supprimer toutes les fonctions `extract*` locales
5. Adapter les tests

**Ordre :**

| Etape | Module | Justification |
|-------|--------|---------------|
| 2.1 | `filter/set` | Le plus simple (2 champs). Valide le pattern. |
| 2.2 | `filter/remove` | Simple (2 champs). |
| 2.3 | `filter/mapping` | Moyenne complexité, sous-structs `FieldMapping`/`TransformOp`. |
| 2.4 | `input/webhook` | Simple, pas d'auth. |
| 2.5 | `input/database` | Premier module avec `SQLRequestBase`. |
| 2.6 | `output/database` | Meme base SQL, consolide. |
| 2.7 | `input/http_polling` | Premier module avec `HTTPRequestBase` + auth. |
| 2.8 | `output/http_request` | HTTP + retry + keys + body template. |
| 2.9 | `filter/http_call` | HTTP + cache + auth. Plus de `auth` en parametre separé. |
| 2.10 | `filter/sql_call` | SQL + cache. |
| 2.11 | `filter/condition` | Récursivité : `then`/`else` contiennent des filter configs raw. |
| 2.12 | `filter/script` | Simple (script/scriptFile). |

### Phase 3 : Nettoyage

**3.1 — Supprimer le package `internal/httpconfig/`**
- `HTTPRequestBase` dans `internal/moduleconfig/shared.go` le remplace entierement
- `ExtractBaseConfig`, `ExtractKeysConfig`, `ExtractBodyTemplateConfig`, etc. → supprimés
- `GetTimeoutDuration` → utilitaire simple dans `moduleconfig` ou directement dans les modules

**3.2 — Refondre `internal/errhandling/retry.go`**
- `ParseRetryConfig(map[string]interface{})` n'est plus nécessaire — les modules ont `*connector.RetryConfig` typé
- `ResolveRetryConfig` et `ResolveErrorHandlingConfig` → simplifiés, operent sur les structs typés
- `RetryExecutor` accepte `*connector.RetryConfig` directement

**3.3 — Supprimer le code mort**
- Toutes les fonctions `extract*` / `parse*Field` locales dans les modules
- Toutes les re-implementations de `getNestedValue` (garder uniquement `moduleconfig.GetNestedValue`)
- Toutes les constantes `defaultTimeout` locales
- `DynamicParamsConfig` dans `httpconfig/config.go`
- L'ancien `AuthConfig.Credentials map[string]string` si on passe a `json.RawMessage`
- Supprimer `filter/path.go` apres migration de `GetNestedValue`/`SetNestedValue` vers `moduleconfig/`

**3.4 — Synchroniser validateur et registre**
- Extraire les types supportes en constantes partagées
- Ou mieux : le validateur interroge le registre

**3.5 — Adapter le package `internal/auth/`**
- `NewHandler` prend `*connector.AuthConfig` et fait un `json.Unmarshal` sur `Credentials` selon `Type`
- Plus de `Credentials map[string]string` avec des lectures par clé (`config.Credentials["token"]`)
- Chaque handler recoit sa struct de credentials typée

---

## Résumé des fichiers impactés

| Fichier | Action |
|---------|--------|
| `pkg/connector/types.go` | Refonte : `ModuleBase`, `RetryConfig`, `AuthConfig` (json.RawMessage), `ModuleConfig` (Raw json.RawMessage), sous-structs partagées |
| `pkg/connector/types_test.go` | Adapter |
| `internal/moduleconfig/parse.go` | **NOUVEAU** — `ParseConfig[T](json.RawMessage)` |
| `internal/moduleconfig/parse_test.go` | **NOUVEAU** |
| `internal/moduleconfig/shared.go` | **NOUVEAU** — `HTTPRequestBase`, `SQLRequestBase` |
| `internal/moduleconfig/nested.go` | **NOUVEAU** — `GetNestedValue`, `SetNestedValue` (migré de `filter/path.go`) |
| `internal/moduleconfig/nested_test.go` | **NOUVEAU** |
| `internal/config/converter.go` | Simplifié : plus d'extraction auth, resolution defaults dans raw map |
| `internal/config/converter_test.go` | Adapter |
| `internal/modules/input/config.go` | **NOUVEAU** — `HTTPPollingConfig`, `WebhookConfig`, `DatabaseInputConfig` |
| `internal/modules/input/*.go` | Migration vers `ParseConfig[T]` |
| `internal/modules/filter/config.go` | **NOUVEAU** — tous les filter configs |
| `internal/modules/filter/*.go` | Migration vers `ParseConfig[T]` |
| `internal/modules/output/config.go` | **NOUVEAU** — `HTTPRequestConfig`, `DatabaseOutputConfig` |
| `internal/modules/output/*.go` | Migration vers `ParseConfig[T]` |
| `internal/factory/modules.go` | Adapter signatures, plus de threading `auth` explicite |
| `internal/errhandling/retry.go` | Simplifié, utilise `*connector.RetryConfig` |
| `internal/auth/auth.go` | Accepte `*connector.AuthConfig` avec `json.RawMessage` |
| `internal/auth/oauth2.go` | Unmarshal `CredentialsOAuth2` depuis `AuthConfig.Credentials` |
| `internal/httpconfig/` | **SUPPRIMÉ** (phase 3) |
| `internal/modules/filter/path.go` | **SUPPRIMÉ** (migré vers `moduleconfig/nested.go`) |
| `internal/config/validator.go` | Sync types avec registre |

---

## Ce qui est supprimé

| Element supprimé | Raison |
|------------------|--------|
| `ModuleConfig.Authentication` | L'auth est dans la raw map, parsée par les modules qui en ont besoin |
| `ModuleConfig.Config map[string]interface{}` | Remplacé par `Raw json.RawMessage` |
| `internal/httpconfig/` (tout le package) | Remplacé par `HTTPRequestBase` + `ParseConfig[T]` |
| `filter/path.go` | Migré vers `moduleconfig/nested.go` |
| Fonctions `extract*` dans chaque module | Remplacées par `ParseConfig[T]` |
| `errhandling.ParseRetryConfig(map)` | Les modules ont directement `*connector.RetryConfig` |
| Constantes `defaultTimeout` locales | Centralisée ou inutile (defaults resolus par converter) |
| `DynamicParamsConfig` | Code mort |

---

*Generé le 2026-02-18*
