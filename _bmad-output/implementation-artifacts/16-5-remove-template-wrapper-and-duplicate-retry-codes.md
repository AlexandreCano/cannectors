# Story 16.5: Remove `output/template.go` wrapper and duplicate retryable status codes

Status: backlog

## Story

En tant que développeur,
je veux supprimer le wrapper `output/template.go` (60 LOC de ré-exports de `internal/template`) et la liste dupliquée `defaultRetryableStatusCodes` (présente deux fois : pkg/connector/retry.go:18 et errhandling/errors.go:409),
afin d'éliminer du code mort et des sources de divergence silencieuse.

## Acceptance Criteria

1. **Given** `output/template.go`
   **When** je tente de le supprimer
   **Then** les seuls consommateurs (`output/http_request.go`, `output/database.go` si applicable) ont été migrés pour utiliser `template.Evaluator` directement
   **And** le fichier est supprimé.

2. **Given** `output/http_request.go`
   **When** je lis son code
   **Then** `templateEvaluator *TemplateEvaluator` devient `templateEvaluator *template.Evaluator`
   **And** les appels `h.templateEvaluator.EvaluateTemplate(...)` deviennent `h.templateEvaluator.Evaluate(...)`.

3. **Given** le code du projet
   **When** je cherche `defaultRetryableStatusCodes` ou une liste littérale `{429, 500, 502, 503, 504}`
   **Then** il n'existe qu'une seule source : `connector.DefaultRetryableStatusCodes()` dans `pkg/connector/retry.go`
   **And** `errhandling/errors.go:408-417` est modifié pour retourner `connector.DefaultRetryableStatusCodes()`.

4. **Given** les tests
   **When** `go test ./...`
   **Then** tout passe.

## Tasks / Subtasks

- [ ] Task 1 : Migrer `output/http_request.go` vers `template.Evaluator` direct (AC #1, #2)
  - [ ] Import `internal/template`
  - [ ] Remplacer `*TemplateEvaluator` → `*template.Evaluator`
  - [ ] Renommer les appels `EvaluateTemplate` → `Evaluate`, `EvaluateTemplateForURL` → `EvaluateForURL`
  - [ ] Remplacer `HasTemplateVariables(s)` → `template.HasVariables(s)`
  - [ ] Remplacer `ValidateTemplateSyntax(s)` → `template.ValidateSyntax(s)`

- [ ] Task 2 : Supprimer `output/template.go` (AC #1)
  - [ ] Vérifier qu'aucun autre package n'importe `output.TemplateEvaluator`, `output.HasTemplateVariables`, `output.ValidateTemplateSyntax`
  - [ ] Supprimer le fichier
  - [ ] Supprimer `output/template_test.go` si son contenu était juste un ré-test du wrapper

- [ ] Task 3 : Unifier `defaultRetryableStatusCodes` (AC #3)
  - [ ] `errhandling/errors.go:408-417` : remplacer la variable locale et la fonction par un ré-export `func DefaultRetryableStatusCodes() []int { return connector.DefaultRetryableStatusCodes() }`
  - [ ] Ou supprimer complètement la fonction si elle n'a que `errhandling.IsRetryableStatusCode` comme consommateur — dans ce cas, importer directement depuis `connector`

- [ ] Task 4 : Tests (AC #4)
  - [ ] `go test ./... && golangci-lint run`

## Dev Notes

### Rationale

Deux nettoyages de petite ampleur mais très visibles dans la codebase :

- `output/template.go` (D11 implicite) : 60 LOC de wrappers triviaux. Le commentaire "backward compatibility" est faux — c'est du code interne.
- `defaultRetryableStatusCodes` (D9) : deux définitions identiques avec deux API. Si un jour on ajoute `408` dans l'une, l'autre divergera silencieusement.

### Design Decisions

- **Supprimer plutôt que ré-exporter** : pas de compat externe à maintenir puisque `output.TemplateEvaluator` n'est pas dans `pkg/`.
- **`connector` est la source de vérité** : il est public, accessible à tous les modules.

### Out of Scope

- Refonte de l'API de template → garder les signatures existantes
- Ajout de nouveaux codes retryables → autre story

## References

- docs/AUDIT_TECHNIQUE_2026-04-21.md §3.1 D9, §5 P2.4
- `output/template.go:1-60` (à supprimer)
- `pkg/connector/retry.go:17-20` (canonique)
- `errhandling/errors.go:408-417` (à déduire)

## File List

(à compléter)
