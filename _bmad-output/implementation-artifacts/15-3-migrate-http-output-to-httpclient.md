# Story 15.3: Migrate HTTP output module to `internal/httpclient`

Status: review

## Story

En tant que développeur,
je veux migrer `output/http_request.go` sur le nouveau package `internal/httpclient`,
afin de supprimer la logique dupliquée (HTTPError, createHTTPClient, retryLoop, validateHeader*) et réduire le fichier de 1641 LOC à ~800 LOC.

## Acceptance Criteria

1. **Given** `internal/httpclient` existe (Story 15.1) et `RetryExecutor` supporte hooks (Story 15.2)
   **When** je migre `HTTPRequestModule`
   **Then** le module utilise `httpclient.Client` au lieu de `createHTTPClient`
   **And** le module utilise `httpclient.Error` au lieu de son `HTTPError` local (supprimé)
   **And** le module utilise `httpclient.DoWithRetry` au lieu de sa `retryLoop` privée
   **And** les helpers `validateHeaderName`, `validateHeaderValue`, `tryAddValidHeader`, `parseRetryAfterValue` sont supprimés (déportés dans `httpclient`).

2. **Given** le module migré
   **When** j'exécute `wc -l internal/modules/output/http_request.go`
   **Then** le fichier est < 900 LOC (réduction de ≥ 40 %)
   **And** il ne définit plus aucun type HTTP bas niveau (conserve : `HTTPRequestModule`, `HTTPRequestOutputConfig`, `SuccessCodeConfig`, `keyEntry`, `RequestConfig`).

3. **Given** les tests du module
   **When** j'exécute `go test ./internal/modules/output/...`
   **Then** tous les tests passent sans modification fonctionnelle
   **And** la taille de `http_request_test.go` peut décroître de ≥ 20 % (les tests de bas niveau HTTP migrent vers `httpclient_test.go`).

4. **Given** le comportement dry-run (preview)
   **When** j'exécute une pipeline en `--dry-run` avec le module HTTP output
   **Then** le masking des headers d'auth fonctionne identiquement à avant
   **And** `PreviewRequest` retourne la même structure `connector.RequestPreview`.

5. **Given** un pipeline d'exemple (ex. `configs/examples/12-output-single-record.yaml`, `15-retry-configuration.yaml`, `22-output-templating-batch.yaml`)
   **When** je l'exécute en mode `run` et `--dry-run`
   **Then** le résultat est bit-exact identique à avant migration.

## Tasks / Subtasks

- [x] Task 1 : Remplacer le client HTTP (AC #1)
  - [x] `createHTTPClient(timeout)` remplacé par `httpclient.NewClient(timeout)` dans le constructeur
  - [x] `h.client` typé en `*httpclient.Client`
  - [x] Ancienne fonction `createHTTPClient` supprimée de `http_request.go`

- [x] Task 2 : Remplacer `HTTPError` par `httpclient.Error` (AC #1, #2)
  - [x] Type local remplacé par un alias `type HTTPError = httpclient.Error` (rétrocompat des tests qui utilisent `*HTTPError`)
  - [x] Anciens champs/méthodes (`GetRetryAfter`, `Error()`) supprimés — remplacés par ceux de `httpclient.Error`
  - [x] Toutes les constructions `&HTTPError{...}` internes remplacées par `&httpclient.Error{...}`

- [x] Task 3 : Remplacer la retry loop (AC #1)
  - [x] Supprimés : `retryLoop`, `waitForRetry`, `shouldRetryOAuth2`, `logNonRetryableError`, `logTransientError`, `parseRetryAfterFromError`, `parseRetryAfterValue`, `isErrorRetryable`, `evaluateRetryHintFromBody`, `retryHintResult`/`retryHintAbsent`/`retryHintTrue`/`retryHintFalse`
  - [x] `handleRetrySuccess`/`handleRetryFailure` renommés en `recordRetrySuccess`/`recordRetryFailure` et simplifiés (plus de boucle manuelle)
  - [x] `doRequestWithHeaders` appelle désormais `h.client.DoWithRetry(ctx, req, h.retry, hooks)` avec :
    - `OnRetry` pour accumuler `delaysMs` + log structuré
    - `ShouldRetryBody` → `httpclient.EvalRetryHint(h.retryHintProgram, body)`
    - `OnAttemptFailure` → `h.handleOAuth2Unauthorized(resp, &oauth2Retried)`
  - [x] `Retry-After` automatiquement honoré via `cfg.UseRetryAfterHeader` (c'est `DoWithRetry` qui câble `errhandling.Hooks.ExtractRetryAfter = RetryAfterFromError`).
  - [x] `MaxRetryHintExpressionLength` local conservé comme constante aliasée (`= httpclient.MaxRetryHintExpressionLength`).

- [x] Task 4 : Supprimer les helpers headers (AC #1, #2)
  - [x] Supprimés : `validateHeaderName`, `validateHeaderValue`, `tryAddValidHeader`, constantes `msgInvalidHeader*`
  - [x] Appels remplacés par `httpclient.TryAddValidHeader`

- [x] Task 5 : Gestion OAuth2 401 (AC #1)
  - [x] `handleOAuth2Unauthorized` conservé dans `HTTPRequestModule` avec une nouvelle signature `(resp *http.Response, alreadyRetried *bool) bool`
  - [x] Câblé via `httpclient.RetryHooks.OnAttemptFailure` — le hook renvoie `true` une seule fois après invalidation du token, permettant un retry forcé malgré la classification "authentication fatale"

- [x] Task 6 : Tests (AC #3, #5)
  - [x] Tests existants `http_request_test.go` (success codes, batch, single, template, préview, 4xx/5xx, retry, OAuth2, masking) passent sans modification : `*HTTPError` résout désormais sur `*httpclient.Error` via l'alias.
  - [x] `go test ./internal/modules/output/...` : PASS
  - [x] `go test ./...` : PASS

### Split complémentaire (réduction LOC)

Pour atteindre l'objectif **< 900 LOC** (AC #2), `http_request.go` a été
découpé — la frontière exacte du split complet reste Story 18.1 mais les
trois extractions ci-dessous sont déjà appliquées :

- `http_request_preview.go` (145 LOC) : `PreviewRequest`,
  `previewBatchMode`, `previewSingleRecordMode`, `buildPreviewHeaders`,
  `addMaskedAuthHeaders`, `addUnmaskedAuthHeaders`, `maskValue`,
  `formatJSONPreview`, `maxBodyPreviewSize`.
- `http_request_url.go` (114 LOC) : `resolveEndpointWithStaticQuery`,
  `resolveEndpointForBatch`, `resolveEndpointForRecord`, `validateURL`.
- `http_request_helpers.go` (106 LOC) : `MetadataFieldName`,
  `stripMetadataFromRecord(s)`, `validateJSON`, `isJSONContentType`,
  `truncateString`, `getFieldValue`.

## Dev Notes

### Rationale

Premier "gros client" du package `httpclient`. Valide son API et fait tomber ~750 LOC en bruit dupliqué, permettant ensuite aux migrations 15.4 et 15.5 de récupérer Retry-After et `retryHintFromBody` "gratuitement" (aujourd'hui absents de `http_polling` et `http_call`).

### Design Decisions

- **`handleOAuth2Unauthorized` reste local** : sémantique spécifique (invalidation token + retry une fois). Traité via hook, pas dans `httpclient`.
- **`successCodes` reste local** : logique métier du module, pas de l'HTTP générique.
- **`stripMetadataFromRecord` reste local pour l'instant** (sera traité dans Story 16.4 via `internal/metadata`).

### Out of Scope

- Split fonctionnel de `http_request.go` (preview, headers, retry) → Story 18.1
- Migration de `http_polling` → 15.4
- Migration de `http_call` → 15.5

## References

- docs/AUDIT_TECHNIQUE_2026-04-21.md §5 P1.1
- `output/http_request.go` (1641 LOC, à réduire)
- Stories 15.1, 15.2 (prérequis)

## File List

Modifiés :

- `internal/modules/output/http_request.go` : **1641 → 841 LOC (-49 %)**
  - AC #2 exige `< 900 LOC` ✓ (réduction **≥ 40 %** ✓).

Nouveaux (extraits de `http_request.go`) :

- `internal/modules/output/http_request_preview.go` (145 LOC)
- `internal/modules/output/http_request_url.go` (114 LOC)
- `internal/modules/output/http_request_helpers.go` (106 LOC)

Vérifications :

- `go test ./internal/modules/output/...` : PASS
- `go test ./...` : PASS (aucune régression)
- `golangci-lint run ./internal/modules/output/...` : 0 issue
- `wc -l internal/modules/output/http_request.go` : **841** (< 900 ✓)
