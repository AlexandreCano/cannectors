# Story 19.1: Add trace/correlation ID to execution logs

Status: backlog

## Story

En tant qu'opérateur,
je veux qu'un trace ID unique soit propagé dans tous les logs d'une même exécution de pipeline,
afin de pouvoir corréler les événements d'une exécution dans un système de log agrégé (ELK, Grafana Loki, etc.).

## Acceptance Criteria

1. **Given** une exécution de pipeline (via `cannectors run`)
   **When** le runtime démarre l'exécution
   **Then** un UUID v4 est généré et ajouté à tous les logs de l'exécution sous la clé `trace_id`
   **And** il est accessible via le contexte Go (`context.WithValue`).

2. **Given** un pipeline déclenché par le scheduler (CRON)
   **When** une exécution planifiée démarre
   **Then** chaque invocation a son propre `trace_id`
   **And** le scheduler lui-même log un `trace_id` pour chaque tick.

3. **Given** un pipeline déclenché par un webhook
   **When** une requête entrante déclenche une exécution
   **Then** le `trace_id` est généré à réception du webhook
   **And** si la requête inclut un header `X-Request-Id` ou `Traceparent` (W3C), il est utilisé comme trace_id à la place de l'UUID généré.

4. **Given** les logs structurés (JSON)
   **When** je filtre par `trace_id=abc123`
   **Then** je vois toute la chaîne : start → input fetch → filters → output → end.

5. **Given** les helpers `logger.LogExecutionStart` et similaires
   **When** je les modifie pour prendre le context
   **Then** ils extraient automatiquement le `trace_id` du context
   **And** aucun call site n'a besoin de le passer manuellement.

## Tasks / Subtasks

- [ ] Task 1 : Ajouter la clé de contexte (AC #1)
  - [ ] Dans `logger/logger.go` : `type traceIDKey struct{}` + `func TraceIDFrom(ctx) string` + `func WithTraceID(ctx, id) context.Context`
  - [ ] Générateur d'UUID via `github.com/google/uuid` (déjà en `indirect` dependency — promouvoir en `require`)

- [ ] Task 2 : Propager dans le runtime (AC #1, #4)
  - [ ] `runtime/pipeline.go:ExecuteWithContext` : générer un trace_id au début, `ctx = logger.WithTraceID(ctx, uuid)`
  - [ ] Modifier `logger.LogExecutionStart/End/StageStart/StageEnd` pour accepter un context et extraire le trace_id
  - [ ] Ajouter `trace_id` à `ExecutionContext` struct (accessible dans tous les helpers)

- [ ] Task 3 : Adapter le scheduler (AC #2)
  - [ ] Dans `scheduler/scheduler.go:runPipeline` : créer un ctx avec trace_id avant d'appeler l'exécuteur
  - [ ] Logger le tick avec ce trace_id

- [ ] Task 4 : Adapter le webhook (AC #3)
  - [ ] Dans `input/webhook.go`, au début de la handler HTTP : générer ou extraire trace_id
  - [ ] Propager dans `ExecuteWithRecordsContext(ctx, ...)`
  - [ ] Support de `X-Request-Id` en priorité, puis `Traceparent` (W3C), sinon nouveau UUID

- [ ] Task 5 : Tests (AC #1, #4)
  - [ ] Test : exécution → tous les logs ont le même trace_id
  - [ ] Test : 2 exécutions parallèles → 2 trace_ids distincts
  - [ ] Test : webhook avec `X-Request-Id` → trace_id = valeur du header

## Dev Notes

### Rationale

Plan.md §7.1 et audit §5 P3.5. En production, un log "filter module execution failed" sans trace_id est inutilisable si 50 pipelines tournent simultanément.

### Design Decisions

- **UUID v4** : standard, suffisamment unique, pas besoin de coordination.
- **Respect de W3C Traceparent** : si un client fournit déjà un trace, on s'y raccroche (utile pour tracing distribué futur).
- **Pas de span hierarchy** (OpenTelemetry) dans cette story : uniquement l'ID top-level.

### Out of Scope

- OpenTelemetry integration → story future potentielle
- Sampling de logs → orthogonal

## References

- docs/AUDIT_TECHNIQUE_2026-04-21.md §5 P3.5
- docs/plan.md §7.1
- `logger/logger.go`
- `runtime/pipeline.go:ExecuteWithContext`

## File List

(à compléter)
