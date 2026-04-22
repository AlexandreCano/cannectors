# Story 17.1: Fix webhook `path` vs `endpoint` schema mismatch

Status: backlog

## Story

En tant qu'utilisateur,
je veux que ma configuration webhook conforme au JSON Schema soit acceptée à l'exécution,
afin que le champ `path` défini par le schéma ne déclenche plus `ErrMissingEndpoint` parce que le code lit `endpoint` à la place.

## Acceptance Criteria

1. **Given** un pipeline avec `input.type: webhook` et `input.path: "/hook/orders"`
   **When** je lance `cannectors run config.yaml`
   **Then** le module webhook démarre sans erreur
   **And** il écoute sur le path `/hook/orders`.

2. **Given** un pipeline legacy avec `input.type: webhook` et `input.endpoint: "/hook/orders"` (ancien format)
   **When** je lance `cannectors run`
   **Then** le module démarre sans erreur (rétrocompatibilité)
   **And** un warning log indique que `endpoint` est déprécié au profit de `path` pour les modules webhook.

3. **Given** les deux champs présents (`path` ET `endpoint`)
   **When** je lance le pipeline
   **Then** `path` prend la priorité
   **And** un warning signale l'ambiguïté.

4. **Given** le JSON Schema `input-schema.json`
   **When** je le consulte
   **Then** il définit `webhookInputConfig.path` comme requis (état actuel préservé) et `endpoint` comme optionnel/déprécié.

5. **Given** les configs d'exemple
   **When** je les examine
   **Then** `configs/examples/14-webhook.yaml` utilise `path` (à aligner si actuellement sur `endpoint`).

## Tasks / Subtasks

- [ ] Task 1 : Modifier le parsing webhook (AC #1, #2, #3)
  - [ ] Dans `input/webhook.go:NewWebhookFromConfig`, accepter `path` ET `endpoint`
  - [ ] Le struct `WebhookInputConfig` gagne un champ `Path string` json:"path"`
  - [ ] Si `cfg.Path != ""` → utiliser ; sinon si `cfg.Endpoint != ""` → utiliser + log warning ; sinon `ErrMissingEndpoint`

- [ ] Task 2 : Aligner le schéma (AC #4)
  - [ ] Vérifier `internal/config/schema/input-schema.json` pour webhook — s'assurer que `path` est bien défini
  - [ ] Si `endpoint` n'y est pas : l'ajouter comme optionnel (pour validation des anciennes configs)

- [ ] Task 3 : Tests (AC #1, #2, #3)
  - [ ] Nouveau test `TestWebhook_PathField` : config avec `path` → succès
  - [ ] Nouveau test `TestWebhook_LegacyEndpointField` : config avec `endpoint` → succès + warning
  - [ ] Nouveau test `TestWebhook_BothFields` : priorité à `path` + warning

- [ ] Task 4 : Exemples (AC #5)
  - [ ] Aligner `configs/examples/14-webhook.yaml` sur `path`

## Dev Notes

### Rationale

Bug critique P0 documenté dans `docs/plan.md` §3 "Bug 1". Le schéma défini par l'équipe utilise `path`, mais `input/webhook.go:145` lit `cfg["endpoint"]`. Résultat : **toute config webhook conforme au schéma échoue au runtime avec `ErrMissingEndpoint`**.

### Design Decisions

- **Rétrocompatibilité** : ne pas casser les configs existantes qui utilisent `endpoint`. Warning de dépréciation.
- **Priorité à `path`** : c'est le nom officiel du schéma. `endpoint` sera retiré dans une future major version.

### Out of Scope

- Supprimer complètement le support `endpoint` → Epic futur (major version)
- Renommer le champ `Endpoint` dans le struct interne → garder pour éviter un diff massif

## References

- docs/AUDIT_TECHNIQUE_2026-04-21.md §5 P1.2
- docs/plan.md §3 "Bug 1"
- `input/webhook.go:135-148`
- `internal/config/schema/input-schema.json`

## File List

(à compléter)
