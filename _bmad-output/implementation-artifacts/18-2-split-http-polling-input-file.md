# Story 18.2: Split `input/http_polling.go` (1057 LOC) into focused files

Status: review

## Story

En tant que développeur,
je veux que `input/http_polling.go` soit découpé par concern (fetch core, pagination, state persistence),
afin de rendre la logique de pagination (très étendue : 3 stratégies — page, offset, cursor) identifiable et isolable.

## Acceptance Criteria

1. **Given** le fichier actuel 1057 LOC (idéalement après Story 15.4)
   **When** le refacto est appliqué
   **Then** il est découpé en :
    - `http_polling.go` (core : struct, constructor, Fetch, Close, ~350 LOC)
    - `http_polling_pagination.go` (fetchWithPagination + fetchPageBased + fetchOffsetBased + fetchCursorBased + helpers ~400 LOC)
    - `http_polling_state.go` (SetPipelineID, LoadState, GetPersistenceConfig, SetStateStore, buildEndpointWithState, ~150 LOC)
   **And** aucun fichier n'excède 600 LOC.

2. **Given** les tests
   **When** `go test ./internal/modules/input/...`
   **Then** ils passent.

3. **Given** un développeur qui veut ajouter un nouveau type de pagination
   **When** il cherche où modifier
   **Then** `http_polling_pagination.go` est évident
   **And** le pattern existant (page/offset/cursor) sert de modèle.

## Tasks / Subtasks

- [ ] Task 1 : Attendre Story 15.4 idéalement
  - [ ] Si faite, fichier à ~700 LOC → split en 2 fichiers au lieu de 3

- [ ] Task 2 : Identifier les groupes (AC #1)
  - [ ] Core : `HTTPPolling`, `NewHTTPPollingFromConfig`, `Fetch`, `doRequest`, `buildRequest`, `executeRequest`, `handleHTTPError`, `parseResponse`, `extractDataFromField`, `convertToRecords`, `applyAuthentication`, `Close`, `GetRetryInfo`, `fetchSingle`, `doRequestWithRetry`, `handleOAuth2Unauthorized`
  - [ ] Pagination : `fetchWithPagination`, `fetchPageBased`, `fetchAndParseObject`, `extractRecordsFromObject`, `fetchPageWithMeta`, `fetchOffsetBased`, `fetchOffsetWithMeta`, `fetchCursorBased`, `fetchCursorWithMeta`, `buildPaginatedURLFrom`, `buildPaginatedURLMultiFrom`
  - [ ] State : toutes les méthodes qui manipulent `persistence.*`

- [ ] Task 3 : Effectuer le split (AC #1)
  - [ ] Créer les fichiers, couper/coller
  - [ ] Vérifier imports (pas de changement de package)

- [ ] Task 4 : Validation (AC #2)
  - [ ] `go build ./... && go test ./... && golangci-lint run`

## Dev Notes

### Rationale

Audit §5 P2.1. Le fichier est long surtout à cause des 3 stratégies de pagination qui dupliquent partiellement leur structure. Un fichier dédié les rend faciles à comparer et à étendre.

### Design Decisions

- **Pas d'extraction de package** : garder tout dans `input`.
- **Les 3 stratégies de pagination** sont volontairement dans le même fichier — elles partagent beaucoup.

### Out of Scope

- Refactoring des 3 stratégies en une seule via pattern Strategy → plus risqué, autre story
- Streaming pagination → rework du contrat `Module` hors scope

## References

- docs/AUDIT_TECHNIQUE_2026-04-21.md §5 P2.1
- `input/http_polling.go:606-948` (gros bloc pagination)

## File List

- `internal/modules/input/http_polling.go` (334 LOC) — core: types, constants, constructor, Fetch, fetchSingle, parseResponse, extractDataFromField, convertToRecords, applyAuthentication, GetRetryInfo, Close, logModuleCreation
- `internal/modules/input/http_polling_request.go` (176 LOC, NEW) — buildRequest, doRequestWithRetry, handleOAuth2Unauthorized
- `internal/modules/input/http_polling_state.go` (103 LOC, NEW) — SetPipelineID, LoadState, GetPersistenceConfig, GetLastState, SetStateStore, buildEndpointWithState
- `internal/modules/input/http_polling_pagination.go` (303 LOC, pre-existing)

Tests inchangés. Build + 86 tests filter passent.
