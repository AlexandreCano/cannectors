# Story 24.2: Align root pipeline and ModuleBase contracts

Status: ready-for-dev

## Story

En tant qu'utilisateur de Cannectors,
je veux que les champs communs de pipeline et de modules soient coherents partout,
afin de pouvoir utiliser `id`, `enabled`, `onError`, metadata et dry-run options sans surprises.

## Acceptance Criteria

1. **Given** un pipeline avec `id`
   **When** le schema le valide
   **Then** le convertisseur mappe la valeur vers `Pipeline.ID`.

2. **Given** un pipeline avec `tags`
   **When** le schema le valide
   **Then** le champ reste accepte pour l'UX, meme si le runtime l'ignore explicitement.

3. **Given** un pipeline sans `filters`
   **When** il contient un input et un output valides
   **Then** la config est valide et s'execute en input-output direct.

4. **Given** un pipeline avec `enabled`
   **When** il est converti
   **Then** `Pipeline.Enabled` est correctement renseigne, avec `true` par defaut si omis.

5. **Given** un pipeline avec `dryRunOptions.showCredentials`
   **When** il est converti
   **Then** l'option est transmise au runtime dry-run.

6. **Given** un pipeline avec `version`
   **When** le schema valide la config
   **Then** toute string non vide est acceptee, sans pattern semver strict.

7. **Given** un pipeline avec `description`
   **When** le schema valide la config
   **Then** la longueur maximale reste 1024 caracteres.

8. **Given** n'importe quel module input/filter/output
   **When** il declare `enabled` ou `onError`
   **Then** ces champs sont applicables via `connector.ModuleBase`.

9. **Given** un `onError` avec casing mixte
   **When** la config est chargee
   **Then** la valeur est normalisee vers `fail`, `skip` ou `log`.

10. **Given** un `onError` inconnu
   **When** le module est instancie
   **Then** le runtime renvoie une erreur claire, sans fallback silencieux vers `fail`.

11. **Given** un module avec `tags`
    **When** la config est validee
    **Then** les tags restent acceptes comme metadata, meme si le module ne les consomme pas.

## Tasks / Subtasks

- [ ] Task 1 : Mettre a jour le schema pipeline racine
  - [ ] Ajouter `id`
  - [ ] Ajouter `enabled`
  - [ ] Ajouter `dryRunOptions.showCredentials`
  - [ ] Rendre `filters` optionnel
  - [ ] Garder `tags` accepte
  - [ ] Garder `name` non vide et max 128
  - [ ] Assouplir `version` en string optionnelle non vide sans pattern semver strict
  - [ ] Garder `description` max 1024

- [ ] Task 2 : Aligner conversion et types publics
  - [ ] Mapper `id`
  - [ ] Mapper `enabled`
  - [ ] Mapper `dryRunOptions`
  - [ ] Conserver `tags` comme champ accepte mais ignore si aucun champ runtime n'existe

- [ ] Task 3 : Integrer `ModuleBase` partout
  - [ ] Inputs
  - [ ] Filters
  - [ ] Outputs
  - [ ] Webhook
  - [ ] Set
  - [ ] Remove

- [ ] Task 4 : Rendre `onError` strict et normalise
  - [ ] Centraliser la normalisation
  - [ ] Refuser les valeurs inconnues
  - [ ] Adapter les tests existants

## Dev Notes

### Product Rules

`tags` est volontairement accepte par le schema pour l'experience utilisateur, mais n'a pas besoin d'effet runtime immediat. `enabled` et `onError`, eux, sont fonctionnels et doivent etre appliques partout.

## References

- `pkg/connector/types.go`
- `internal/config/schema/pipeline-schema.json`
- `internal/config/schema/common-schema.json`
- `internal/config/converter.go`
- `internal/errhandling`
- `internal/modules/*`

## File List

- `internal/config/schema/*.json`
- `internal/config/converter.go`
- `pkg/connector/types.go`
- `internal/modules/**/*.go`
- `internal/errhandling/*.go`
