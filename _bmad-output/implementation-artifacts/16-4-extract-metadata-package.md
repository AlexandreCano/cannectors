# Story 16.4: Extract `MetadataAccessor` into a dedicated `internal/metadata` package

Status: backlog

## Story

En tant que développeur,
je veux un package autonome `internal/metadata` qui porte `MetadataAccessor` et les helpers de strip,
afin que les modules output puissent l'importer sans créer de cycle avec `runtime`, ce qui éliminera la duplication actuelle de `stripMetadataFromRecord` dans `output/http_request.go`.

## Acceptance Criteria

1. **Given** un nouveau package `internal/metadata`
   **When** je le consulte
   **Then** il contient `type Accessor`, `NewAccessor(fieldName string)`, les méthodes `Get/Set/Delete/GetAll/SetAll/Merge/Copy/HasMetadata/Strip/StripCopy`, et les helpers `StripFromRecord(record, fieldName)` / `StripFromRecords(records, fieldName)` utilisables sans instancier un Accessor.

2. **Given** `runtime/metadata.go`
   **When** je le regarde
   **Then** il ne contient plus que `DefaultMetadataFieldName` (qui peut aussi migrer dans `metadata`) et un ré-export facultatif pour compatibilité interne
   **And** l'essentiel du code est dans `internal/metadata/accessor.go`.

3. **Given** `output/http_request.go`
   **When** je consulte son code
   **Then** `stripMetadataFromRecord` et `stripMetadataFromRecords` locaux sont supprimés
   **And** `const MetadataFieldName = "_metadata"` local est supprimé
   **And** les appels utilisent `metadata.StripFromRecord` / `metadata.StripFromRecords`.

4. **Given** les tests de métadonnées
   **When** `go test ./...`
   **Then** tous passent (tests actuels dans `runtime/metadata_test.go` migrent vers `internal/metadata`).

5. **Given** les modules output (database, http_request)
   **When** ils doivent strip les metadata avant de sérialiser vers la destination
   **Then** ils utilisent le même helper — aucune duplication résiduelle.

## Tasks / Subtasks

- [ ] Task 1 : Créer `internal/metadata` (AC #1)
  - [ ] Fichier `internal/metadata/accessor.go` : déplacer `MetadataAccessor` depuis `runtime/metadata.go`
  - [ ] Renommer `MetadataAccessor` → `Accessor` (convention Go : le package porte le contexte)
  - [ ] Déplacer constante `DefaultMetadataFieldName` → `metadata.DefaultFieldName`
  - [ ] Ajouter helpers stateless : `StripFromRecord(record, fieldName)`, `StripFromRecords(records, fieldName)`

- [ ] Task 2 : Remplacer la duplication dans `output/http_request.go` (AC #3)
  - [ ] Supprimer `const MetadataFieldName = "_metadata"` (ligne 1306)
  - [ ] Supprimer `stripMetadataFromRecord` et `stripMetadataFromRecords` (lignes 1308-1340)
  - [ ] Remplacer les 2 appels par `metadata.StripFromRecord(record, metadata.DefaultFieldName)` etc.
  - [ ] Ajouter import `github.com/cannectors/runtime/internal/metadata`

- [ ] Task 3 : Unifier `output/database.go` (AC #5)
  - [ ] Si `output/database.go` fait aussi un strip (à vérifier), migrer sur le même helper

- [ ] Task 4 : Gestion du cycle d'import (AC #2)
  - [ ] Vérifier que `runtime` peut toujours importer `metadata` (pas de cycle)
  - [ ] Ré-export minimal dans `runtime/metadata.go` si code externe utilise `runtime.MetadataAccessor`

- [ ] Task 5 : Tests (AC #4)
  - [ ] Migrer `runtime/metadata_test.go` → `internal/metadata/accessor_test.go`
  - [ ] Vérifier que `go test ./...` passe

## Dev Notes

### Rationale

Duplication D7. Le commentaire `output/http_request.go:1305` dit textuellement : *"This matches the behavior of runtime.MetadataAccessor.StripCopy but is implemented here to avoid import cycles between output and runtime packages."* — c'est exactement le symptôme d'une mauvaise factorisation. Déplacer la logique dans un package feuille la rend importable par tout le monde.

### Design Decisions

- **Nom `Accessor` (pas `MetadataAccessor`)** : convention Go — le nom du package fournit le contexte.
- **Helpers stateless `StripFromRecord`** en parallèle du `Accessor` : utile quand on n'a pas besoin de l'overhead du wrapper.
- **Le champ `_metadata` reste conventionnel** (pas obligatoire), exposé via `metadata.DefaultFieldName`.

### Out of Scope

- Changer la convention du nom de champ (underscore prefix) → pas de changement sémantique ici
- Ajouter un schéma de metadata typé → autre story

## References

- docs/AUDIT_TECHNIQUE_2026-04-21.md §3.1 D7, §5 P2.3
- `runtime/metadata.go:1-302`
- `output/http_request.go:1303-1340`

## File List

(à compléter)
