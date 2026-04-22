# Story 18.1: Split `output/http_request.go` (1641 LOC) into focused files

Status: backlog

## Story

En tant que développeur,
je veux que `output/http_request.go` soit découpé en plusieurs fichiers ciblés,
afin qu'aucun fichier du projet ne dépasse ~600 LOC et que chaque concern (HTTP request, preview, headers, templates) soit localisable rapidement.

## Acceptance Criteria

1. **Given** le fichier actuel `output/http_request.go` à 1641 LOC
   **When** le refacto est appliqué (idéalement après Story 15.3 qui aura déjà réduit le fichier)
   **Then** il est découpé en :
    - `http_request.go` (core : struct, constructor, Send, ~400 LOC)
    - `http_request_preview.go` (PreviewRequest + buildPreviewHeaders + masking ~250 LOC)
    - `http_request_body.go` (buildBodyForRecord, stripMetadata, validateJSON, isJSONContentType, formatJSONPreview ~200 LOC)
    - `http_request_endpoint.go` (resolveEndpointForBatch/Record/WithStaticQuery, validateURL ~150 LOC)
   **And** aucun fichier n'excède 600 LOC.

2. **Given** le split
   **When** `go test ./internal/modules/output/...`
   **Then** tous les tests passent sans modification
   **And** le package API externe reste inchangée (mêmes exports publics).

3. **Given** `http_request_test.go` à 4461 LOC
   **When** le refacto de test est fait (dans un commit suivant, optionnel pour cette story)
   **Then** les tests sont répartis sur `http_request_test.go`, `http_request_preview_test.go`, `http_request_body_test.go`, `http_request_endpoint_test.go`
   **And** chaque fichier de test fait < 1500 LOC.

4. **Given** un développeur qui cherche "comment le module construit-il l'URL finale ?"
   **When** il parcourt le package
   **Then** `http_request_endpoint.go` est identifiable au nom
   **And** la navigation est plus rapide qu'un `Ctrl+F` dans 1641 lignes.

## Tasks / Subtasks

- [ ] Task 1 : Attendre Story 15.3 idéalement (préférence)
  - [ ] Si 15.3 n'est pas faite, le split sera fait mais devra être redéfait partiellement après
  - [ ] Décision : faire 15.3 d'abord pour descendre à ~800 LOC, puis split en 2-3 fichiers au lieu de 4

- [ ] Task 2 : Identifier les groupes cohérents (AC #1)
  - [ ] Lister toutes les méthodes/fonctions et les grouper par concern :
    - Core : `HTTPRequestModule`, `NewHTTPRequestFromConfig`, `Send`, `Close`, `GetRetryInfo`
    - Send modes : `sendBatchMode`, `sendSingleRecordMode`, `executeRequestAndLog`
    - Preview : `PreviewRequest`, `previewBatchMode`, `previewSingleRecordMode`, `buildPreviewHeaders`, `addMaskedAuthHeaders`, `addUnmaskedAuthHeaders`, `maskValue`, `formatJSONPreview`
    - Body : `buildBodyForRecord`, `stripMetadataFromRecord(s)`, `validateJSON`, `isJSONContentType`, `truncateString`
    - Endpoint : `resolveEndpoint*`, `validateURL`, `extractHeadersFromRecord`, `buildBaseHeadersMap`

- [ ] Task 3 : Effectuer le split (AC #1, #2)
  - [ ] Créer les nouveaux fichiers
  - [ ] Couper/coller les méthodes — même package `output`, donc pas de refacto de visibilité nécessaire
  - [ ] Conserver un `http_request.go` qui contient le type principal et ses méthodes "entrée"

- [ ] Task 4 : Validation (AC #2)
  - [ ] `go build ./... && go test ./... && golangci-lint run`
  - [ ] Vérifier les line counts : `wc -l internal/modules/output/http_request*.go`

- [ ] Task 5 : Split des tests (AC #3) - OPTIONNEL, peut être une sous-story
  - [ ] Répartir les tests par concern testé
  - [ ] Éviter de dupliquer les helpers : extraire dans `http_request_testhelpers_test.go`

## Dev Notes

### Rationale

Audit §4.2 : `http_request_test.go` à 4461 LOC (111 KB) est "impossible à relire d'un trait". Le fichier source à 1641 LOC porte 4 concerns distincts qui se prêtent mal à la lecture séquentielle. Le split améliore :
- Navigation (plus court fichier = moins de scroll)
- Blame git (changements mieux localisés)
- Merge conflicts (concerns séparés = moins de conflits)

### Design Decisions

- **Fichiers dans le même package** : pas besoin de nouveau package, juste une séparation de fichiers.
- **Faire après 15.3** : si httpclient migration est faite, le fichier fera ~800 LOC, un split en 2 fichiers suffit.
- **Split des tests peut être différé** : le source split est prioritaire, les tests peuvent suivre en nettoyage.

### Out of Scope

- Extraction de packages séparés → non pertinent (tout reste `output`)
- Refonte des tests unitaires → garder la logique, juste réorganiser

## References

- docs/AUDIT_TECHNIQUE_2026-04-21.md §5 P2.1, §4
- `output/http_request.go` (état actuel)

## File List

(à compléter)
