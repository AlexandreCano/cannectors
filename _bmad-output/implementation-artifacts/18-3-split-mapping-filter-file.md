# Story 18.3: Split `filter/mapping.go` (899 LOC) into focused files

Status: backlog

## Story

En tant que développeur,
je veux que `filter/mapping.go` soit séparé en core + transforms + parse,
afin que la longue liste des opérations de transformation (trim, lowercase, toInt, dateFormat, replace, split, join, etc.) soit isolée dans un fichier dédié et facilement extensible.

## Acceptance Criteria

1. **Given** le fichier actuel 899 LOC
   **When** le refacto est appliqué
   **Then** il est découpé en :
    - `mapping.go` (core : struct, NewMappingFromConfig, Process, types FieldMapping/TransformOp/MappingConfig, ~300 LOC)
    - `mapping_transforms.go` (toutes les ops : trim/lowercase/uppercase/toString/toInt/toFloat/toBool/toArray/toObject/dateFormat/replace/split/join, + typeConversionOps map, ~400 LOC)
    - `mapping_parse.go` (parseMappingConfig, parseTransformConfig, compilation regex, ~200 LOC)
   **And** aucun fichier n'excède 600 LOC.

2. **Given** les tests
   **When** `go test ./internal/modules/filter/...`
   **Then** ils passent.

3. **Given** un développeur qui veut ajouter une nouvelle op de transform (ex: `toSlug`)
   **When** il cherche où ajouter
   **Then** il va directement dans `mapping_transforms.go`, trouve le switch des ops, ajoute la sienne
   **And** le pattern est explicite.

## Tasks / Subtasks

- [ ] Task 1 : Identifier les groupes (AC #1)
  - [ ] Core : types + MappingModule + Process + processRecord
  - [ ] Transforms : fonction(s) qui appliquent les ops ; le switch/dispatch ; typeConversionOps
  - [ ] Parse : parseMappingConfig, parseTransformConfig, compilePattern pour regex, validateTransformOp

- [ ] Task 2 : Effectuer le split (AC #1)
  - [ ] Créer les fichiers
  - [ ] Déplacer sans modifier la logique

- [ ] Task 3 : Validation (AC #2)
  - [ ] `go test ./... && golangci-lint run`

- [ ] Task 4 : Documentation "comment ajouter une op" (AC #3)
  - [ ] Court paragraphe en tête de `mapping_transforms.go` : "Pour ajouter une op, ajoutez un case dans `applyTransform(op, value)` et un entry dans `typeConversionOps` si applicable. Ajoutez un test dans `mapping_transforms_test.go`."

## Dev Notes

### Rationale

Audit §5 P2.1. Le fichier mapping.go mélange :
- Définition des types (~100 LOC)
- Logique d'exécution (~200 LOC)
- Implémentation des 13 ops de transform (~400 LOC)
- Parsing et validation (~200 LOC)

Le split par concern permet d'ajouter une nouvelle op sans re-scanner tout le fichier.

### Design Decisions

- **Transforms en premier dans le split** : c'est le gros bloc et la partie où les contributions seront les plus fréquentes.

### Out of Scope

- Refactoring des ops en interface `Transform { Apply(v interface{}) (interface{}, error) }` → plus invasif, autre story
- Ajout de nouvelles ops → hors scope

## References

- docs/AUDIT_TECHNIQUE_2026-04-21.md §5 P2.1
- `filter/mapping.go:516-565` (bloc des ops)

## File List

(à compléter)
