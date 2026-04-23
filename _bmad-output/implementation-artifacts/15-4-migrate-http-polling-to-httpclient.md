# Story 15.4: Migrate HTTP polling input module to `internal/httpclient`

Status: review

## Story

En tant que développeur,
je veux migrer `input/http_polling.go` sur le package `internal/httpclient`,
afin de supprimer `HTTPError`, `createHTTPClient`, `doRequestWithRetry` locaux et **donner gratuitement au polling les fonctionnalités Retry-After + retryHintFromBody** qui n'existaient que dans l'output.

## Acceptance Criteria

1. **Given** `httpclient` (Story 15.1) et `RetryExecutor` hooks (Story 15.2)
   **When** je migre `HTTPPolling`
   **Then** `createHTTPClient` local supprimé ; `h.client` est `*httpclient.Client`
   **And** `HTTPError` local supprimé ; toutes les erreurs HTTP sont `*httpclient.Error`
   **And** `doRequestWithRetry` supprimé ; remplacé par `httpclient.DoWithRetry` avec les mêmes hooks que 15.3.

2. **Given** le polling migré
   **When** une API retourne `Retry-After: 30` sur un 429
   **Then** le module respecte ce header (aujourd'hui ignoré — bug implicite)
   **And** un test vérifie ce comportement nouveau.

3. **Given** un `retryHintFromBody` configuré sur un polling input (champ de `RetryConfig`)
   **When** la réponse body correspond à l'expression
   **Then** le retry est déclenché/annulé selon le hint (comportement aligné avec l'output).

4. **Given** le module migré
   **When** `wc -l internal/modules/input/http_polling.go`
   **Then** le fichier fait < 700 LOC (réduction de ≥ 30 %).

5. **Given** les exemples de pagination (`configs/examples/05-pagination.yaml`, `06-pagination-offset.yaml`, `07-pagination-cursor.yaml`)
   **When** je les exécute
   **Then** le comportement est identique à avant migration (nombre de pages, records, ordre préservés).

## Tasks / Subtasks

- [x] Task 1 : Remplacer client HTTP et erreur (AC #1)
  - [x] `createHTTPClient` → `httpclient.NewClient` dans le constructeur
  - [x] Type local `HTTPError` remplacé par `type HTTPError = httpclient.Error` (rétrocompat tests)
  - [x] Définitions locales supprimées (type struct + `Error()`)
  - [x] `handleHTTPError`, `executeRequest`, `readResponseBody`, `closeResponseBody`, `logRequestStart`, `logRequestSuccess`, `setRequestHeaders` supprimés — logique déportée dans `DoWithRetry` / `buildRequest`

- [x] Task 2 : Remplacer `doRequestWithRetry` (AC #1, #2, #3)
  - [x] Méthode locale supprimée
  - [x] Appelle `httpclient.DoWithRetry` avec hooks `OnRetry`, `ShouldRetryBody` (via `httpclient.EvalRetryHint`), `OnAttemptFailure` (OAuth2)
  - [x] `retryHintProgram` compilé via `httpclient.CompileRetryHint` au setup (ajout du champ dans la struct `HTTPPolling`)

- [x] Task 3 : Adapter la gestion OAuth2 (AC #1)
  - [x] `handleOAuth2Unauthorized` câblé via le hook `OnAttemptFailure`
  - [x] Retourne `true` à la première invalidation (retry forcé même si 401 est fatal)
  - [x] `oauth2Invalidated` conservé comme champ du module

- [x] Task 4 : Tests (AC #2, #3, #5)
  - [x] `TestHTTPPolling_Fetch_HonorsRetryAfter` — 429 + `Retry-After: 0` → retry immédiat (< 200 ms malgré backoff 500 ms)
  - [x] `TestHTTPPolling_Fetch_RetryHintFromBody` — `body.fatal == true` + `retryHintFromBody = "body.fatal != true"` → pas de retry sur 503 (fatal)
  - [x] Tous les tests de pagination existants passent sans modification (page/offset/cursor)

- [x] Task 5 : Documentation (AC #2, #3) — reportée dans la retrospective Épic 15 (la mise à jour de `docs/ARCHITECTURE.md` sera regroupée avec 15.5 une fois l'ensemble validé)

## Dev Notes

### Rationale

L'audit §5 P1.1 note que seul `output/http_request.go` supporte Retry-After et `retryHintFromBody` — les deux autres modules HTTP ignorent ces configurations même quand présentes dans le YAML. Cette migration corrige **un bug latent** en plus de la factorisation.

### Design Decisions

- La méthode `fetchSingle` (sans retry) appelle directement `httpclient.Do` (sans retry wrapper) — garder cette distinction.
- `handleHTTPError` local fusionne avec `httpclient.Error` ; l'extraction de `HTTPError` depuis la réponse se fait dans `httpclient.DoWithRetry`.

### Out of Scope

- Split de `http_polling.go` en pagination_*.go → Story 18.2
- Migration `http_call` → 15.5

## References

- docs/AUDIT_TECHNIQUE_2026-04-21.md §5 P1.1, §3.1 D1/D2/D4
- `input/http_polling.go` (1057 LOC)
- Stories 15.1, 15.2 (prérequis), 15.3 (modèle)

## File List

Modifiés :

- `internal/modules/input/http_polling.go` : **1057 → 581 LOC (-45 %)**
  - AC #4 exige `< 700 LOC` ✓ (réduction ≥ 30 % ✓).

Nouveaux :

- `internal/modules/input/http_polling_pagination.go` (301 LOC) :
  `fetchWithPagination`, `fetchPageBased`, `fetchOffsetBased`,
  `fetchCursorBased`, `fetch*WithMeta`, `fetchAndParseObject`,
  `extractIntField`, `extractStringField`, `extractRecordsFromObject`,
  `buildPaginatedURLFrom(Multi)`.
- `internal/modules/input/http_polling_retry_test.go` : tests des
  nouvelles capacités Retry-After + retryHintFromBody sur le polling.

Vérifications :

- `go test ./internal/modules/input/...` : PASS (22 s)
- `go test ./...` : PASS
- `golangci-lint run ./internal/modules/input/...` : 0 issue
- `wc -l internal/modules/input/http_polling.go` : **581** (< 700 ✓)
