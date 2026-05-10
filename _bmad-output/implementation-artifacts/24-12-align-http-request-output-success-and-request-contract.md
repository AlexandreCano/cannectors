# Story 24.12: Align httpRequest output success and request contract

Status: ready-for-dev

## Story

En tant qu'utilisateur de l'output `httpRequest`,
je veux controler clairement les requetes envoyees et les conditions de succes,
afin que les integrations HTTP de destination soient fiables.

## Acceptance Criteria

1. **Given** `method` omis
   **When** le module `httpRequest` est cree
   **Then** la methode par defaut est `POST`.

2. **Given** une methode HTTP valide
   **When** le module est cree
   **Then** elle est acceptee.

3. **Given** un body configure
   **When** la requete est envoyee
   **Then** le body est applique quelle que soit la methode.

4. **Given** aucun body configure
   **When** la methode est `POST`, `PUT` ou `PATCH`
   **Then** le record est envoye en JSON par defaut.

5. **Given** aucun body configure
   **When** la methode est `GET` ou `DELETE`
   **Then** aucun body par defaut n'est envoye.

6. **Given** `success.lang`
   **When** le schema valide la config
   **Then** la config est rejetee.

7. **Given** `success.expression`
   **When** la config est chargee
   **Then** l'expression `expr` est compilee et evaluee au runtime.

8. **Given** `success.expression` seule
   **When** une reponse HTTP arrive
   **Then** le succes est determine uniquement par l'expression.

9. **Given** `success.statusCodes` seul
   **When** une reponse HTTP arrive
   **Then** le succes est determine uniquement par le status code.

10. **Given** `success.expression` et `success.statusCodes`
    **When** une reponse HTTP arrive
    **Then** les deux conditions doivent etre vraies.

11. **Given** `success.expression` utilise `body`
    **When** la reponse n'est pas un JSON exploitable
    **Then** le runtime renvoie une erreur claire.

12. **Given** `success.expression` est invalide
    **When** le module est cree
    **Then** la config est rejetee avant execution.

13. **Given** `success` absent
    **When** une reponse HTTP arrive
    **Then** les status codes par defaut de succes sont `200`, `201`, `202`, `203`, `204`.

14. **Given** `success.statusCodes` fourni
    **When** la liste est vide, contient un doublon ou un code hors `100..599`
    **Then** la config est rejetee.

15. **Given** `requestMode` inconnu
    **When** le module est cree
    **Then** le runtime renvoie une erreur claire.

16. **Given** `keys` avec champ absent/null/vide dans un record
    **When** la key est utilisee
    **Then** le runtime renvoie une erreur claire.

17. **Given** une key header ou un header template produit un header invalide
    **When** la requete est construite
    **Then** le runtime renvoie une erreur claire.

## Tasks / Subtasks

- [ ] Task 1 : Aligner methode et body
  - [ ] Default `POST`
  - [ ] Toute methode HTTP valide
  - [ ] Body sur toutes methodes
  - [ ] JSON par default pour `POST|PUT|PATCH`
  - [ ] Pas de body par default pour `GET|DELETE`

- [ ] Task 2 : Implementer `success.expression`
  - [ ] Retirer `success.lang`
  - [ ] Compiler avec `expr`
  - [ ] Exposer `statusCode`, `headers`, `body`
  - [ ] Expression seule, status seuls, ou AND des deux
  - [ ] Erreurs claires pour expression invalide ou body non exploitable

- [ ] Task 3 : Valider `success.statusCodes`
  - [ ] Default `200..204`
  - [ ] `minItems: 1`
  - [ ] `uniqueItems: true`
  - [ ] Runtime strict `100..599`
  - [ ] Doublons refuses

- [ ] Task 4 : Rendre `requestMode` et `keys` stricts
  - [ ] `requestMode` absent = `batch`
  - [ ] `requestMode` inconnu = erreur
  - [ ] `keys[].field` minLength 1
  - [ ] `keys[].paramName` minLength 1
  - [ ] `paramType` strict
  - [ ] Missing/null/empty record value = erreur

## Dev Notes

### Product Rules

`success.expression` est une feature produit importante. Si elle est fournie sans `statusCodes`, elle remplace completement la validation par status code.

## References

- `internal/config/schema/output-schema.json`
- `internal/modules/output/http_request.go`
- `internal/modules/output/http_request_send.go`
- `internal/modules/output/http_request_url.go`
- `internal/modules/output/http_request_headers.go`
- `internal/httpclient`

## File List

- `internal/config/schema/output-schema.json`
- `internal/modules/output/http_request*.go`
- `internal/httpclient/*.go`
