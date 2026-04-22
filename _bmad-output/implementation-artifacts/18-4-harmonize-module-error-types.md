# Story 18.4: Harmonize module error types via a common interface

Status: backlog

## Story

En tant que développeur,
je veux une interface `ModuleError` implémentée par toutes les erreurs typées des modules (`MappingError`, `ConditionError`, `ScriptError`, `HTTPCallError`, etc.),
afin que le runtime puisse enrichir `ExecutionError.Details` uniformément sans connaître le type concret.

## Acceptance Criteria

1. **Given** un nouveau type d'interface exporté
   **When** je consulte `errhandling/module_error.go`
   **Then** il définit :
   ```go
   type ModuleError interface {
       error
       Code() string
       Module() string
       RecordIndex() int
       Details() map[string]interface{}
   }
   ```
   **And** les erreurs `MappingError`, `ConditionError`, `ScriptError`, `HTTPCallError`, `SQLCallError` (si existe) implémentent cette interface.

2. **Given** `runtime/pipeline.go:buildExecutionError`
   **When** l'erreur du module est une `ModuleError`
   **Then** il copie `err.Details()` dans `ExecutionError.Details`, et `err.Code()` dans le code
   **And** la détection via `errors.As(err, &me ModuleError)` fonctionne.

3. **Given** chacun des types d'erreur existants
   **When** je les adapte
   **Then** les champs communs (Code, Module, RecordIndex, Details) sont accessibles via les méthodes
   **And** les champs spécifiques (StackTrace, Endpoint, StatusCode, etc.) restent accessibles via assertion vers le type concret si besoin.

4. **Given** les tests existants
   **When** `go test ./...`
   **Then** tous passent (les signatures des constructeurs internes peuvent changer sans impact externe).

5. **Given** un nouveau module qui définit son type d'erreur
   **When** le développeur l'implémente
   **Then** il implémente l'interface `ModuleError` ; le runtime récupère automatiquement Details et Code sans changement ailleurs.

## Tasks / Subtasks

- [ ] Task 1 : Définir l'interface (AC #1)
  - [ ] `errhandling/module_error.go` : interface + doc
  - [ ] Optionnel : un type `BaseModuleError` struct qui implémente la moitié des méthodes, embedable dans les autres

- [ ] Task 2 : Adapter les types existants (AC #1, #3)
  - [ ] `filter/mapping.go:MappingError` : ajouter méthodes manquantes
  - [ ] `filter/condition.go:ConditionError`
  - [ ] `filter/script.go:ScriptError`
  - [ ] `filter/http_call.go:HTTPCallError`
  - [ ] `filter/sql_call.go:SQLCallError` (si existe ; sinon le créer)
  - [ ] Chaque type gagne une méthode `func (e *XxxError) Code() string` et similaires

- [ ] Task 3 : Utiliser l'interface dans le runtime (AC #2)
  - [ ] `runtime/pipeline.go:buildExecutionError` :
    - Si `errhandling.IsModuleError(err, &me)` → copier `me.Details()` dans `ExecutionError.Details`
    - Utiliser `me.Code()` au lieu du code générique
  - [ ] Ajouter helper `errhandling.AsModuleError(err) (ModuleError, bool)` pour les call sites

- [ ] Task 4 : Documenter (AC #5)
  - [ ] Section dans `docs/MODULE_EXTENSIBILITY.md` : "Typed errors" → implémenter `ModuleError`
  - [ ] Exemple minimal d'un module avec `MyModuleError`

- [ ] Task 5 : Tests (AC #4)
  - [ ] `go test ./... && golangci-lint run`

## Dev Notes

### Rationale

Audit §3.3 : chaque module définit son propre type d'erreur avec des champs différents. Le runtime ne peut pas les traiter uniformément. L'interface `ModuleError` résout ça sans forcer tous les modules à hériter d'un type commun.

### Design Decisions

- **Interface minimaliste** : 4 méthodes seulement.
- **Pas de type `BaseModuleError` obligatoire** : embed optionnel, liberté de réimplémenter.
- **Rétrocompatibilité via assertions** : le code qui fait `var e *MappingError; if errors.As(err, &e)` continue de marcher.

### Out of Scope

- Registry centralisé des codes d'erreur → possible évolution future
- Sérialisation JSON standardisée des erreurs → cosmétique

## References

- docs/AUDIT_TECHNIQUE_2026-04-21.md §3.3, §5 P2.5
- `filter/mapping.go:152-189`
- `filter/condition.go:129-166`
- `filter/script.go:83-113`
- `filter/http_call.go:112-150`

## File List

(à compléter)
