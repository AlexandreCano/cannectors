# Story 20.3: Refactor `mapping_test.go` with fixture builders

Status: in-progress

## Story

En tant que développeur,
je veux que `filter/mapping_test.go` (2309 LOC) soit réduit via l'extraction de builders de fixtures,
afin de rendre chaque test plus lisible et de supprimer ~30 % de duplication de setup.

## Acceptance Criteria

1. **Given** `mapping_test.go` actuel
   **When** je mesure ligne par ligne
   **Then** identifier les patterns de duplication (setup de records, builder de `FieldMapping`, comparaisons) — au moins 3 patterns répétés ≥ 10 fois chacun.

2. **Given** un nouveau fichier `mapping_fixtures_test.go` (même package, donc accessible aux tests)
   **When** il est écrit
   **Then** il contient :
    - `recordBuilder` : builder fluent pour `map[string]any` (ex: `rb().Set("id", 1).Set("name", "x").Build()`)
    - `mappingBuilder` : builder pour `FieldMapping` (ex: `mb().Source("a").Target("b").Transform("toString").Build()`)
    - `assertRecordsEqual(t, got, want)` helper uniforme

3. **Given** `mapping_test.go` après refacto
   **When** je mesure sa taille
   **Then** ≤ 1700 LOC (réduction ≥ 25 %)
   **And** la logique de chaque test est plus facilement lisible.

4. **Given** les tests refactorés
   **When** `go test ./internal/modules/filter/` tourne
   **Then** tous passent, avec le même nombre d'assertions (pas de perte de couverture).

5. **Given** le pattern est validé sur `mapping_test.go`
   **When** on évalue `condition_test.go` (2218 LOC), `http_request_test.go` (4461 LOC), etc.
   **Then** les builders sont réutilisables et peuvent être étendus pour ces fichiers (sous-story future).

## Tasks / Subtasks

- [ ] Task 1 : Analyser les duplications (AC #1)
  - [ ] Identifier les 5-10 patterns les plus fréquents
  - [ ] Documenter dans un commentaire au début de `mapping_fixtures_test.go`

- [ ] Task 2 : Écrire les builders (AC #2)
  - [ ] `recordBuilder` : simple wrapper sur `map[string]any`
  - [ ] `mappingBuilder` : toutes les options de `FieldMapping` (Source, Target, DefaultValue, OnMissing, Transforms)
  - [ ] `transformBuilder` : pour construire une liste de `TransformOp`
  - [ ] Helpers : `oneRecord(k, v ...)`, `assertMapped(t, input, config, want)`

- [ ] Task 3 : Migrer les tests (AC #3, #4)
  - [ ] Groupe par groupe, pas tout d'un coup
  - [ ] Vérifier après chaque groupe que `go test` passe

- [ ] Task 4 : Documenter le pattern (AC #5)
  - [ ] Court README dans `_bmad-output/implementation-artifacts/test-patterns.md` ou dans godoc des builders
  - [ ] Explication du pourquoi pour les futurs contributeurs

## Dev Notes

### Rationale

Audit §4.1 et §4.3. `mapping_test.go` est le 2e plus gros fichier de test (2309 LOC, 2.57× le source). La plupart de ce poids vient de setup redondants.

### Design Decisions

- **Builders fluents** : idiomatique en Go (ex : `testify/require`).
- **Pas de framework externe** : éviter d'ajouter une dépendance comme `testify/suite`. Standard lib + builders maison suffit.
- **Un seul fichier fixtures** : pas la peine de sur-modulariser pour des helpers de test.

### Out of Scope

- Migration des autres tests (condition, http_request) → sous-stories futures
- Remplacement du framework de test → garder `testing` standard

## References

- docs/AUDIT_TECHNIQUE_2026-04-21.md §4.1, §4.3
- `filter/mapping_test.go` (2309 LOC)

## File List

- `internal/modules/filter/mapping_test.go`
- `internal/modules/filter/mapping_fixtures_test.go`
- `internal/modules/filter/mapping_scenarios_test.go`
- `test-patterns.md`
