# Story 22.4: Cover `http_call` enrichment scenarios

Status: ready-for-dev

## Story

En tant que développeur,
je veux tester `http_call` contre WireMock,
afin de valider les enrichissements HTTP avec keys, templates, cache et merge strategies.

## Acceptance Criteria

1. **Given** un record contenant une clé
   **When** `http_call` utilise une key path, query ou header
   **Then** WireMock reçoit la requête attendue.

2. **Given** `method=POST` ou `PUT` avec `bodyTemplateFile`
   **When** le pipeline s'exécute
   **Then** le body envoyé à WireMock contient les valeurs du record.

3. **Given** `mergeStrategy=merge`, `replace` ou `append`
   **When** WireMock retourne une réponse JSON
   **Then** le record envoyé à l'output reflète la stratégie configurée.

4. **Given** le cache activé
   **When** deux records utilisent la même clé de cache
   **Then** l'appel HTTP d'enrichissement est évité pour le second record.

## Tasks / Subtasks

- [ ] Task 1 : Ajouter les stubs WireMock d'enrichissement
  - [ ] GET avec path param
  - [ ] GET avec query param
  - [ ] Header key
  - [ ] POST avec body template

- [ ] Task 2 : Ajouter les pipelines `http_call`
  - [ ] Merge
  - [ ] Replace
  - [ ] Append
  - [ ] Cache key

- [ ] Task 3 : Ajouter les vérifications
  - [ ] Requêtes WireMock reçues
  - [ ] Payload final envoyé à destination
  - [ ] Nombre d'appels d'enrichissement

## Dev Notes

### Rationale

`http_call` est proche d'un input dans un filter. Les scénarios doivent vérifier à la fois la requête émise et le record enrichi.

### Out of Scope

- Auth HTTP avancée
- Retry sur `http_call`

## References

- `internal/modules/filter/http_call.go`
- `internal/modules/filter/http_call_config.go`
- `examples/14-http-call-get-merge-cache.yaml`
- `examples/16-http-call-post-template-replace.yaml`

## File List

- `test-lab/wiremock/mappings/enrichment/*.json`
- `test-lab/pipelines/http-call-*.yaml`
