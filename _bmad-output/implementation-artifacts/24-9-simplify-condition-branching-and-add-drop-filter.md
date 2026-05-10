# Story 24.9: Simplify condition branching and add drop filter

Status: ready-for-dev

## Story

En tant qu'utilisateur,
je veux que `condition` route vers `then` et `else`, et que le drop soit un filter explicite,
afin d'eviter la redondance entre `onTrue/onFalse` et les branches imbriquees.

## Acceptance Criteria

1. **Given** une condition avec ancien champ `lang`
   **When** le schema valide la config
   **Then** la config est rejetee.

2. **Given** une condition avec ancien champ `onTrue` ou `onFalse`
   **When** le schema valide la config
   **Then** la config est rejetee.

3. **Given** une condition avec `expression` vide ou whitespace-only
   **When** la config est validee ou instanciee
   **Then** elle est rejetee.

4. **Given** une condition avec une expression invalide
   **When** le module est cree
   **Then** l'expression est compilee au demarrage et l'erreur est explicite.

5. **Given** une condition true avec `then`
   **When** le filter s'execute
   **Then** les filters `then` sont appliques.

6. **Given** une condition false avec `else`
   **When** le filter s'execute
   **Then** les filters `else` sont appliques.

7. **Given** une branche absente
   **When** un record tombe dans cette branche
   **Then** le record est garde inchange.

8. **Given** un filter `drop`
   **When** il s'execute sur un record
   **Then** le record est retire du flux.

9. **Given** des conditions imbriquees
   **When** elles sont validees
   **Then** aucune limite artificielle de profondeur n'est appliquee.

## Tasks / Subtasks

- [ ] Task 1 : Supprimer les champs inutiles
  - [ ] Retirer `lang` du schema et du code
  - [ ] Retirer `onTrue` du schema et du code
  - [ ] Retirer `onFalse` du schema et du code
  - [ ] Adapter tests existants

- [ ] Task 2 : Stabiliser `expression`
  - [ ] `expression` requis
  - [ ] `minLength: 1`
  - [ ] Runtime reject whitespace-only
  - [ ] Compilation runtime avec `expr`

- [ ] Task 3 : Implementer `drop`
  - [ ] Schema `type: drop`
  - [ ] Registry/factory
  - [ ] Module filter
  - [ ] Tests unitaires et condition branches

- [ ] Task 4 : Adapter condition routing
  - [ ] Branche absente garde le record
  - [ ] Then/else executent des filters imbriques
  - [ ] Pas de limite de profondeur

## Dev Notes

### Product Rules

`condition` ne decide plus implicitement de garder ou supprimer. Pour filtrer, l'utilisateur place explicitement `drop` dans une branche.

## References

- `internal/config/schema/filter-schema.json`
- `internal/modules/filter/condition.go`
- `internal/modules/filter/filter.go`
- `internal/registry/builtins.go`

## File List

- `internal/config/schema/filter-schema.json`
- `internal/modules/filter/condition*.go`
- `internal/modules/filter/drop*.go`
- `internal/registry/*.go`
