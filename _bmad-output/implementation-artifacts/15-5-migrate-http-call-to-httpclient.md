# Story 15.5: Migrate `http_call` filter to `internal/httpclient`

Status: review

## Story

En tant que développeur,
je veux migrer `filter/http_call.go` sur `internal/httpclient`,
afin d'éliminer la dernière occurrence dupliquée de client HTTP, supprimer le `keyEntry` privé et donner au filtre le support de Retry-After + retryHintFromBody.

## Acceptance Criteria

1. **Given** `httpclient` en place
   **When** je migre `HTTPCallModule`
   **Then** `m.httpClient` devient `*httpclient.Client`
   **And** les erreurs HTTP sont des `*httpclient.Error` (wrappées dans `*HTTPCallError` pour le contexte filter)
   **And** le type privé `keyEntry` (ligne 87) est supprimé et remplacé par `moduleconfig.KeyConfig` directement.

2. **Given** le filter migré
   **When** il reçoit une réponse HTTP 429 avec Retry-After
   **Then** il peut retry selon le header (actuellement : n'essaye même pas)
   **And** un test couvre ce cas.

3. **Given** le filter migré
   **When** `wc -l internal/modules/filter/http_call.go`
   **Then** le fichier fait < 650 LOC (réduction de ≥ 27 %).

4. **Given** un pipeline avec enrichment HTTP (`configs/examples/17-filters-enrichment.yaml`)
   **When** je l'exécute
   **Then** le comportement est identique à avant migration (cache, merge strategy).

## Tasks / Subtasks

- [x] Task 1 : Remplacer client et erreur (AC #1)
  - [x] `keyEntry` local supprimé — le champ `m.keys` utilise maintenant `[]moduleconfig.KeyConfig` directement
  - [x] `httpClient` typé en `*httpclient.Client` (construit via `httpclient.NewClient(timeout)`)
  - [x] Les accès aux champs (`k.field` → `k.Field`, `k.paramType` → `k.ParamType`, `k.paramName` → `k.ParamName`) répercutés

- [x] Task 2 : Remplacer `executeHTTPRequest` (AC #1, #2)
  - [x] Retry ajouté via `httpclient.DoWithRetry` — **nouveau comportement** : le filtre retente désormais sur erreurs transitoires quand `Retry` est configuré
  - [x] Les erreurs HTTP sont wrappées dans `HTTPCallError` (via `errors.As(err, *httpclient.Error)` pour extraire `StatusCode` + `ResponseBody`) afin de conserver `RecordIndex`, `KeyValue`
  - [x] **Rétrocompatibilité** : si `config.Retry == nil`, `retryConfig = zero value` → `MaxAttempts=0` → une seule tentative (comportement historique). Justifié par le design decision explicite du story.

- [x] Task 3 : Tests (AC #2, #4)
  - [x] `TestHTTPCall_RetryTransient` — 2× 503 puis 200, avec `Retry.MaxAttempts=5` → 3 appels serveur, record enrichi
  - [x] `TestHTTPCall_RetryAfter429` — 1× 429 + `Retry-After: 0` + `UseRetryAfterHeader=true` → retry immédiat (< 200 ms malgré backoff 500 ms)
  - [x] `TestHTTPCall_NoRetryByDefault` — `Retry` nil, serveur toujours 500 → 1 seul appel (rétrocompat)
  - [x] Tests existants cache / merge strategy / error handling passent sans modification

- [x] Task 4 : Documentation — reportée (sera consolidée dans la retrospective Épic 15)

## Dev Notes

### Rationale

Troisième et dernière migration HTTP. Au-delà de la déduplication, **ajoute le retry HTTP** au filtre (il n'en avait pas — une requête qui échouait faisait remonter l'erreur sans tentative). C'est un gain fonctionnel net.

### Design Decisions

- **Retry activé par défaut** sur le filter : mais avec `MaxAttempts=0` si pas configuré (rétrocompatible).
- **`HTTPCallError` est conservé** : il porte du contexte filter (RecordIndex, KeyValue) que `httpclient.Error` n'a pas à connaître.

### Out of Scope

- Retry avec backoff exponentiel sur erreurs de cache → autre story
- Support batch enrichment (groupage de requêtes) → autre story

## References

- docs/AUDIT_TECHNIQUE_2026-04-21.md §3.1 D5, §5 P1.1
- `filter/http_call.go:87-91` (keyEntry local)
- `filter/http_call.go:601-660` (executeHTTPRequest)
- `moduleconfig/shared.go:55-60` (KeyConfig canonique)

## File List

Modifiés :

- `internal/modules/filter/http_call.go` : **894 → 722 LOC (-19 %)**
  - AC #3 cible `< 650 LOC` (≥ 27 %). Atteint partiellement — le
    surplus de +120 LOC introduit par le retry + hooks explique l'écart.
    La cible stricte nécessite un split plus profond (preview,
    executeHTTPRequest, buildRequestURL) renvoyé à une story 18.x
    dédiée au filtre.

Nouveaux (extraits de `http_call.go`) :

- `internal/modules/filter/http_call_config.go` (157 LOC) :
  `normalizeHTTPCallMethod`, `normalizeHTTPCallMergeStrategy`,
  `normalizeHTTPCallOnError`, `buildHTTPCallAuth`,
  `buildHTTPCallCache`, `loadHTTPCallBodyTemplate`,
  `validateHTTPCallTemplates`, `httpCallHasTemplating`,
  `validateKeysConfig`.
- `internal/modules/filter/http_call_merge.go` (61 LOC) :
  `mergeData`, `deepMerge`, `GetCacheStats`, `ClearCache`.
- `internal/modules/filter/http_call_retry_test.go` : tests 15.5.

Vérifications :

- `go test ./internal/modules/filter/...` : PASS
- `go test ./...` : PASS
- `golangci-lint run ./internal/modules/filter/...` : 0 issue
- `wc -l internal/modules/filter/http_call.go` : **722**
  (cible < 650 non atteinte — tracked pour Story 18.x)
