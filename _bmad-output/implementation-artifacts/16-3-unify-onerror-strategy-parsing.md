# Story 16.3: Unify `onError` strategy parsing across modules

Status: backlog

## Story

En tant que développeur,
je veux un seul point d'entrée pour parser la stratégie `onError` (fail/skip/log),
afin d'éliminer les 4 implémentations qui co-existent (`errhandling.ParseOnErrorStrategy`, `filter.normalizeOnError`, `filter.resolveOnError`, `config.resolveOnError`, plus du code inline dans mapping.go).

## Acceptance Criteria

1. **Given** le code du projet
   **When** je cherche les fonctions `*OnError*` ou `normalizeOnError` / `resolveOnError` qui retournent un string/strategy
   **Then** seul `errhandling.ParseOnErrorStrategy` subsiste pour le parsing d'un string user-input
   **And** `config/converter.go:resolveOnError` (qui a une sémantique différente : "résout l'héritage module > defaults") conserve son nom distinct et ne fait pas de parsing.

2. **Given** les modules filter (`mapping`, `condition`, `script`, `sql_call`, `http_call`, `set`, `remove`)
   **When** ils normalisent leur `onError`
   **Then** ils appellent `errhandling.ParseOnErrorStrategy(cfg.OnError)` et stockent le résultat en `errhandling.OnErrorStrategy`
   **And** ils ne comparent plus à des strings littéraux `"fail"`/`"skip"`/`"log"` (utilisent les constantes typées).

3. **Given** les anciennes fonctions locales
   **When** je les supprime
   **Then** `filter/condition.go:normalizeOnError`, `filter/sql_call.go:resolveOnError`, et la logique inline dans `filter/mapping.go:127-129` sont supprimées
   **And** `filter.OnErrorFail`/`OnErrorSkip`/`OnErrorLog` constantes locales sont supprimées au profit d'`errhandling.OnErrorFail` etc.

4. **Given** les tests existants
   **When** `go test ./...`
   **Then** tous passent.

5. **Given** un développeur qui ajoute un nouveau module
   **When** il lit la doc
   **Then** une convention claire : "utiliser `errhandling.ParseOnErrorStrategy` + comparer à `errhandling.OnErrorXxx`".

## Tasks / Subtasks

- [ ] Task 1 : Inventaire (AC #1)
  - [ ] Grep `normalizeOnError|resolveOnError|OnErrorFail|OnErrorSkip|OnErrorLog` → lister tous les sites
  - [ ] Séparer : parsing (à unifier) vs résolution d'héritage (à garder)

- [ ] Task 2 : Modules filter (AC #2, #3)
  - [ ] `filter/condition.go` : remplacer `normalizeOnError` par `errhandling.ParseOnErrorStrategy`
  - [ ] `filter/sql_call.go` : idem pour `resolveOnError`
  - [ ] `filter/mapping.go` : remplacer la validation inline `if onError != OnErrorFail && ...` par `errhandling.ParseOnErrorStrategy(onError)`
  - [ ] `filter/script.go` : remplacer `normalizeScriptOnError` si existe
  - [ ] `filter/http_call.go` : idem

- [ ] Task 3 : Supprimer les constantes locales (AC #3)
  - [ ] `filter/mapping.go:40-44` : supprimer `OnErrorFail`/`Skip`/`Log` locaux
  - [ ] Importer `errhandling` et utiliser `errhandling.OnErrorFail` etc.
  - [ ] Adapter les tests qui comparent à des strings littéraux

- [ ] Task 4 : Conserver `config.resolveOnError` (AC #1)
  - [ ] Renommer en `resolveOnErrorInheritance` pour lever l'ambiguïté sémantique
  - [ ] Garder dans `config/converter.go` — n'est pas un parser, mais un merger de defaults

- [ ] Task 5 : Documentation (AC #5)
  - [ ] Ajouter un paragraphe dans `docs/MODULE_EXTENSIBILITY.md` : "Parsing onError"

## Dev Notes

### Rationale

Duplication D6. Quatre implémentations avec sémantiques légèrement différentes :
- `errhandling.ParseOnErrorStrategy` : pur parsing user-input → OnErrorStrategy typée
- `filter/condition.go:normalizeOnError` : parsing + retourne string
- `filter/sql_call.go:resolveOnError` : parsing + default `"fail"`
- `filter/mapping.go:124-129` : inline, pas de fonction nommée
- `config/converter.go:resolveOnError` : **résolution d'héritage** (module > defaults), sémantique différente — à renommer.

### Design Decisions

- **Une seule source typée** : `errhandling.OnErrorStrategy` (type string déjà défini).
- **Les modules stockent `errhandling.OnErrorStrategy`, pas `string`** : permet au compilateur de détecter les comparaisons littérales.

### Out of Scope

- Ajouter une nouvelle stratégie `onError: retry-then-skip` → autre story
- Uniformiser avec le style `ParseOnErrorStrategy` (verbeux) ou créer un alias plus court

## References

- docs/AUDIT_TECHNIQUE_2026-04-21.md §3.1 D6, §5 P2.2
- `errhandling/retry.go:170-181` (canonique)
- `filter/condition.go:269-275`
- `filter/sql_call.go:170-180`
- `filter/mapping.go:40-44, 124-129`
- `config/converter.go:289-297` (à renommer)

## File List

(à compléter)
