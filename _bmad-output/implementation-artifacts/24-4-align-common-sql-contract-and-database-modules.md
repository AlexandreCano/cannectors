# Story 24.4: Align common SQL contract and database modules

Status: ready-for-dev

## Story

En tant qu'utilisateur de modules SQL,
je veux un contrat coherent entre input database, output database et `sql_call`,
afin que connexions, queries, fichiers SQL, pool et timeouts se comportent de la meme facon partout.

## Acceptance Criteria

1. **Given** `connectionString` et `connectionStringRef` fournis ensemble
   **When** la config est validee ou instanciee
   **Then** elle est rejetee.

2. **Given** aucun des deux champs de connexion
   **When** la config est chargee
   **Then** elle est rejetee.

3. **Given** `query` et `queryFile` fournis ensemble
   **When** la config est validee ou instanciee
   **Then** elle est rejetee.

4. **Given** `connectionStringRef`
   **When** il ne respecte pas `${ENV_VAR_NAME}`
   **Then** il est rejete.

5. **Given** les champs numeriques SQL fournis
   **When** une valeur vaut `0`
   **Then** le schema la rejette.

6. **Given** ces champs numeriques omis
   **When** le runtime instancie le module
   **Then** les defaults runtime sont appliques.

7. **Given** un `queryFile`
   **When** le module demarre
   **Then** le fichier est valide, lu et les erreurs sont explicites.

8. **Given** un placeholder SQL `{{record.field}}` vers un champ absent dans output database
   **When** le query est execute
   **Then** le parametre SQL recoit `NULL`.

9. **Given** un `sql_call` avec `mergeStrategy: append`
   **When** `resultKey` est absent
   **Then** la config est rejetee.

10. **Given** le cache `sql_call`
    **When** `enabled` est false ou absent
    **Then** aucun cache n'est utilise.

11. **Given** un input `database` sans `schedule`
    **When** il est lance via la CLI
    **Then** il s'execute une seule fois.

12. **Given** un input `database` avec `schedule`
    **When** il est enregistre dans le scheduler
    **Then** il s'execute selon la planification.

13. **Given** une pagination database `cursor` ou `limit-offset`
    **When** elle configure le parametre principal
    **Then** le champ canonique est `param`, pas `cursorParam` ni `offsetParam`.

14. **Given** une pagination database avec `limit` omis
    **When** le module s'execute
    **Then** le runtime applique son default.

15. **Given** une pagination database avec `limit: 0`
    **When** le schema valide la config
    **Then** la config est rejetee.

16. **Given** une pagination database avec type inconnu
    **When** le module est instancie
    **Then** le runtime renvoie une erreur claire.

17. **Given** une config `sql_call` avec `retry`
    **When** le schema valide la config
    **Then** le champ est rejete; le retry DB n'est pas une feature produit pour l'instant.

## Tasks / Subtasks

- [ ] Task 1 : Aligner le schema SQL commun
  - [ ] Garder les `oneOf`
  - [ ] Ajouter `minLength: 1` sur `query` et `queryFile`
  - [ ] Passer les minimums numeriques a 1 si le champ est fourni

- [ ] Task 2 : Renforcer les validations runtime SQL
  - [ ] Refuser double connection field
  - [ ] Refuser double query field
  - [ ] Aligner `connectionStringRef`
  - [ ] Garder validation fichier/template runtime

- [ ] Task 3 : Aligner `sql_call`
  - [ ] `mergeStrategy` strict `merge|replace|append`
  - [ ] `resultKey` obligatoire pour `append`
  - [ ] Cache `enabled|maxSize|ttlSeconds|key`
  - [ ] Retirer `defaultTTL`, sans compat

- [ ] Task 4 : Aligner output database
  - [ ] Garder `transaction` optionnel, default false
  - [ ] Garder `NULL` pour placeholders absents
  - [ ] Appliquer `onError` strict via ModuleBase

- [ ] Task 5 : Aligner input database
  - [ ] `schedule` optionnel
  - [ ] Absence de `schedule` = one-shot
  - [ ] Presence de `schedule` = planifie
  - [ ] Pagination database avec `param`
  - [ ] Retirer `cursorParam` et `offsetParam`, sans compat
  - [ ] `limit` optionnel, minimum 1 si fourni
  - [ ] Type pagination inconnu = erreur

## Dev Notes

### Product Rules

Pas de retrocompatibilite pour `defaultTTL`. Le champ canonique devient `ttlSeconds`. Les paires exclusives restent strictes car le produit n'est pas release et une API propre prime.

## References

- `internal/config/schema/common-schema.json`
- `internal/modules/input/database.go`
- `internal/modules/output/database.go`
- `internal/modules/filter/sql_call.go`
- `internal/moduleconfig/shared.go`
- `internal/database`

## File List

- `internal/config/schema/common-schema.json`
- `internal/modules/input/database*.go`
- `internal/modules/output/database*.go`
- `internal/modules/filter/sql_call*.go`
- `internal/moduleconfig/*.go`
