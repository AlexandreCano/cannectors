# Story 21.3: Add WireMock source API stubs

Status: ready-for-dev

## Story

En tant que développeur,
je veux des endpoints WireMock source représentant des APIs externes,
afin de tester `httpPolling` avec des réponses HTTP contrôlées.

## Acceptance Criteria

1. **Given** WireMock est démarré
   **When** j'appelle les endpoints source
   **Then** ils retournent des payloads JSON déterministes compatibles avec les pipelines de test.

2. **Given** un pipeline `httpPolling`
   **When** il configure `dataField`
   **Then** au moins un endpoint source retourne les records dans un champ imbriqué.

3. **Given** un endpoint source protégé par header
   **When** le header attendu est absent
   **Then** WireMock retourne une erreur contrôlée.

4. **Given** les mappings WireMock
   **When** je lis les fichiers
   **Then** chaque endpoint est nommé selon le scénario qu'il supporte.

## Tasks / Subtasks

- [ ] Task 1 : Ajouter les endpoints source nominaux
  - [ ] Source simple avec tableau JSON racine
  - [ ] Source avec `dataField`
  - [ ] Source avec headers attendus

- [ ] Task 2 : Ajouter les fixtures JSON
  - [ ] Customers
  - [ ] Orders
  - [ ] Inventory
  - [ ] Payloads avec champs optionnels

- [ ] Task 3 : Ajouter une vérification manuelle
  - [ ] Commandes `curl` documentées
  - [ ] Exemple de réponse attendu

## Dev Notes

### Rationale

Cette story fournit les sources HTTP nécessaires avant d'ajouter les scénarios plus avancés comme pagination, auth et retry.

### Design Decisions

- Les stubs restent statiques dans cette story.
- Les comportements dynamiques sont couverts par les stories suivantes.

### Out of Scope

- Pagination
- Retry et erreurs transitoires
- OAuth2

## References

- `internal/modules/input/http_polling.go`
- `examples/01-http-polling-basic-to-http-batch.yaml`

## File List

- `test-lab/wiremock/mappings/source/*.json`
- `test-lab/wiremock/__files/source/*.json`
- `test-lab/README.md`
