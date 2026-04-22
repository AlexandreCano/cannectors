# Story 18.5: Split `internal/moduleconfig` by responsibility

Status: backlog

## Story

En tant que développeur,
je veux que le package `moduleconfig` soit séparé en plusieurs packages selon la responsabilité,
afin que le couplage entre "parsing générique", "types partagés" et "navigation nested path" disparaisse — chaque concern devient importable indépendamment.

## Acceptance Criteria

1. **Given** le package actuel `moduleconfig` qui contient 3 responsabilités non reliées
   **When** le refacto est appliqué
   **Then** les 3 fichiers actuels sont répartis :
    - `internal/moduleconfig/parse.go` + `shared.go` (types HTTP/SQL/Key/Cache + generic `ParseConfig[T]`) restent dans `moduleconfig`
    - `internal/moduleconfig/nested.go` → **déplacé** dans `internal/recordpath/` (nouveau package)
    - La navigation nested devient `recordpath.Get`, `recordpath.Set`, `recordpath.Delete`, `recordpath.IsNested`

2. **Given** le nouveau package `recordpath`
   **When** je regarde ses dépendances
   **Then** il ne dépend d'aucun autre package interne du projet (package feuille)
   **And** il est importable par `template`, `runtime/metadata`, `output/http_request`, `filter/mapping` etc.

3. **Given** les consommateurs actuels de `moduleconfig.GetNestedValue`
   **When** je les adapte
   **Then** ils importent `recordpath` et utilisent `recordpath.Get`
   **And** aucun cycle d'import n'est créé.

4. **Given** les tests
   **When** `go test ./...`
   **Then** tous passent.

## Tasks / Subtasks

- [ ] Task 1 : Faire la Story 16.1 d'abord (prérequis fort)
  - [ ] Elle unifie déjà la navigation nested — les call sites passent tous par `moduleconfig.GetNestedValue`

- [ ] Task 2 : Créer le package `internal/recordpath` (AC #1, #2)
  - [ ] `recordpath/path.go` : déplacer le contenu de `moduleconfig/nested.go`
  - [ ] Renommer les exports selon la convention Go : `GetNestedValue` → `Get`, `SetNestedValue` → `Set`, etc.
  - [ ] Godoc de package expliquant le format de path

- [ ] Task 3 : Migrer les imports (AC #3)
  - [ ] Grep `moduleconfig.GetNestedValue|SetNestedValue|DeleteNestedValue|IsNestedPath` → remplacer par `recordpath.*`
  - [ ] Sites attendus après Story 16.1 : `template/template.go`, `runtime/metadata.go`, `output/http_request.go`, `filter/mapping.go`, `filter/set.go`, `filter/remove.go`, `filter/condition.go`, `filter/http_call.go`, `filter/sql_call.go`

- [ ] Task 4 : Supprimer `moduleconfig/nested.go` (AC #1)
  - [ ] Fichier supprimé
  - [ ] `moduleconfig/nested_test.go` déplacé dans `recordpath/`

- [ ] Task 5 : Validation (AC #4)
  - [ ] `go build ./... && go test ./... && golangci-lint run`

## Dev Notes

### Rationale

Audit §2.1 note que `moduleconfig` porte 3 responsabilités non reliées :
- `parse.go` : generic `ParseConfig[T]` (cohérent avec le nom du package)
- `shared.go` : types partagés `HTTPRequestBase`, `KeyConfig`, `CacheConfig`, etc. (cohérent)
- `nested.go` : navigation dans map[string]interface{} (pas lié au concept de "module config")

### Design Decisions

- **`recordpath` est un package feuille** : important pour éviter les cycles, et pour être importable partout.
- **Renommage des exports** : `recordpath.Get` plus concis que `moduleconfig.GetNestedValue`.
- **Garder `moduleconfig` pour les types communs** : sa responsabilité devient plus claire une fois `nested.go` parti.

### Out of Scope

- Renommer `moduleconfig` en `modulecfg` ou autre → cosmétique
- Splitter `shared.go` par type de module → pas nécessaire

## References

- docs/AUDIT_TECHNIQUE_2026-04-21.md §2.1
- `moduleconfig/nested.go` (à déplacer)
- Story 16.1 (prérequis)

## File List

(à compléter)
