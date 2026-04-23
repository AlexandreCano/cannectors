# Story 19.5: Remove dead code (NewExecutor, Execute duplication, others)

Status: backlog

## Story

En tant que développeur,
je veux supprimer les constructeurs, fonctions et ré-exports non utilisés,
afin de réduire la surface du code et les points de confusion (deux façons de créer un executor, deux méthodes Execute, etc.).

## Acceptance Criteria

1. **Given** `runtime.NewExecutor(dryRun bool)`
   **When** je grep ses appels
   **Then** aucun call site externe n'existe (le projet utilise toujours `NewExecutorWithModules`)
   **And** le constructeur est supprimé.

2. **Given** `ExecuteWithRecords` et `ExecuteWithRecordsContext` (`runtime/pipeline.go:793, :802`)
   **When** je les compare avec `Execute` / `ExecuteWithContext`
   **Then** leur corps est ~80 % identique
   **And** ils sont unifiés via une source d'input (interface ou fonction) passée en paramètre, OU le webhook appelle `ExecuteWithContext` après avoir pré-chargé les records dans un input stub.

3. **Given** les constantes ré-exportées de `errhandling` dans `runtime/retry.go` (documentées dans `docs/REFACTORING_NOTES.md`)
   **When** je vérifie leur usage
   **Then** si aucune n'est utilisée, le fichier est supprimé
   **And** les imports sont nettoyés.

4. **Given** le stub filter module (`filter/stub.go`) si présent
   **When** je cherche son usage
   **Then** si la factory refuse désormais les types inconnus (état actuel confirmé), le stub est supprimé du code de production (garder en test helper si utile)
   **And** idem pour `input/stub.go` et `output/stub.go`.

5. **Given** les tests
   **When** `go test ./... && golangci-lint run`
   **Then** tout passe.

## Tasks / Subtasks

- [ ] Task 1 : Supprimer `NewExecutor` (AC #1)
  - [ ] Grep `runtime.NewExecutor(` — confirmer zéro call site externe
  - [ ] Supprimer la fonction (`runtime/pipeline.go:102-106`)
  - [ ] Adapter les tests si utilisé en interne test (garder le builder setup via `NewExecutorWithModules`)

- [ ] Task 2 : Unifier Execute et ExecuteWithRecords (AC #2)
  - [ ] Option A : `ExecuteWithRecords` reste une API publique distincte pour les webhooks, mais son corps délègue à une méthode privée `executeInternal(ctx, pipeline, records []record)` où `records=nil` signifie "utiliser inputModule.Fetch"
  - [ ] Option B : créer un `recordsInput Module` qui implémente `Fetch` en retournant les records pré-chargés — webhook l'utilise
  - [ ] Recommandation : option A (moins disruptif)

- [ ] Task 3 : Nettoyer `runtime/retry.go` (AC #3)
  - [ ] Vérifier les ré-exports actuels
  - [ ] Si aucun call externe : supprimer le fichier

- [ ] Task 4 : Évaluer les stubs (AC #4)
  - [ ] Grep `input.NewStub|filter.NewStub|output.NewStub`
  - [ ] Si uniquement utilisés dans les tests : déplacer dans `input/testutil/stub.go` etc., ou garder en production si pattern "fallback safe" utile
  - [ ] Décision : garder si utile en démo, supprimer sinon

- [ ] Task 5 : Tests (AC #5)
  - [ ] `go test ./... && golangci-lint run`

## Dev Notes

### Rationale

Audit §5 P3.2, P3.3. Le code mort diminue la charge cognitive pour les nouveaux contributeurs. Deux API `Execute*` ressemblantes à 80% est un piège à bug (fix appliqué à l'une, oublié dans l'autre).

### Design Decisions

- **NewExecutor supprimé sans rétrocompatibilité** : c'est un package `internal/`, aucun consommateur externe.
- **Stubs conservés si utiles en démo** : les stubs permettent de tester une config sans implémenter tous les modules (cf. Story 13.1 original).

### Out of Scope

- Refactoring de Scheduler → autre story
- Suppression de `runtime/metadata.go` (traité dans Story 16.4)

## References

- docs/AUDIT_TECHNIQUE_2026-04-21.md §5 P3.2, P3.3
- docs/REFACTORING_NOTES.md (constantes runtime/retry.go)
- `runtime/pipeline.go:102-106, 793-901`

## File List

(à compléter)
