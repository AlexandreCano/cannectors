# Story 22.1: Cover HTTP pagination scenarios

Status: ready-for-dev

## Story

En tant que développeur,
je veux tester les trois modes de pagination HTTP,
afin de garantir que `httpPolling` agrège correctement les records paginés.

## Acceptance Criteria

1. **Given** un endpoint paginé par numéro de page
   **When** le pipeline utilise `pagination.type=page`
   **Then** tous les records attendus sont récupérés dans l'ordre.

2. **Given** un endpoint paginé par offset
   **When** le pipeline utilise `pagination.type=offset`
   **Then** les requêtes incluent les bons paramètres offset/limit.

3. **Given** un endpoint paginé par cursor
   **When** le pipeline utilise `pagination.type=cursor`
   **Then** le cursor retourné par la page précédente est utilisé pour la suivante.

4. **Given** la dernière page
   **When** elle ne contient plus de records ou plus de cursor
   **Then** le polling s'arrête proprement sans boucle infinie.

## Tasks / Subtasks

- [ ] Task 1 : Ajouter les stubs WireMock paginés
  - [ ] Page pagination
  - [ ] Offset pagination
  - [ ] Cursor pagination

- [ ] Task 2 : Ajouter les pipelines de scénario
  - [ ] Pipeline page vers destination HTTP
  - [ ] Pipeline offset vers destination HTTP
  - [ ] Pipeline cursor vers destination HTTP

- [ ] Task 3 : Ajouter les vérifications manuelles
  - [ ] Nombre de pages appelées
  - [ ] Nombre de records envoyés
  - [ ] Ordre des IDs

## Dev Notes

### Rationale

La pagination est une source fréquente de régression. Le lab doit couvrir les trois stratégies supportées par le code réel.

### Out of Scope

- State persistence
- Retry sur pages en erreur

## References

- `internal/modules/input/http_polling_pagination.go`
- `examples/02-http-polling-page-pagination.yaml`
- `examples/03-http-polling-offset-pagination-state.yaml`
- `examples/04-http-polling-cursor-oauth2.yaml`

## File List

- `test-lab/wiremock/mappings/source/pagination-*.json`
- `test-lab/pipelines/pagination-*.yaml`
