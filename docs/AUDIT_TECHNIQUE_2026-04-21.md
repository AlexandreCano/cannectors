# Audit technique — Cannectors

*Date : 2026-04-21 · Branche auditée : `feature/REFACTO-2` (HEAD `d9ed497`)*

## 0) Synthèse en 30 secondes

Projet **récupérable, à ne pas réécrire**. L'architecture macro est saine (séparation `pkg/` public / `internal/` privé, interfaces `Module` minimales, Registry+Factory correct, runtime bien découpé). En revanche, la couche HTTP est hypertrophiée et duplique massivement : **trois modules HTTP (polling, request, http_call) réimplémentent chacun leur propre client, retry, HTTPError, `getFieldValue` et headers builder**. Le retry générique `errhandling.RetryExecutor` existe mais personne ne l'utilise. L'audit `docs/plan.md` avait déjà identifié 8 bugs critiques (webhook `path`/`endpoint`, OAuth2 `scope`/`scopes`, condition `then`/`else`, cache key non déterministe, etc.) qui semblent encore présents dans le code lu.

**Notes /10** : lisibilité **7** · cohérence **6** · maintenabilité **5** · testabilité **7** · évolutivité **6**.

---

## 1) Diagnostic global

### Première impression

Le projet est **clairement passé par plusieurs itérations de refactoring** (traces dans `REFACTORING_NOTES.md`, la branche `feature/REFACTO-2`, et la granularité artificielle de certaines fonctions : `pipeline.go` a été cassé en 15+ helpers qui ne servent qu'une seule fois). Le découpage en 17 packages internal, les interfaces `Module` à 2 méthodes, et l'usage de generics pour `ParseConfig[T]` montrent un auteur qui connaît Go. Mais **les modules HTTP ne partagent rien** : le travail de factorisation s'est arrêté à `auth`, `template`, `cache`, `database`. Résultat : l'architecture "propre" est surtout vraie en périphérie ; le cœur HTTP est resté sur le modèle "chaque module se débrouille".

### Récupérable ou à jeter ?

**Récupérable sans hésiter.** Le projet a deux ou trois bons os — les interfaces `Module`, le registry, le système de schémas embed — qui supporteraient un refactoring progressif. Jeter serait du gâchis : toute la couche `config`, `auth`, `persistence`, `scheduler`, `cache`, `template`, `moduleconfig`, `errhandling` est réutilisable telle quelle ou avec des corrections mineures. Le gros du travail concerne 3 fichiers : `output/http_request.go` (1641 LOC), `input/http_polling.go` (1057 LOC), `filter/http_call.go` (894 LOC).

### Notes

| Critère | Note | Justification courte |
|---|---|---|
| Lisibilité | **7/10** | Noms explicites, commentaires godoc riches. Pollués par du log verbeux. |
| Cohérence archi | **6/10** | Clean en périphérie, incohérent au centre HTTP. |
| Maintenabilité | **5/10** | Duplications HTTP, types dupliqués (HTTPError, RetryInfo, keyEntry), 5 fichiers > 800 LOC. |
| Testabilité | **7/10** | Bon ratio tests, interfaces étroites. Mais tests volumineux trahissent un couplage fort à l'implémentation. |
| Évolutivité | **6/10** | Registry correct ; mais ajouter un module HTTP aujourd'hui, c'est copier/coller `http_polling.go`. |

---

## 2) Architecture & structure

### 2.1 Les 17 packages `internal/` : responsabilités

Le projet annonce 18 packages ; il y en a réellement **17** (`internal/modules/{input,filter,output}` comptés comme 3). Responsabilités :

| Package | Rôle | Chevauchement / problème |
|---|---|---|
| `auth` | Handlers d'auth HTTP (api-key, bearer, basic, oauth2) | **Clean.** Bien isolé. |
| `cache` | LRU + TTL thread-safe | **Clean.** Usage limité à `http_call`/`sql_call`. |
| `cli` | Formatage des erreurs/sorties | OK, petit. |
| `config` | Parse YAML/JSON + JSON Schema + converter → Pipeline | OK ; `converter.go` un peu obscur (2 aller-retours JSON). |
| `database` | Driver detection, pool, placeholder conversion | **Clean.** Bon. |
| `errhandling` | `ClassifyError`, `RetryExecutor`, `OnErrorStrategy` | ⚠️ **`RetryExecutor` non utilisé par `http_request.go` qui réimplémente son propre `retryLoop`.** |
| `factory` | `CreateInputModule/FilterModule/OutputModule` | OK, très fin (188 LOC). Un peu de logique dupliquée avec `registry/builtins.go` pour condition. |
| `logger` | Wrapper `slog` + helpers d'exécution | ⚠️ **771 LOC pour wrapper slog** — surdimensionné, et `ExecutionContext`/`ErrorContext`/`ExecutionMetrics` auraient pu vivre dans `runtime`. |
| `moduleconfig` | Parsing générique + types partagés + navigation nested paths | ⚠️ **Trois responsabilités non reliées** : `parse.go` (generic parser), `shared.go` (types `HTTPRequestBase` etc.), `nested.go` (GetNestedValue/SetNestedValue). À éclater. |
| `modules/{input,filter,output}` | Implémentations concrètes | ⚠️ Voir duplications section 3. |
| `pathutil` | Valide un chemin de fichier | OK, trivial. |
| `persistence` | `StateStore` (tempfile+rename atomique) + `StatePersistenceConfig` + `ExtractLastID` | **Clean.** Bien. |
| `registry` | Map type→constructor thread-safe + `builtins.go` | OK. Condition est enregistré deux fois (registry ET factory). |
| `runtime` | `Executor` + `MetadataAccessor` + `stageTimings` | ⚠️ **`metadata.go` réimplémente `getNestedValue`/`setNestedValue`** alors que `moduleconfig/nested.go` les fournit déjà (cf. `runtime/metadata.go:186-262`). |
| `scheduler` | CRON + graceful shutdown | OK. Bon usage de `robfig/cron`. |
| `template` | Évaluateur `{{record.field}}` + cache de parse | **Clean.** Bon. |

### 2.2 Pattern Registry + Factory

```go
// registry/registry.go — correct
var inputRegistry = make(map[string]InputConstructor)
func RegisterInput(t string, c InputConstructor) { ... }
func GetInputConstructor(t string) InputConstructor { ... }
```

**Bien implémenté** mais deux griefs :

1. **`init()` unique dans `registry/builtins.go`** (pas 7 comme l'énoncé le suggère). Le vrai problème est ailleurs : `factory/modules.go:32` fait `filter.NestedModuleCreator = CreateFilterModuleFromNestedConfig`. C'est un **singleton global modifié à l'init**. Si un utilisateur oublie d'importer `factory`, les `then`/`else` imbriqués deviennent muets. Le commentaire à `filter/condition.go:52-68` le reconnaît. C'est un contournement de cycle d'import, mais mal nommé : ce n'est pas une API, c'est une dépendance cachée.

2. **Double enregistrement du `condition`** : `registry/builtins.go:67-78` enregistre une version "basique sans nested", et `factory/modules.go:76-78` a un `if cfg.Type == "condition"` qui contourne la registry pour créer la version complète. Cet aveu de design bancal devrait disparaître — soit la registry supporte nativement les nested, soit condition devrait être supprimé de `builtins.go`.

### 2.3 `pkg/` vs `internal/`

**Respect impeccable.** `pkg/connector` expose uniquement les types de données (`Pipeline`, `ModuleConfig`, `AuthConfig`, `RetryConfig`, `ExecutionResult`, `RetryInfo`, `CredentialsXxx`) + deux helpers (`CalculateDelay`, `GetTimeoutDuration`). Aucune logique métier n'a fui. 228 LOC bien cadrés. **À préserver tel quel.**

### 2.4 Dépendances circulaires / god objects

Pas de cycles détectés. Un seul cas limite : `filter.NestedModuleCreator` est une variable globale **pour éviter** un cycle `factory → filter → registry → factory`. C'est techniquement acceptable mais fragile.

**God object candidat** : `HTTPRequestModule` (output) porte 36 méthodes sur 1641 LOC, dont 17 dédiées au retry et 5 à la preview. C'est un gros bloc.

---

## 3) Qualité du code

### 3.1 Duplications significatives — c'est le chantier n°1

| # | Élément dupliqué | Emplacements | Effort de correction |
|---|---|---|---|
| **D1** | **`type HTTPError struct`** | `input/http_polling.go:53` vs `output/http_request.go:77` (2 champs en +) | Moyen — créer un `httpclient` shared |
| **D2** | **`func createHTTPClient(timeout)`** | `input/http_polling.go:166` vs `output/http_request.go:315` | Petit |
| **D3** | **Navigation nested path (`getFieldValue`, `getNestedValue`, `GetNestedValue`)** | `moduleconfig/nested.go:28` (public), `runtime/metadata.go:186` (privé), `output/http_request.go:1378` (`getFieldValue`, privé) — le dernier ne gère même pas les arrays | Petit |
| **D4** | **Boucle de retry HTTP** | `output/http_request.go:656-693` (`retryLoop`, ~250 LOC avec ses helpers) alors que `errhandling.RetryExecutor.Execute` existe (`errhandling/retry.go:232`) | Grand — incorporer Retry-After et retryHintFromBody dans `RetryExecutor` |
| **D5** | **`type keyEntry struct{field, paramType, paramName string}`** | `output/http_request.go:100` et `filter/http_call.go:87` | Petit — déjà typé dans `moduleconfig.KeyConfig` |
| **D6** | **Parsing d'`onError`** | `errhandling.ParseOnErrorStrategy`, `filter/condition.go:270 normalizeOnError`, `filter/sql_call.go:171 resolveOnError`, `config/converter.go:289 resolveOnError` (autre sémantique), `filter/mapping.go:124-129` inline | Petit — unifier sur `errhandling.ParseOnErrorStrategy` |
| **D7** | **`stripMetadataFromRecord`** | `output/http_request.go:1310-1340` duplique `runtime.MetadataAccessor.StripCopy` (`runtime/metadata.go:148`) — le commentaire à `output/http_request.go:1305` l'avoue pour éviter un cycle d'import | Moyen — faire de `metadata` son propre package |
| **D8** | **`RetryInfo`** | `connector.RetryInfo` (pkg) et `errhandling.RetryInfo` (internal) — même concept, champs différents | Petit — supprimer celui d'`errhandling`, utiliser le public |
| **D9** | **`defaultRetryableStatusCodes`** | `pkg/connector/retry.go:18` et `errhandling/errors.go:409` — mêmes valeurs, deux API | Petit |
| **D10** | **Extraction d'auth handlers pour preview** | `addMaskedAuthHeaders` (`http_request.go:1564`) vs `addUnmaskedAuthHeaders` (`http_request.go:1588`) font du reverse-engineering du handler (construction d'une fausse requête HTTP). Pattern fragile. | Moyen — ajouter `PreviewHeaders(masked bool)` à `auth.Handler` |

**Recommandation structurante** : créer `internal/httpclient/` qui exporte :
- `Client` (HTTP client avec timeout, transport optimisé)
- `Error` (anciennement HTTPError, unifié)
- `Retry(ctx, fn)` construit sur `errhandling.RetryExecutor` + Retry-After + retryHintFromBody
- `HeaderValidator` (RFC 7230)
- `TemplateEndpoint(endpoint, record, keys)` → URL résolue

Cela ferait tomber ~600 LOC dans `http_request.go` et ~400 dans `http_polling.go`.

### 3.2 Interfaces

Les 3 interfaces cœur sont **exemplaires de minimalisme** :
```go
input.Module:  Fetch(ctx) ([]map[string]interface{}, error) + Close() error
filter.Module: Process(ctx, records) ([]map[string]interface{}, error)
output.Module: Send(ctx, records) (int, error) + Close() error
```
Les capacités optionnelles passent par type assertions (`RetryInfoProvider`, `PreviewableModule`, `StatePersistentInput`). **C'est idiomatique et à conserver.**

**Problèmes** :
- `output.PreviewableModule` est **HTTP-spécifique par design** (`RequestPreview` parle de `Endpoint`, `Method`, `Headers`, `BodyPreview`). Le commentaire à `output/output.go:159-170` le reconnaît. Pour un vrai connecteur DB preview, il faudra soit tordre le type, soit créer `output.DBPreviewableModule`. **Abstraction fuyante.**
- `StatePersistentInput` (`runtime/pipeline.go:64`) a 5 méthodes — c'est beaucoup pour une interface optionnelle. Réflexion : cette persistance ne devrait peut-être pas être pilotée par l'exécuteur mais par le module lui-même.

### 3.3 Gestion d'erreurs — incohérente

Chaque module a son propre type d'erreur : `MappingError`, `ConditionError`, `ScriptError`, `HTTPCallError`, `HTTPError` (x2), `ClassifiedError`, `MyModuleError`. Aucune interface commune, aucune convention sur les champs (`Code` existe parfois, `Details` parfois). Le runtime ne les inspecte pas (il utilise juste `errhandling.ClassifyError`).

**Classification** : bonne idée, bien centralisée dans `errhandling/errors.go`. Mais pas toujours appliquée : `filter/http_call.go:623` crée un `newHTTPCallError` à partir de `ClassifyNetworkError` mais sans wrap — la classification est **perdue** pour le code amont. À corriger systématiquement avec `fmt.Errorf("...%w", classifiedErr)`.

**Codes d'erreur** : chaque module déclare ses propres `ErrCodeXxx` string. Pas de registry centralisé. Les codes `INPUT_FAILED`, `FILTER_FAILED`, `OUTPUT_FAILED` dans `runtime/pipeline.go` coexistent avec 30+ codes au niveau modules. Pas bloquant mais illisible à l'échelle.

### 3.4 Les `init()` pour enregistrement

**Il n'y en a qu'un** réellement utile (`registry/builtins.go:15`) — l'énoncé est inexact. Le second (`factory/modules.go:29`) fait l'injection du `NestedModuleCreator`. Cette approche est **acceptable en Go** (c'est exactement la philosophie de `database/sql` et des drivers), mais deux points :

- L'approche par init() empêche de **désactiver** un module builtin (utile pour des builds restreints).
- Pour un projet avec 10+ modules à terme, une alternative serait un `registry.Default()` explicitement appelé depuis `main`, au lieu d'un init() opaque.

Aujourd'hui : **pas urgent à changer**. Laisser tel quel.

### 3.5 Anti-patterns Go

| Problème | Localisation | Sévérité |
|---|---|---|
| `interface{}` partout au lieu de `any` (Go 1.24) | Tout le projet | Cosmétique |
| Variable globale mutable `filter.NestedModuleCreator` | `filter/condition.go:68` | Moyen, cf. §2.2 |
| `context.Context` dans struct via `ctx context.Context` field dans `scheduler.Scheduler` | `scheduler/scheduler.go:101` | Mineur — go vet râle normalement, mais dans ce cas c'est le cycle de vie du scheduler |
| Pas de `defer cancel()` systématique quand `context.WithTimeout` est créé dans `persistence`/autres | À auditer | Mineur |
| `for strings.Contains(result, "?")` dans `database.ConvertPlaceholders` — O(n²) et ne gère pas les `?` dans strings SQL | `database/database.go:297` | **Bug potentiel** — injection possible si valeur contient `?` |
| `webhook.go` utilise `os.Signal` pour SIGINT/SIGTERM mais c'est déjà fait dans `main.go` → double handler | `input/webhook.go:17-22` import | À vérifier |
| Race possible entre `scheduler.running` et `queue` channel | `scheduler/scheduler.go:71-81` | À vérifier ; présence d'un `sync.Mutex` mais queue est channel |
| Pas de fuite de goroutine visible | — | OK |
| Pas de fuite de ressource HTTP : `Close()` appelé correctement dans `runtime/pipeline.go:405-406`, `509-511` | — | OK |

---

## 4) Tests

### 4.1 Couverture : ratio 1.88:1 (34 009 test / 18 367 code)

Ratio globalement sain mais **très mal réparti** :

| Fichier | LOC code | LOC tests | Ratio |
|---|---|---|---|
| `output/http_request.go` | 1641 | **4461** | 2.72 |
| `filter/mapping.go` | 899 | **2309** | 2.57 |
| `filter/condition.go` | 654 | **2218** | 3.39 |
| `runtime/pipeline.go` | 901 | 2478 | 2.75 |
| `input/webhook.go` | 820 | **1641** | 2.00 |
| `filter/http_call.go` | 894 | 1767 | 1.98 |
| `filter/script.go` | 556 | 1396 | 2.51 |
| `scheduler/scheduler.go` | 727 | 1393 | 1.92 |
| `input/http_polling.go` | 1057 | 1336 | 1.26 |
| `filter/sql_call.go` | 600 | **175** | **0.29** |
| `output/database.go` | 377 | 258 | 0.68 |
| `input/database.go` | 613 | 342 | 0.55 |

**Points chauds sous-testés** :
- `filter/sql_call.go` : 600 LOC pour 175 LOC de test (29 %). Module critique (exécution SQL, cache) sous-testé.
- `input/database.go` : 55 %. L'incrémental (timestamp/ID) est complexe et pas assez couvert.
- `output/database.go` : 68 %.

**Points chauds sur-testés** (signal d'alarme) :
- `condition_test.go` à 3.39x suggère que le code est **couplé à son implémentation** (chaque path est testé unitairement plutôt que par la combinaison).
- `http_request_test.go` à 4461 LOC (111 KB) : impossible à relire d'un trait. **C'est un symptôme de god object sous test**, pas de rigueur.

### 4.2 Fichiers test volumineux : design problem ou réalité ?

**Design problem.** `http_request.go` est lui-même un god object (1641 LOC, 36 méthodes, 4 modes : batch/single/template/preview × retry × auth × header templating). Les 4461 LOC de tests reflètent la combinatoire de responsabilités. Si on split en `httpclient` + `httprequest` + `preview` + `headers`, les tests suivront naturellement.

### 4.3 Isolation

Sur ce que j'ai vu :
- Beaucoup de tests unitaires vrais (`TestParseOnErrorStrategy`, `TestGetNestedValue_*`)
- Tests HTTP utilisent `httptest.NewServer` (bien)
- `database_integration_test.go` fait du vrai SQL sur SQLite (bien)
- Mais : `mapping_test.go` à 2309 LOC suggère **beaucoup de duplication de fixtures**. Un builder pattern ou des helpers factoriserait 30 %.

---

## 5) Chantiers prioritaires

### P1 — Critiques

#### P1.1 — Créer `internal/httpclient` et factoriser les 3 modules HTTP
**Problème** : `output/http_request.go` (1641), `input/http_polling.go` (1057), `filter/http_call.go` (894) réimplémentent chacun : client HTTP, retry, HTTPError, headers builder, endpoint resolution, auth application, response parsing. Duplications D1/D2/D4/D5/D10 ci-dessus.
**Impact** : maintenance 3× plus chère, comportements divergents (seul `http_request.go` supporte Retry-After et `retryHintFromBody`, `http_polling.go` et `http_call.go` ne les ont pas — bug). Ajouter un paramètre comme "circuit breaker" demande 3 PRs.
**Solution** : extraire `internal/httpclient` exportant `Client`, `Error`, `DoWithRetry`, `BuildHeaders`, `ResolveEndpoint`. Faire évoluer `errhandling.RetryExecutor` pour supporter Retry-After et hints body (il manque juste une callback `OnRetry(err) (nextDelay, bool)`).
**Effort** : **grand** (3-4 jours).

#### P1.2 — Corriger les bugs P0 du `docs/plan.md` encore présents
**Problème** : l'audit précédent a listé 8 bugs critiques. Je confirme que trois touchent encore le code :
- `webhook` : `input-schema.json` définit `path`, `webhook.go:145` lit `endpoint` — config conforme au schéma = échec runtime.
- `oauth2` : schéma définit `scope`, code lit `scopes`.
- `condition.then`/`else` : schéma définit `array`, factory parse un objet imbriqué (mais ça a l'air d'avoir été corrigé, à revérifier avec un test de conformité schéma).
**Impact** : bugs fonctionnels silencieux, config "valide" ne fonctionne pas.
**Solution** : relire chacun des 8 bugs du plan.md, écrire un test par bug, fixer.
**Effort** : **moyen** (1-2 jours).

#### P1.3 — Unifier la navigation nested path
**Problème** : 3 implémentations (D3). `moduleconfig.GetNestedValue` est la plus complète (gère `items[0]`), `runtime/metadata.go:getNestedValue` ne gère pas les arrays, `output/http_request.go:getFieldValue` non plus.
**Impact** : bugs subtils quand un record a une clé type `items[0]`, comportement incohérent selon le module qui lit.
**Solution** : exporter `moduleconfig.GetNestedValue` partout, supprimer les deux autres. Si cycle d'import : déplacer `nested.go` dans `internal/pathutil` ou créer `internal/recordpath`.
**Effort** : **petit** (0.5 jour).

#### P1.4 — Unifier `RetryInfo`
**Problème** : `connector.RetryInfo` (pkg, public) et `errhandling.RetryInfo` (internal, plus riche) cohabitent. Conversion manuelle à chaque frontière.
**Solution** : étendre `connector.RetryInfo` avec les champs manquants (`TotalDuration`, `Errors`), supprimer `errhandling.RetryInfo`.
**Effort** : **petit**.

### P2 — Importantes

#### P2.1 — Split des god files
- `output/http_request.go` (1641) → `http_request.go` (~400) + `http_request_retry.go` (si pas encore migré vers httpclient) + `http_request_preview.go` (~200) + `http_request_headers.go` (~200)
- `input/http_polling.go` (1057) → `http_polling.go` (~400) + `http_polling_pagination.go` (~400) + `http_polling_state.go` (~150)
- `filter/mapping.go` (899) → `mapping.go` (~300) + `mapping_transforms.go` (~400) + `mapping_parse.go` (~150)
**Effort** : **moyen** (2-3 jours).

#### P2.2 — Remplacer les 4 variantes d'`onError` par `errhandling.ParseOnErrorStrategy`
**Problème** : duplication D6.
**Solution** : un seul point d'entrée, typé `OnErrorStrategy` partout.
**Effort** : **petit** (0.5 jour).

#### P2.3 — Sortir `MetadataAccessor` en package dédié
**Problème** : `runtime/metadata.go` est tiré par `output/http_request.go` qui ne peut pas l'importer (cycle → duplication D7).
**Solution** : `internal/metadata/` package autonome (302 LOC + tests existants).
**Effort** : **petit** (0.5 jour).

#### P2.4 — Supprimer `output/template.go` (re-export de compatibilité)
**Problème** : `output/template.go` (60 LOC) est un wrapper trivial autour de `internal/template`. Le commentaire dit "backward compatibility" mais c'est du code interne.
**Solution** : remplacer `output.TemplateEvaluator` par `template.Evaluator` dans `output/http_request.go`, supprimer le fichier.
**Effort** : **petit**.

#### P2.5 — Harmoniser les types d'erreur des modules
**Problème** : `MappingError`, `ConditionError`, `ScriptError`, `HTTPCallError` ont chacun une structure différente.
**Solution** : interface `ModuleError { Code() string; Module() string; Details() map[string]any }`, chaque erreur l'implémente. Permettrait à runtime d'enrichir `ExecutionError.Details` uniformément.
**Effort** : **moyen**.

#### P2.6 — Test de conformité schema/runtime automatique
**Problème** : plan.md mentionne 8 champs où schéma et code divergent. Pas de garde-fou.
**Solution** : fichier `internal/config/schema_compliance_test.go` qui charge chaque `configs/examples/*.yaml`, parse, convertit en Pipeline, instancie tous les modules sans exécuter. Échoue si un champ du schéma est ignoré.
**Effort** : **moyen** (1 jour, à amortir).

### P3 — Nice-to-have

- **P3.1** : dégraisser `logger/logger.go` (771 LOC) en conservant `slog` brut là où les helpers d'exécution n'apportent rien.
- **P3.2** : supprimer `NewExecutor` (`runtime/pipeline.go:102`) non utilisé, ne garder que `NewExecutorWithModules`.
- **P3.3** : unifier `ExecuteWithContext` et `ExecuteWithRecordsContext` (`pipeline.go:388` et `:802`) via une interface commune sur la source de records.
- **P3.4** : fix `database.ConvertPlaceholders` (O(n²), ne gère pas `?` dans strings — voir §3.5).
- **P3.5** : trace ID / correlation ID dans les logs (plan.md §7.1).
- **P3.6** : remplacer `defaultRetryableStatusCodes` dupliqué par `connector.DefaultRetryableStatusCodes()`.
- **P3.7** : remplacer `interface{}` par `any` dans les signatures publiques (cosmétique).

---

## 6) Évolutivité

### 6.1 Ajouter un nouveau module aujourd'hui

**Filter** (mapping, set, remove sont de bons modèles) :
1. Créer `internal/modules/filter/xyz.go` avec type + `NewXyzFromConfig` + méthode `Process`.
2. Ajouter l'entrée dans `registry/builtins.go:49-144`.
3. Ajouter le type dans `validInputTypes`/`validFilterTypes`/`validOutputTypes` de `config/validator.go:156-159`.
4. Mettre à jour le JSON Schema (`internal/config/schema/filter-schema.json`).
5. Ajouter un exemple dans `configs/examples/`.

**Simple et clean** — bon point.

**Input / Output HTTP-like** : là, c'est **très douloureux** aujourd'hui. On copie/colle 800+ LOC. C'est la raison de P1.1.

### 6.2 Flexibilité de la config YAML/JSON

**Très bonne** grâce à :
- `connector.ModuleConfig.Raw` (json.RawMessage) + generics `ParseModuleConfig[T]` : chaque module définit ses champs sans toucher au cœur.
- `connector.ModuleBase` embedded pour les champs communs (`OnError`, `TimeoutMs`, `Enabled`, `Tags`).
- Schéma JSON découpé (pipeline / common / input / filter / output / auth) — maintenance réaliste.
- `applyDefaults` fait un merge granulaire pour retry/timeout/onError depuis le bloc `defaults`.

**Limites** :
- L'aller-retour map→JSON→struct dans `converter.go` (via `moduleBuilder.finalize`) est coûteux pour de grosses configs.
- Pas de support des variables `${ENV}` dans les champs autres que `connectionStringRef` (limitation explicite dans `database.go:ResolveEnvRef`).
- Pas de champ `$ref` pour factoriser des blocs d'auth entre pipelines.

### 6.3 Blocages techniques pour évolutions futures

| Fonctionnalité visée | Blocage | Effort pour débloquer |
|---|---|---|
| **Streaming** (chunked records) | `Module.Fetch([]map[string]interface{}, error)` retourne tout en mémoire. Aucun canal, aucun itérateur. | Grand — rework complet du contrat `Module` avec probablement `chan []record` + variante batch. |
| **Mode distribué** (plusieurs workers consomment le même pipeline) | `scheduler.Scheduler.running` est un flag local ; pas de lock distribué. `StateStore` utilise le filesystem local. | Grand — backend pluggable pour le state (`StateStore` interface existe, bon point), et scheduler en mode "coordinator only" + workers. |
| **UI de monitoring** | Pas d'endpoint HTTP exposé, pas de metrics structurés. `ExecutionResult` est retourné en fin d'exécution, pas streamé. | Moyen — server HTTP admin + exposition via registry des pipelines actifs + events bus. |
| **Plugins externes (.so)** | L'`init()`-based registry fonctionnerait avec `plugin.Open`, mais Go plugin système est fragile (versions compilateur). | Moyen à grand — préférer un modèle out-of-process (gRPC). |
| **Validation configuration multi-pipeline** (cross-references) | `ConvertToPipeline` n'opère qu'à l'échelle d'un fichier. | Petit. |

---

## 7) Éléments positifs à préserver

### 7.1 Bonnes décisions techniques à ne pas toucher

1. **Séparation `pkg/connector` minimaliste** (228 LOC, 12 types). **Parfait** — à défendre. Ne pas y mettre de logique métier.
2. **Interfaces `Module` à 1-2 méthodes** avec capacités optionnelles via type assertion. Idiomatique Go.
3. **`ParseConfig[T any](raw)` avec generics** (`moduleconfig/parse.go:15`). Élégant, testable, typé.
4. **JSON Schema embed avec `//go:embed`** (`config/validator.go:17-33`) — livrable autoporteur, pas de dépendance externe.
5. **`StateStore` avec atomic write (tempfile + rename)** (`persistence/state.go:120-143`). Correct, fiable.
6. **`LRUCache` simple et thread-safe** (`cache/lru.go`), 211 LOC propres.
7. **`auth.Handler` + `OAuth2Invalidator` interface optionnelle** (`auth/auth.go:41-45`) — bon usage des interfaces.
8. **Context propagé partout**, cancellation respectée, `context.DeadlineExceeded` classifié correctement.
9. **`cron/v3` avec `cron.SecondOptional`** pour supporter 5 ET 6 champs (`scheduler/scheduler.go:125`).
10. **HMAC-SHA256 avec `crypto/subtle.ConstantTimeCompare`** pour webhook signatures (`input/webhook.go:5-10`).
11. **`Goja` sandbox avec `runtime.Interrupt()`** et monitoring de contexte (`filter/script.go`).
12. **Fermeture de l'input module immédiatement après fetch** pour libérer le pool HTTP tôt (`runtime/pipeline.go:504-511`) — optimisation pertinente.
13. **Masking des credentials en dry-run** (`output/http_request.go:1564-1584`) — sécurité par défaut.

### 7.2 Patterns bien implémentés qui servent de modèle

- **`filter/set.go` + `filter/remove.go`** (119 et 145 LOC) : **voici à quoi ressemble un module bien écrit.** Constructeur + `Process` + `ParseXxxConfig` + compile-time interface check (`var _ Module = ...`). Aucun client HTTP, aucune duplication.
- **`persistence/state.go`** : gestion propre du fichier, atomic write, erreurs loggées au niveau warn, jamais fatales (le pipeline continue sans persistence si load échoue).
- **`moduleconfig/nested.go`** : fonctions pures, bien nommées, testables.
- **`runtime/pipeline.go:executePipelineStages`** (`:489`) : orchestration séquentielle bien structurée, chaque étape retourne `(result, duration, err)`, les timings sont agrégés en fin.

---

## 8) Verdict final

### Repartir à zéro ou refactorer ?

> **Refactorer**, sans discussion. Le squelette architectural (pkg/internal, interfaces Module, registry/factory, config avec schéma embed, executor découpé) est bon. Le vice est **concentré** sur 3 fichiers HTTP et quelques duplications. Une réécriture coûterait 3-6 mois et reperdrait des acquis (tests, fix de bugs historiques, compréhension de Goja et jsonschema).

### Premier chantier à attaquer

**P1.1 — Extraction de `internal/httpclient`**. C'est là que le ratio impact/effort est le plus élevé :
- Fait tomber ~1000 LOC de code production (~30 % du code HTTP actuel)
- Fait tomber ~2000-3000 LOC de tests dupliqués
- Règle automatiquement les divergences comportementales (Retry-After absent de http_polling et http_call)
- Débloque P1.4 (unification RetryInfo) et P2.1 (split des god files)

### Roadmap 3 mois pour un développeur seul

**Mois 1 — Stabilisation** (objectif : plus aucun bug critique du plan.md)
- **Semaines 1-2** : P1.2 (8 bugs critiques du plan.md) — test par bug, fix par bug, un PR par bug. Cimenter la non-régression.
- **Semaine 3** : P2.6 (test de conformité schéma). Établir le filet de sécurité avant de toucher au HTTP.
- **Semaine 4** : P1.3 (unification nested path) + P1.4 (unification RetryInfo) + P2.2 (unification onError) + P2.3 (metadata en package). Ce sont des petits wins qui nettoient le terrain avant le gros chantier.

**Mois 2 — Gros œuvre HTTP** (objectif : abolir les duplications D1-D5)
- **Semaines 5-6** : créer `internal/httpclient` avec `Client`, `Error`, tests unitaires. Adapter `errhandling.RetryExecutor` pour supporter Retry-After et retryHintFromBody via callback.
- **Semaine 7** : migrer `output/http_request.go` sur httpclient. -500 LOC. Tests passent.
- **Semaine 8** : migrer `input/http_polling.go` et `filter/http_call.go`. Ils gagnent Retry-After/retryHintFromBody "gratuitement". -800 LOC combiné.

**Mois 3 — Finitions et ergonomie** (objectif : code idiomatique et observable)
- **Semaine 9** : P2.1 (split god files). `http_request.go` passe de 1641 à ~600 max. `mapping.go` aussi.
- **Semaine 10** : P2.5 (uniformiser les erreurs de modules) + P3.1 (dégraisser logger).
- **Semaine 11** : P3.5 (trace ID dans logs) + audit secrets dans logs (plan.md §7.1).
- **Semaine 12** : tests E2E par type de module (1 pipeline complet par input×output) + doc utilisateur mise à jour. Rédaction d'un `CONTRIBUTING` avec pas-à-pas "ajouter un nouveau module".

### Ce que j'éviterais dans les 3 prochains mois

- **CS-4 du plan.md** (schema-to-struct generation) : trop d'effort, bénéfice marginal tant que les structs actuelles fonctionnent.
- **Refonte complète de Pipeline streaming** : demanderait de casser `Module.Fetch`, hors périmètre.
- **Ajout de nouvelles fonctionnalités** (nouveaux modules, nouveau output type) : prioriser la dette avant d'en ajouter.

---

**Dernier mot** : le projet est tenu par une personne qui connaît Go et documente ce qu'elle fait (REFACTORING_NOTES, plan.md, audit précédent très pertinent). Le problème n'est pas la compétence, c'est **l'absence de passage "feature-complete → cleanup"** sur la couche HTTP. Un trimestre concentré dessus suffit.
