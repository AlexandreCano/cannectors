# Story 22.5: Cover transformation filter scenarios

Status: ready-for-dev

## Story

En tant que développeur,
je veux couvrir les filters de transformation dans des pipelines réels,
afin de vérifier les transformations de records hors tests unitaires.

## Acceptance Criteria

1. **Given** un record source contenant strings, nombres, booléens, dates, arrays et objets
   **When** le filter `mapping` s'exécute
   **Then** chaque transform supporté produit la valeur attendue.

2. **Given** des champs manquants
   **When** `onMissing` vaut `setNull`, `skipField`, `useDefault` ou `fail`
   **Then** le comportement observé correspond à la configuration.

3. **Given** des filters `set` et `remove`
   **When** ils ciblent des chemins imbriqués
   **Then** les champs sont créés, modifiés ou supprimés correctement.

4. **Given** un filter `condition` avec `then` et `else`
   **When** les records passent dans le pipeline
   **Then** les branches imbriquées sont appliquées correctement.

5. **Given** un filter `script`
   **When** il utilise inline script et `scriptFile`
   **Then** les deux modes transforment les records comme attendu.

## Tasks / Subtasks

- [ ] Task 1 : Ajouter les scénarios mapping
  - [ ] Tous les transforms supportés
  - [ ] Tous les `onMissing`
  - [ ] Suppression par mapping sans source

- [ ] Task 2 : Ajouter les scénarios set/remove/condition
  - [ ] Set champ plat et imbriqué
  - [ ] Remove champ plat et imbriqué
  - [ ] Condition then/else avec filters imbriqués

- [ ] Task 3 : Ajouter les scénarios script
  - [ ] Inline script
  - [ ] Script file
  - [ ] `onError` skip/log/fail sur erreur script

## Dev Notes

### Rationale

Les filters purs sont déjà unit-testables, mais les pipelines locaux valident leur intégration avec la sérialisation YAML, le runtime et les outputs.

### Out of Scope

- Fuzz testing des transforms
- Scripts JavaScript longs ou malicieux

## References

- `internal/modules/filter/mapping.go`
- `internal/modules/filter/set.go`
- `internal/modules/filter/remove.go`
- `internal/modules/filter/condition.go`
- `internal/modules/filter/script.go`
- `examples/10-mapping-transforms-all.yaml`
- `examples/11-condition-nested-routing.yaml`

## File List

- `test-lab/pipelines/filters-*.yaml`
- `test-lab/assets/scripts/*.js`
