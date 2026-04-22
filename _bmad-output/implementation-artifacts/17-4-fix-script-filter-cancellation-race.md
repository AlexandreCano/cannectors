# Story 17.4: Fix `script` filter context cancellation race condition

Status: backlog

## Story

En tant que développeur,
je veux que l'annulation de contexte sur le filter `script` soit sûre même si le script a déjà terminé,
afin d'éviter des panics ou des fuites de goroutines quand `context.Cancel` est appelé pendant une exécution rapide.

## Acceptance Criteria

1. **Given** un filter `script` qui termine normalement en < 1ms
   **When** le contexte est annulé au même moment
   **Then** aucune panic ni goroutine leak
   **And** le résultat du script est retourné normalement si terminé avant l'interrupt.

2. **Given** un filter `script` qui boucle indéfiniment
   **When** le contexte est annulé
   **Then** le Goja runtime est interrompu via `runtime.Interrupt(...)` dans un délai raisonnable (< 100ms)
   **And** une erreur d'annulation est retournée.

3. **Given** l'implémentation actuelle
   **When** je l'analyse
   **Then** la goroutine de monitoring utilise un flag atomic pour vérifier l'état du script avant d'appeler `Interrupt`
   **And** la goroutine se termine proprement dès que le script se termine (pas de leak).

4. **Given** un test de stress
   **When** je lance 1000 itérations rapides en parallèle avec cancel aléatoire
   **Then** zéro panic, zéro goroutine leak (vérifié via `runtime.NumGoroutine()` avant/après).

## Tasks / Subtasks

- [ ] Task 1 : Analyser la race actuelle (AC #3)
  - [ ] Lire `filter/script.go:274` et le flow autour de `runtime.Interrupt`
  - [ ] Identifier les moments où `Interrupt` peut être appelé après la fin du script
  - [ ] Documenter la race dans un commentaire

- [ ] Task 2 : Introduire un flag atomic (AC #1, #3)
  - [ ] Ajouter `scriptFinished atomic.Bool` sur `ScriptModule`
  - [ ] La goroutine de monitoring : `select { case <-ctx.Done(): if !m.scriptFinished.Load() { m.runtime.Interrupt(...) } case <-done: }`
  - [ ] Canal `done` signalé en defer après l'exécution du script

- [ ] Task 3 : Tests (AC #1, #2, #4)
  - [ ] Test court : script normal, cancel simultané → pas de panic
  - [ ] Test long : script infini, cancel → interrupt + erreur
  - [ ] Test stress : 1000 itérations parallèles, vérifier `runtime.NumGoroutine` stable

## Dev Notes

### Rationale

Bug moyen P1 de `docs/plan.md` §3 "Bug 6". `runtime.Interrupt()` sur un Goja runtime déjà libéré peut paniquer. La goroutine de monitoring n'a pas de protection contre ce cas.

### Design Decisions

- **Atomic bool** plutôt que mutex : la vérif + interrupt doit être non-bloquante.
- **Goroutine self-cleanup via canal `done`** : pas de GC forcé, la goroutine se termine dès que le script rend la main.

### Out of Scope

- Timeout sur script (déjà présent via context timeout) → pas de changement
- Rebuild du Goja runtime à chaque Process → trop coûteux

## References

- docs/AUDIT_TECHNIQUE_2026-04-21.md §5 P1.2
- docs/plan.md §3 "Bug 6"
- `filter/script.go` autour de la ligne 274

## File List

(à compléter)
