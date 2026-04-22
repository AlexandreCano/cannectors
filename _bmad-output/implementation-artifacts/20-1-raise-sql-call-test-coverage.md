# Story 20.1: Raise `sql_call` filter test coverage to 70 %+

Status: backlog

## Story

En tant que développeur,
je veux que le filter `sql_call` (600 LOC) soit testé avec une couverture ≥ 70 %,
afin de sécuriser un module critique actuellement sous-testé (29 % — 175 LOC de test seulement).

## Acceptance Criteria

1. **Given** `filter/sql_call.go` (600 LOC) et `filter/sql_call_test.go` (175 LOC actuels)
   **When** je mesure la couverture via `go test -cover ./internal/modules/filter/`
   **Then** sql_call atteint ≥ 70 % ligne/branche
   **And** les nouveaux tests utilisent SQLite en mémoire (`:memory:`) pour ne pas dépendre d'un serveur externe.

2. **Given** les chemins critiques non testés
   **When** je les identifie
   **Then** au minimum les cas suivants sont couverts :
    - Query templating avec valeurs simples et complexes
    - Cache hit / miss / eviction
    - Merge strategies (merge/replace/append)
    - Résolution de `ResultKey` en mode `append`
    - Erreurs SQL (connexion perdue, query invalide)
    - Comportement `onError` (fail/skip/log)
    - Configuration avec `queryFile` au lieu d'inline
    - Timeout respecté (via context)
    - Connection string via `${ENV}` variable

3. **Given** les tests ajoutés
   **When** `go test -race ./internal/modules/filter/` est lancé
   **Then** aucune race detected (cache thread-safety vérifiée).

4. **Given** l'exécution des tests
   **When** elle tourne
   **Then** elle dure < 10 secondes (pas de timeout, pas de vrai réseau).

## Tasks / Subtasks

- [ ] Task 1 : Setup de test infrastructure (AC #1)
  - [ ] Helper `setupSQLiteDB(t *testing.T) *sql.DB` — crée une table de fixtures
  - [ ] Helper `newSQLCallModule(t, cfg SQLCallConfig) *SQLCallModule` — avec DB stub injecté

- [ ] Task 2 : Tests des chemins principaux (AC #2)
  - [ ] `TestSQLCall_QueryTemplating` : record → query avec `{{record.id}}` → résultat
  - [ ] `TestSQLCall_CacheHit` / `TestSQLCall_CacheMiss`
  - [ ] `TestSQLCall_CacheTTL` : entrée expire après TTL
  - [ ] `TestSQLCall_MergeStrategies` : chacun des 3 modes
  - [ ] `TestSQLCall_ErrorSkip` / `TestSQLCall_ErrorFail` / `TestSQLCall_ErrorLog`

- [ ] Task 3 : Tests d'edge cases (AC #2)
  - [ ] `TestSQLCall_QueryFile` : lecture depuis disque
  - [ ] `TestSQLCall_EnvConnectionString` : résolution ${ENV}
  - [ ] `TestSQLCall_Timeout` : ctx avec timeout court → erreur
  - [ ] `TestSQLCall_EmptyResult` : query retourne 0 rows

- [ ] Task 4 : Tests de concurrence (AC #3)
  - [ ] `TestSQLCall_ConcurrentProcess` : 100 goroutines traitent des records simultanément via la même module instance
  - [ ] Vérifier cache coherence et pas de deadlock

- [ ] Task 5 : Validation (AC #1, #4)
  - [ ] `go test -cover -race ./internal/modules/filter/... | grep sql_call`
  - [ ] Vérifier le % atteint

## Dev Notes

### Rationale

Audit §4.1 identifie sql_call comme point chaud sous-testé (29 %). Module critique car exécute du SQL arbitraire, gère un cache, et a un impact direct sur la performance des pipelines d'enrichissement.

### Design Decisions

- **SQLite in-memory** : rapide, pas de setup, suffisant pour les cas testés.
- **Pas de mock de `database/sql`** : utiliser un vrai driver SQLite, plus réaliste.
- **Tests parallélisables** : `t.Parallel()` où sensé.

### Out of Scope

- Tests avec PostgreSQL / MySQL réels → Story 20.2 (integration tests)
- Bench performance → autre story

## References

- docs/AUDIT_TECHNIQUE_2026-04-21.md §4.1
- `filter/sql_call.go`
- `filter/sql_call_test.go` (état actuel)

## File List

(à compléter)
