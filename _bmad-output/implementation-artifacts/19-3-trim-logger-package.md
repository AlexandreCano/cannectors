# Story 19.3: Trim `internal/logger` package to essentials

Status: backlog

## Story

En tant que développeur,
je veux que le package `logger` soit dégraissé de ses helpers superflus,
afin de ramener les 771 LOC actuels à ~300 LOC tout en gardant les fonctionnalités utilisées.

## Acceptance Criteria

1. **Given** le package `logger` actuel
   **When** je l'inspecte
   **Then** il fait < 400 LOC (réduction ≥ 40 %)
   **And** chaque fonction exportée est utilisée au moins 2 fois dans le reste du code.

2. **Given** les helpers `LogExecutionStart/End`, `LogStageStart/End`, `LogMetrics`
   **When** ils sont évalués
   **Then** ceux qui n'apportent pas de valeur par rapport à `slog.Info(msg, slog.String(...))` direct sont supprimés
   **And** les attributs communs (pipeline_id, stage) sont factorisés via `slog.Logger.With(...)` plutôt que par helpers.

3. **Given** les types `ExecutionContext`, `ErrorContext`, `ExecutionMetrics`, `ExecutionError`
   **When** je décide lesquels garder
   **Then** `ExecutionContext` est conservé (wrappe `With`), les autres sont supprimés ou remplacés par des helpers plus simples
   **And** leur code migre dans le package `runtime` si plus pertinent là-bas.

4. **Given** tous les call sites de `logger.LogXxx`
   **When** le refacto est fait
   **Then** ils utilisent soit `slog` direct, soit un `ExecutionContext.Logger()` qui retourne un `*slog.Logger` pré-configuré.

5. **Given** les tests
   **When** `go test ./... && golangci-lint run`
   **Then** tout passe.

## Tasks / Subtasks

- [ ] Task 1 : Inventaire (AC #1, #2)
  - [ ] Lister tous les exports de `logger` actuel (Logger, SetLevel, Info, Debug, Warn, Error, WithPipeline, WithModule, ExecutionContext, ExecutionError, ErrorContext, ExecutionMetrics, WithExecution, LogExecutionStart, LogExecutionEnd, LogStageStart, LogStageEnd, LogError, LogMetrics, SetLogFile, CloseLogFile, SetLevelAndFormat, FormatJSON, FormatHuman, ...)
  - [ ] Pour chacun : `grep -rc "logger\.XxxName"` pour compter les usages
  - [ ] Lister ceux à supprimer vs à garder

- [ ] Task 2 : Simplifier (AC #2, #4)
  - [ ] `Info/Debug/Warn/Error` globaux : garder (wrapper simple autour de `Logger.*`)
  - [ ] `LogExecutionStart/End` : remplacer par `ctx.Logger().Info("execution started", ...)` au call site, ou un helper minimal
  - [ ] `LogStageStart/End` : idem
  - [ ] `LogError` : supprimer si son apport par rapport à `Logger.Error` est juste du nommage de champs — documenter la convention sans helper

- [ ] Task 3 : Déplacer `ExecutionContext`/`ExecutionMetrics` dans `runtime` (AC #3)
  - [ ] Ces types sont spécifiques au runtime, pas au logging
  - [ ] Déplacer `runtime/execution_context.go` si pertinent
  - [ ] `logger` conserve seulement `WithPipeline`, `WithModule`, `WithExecution` si utiles

- [ ] Task 4 : Garder la gestion des formats (AC #1)
  - [ ] `FormatJSON` / `FormatHuman`, `SetLevelAndFormat`, `SetLogFile`, `CloseLogFile` : conservés — ils ont une vraie valeur (setup à partir des flags CLI)

- [ ] Task 5 : Adapter les call sites (AC #4)
  - [ ] Grep + remplacement systématique
  - [ ] Pipeline.go : le plus gros consommateur, prévoir un commit dédié

- [ ] Task 6 : Tests (AC #5)
  - [ ] Conserver les tests existants pertinents
  - [ ] `go test ./... && golangci-lint run`

## Dev Notes

### Rationale

Audit §5 P3.1. 771 LOC pour wrapper slog est disproportionné. La plupart des helpers ne factorisent qu'un ou deux champs — un `Logger.With(...)` suffit.

### Design Decisions

- **Pas de deuxième abstraction par-dessus slog** : slog est déjà idiomatique.
- **ExecutionContext reste utile** comme carrier de fields communs — juste déplacé dans `runtime`.

### Out of Scope

- Remplacement de slog par une autre lib → non
- Support d'un backend custom (Loki, Datadog) → hors scope, géré via `slog.Handler` custom à l'infra

## References

- docs/AUDIT_TECHNIQUE_2026-04-21.md §2.1, §5 P3.1
- `logger/logger.go` (771 LOC actuels)

## File List

(à compléter)
