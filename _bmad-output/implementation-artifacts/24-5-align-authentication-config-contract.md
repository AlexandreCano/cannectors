# Story 24.5: Align authentication config contract

Status: ready-for-dev

## Story

En tant qu'utilisateur,
je veux que la configuration d'authentification soit stricte et previsible,
afin qu'une erreur d'auth ne soit jamais ignoree silencieusement.

## Acceptance Criteria

1. **Given** une auth `api-key` avec `location` omis
   **When** le handler est cree
   **Then** la location par defaut est `header`.

2. **Given** une auth `api-key` avec `location: header`
   **When** la requete est construite
   **Then** la cle est appliquee dans un header.

3. **Given** une auth `api-key` avec `location: query`
   **When** la requete est construite
   **Then** la cle est appliquee dans la query string.

4. **Given** une auth `api-key` avec location inconnue
   **When** la config est instanciee
   **Then** le runtime renvoie une erreur claire.

5. **Given** une config OAuth2 avec `scope`
   **When** le handler OAuth2 est cree
   **Then** le scope string est transmis et split localement avec `strings.Fields`.

6. **Given** le type `connector.CredentialsOAuth2`
   **When** il represente la config utilisateur
   **Then** il expose `Scope string`, pas `Scopes []string`.

7. **Given** des champs inconnus dans une config auth utilisateur
   **When** le pipeline passe par le schema
   **Then** ils sont rejetes par le schema strict.

8. **Given** une config auth decodee directement en Go avec des champs inconnus
   **When** elle bypass le schema utilisateur
   **Then** le code Go peut continuer a les ignorer; ce n'est pas une erreur runtime produit.

## Tasks / Subtasks

- [ ] Task 1 : Rendre `api-key.location` strict runtime
  - [ ] `header`
  - [ ] `query`
  - [ ] default `header`
  - [ ] erreur sur valeur inconnue

- [ ] Task 2 : Aligner OAuth2 scope
  - [ ] Remplacer `Scopes []string` par `Scope string`
  - [ ] Split local dans le handler OAuth2
  - [ ] Adapter les tests et callers

- [ ] Task 3 : Verifier schema auth
  - [ ] Garder `additionalProperties: false`
  - [ ] Garder `scope` comme string
  - [ ] Ne pas ajouter `scopes`

## Dev Notes

### Product Rules

OAuth2 expose `scope` string parce que c'est le format standard de config. Le split est un detail interne du handler.

## References

- `internal/config/schema/auth-schema.json`
- `internal/auth`
- `pkg/connector/types.go`

## File List

- `internal/config/schema/auth-schema.json`
- `internal/auth/*.go`
- `pkg/connector/types.go`
- `internal/auth/*_test.go`
