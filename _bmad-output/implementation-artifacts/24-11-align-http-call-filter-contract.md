# Story 24.11: Align http_call filter contract

Status: ready-for-dev

## Story

En tant qu'utilisateur de `http_call`,
je veux pouvoir enrichir mes records avec n'importe quelle API HTTP tout en controlant cache, retry, body et merge,
afin que le module soit fiable et previsible.

## Acceptance Criteria

1. **Given** `http_call.method` avec une methode HTTP valide
   **When** le module est cree
   **Then** la methode est acceptee.

2. **Given** un body configure sur `http_call`
   **When** la requete est envoyee
   **Then** le body est applique quelle que soit la methode HTTP.

3. **Given** `http_call.retry`
   **When** la config est valide
   **Then** le schema l'accepte et le runtime l'applique.

4. **Given** `cache.enabled` absent ou false
   **When** le module s'execute
   **Then** le cache n'est pas utilise.

5. **Given** `cache.enabled: true`
   **When** `maxSize` ou `ttlSeconds` sont omis
   **Then** les defaults runtime sont appliques.

6. **Given** `cache.defaultTTL`
   **When** le schema valide la config
   **Then** la config est rejetee, sans compatibilite.

7. **Given** `mergeStrategy` inconnue
   **When** le module est cree
   **Then** le runtime renvoie une erreur claire.

8. **Given** `mergeStrategy: append`
   **When** le module merge la reponse
   **Then** le comportement existant `append` est conserve.

9. **Given** endpoint, headers, body ou cache key avec template invalide
   **When** le module est cree
   **Then** le runtime renvoie une erreur claire.

10. **Given** un header final invalide apres resolution
    **When** la requete est construite
    **Then** le runtime renvoie une erreur.

11. **Given** une key `http_call`
    **When** `field`, `paramType` ou `paramName` est invalide
    **Then** le runtime renvoie une erreur au demarrage.

12. **Given** une key `http_call` reference un champ record absent, null ou vide
    **When** la requete est construite
    **Then** le runtime renvoie une erreur claire.

13. **Given** `cache.maxSize` ou `cache.ttlSeconds` vaut `0`
    **When** le schema valide la config
    **Then** la config est rejetee si le champ est fourni.

## Tasks / Subtasks

- [ ] Task 1 : Aligner methode et body
  - [ ] Toute methode HTTP valide
  - [ ] Body pour toutes methodes
  - [ ] Retirer restrictions `POST/PUT`

- [ ] Task 2 : Aligner retry
  - [ ] Ajouter schema
  - [ ] Appliquer runtime
  - [ ] Valider bornes retry

- [ ] Task 3 : Aligner cache
  - [ ] Honorer `enabled`
  - [ ] Cache off par default
  - [ ] Renommer `defaultTTL` en `ttlSeconds`
  - [ ] Retirer compat `defaultTTL`
  - [ ] `maxSize` et `ttlSeconds` minimum 1 si fournis

- [ ] Task 4 : Aligner merge et keys
  - [ ] `mergeStrategy` strict
  - [ ] Garder `merge|replace|append`
  - [ ] Valider `keys[].field`, `keys[].paramType`, `keys[].paramName`
  - [ ] Missing/null/empty record value pour une key = erreur
  - [ ] Valider templates
  - [ ] Headers invalides = erreur

## Dev Notes

### Product Rules

`append` est conserve volontairement. `cache.enabled` doit etre reellement honore car l'utilisateur s'attend a pouvoir desactiver le cache.

## References

- `internal/config/schema/filter-schema.json`
- `internal/modules/filter/http_call.go`
- `internal/modules/filter/http_call_config.go`
- `internal/moduleconfig/shared.go`

## File List

- `internal/config/schema/filter-schema.json`
- `internal/modules/filter/http_call*.go`
- `internal/moduleconfig/*.go`
