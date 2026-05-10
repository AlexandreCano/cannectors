# Story 24.1: Enforce schema validation before pipeline execution

Status: ready-for-dev

## Story

En tant que mainteneur du runtime,
je veux que toute execution utilisateur passe obligatoirement par la validation schema,
afin que le schema devienne le contrat produit unique avant conversion et runtime.

## Acceptance Criteria

1. **Given** une config pipeline chargee depuis fichier
   **When** l'utilisateur lance une execution normale
   **Then** la validation JSON Schema est executee avant la conversion en `connector.Pipeline`.

2. **Given** une config invalide selon le schema
   **When** l'utilisateur lance le pipeline
   **Then** l'execution s'arrete avant instanciation des modules avec une erreur de validation claire.

3. **Given** une config valide selon le schema
   **When** elle est convertie puis executee
   **Then** le runtime ne refait pas de decisions produit contradictoires avec le schema.

4. **Given** un constructeur Go appele directement par un test ou par du code interne
   **When** la config bypass le schema
   **Then** les validations defensives runtime restent en place pour les erreurs critiques.

5. **Given** un champ inconnu dans une config utilisateur
   **When** le schema le rejette
   **Then** le pipeline ne demarre pas, meme si `json.Unmarshal` l'aurait ignore.

## Tasks / Subtasks

- [ ] Task 1 : Identifier tous les chemins CLI/runtime qui chargent un pipeline
  - [ ] Execution normale
  - [ ] Dry-run
  - [ ] Validation command
  - [ ] Execution planifiee
  - [ ] Execution webhook/callback

- [ ] Task 2 : Ajouter le passage obligatoire par `internal/config/validator`
  - [ ] Valider avant conversion
  - [ ] Propager les erreurs schema sans les masquer
  - [ ] Eviter les doubles chemins d'execution non valides

- [ ] Task 3 : Ajouter des tests de garde-fou
  - [ ] Config avec champ inconnu rejetee avant runtime
  - [ ] Config avec module invalide rejetee avant factory
  - [ ] Config valide executee normalement

## Dev Notes

### Product Rules

Le schema est le contrat utilisateur. Le code Go peut continuer a ignorer les champs inconnus lorsqu'il est appele directement, mais ce chemin n'est pas le chemin produit.

### Verification

Executer au minimum les tests config, CLI/runtime touches, puis `go test ./...` si le chemin de chargement commun est modifie. Executer `golangci-lint run ./...`.

## References

- `internal/config/validator.go`
- `internal/config/converter.go`
- `cmd/cannectors/main.go`
- `internal/runtime/pipeline.go`
- `CLAUDE.md`

## File List

- `cmd/cannectors/main.go`
- `internal/config/validator.go`
- `internal/config/*_test.go`
- `internal/runtime/*_test.go`
