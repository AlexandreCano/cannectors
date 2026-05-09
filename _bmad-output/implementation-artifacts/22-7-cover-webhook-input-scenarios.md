# Story 22.7: Cover webhook input scenarios

Status: done

## Story

En tant que développeur,
je veux tester l'input `webhook` dans le lab local,
afin de valider les payloads push, signatures HMAC, rate limit et queue workers.

## Acceptance Criteria

1. **Given** un pipeline webhook démarré
   **When** un client POST un payload JSON valide
   **Then** les records sont transmis aux filters et à l'output attendu.

2. **Given** une signature HMAC valide
   **When** la requête est reçue
   **Then** le webhook accepte la payload.

3. **Given** une signature HMAC invalide ou absente
   **When** la requête est reçue
   **Then** le webhook rejette la payload avec un statut attendu.

4. **Given** `queueSize`, `maxConcurrent` et `rateLimit`
   **When** plusieurs requêtes arrivent rapidement
   **Then** le comportement de queue/rate limit est observable.

## Tasks / Subtasks

- [ ] Task 1 : Ajouter les pipelines webhook
  - [ ] Webhook simple
  - [ ] Webhook HMAC
  - [ ] Webhook queue + rate limit

- [ ] Task 2 : Ajouter les scripts d'appel local
  - [ ] POST payload valide
  - [ ] POST signature valide
  - [ ] POST signature invalide
  - [ ] Burst de requêtes

- [ ] Task 3 : Ajouter les vérifications
  - [ ] Payload destination reçue
  - [ ] Statuts HTTP webhook
  - [ ] Logs utiles en cas d'erreur

## Dev Notes

### Rationale

Le webhook est event-driven, donc différent des pipelines polling classiques. Le lab doit prouver qu'un déclenchement HTTP externe traverse bien le runtime.

### Out of Scope

- Load testing
- Déploiement webhook public

## References

- `internal/modules/input/webhook.go`
- `examples/05-webhook-hmac-to-http-single.yaml`
- `examples/06-webhook-queue-rate-limit-to-database.yaml`

## File List

- `test-lab/pipelines/webhook-*.yaml`
- `test-lab/scripts/send-webhook*.sh`
