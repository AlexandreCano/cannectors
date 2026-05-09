# Story 23.2: Cover retry, error and timeout scenarios

Status: in-review

## Story

En tant que développeur,
je veux tester les erreurs HTTP, retries et timeouts dans le lab local,
afin de valider les comportements de fiabilité du runtime.

## Acceptance Criteria

1. **Given** un endpoint retourne 500 puis succès
   **When** le retry est configuré
   **Then** le pipeline retente et finit en succès.

2. **Given** un endpoint retourne 429 avec `Retry-After`
   **When** `useRetryAfterHeader=true`
   **Then** le retry respecte l'information de délai dans les limites configurées.

3. **Given** un body d'erreur contient une indication retryable
   **When** `retryHintFromBody` est configuré
   **Then** le retry dépend de l'expression configurée.

4. **Given** un endpoint est lent
   **When** `timeoutMs` est inférieur au délai de réponse
   **Then** le pipeline échoue avec une erreur de timeout claire.

5. **Given** une réponse JSON invalide
   **When** un module tente de parser la réponse
   **Then** l'erreur remonte sans panic.

## Tasks / Subtasks

- [ ] Task 1 : Ajouter les stubs WireMock d'erreur
  - [ ] 500 puis 200
  - [ ] 429 avec Retry-After
  - [ ] Body retryable true/false
  - [ ] Fixed delay supérieur au timeout
  - [ ] JSON invalide

- [ ] Task 2 : Ajouter les pipelines reliability
  - [ ] Retry input `httpPolling`
  - [ ] Retry output `httpRequest`
  - [ ] Retry `http_call` si supporté par config

- [ ] Task 3 : Ajouter les vérifications
  - [ ] Nombre de requêtes reçues par WireMock
  - [ ] Statut final du pipeline
  - [ ] Message d'erreur utile

## Dev Notes

### Rationale

Les cas de fiabilité sont difficiles à reproduire à la main. WireMock permet de contrôler précisément les séquences de réponses et les délais.

### Out of Scope

- Chaos testing
- Tests longue durée

## References

- `internal/httpclient`
- `pkg/connector/retry.go`
- `internal/modules/output/http_request.go`
- `internal/modules/input/http_polling_request.go`

## File List

- `test-lab/wiremock/mappings/reliability/*.json`
- `test-lab/pipelines/retry-*.yaml`
