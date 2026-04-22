# Story 16.1: Unify record path navigation helpers

Status: backlog

## Story

En tant que développeur,
je veux une seule implémentation de la navigation nested paths (dot notation + array indices) utilisée par tous les modules,
afin de supprimer les trois implémentations divergentes (dont deux qui ne gèrent même pas les arrays) et éliminer les bugs subtils de comportement incohérent.

## Acceptance Criteria

1. **Given** le code du projet
   **When** je recherche toute fonction qui navigue un `map[string]interface{}` avec une chaîne "a.b.c" ou "a[0].b"
   **Then** il ne reste qu'une seule implémentation publique : `moduleconfig.GetNestedValue`, `moduleconfig.SetNestedValue`, `moduleconfig.DeleteNestedValue`, `moduleconfig.IsNestedPath`
   **And** les implémentations privées suivantes sont supprimées : `runtime/metadata.go:getNestedValue/setNestedValue/deleteNestedKey` et `output/http_request.go:getFieldValue`.

2. **Given** `output/http_request.go`
   **When** je lis son code
   **Then** les path lookups utilisent `moduleconfig.GetNestedValue` (et donc gèrent désormais les arrays, ce qui n'était pas le cas avant).

3. **Given** `runtime/metadata.go`
   **When** je lis son code
   **Then** `MetadataAccessor.Get/Set/Delete` délèguent à `moduleconfig.GetNestedValue`/`SetNestedValue`/`DeleteNestedValue`
   **And** `deepCopyMap` / `deepCopySlice` restent dans `metadata.go` (ils ne concernent pas la navigation).

4. **Given** un test `TestGetFieldValue_WithArrayIndex`
   **When** il passe une path `items[0].name` sur un record contenant un array
   **Then** la valeur est correctement extraite (test nouveau, révèle le bug actuel de `getFieldValue`).

5. **Given** les tests existants
   **When** `go test ./... && golangci-lint run`
   **Then** tout passe sans régression.

## Tasks / Subtasks

- [ ] Task 1 : Supprimer `getFieldValue` dans `output/http_request.go` (AC #1, #2, #4)
  - [ ] Remplacer les 3 appels (`:1137`, `:1166`, `:1290`) par `moduleconfig.GetNestedValue` + conversion en string via `template.ValueToString`
  - [ ] Supprimer la fonction (lignes 1377-1412)
  - [ ] Ajouter test `TestResolveEndpointForRecord_WithArrayIndex`

- [ ] Task 2 : Refactorer `runtime/metadata.go` (AC #1, #3)
  - [ ] `getNestedValue` (privé) → appel à `moduleconfig.GetNestedValue`
  - [ ] `setNestedValue` (privé) → appel à `moduleconfig.SetNestedValue`
  - [ ] `deleteNestedKey` (privé) → appel à `moduleconfig.DeleteNestedValue`
  - [ ] Vérifier qu'il n'y a pas de cycle d'import (runtime → moduleconfig : OK, inverse n'existe pas)

- [ ] Task 3 : Tests (AC #4, #5)
  - [ ] Ajouter test d'intégration : output HTTP avec endpoint `/users/{id}/items/{itemId}` où `itemId = items[0].id`
  - [ ] Exécuter `go test ./...`

## Dev Notes

### Rationale

Duplication D3 de l'audit technique. Trois implémentations :
- `moduleconfig/nested.go:28-31` (publique, complète, gère arrays)
- `runtime/metadata.go:186-211` (privée, sans arrays)
- `output/http_request.go:1377-1412` (privée, sans arrays, aussi convertit en string)

Les deux dernières sont des réinventions de la première — probablement par peur d'un cycle d'import qui n'existe pas.

### Design Decisions

- **`moduleconfig.GetNestedValue` retourne `(interface{}, bool)`** — le call site convertit en string si besoin via `template.ValueToString`.
- **`MetadataAccessor` reste une façade** qui encapsule `m.fieldName` et délègue — API plus typée.

### Out of Scope

- Renommer le package `moduleconfig` (il contient en fait 3 responsabilités) → 18.5
- Extraire un `internal/recordpath` dédié — pas nécessaire si le cycle d'import n'existe pas

## References

- docs/AUDIT_TECHNIQUE_2026-04-21.md §3.1 D3, §5 P1.3
- `moduleconfig/nested.go` (canonique)
- `runtime/metadata.go:185-262` (à supprimer)
- `output/http_request.go:1377-1412` (à supprimer)

## File List

(à compléter)
