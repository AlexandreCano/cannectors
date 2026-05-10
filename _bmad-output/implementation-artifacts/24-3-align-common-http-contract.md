# Story 24.3: Align common HTTP contract

Status: ready-for-dev

## Story

En tant qu'utilisateur,
je veux que tous les modules HTTP partagent un contrat clair pour methodes, body, headers, endpoint, retry et timeout,
afin de pouvoir appeler des APIs reelles sans limitation artificielle.

## Acceptance Criteria

1. **Given** un module HTTP sortant (`httpPolling`, `http_call`, `httpRequest`)
   **When** l'utilisateur configure une methode HTTP valide
   **Then** le runtime l'accepte, y compris `DELETE`, `PATCH`, `HEAD` et autres tokens HTTP valides.

2. **Given** un module HTTP sortant
   **When** l'utilisateur configure un body
   **Then** le body peut etre envoye quelle que soit la methode HTTP.

3. **Given** `httpPolling`
   **When** une methode autre que `GET` ou un body est configure
   **Then** le runtime les applique comme pour les autres modules HTTP.

4. **Given** un endpoint avec template
   **When** le schema valide la config
   **Then** `format: uri` ne bloque pas les placeholders.

5. **Given** l'URL finale apres resolution de template
   **When** elle est invalide
   **Then** le runtime renvoie une erreur claire.

6. **Given** des headers configures ou derives de templates
   **When** un header final est invalide
   **Then** le runtime renvoie une erreur claire, sans ignorer silencieusement le header.

7. **Given** `timeoutMs` fourni sur un module HTTP
   **When** la valeur vaut `0`
   **Then** le schema la rejette.

8. **Given** `timeoutMs` omis
   **When** le runtime instancie le module
   **Then** il applique son default interne.

9. **Given** `retry` sur `httpPolling`, `http_call` ou `httpRequest`
   **When** la config est valide
   **Then** le retry est applique par le module.

10. **Given** `defaults.retry`
    **When** les defaults sont resolus
    **Then** ils ne s'appliquent qu'a `httpPolling`, `http_call` et `httpRequest`.

11. **Given** une key HTTP partagee
    **When** `field` ou `paramName` est vide
    **Then** le schema rejette la config.

## Tasks / Subtasks

- [ ] Task 1 : Remplacer l'enum HTTP artificielle
  - [ ] Schema: accepter une string non vide
  - [ ] Runtime: valider avec une logique compatible HTTP token
  - [ ] Normaliser le casing lorsque pertinent

- [ ] Task 2 : Generaliser le body HTTP
  - [ ] Ajouter le contrat commun necessaire au schema
  - [ ] Supporter body/body template/body template file selon le pattern retenu
  - [ ] Appliquer le body sur toutes les methodes

- [ ] Task 3 : Corriger endpoint et headers
  - [ ] Retirer `format: uri`
  - [ ] Valider l'URL finale au runtime
  - [ ] Remplacer les `TryAddValidHeader` silencieux par des erreurs

- [ ] Task 4 : Aligner timeout et retry
  - [ ] `timeoutMs` minimum 1 si fourni
  - [ ] Omission = default runtime
  - [ ] Ajouter `retry` aux schemas manquants
  - [ ] Appeler la validation runtime de retry apres resolution des defaults
  - [ ] Appliquer `defaults.retry` uniquement a `httpPolling`, `http_call`, `httpRequest`

- [ ] Task 5 : Aligner `httpCallKeyConfig`
  - [ ] `field.minLength: 1`
  - [ ] `paramName.minLength: 1`
  - [ ] Garder `paramType` strict `query|path|header`

## Dev Notes

### Product Rules

Les modules HTTP ne doivent pas bloquer une methode valide juste parce qu'elle n'etait pas dans une enum courte. Les contraintes stables vont dans le schema; les validations dependantes de la resolution finale restent runtime.

## References

- `internal/config/schema/common-schema.json`
- `internal/config/schema/input-schema.json`
- `internal/config/schema/filter-schema.json`
- `internal/config/schema/output-schema.json`
- `internal/httpclient`
- `internal/modules/input/http_polling*.go`
- `internal/modules/filter/http_call*.go`
- `internal/modules/output/http_request*.go`

## File List

- `internal/config/schema/*.json`
- `internal/moduleconfig/*.go`
- `internal/modules/input/http_polling*.go`
- `internal/modules/filter/http_call*.go`
- `internal/modules/output/http_request*.go`
- `internal/httpclient/*.go`
