# Story 23.1: Cover HTTP authentication scenarios

Status: ready-for-dev

## Story

En tant que développeur,
je veux tester les authentifications HTTP supportées dans le lab local,
afin de valider api-key, bearer, basic et OAuth2 client credentials sans dépendre d'un service tiers.

## Acceptance Criteria

1. **Given** un endpoint WireMock exige une API key en header
   **When** le pipeline fournit la bonne config `api-key`
   **Then** la requête est acceptée.

2. **Given** un endpoint WireMock exige une API key en query param
   **When** le pipeline fournit `location=query`
   **Then** la query contient le param attendu.

3. **Given** un endpoint WireMock exige bearer ou basic auth
   **When** le pipeline fournit les credentials attendus
   **Then** l'appel réussit.

4. **Given** un endpoint token OAuth2 mocké
   **When** le pipeline utilise client credentials
   **Then** le token est obtenu puis utilisé sur l'endpoint protégé.

5. **Given** des credentials invalides
   **When** le pipeline s'exécute
   **Then** l'échec est explicite et ne masque pas la cause.

## Tasks / Subtasks

- [ ] Task 1 : Ajouter les stubs WireMock auth
  - [ ] API key header
  - [ ] API key query
  - [ ] Bearer token
  - [ ] Basic auth
  - [ ] OAuth2 token endpoint

- [ ] Task 2 : Ajouter les pipelines auth
  - [ ] Auth côté input
  - [ ] Auth côté `http_call`
  - [ ] Auth côté output

- [ ] Task 3 : Ajouter les cas négatifs
  - [ ] Missing credential
  - [ ] Wrong token
  - [ ] OAuth2 401 persistant

## Dev Notes

### Rationale

Les auth sont critiques en production et traversent plusieurs modules HTTP. Les tester localement évite les dépendances à des APIs externes.

### Out of Scope

- Rotation réelle de secrets
- Intégration avec vault ou secret manager

## References

- `internal/auth`
- `examples/04-http-polling-cursor-oauth2.yaml`
- `examples/20-http-output-retry-auth-api-key.yaml`
- `examples/23-auth-basic-bearer-query-key.yaml`

## File List

- `test-lab/wiremock/mappings/auth/*.json`
- `test-lab/pipelines/auth-*.yaml`
