# Story 16.2: Unify `RetryInfo` type across `connector` and `errhandling`

Status: backlog

## Story

En tant que développeur,
je veux un seul type `RetryInfo` utilisé partout,
afin d'éliminer la conversion manuelle entre `connector.RetryInfo` (public) et `errhandling.RetryInfo` (interne, plus riche).

## Acceptance Criteria

1. **Given** le code du projet
   **When** je recherche `type RetryInfo`
   **Then** il n'existe qu'une définition, dans `pkg/connector/types.go`
   **And** `errhandling.RetryInfo` est supprimé.

2. **Given** le type `connector.RetryInfo`
   **When** je le consulte
   **Then** il contient les champs nécessaires à `errhandling.RetryExecutor` en plus de ceux existants :
    - `TotalAttempts int` (déjà là)
    - `RetryCount int` (déjà là)
    - `RetryDelaysMs []int64` (déjà là, équivalent de `Delays []time.Duration`)
    - `TotalDurationMs int64` (nouveau — remplace `TotalDuration time.Duration`)
    - Pas de `Errors []error` (les erreurs ne doivent pas fuiter à travers l'API publique).

3. **Given** `errhandling.RetryExecutor.GetRetryInfo()`
   **When** je l'appelle
   **Then** il retourne `*connector.RetryInfo` directement.

4. **Given** un module qui implémente `connector.RetryInfoProvider`
   **When** le runtime l'invoque (`ExecuteWithContext` fait `p.GetRetryInfo()`)
   **Then** aucune conversion intermédiaire n'est requise.

5. **Given** les tests existants dans `errhandling/retry_test.go`
   **When** ils accèdent aux champs de l'ancienne `RetryInfo` interne
   **Then** les tests sont adaptés aux champs publics
   **And** `go test ./...` passe.

## Tasks / Subtasks

- [ ] Task 1 : Étendre `connector.RetryInfo` (AC #2)
  - [ ] Ajouter `TotalDurationMs int64` dans `pkg/connector/types.go`
  - [ ] Conserver `RetryDelaysMs []int64` comme source de vérité (pas de duplication `Delays []time.Duration`)

- [ ] Task 2 : Supprimer `errhandling.RetryInfo` (AC #1, #3)
  - [ ] Remplacer dans `errhandling/retry.go:191-210` : `type RetryInfo struct` → alias ou usage direct de `connector.RetryInfo`
  - [ ] `RetryExecutor.retryInfo` devient `connector.RetryInfo`
  - [ ] `GetRetryInfo()` retourne `*connector.RetryInfo`

3. Task 3 : Gestion des erreurs (AC #2)
  - [ ] Décider : les erreurs intermédiaires (`Errors []error` actuel) ne sortent pas via `RetryInfo` public
  - [ ] Si besoin pour tests internes : exposer via une méthode `LastErrors() []error` sur `RetryExecutor`

- [ ] Task 4 : Adapter tous les consommateurs (AC #4)
  - [ ] Rechercher `errhandling.RetryInfo` dans le code — remplacer par `connector.RetryInfo`
  - [ ] Grep `p.GetRetryInfo()` dans `runtime/pipeline.go` et vérifier la signature

- [ ] Task 5 : Tests (AC #5)
  - [ ] Adapter `errhandling/retry_test.go`
  - [ ] `go test ./... && golangci-lint run`

## Dev Notes

### Rationale

Duplication D8 de l'audit. `connector.RetryInfo` (pkg, 3 champs) et `errhandling.RetryInfo` (internal, 6 champs) décrivent le même concept. À chaque frontière (module → executor → result), conversion manuelle, source de bugs.

### Design Decisions

- **Champ durée en millisecondes** (`int64`), pas `time.Duration` : plus facile à sérialiser en JSON, déjà la convention dans le reste de `RetryConfig`.
- **Pas d'`Errors []error` dans l'API publique** : les erreurs peuvent contenir des données sensibles, et les consommateurs n'en ont pas besoin.

### Out of Scope

- Refonte complète de `ExecutionResult` → garde les champs existants
- Suppression de `RetryInfoProvider` interface → à conserver pour l'extensibilité

## References

- docs/AUDIT_TECHNIQUE_2026-04-21.md §3.1 D8, §5 P1.4
- `pkg/connector/types.go:91-105`
- `errhandling/retry.go:191-210`

## File List

(à compléter)
