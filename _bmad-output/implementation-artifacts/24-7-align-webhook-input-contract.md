# Story 24.7: Align webhook input contract

Status: ready-for-dev

## Story

En tant qu'utilisateur de webhooks,
je veux une configuration claire pour le timeout par requete, la signature, la concurrence et le rate limit,
afin que le webhook reste disponible tout en protegeant chaque requete entrante.

## Acceptance Criteria

1. **Given** un webhook sans timeout configure
   **When** le serveur demarre
   **Then** il utilise le default runtime actuel pour les timeouts de lecture/ecriture.

2. **Given** un webhook avec `requestTimeoutMs`
   **When** le serveur demarre
   **Then** cette valeur controle `ReadTimeout` et `WriteTimeout` par requete.

3. **Given** une config webhook avec ancien champ `timeoutMs`
   **When** le schema valide la config
   **Then** elle est rejetee, sans compatibilite.

4. **Given** une signature webhook sans `header`
   **When** le module est cree
   **Then** le header par default est `X-Hub-Signature-256`.

5. **Given** une signature webhook avec `secret`
   **When** `header` est absent
   **Then** la config est valide.

6. **Given** `queueSize` ou `maxConcurrent` fournis a `0`
   **When** le schema valide la config
   **Then** la config est rejetee.

7. **Given** `queueSize` ou `maxConcurrent` omis
   **When** le runtime demarre le webhook
   **Then** les defaults runtime sont appliques.

8. **Given** `rateLimit` present
   **When** `requestsPerSecond` est absent ou inferieur a 1
   **Then** la config est rejetee.

9. **Given** `rateLimit.burst` omis
   **When** `requestsPerSecond` est valide
   **Then** le runtime met `burst = requestsPerSecond`.

10. **Given** `ModuleBase` fields sur webhook
    **When** la config est chargee
    **Then** `enabled` et `onError` sont applicables.

11. **Given** un webhook lance
    **When** aucune requete n'arrive
    **Then** `requestTimeoutMs` ne coupe pas la duree de vie du serveur; le webhook continue d'ecouter.

12. **Given** `rateLimit` absent
    **When** le webhook demarre
    **Then** aucun rate limit n'est applique.

## Tasks / Subtasks

- [ ] Task 1 : Renommer le timeout produit
  - [ ] Schema `requestTimeoutMs`
  - [ ] Struct Go
  - [ ] Tests ReadTimeout/WriteTimeout
  - [ ] Retirer `timeoutMs` webhook

- [ ] Task 2 : Aligner signature
  - [ ] `secret` requis si `signature` present
  - [ ] `header` optionnel
  - [ ] Default `X-Hub-Signature-256`

- [ ] Task 3 : Aligner backpressure et rate limit
  - [ ] `queueSize` minimum 1 si fourni
  - [ ] `maxConcurrent` minimum 1 si fourni
  - [ ] `requestsPerSecond` integer minimum 1
  - [ ] `burst` optionnel minimum 1 si fourni

- [ ] Task 4 : Integrer ModuleBase
  - [ ] Embed `connector.ModuleBase`
  - [ ] Tests `enabled`/`onError`

## Dev Notes

### Product Rules

Le webhook n'a pas de timeout de duree de vie. `requestTimeoutMs` s'applique uniquement a la requete HTTP entrante.

## References

- `internal/config/schema/input-schema.json`
- `internal/modules/input/webhook.go`
- `internal/modules/input/webhook_test.go`

## File List

- `internal/config/schema/input-schema.json`
- `internal/modules/input/webhook*.go`
