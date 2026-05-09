# Story 21.4: Add WireMock destination API stubs

Status: ready-for-dev

## Story

En tant que développeur,
je veux des endpoints WireMock destination qui capturent les requêtes envoyées,
afin de vérifier les modules `httpRequest` en batch et en single record.

## Acceptance Criteria

1. **Given** WireMock est démarré
   **When** un pipeline envoie un batch
   **Then** l'endpoint destination accepte la requête et la journalise.

2. **Given** un pipeline envoie un record à la fois
   **When** l'URL contient des path params, query params ou headers dynamiques
   **Then** WireMock peut matcher ces éléments.

3. **Given** les requêtes reçues
   **When** un développeur interroge l'admin API WireMock
   **Then** il peut inspecter le body, les headers, la méthode et l'URL.

4. **Given** un payload invalide ou une route inattendue
   **When** le pipeline appelle WireMock
   **Then** l'échec est visible et exploitable pour le debug.

## Tasks / Subtasks

- [ ] Task 1 : Ajouter les endpoints destination batch
  - [ ] POST import customers
  - [ ] POST import orders
  - [ ] POST import inventory

- [ ] Task 2 : Ajouter les endpoints destination single
  - [ ] PUT/PATCH avec path param
  - [ ] Matching query params
  - [ ] Matching headers dynamiques

- [ ] Task 3 : Ajouter les helpers d'inspection
  - [ ] Commande pour lister les requêtes reçues
  - [ ] Commande pour reset le journal WireMock

## Dev Notes

### Rationale

Les scénarios d'intégration doivent vérifier non seulement que le pipeline réussit, mais aussi que la requête HTTP générée est correcte.

### Design Decisions

- Utiliser l'admin API WireMock pour inspecter les requêtes.
- Garder les assertions automatisées pour l'Epic 23.

### Out of Scope

- Runner automatisé
- CI

## References

- `internal/modules/output/http_request.go`
- `internal/modules/output/http_request_url.go`
- `internal/modules/output/http_request_body.go`

## File List

- `test-lab/wiremock/mappings/destination/*.json`
- `test-lab/README.md`
