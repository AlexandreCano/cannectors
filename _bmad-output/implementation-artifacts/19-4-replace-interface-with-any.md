# Story 19.4: Replace `interface{}` with `any` (cosmétique)

Status: backlog

## Story

En tant que développeur,
je veux que le code utilise `any` au lieu de `interface{}` (alias disponible depuis Go 1.18),
afin de rendre les signatures plus lisibles et d'aligner le projet sur les conventions Go modernes.

## Acceptance Criteria

1. **Given** le code actuel
   **When** je grep `interface{}` dans `pkg/` et `internal/`
   **Then** toutes les occurrences hors tests sont remplacées par `any`
   **And** les fichiers de test sont aussi adaptés pour cohérence (optionnel).

2. **Given** le type central `map[string]interface{}` (records)
   **When** je le remplace
   **Then** il devient `map[string]any` partout dans les signatures et types
   **And** la rétrocompatibilité n'est pas affectée (alias exact).

3. **Given** les tests existants
   **When** `go test ./... && golangci-lint run`
   **Then** tout passe (refacto purement syntaxique).

4. **Given** les tags JSON et les méthodes d'API publiques
   **When** je les compare avant/après
   **Then** le sérialisé JSON est identique.

## Tasks / Subtasks

- [ ] Task 1 : Remplacement automatisé (AC #1, #2)
  - [ ] `gofmt -r "interface{} -> any" -w ./...`
  - [ ] Alternative manuelle : `grep -rl 'interface{}' --include='*.go' | xargs sed -i 's/interface{}/any/g'`
  - [ ] **ATTENTION** : ne pas remplacer dans les strings ni commentaires qui décrivent le code historique

- [ ] Task 2 : Revue visuelle (AC #1)
  - [ ] Vérifier les diffs sur les fichiers publics (`pkg/connector/types.go`)
  - [ ] Vérifier les tags et méthodes pour s'assurer rien d'autre n'a été modifié

- [ ] Task 3 : Tests (AC #3, #4)
  - [ ] `go build ./... && go test ./... && golangci-lint run`
  - [ ] `go vet ./...`

## Dev Notes

### Rationale

Audit §5 P3.7. Pur cosmétique mais améliore la lisibilité. Go 1.18+ encourage `any`. Le projet est en Go 1.24, pas de raison de garder la forme verbeuse.

### Design Decisions

- **Faire dans un seul commit** : refacto mécanique, revue facilitée par une diff homogène.
- **Faire en dernier** : éviter les conflits de merge si d'autres stories en cours.

### Out of Scope

- Typage plus strict (remplacer `any` par des types concrets) → potentiellement à faire au cas par cas, autre story

## References

- docs/AUDIT_TECHNIQUE_2026-04-21.md §5 P3.7

## File List

(à compléter)
