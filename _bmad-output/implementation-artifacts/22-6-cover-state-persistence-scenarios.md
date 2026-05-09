# Story 22.6: Cover state persistence scenarios

Status: done

## Story

En tant que développeur,
je veux tester la persistance d'état sur plusieurs exécutions,
afin de garantir que les pipelines reprennent correctement depuis le dernier timestamp ou ID traité.

## Acceptance Criteria

1. **Given** un pipeline avec timestamp persistence
   **When** il est exécuté deux fois
   **Then** le second run ajoute le bon query param timestamp à la source HTTP.

2. **Given** un pipeline avec ID persistence
   **When** il est exécuté deux fois
   **Then** le second run ajoute le bon query param ID à la source HTTP.

3. **Given** timestamp et ID sont activés ensemble
   **When** le pipeline traite des records
   **Then** l'état persiste les deux valeurs attendues.

4. **Given** le storage path du lab
   **When** le lab est reset
   **Then** l'état précédent est supprimé pour garantir un scénario déterministe.

## Tasks / Subtasks

- [ ] Task 1 : Ajouter les pipelines state persistence
  - [ ] Timestamp only
  - [ ] ID only
  - [ ] Timestamp + ID

- [ ] Task 2 : Ajouter les stubs WireMock avec assertions de query params
  - [ ] Premier run sans query param
  - [ ] Second run avec query param attendu

- [ ] Task 3 : Ajouter la gestion de storage local
  - [ ] Dossier `test-lab/state`
  - [ ] Reset avant scénario
  - [ ] Inspection du fichier de state

## Dev Notes

### Rationale

La persistance d'état dépend d'exécutions successives et de fichiers locaux; elle est donc mieux validée par un scénario d'intégration réel.

### Out of Scope

- State persistence database input si non supportée de la même manière que HTTP polling
- Migration de format de state

## References

- `internal/modules/input/http_polling_state.go`
- `internal/persistence`
- `examples/03-http-polling-offset-pagination-state.yaml`

## File List

- `test-lab/pipelines/state-*.yaml`
- `test-lab/state/.gitkeep`
