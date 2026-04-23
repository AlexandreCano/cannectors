# Story 20.2: Raise database input/output modules test coverage

Status: backlog

## Story

En tant que développeur,
je veux une couverture ≥ 75 % sur `input/database.go` (55 % actuel) et `output/database.go` (68 % actuel),
afin de sécuriser les chemins d'incrémental (timestamps, ID cursor, bookmarks) et les modes transactionnels.

## Acceptance Criteria

1. **Given** `input/database.go` (613 LOC, 342 LOC test, 55 %)
   **When** je mesure après ajout de tests
   **Then** couverture ≥ 75 %
   **And** les chemins suivants sont testés :
    - Incrémental par timestamp (WHERE `updated_at > $1`)
    - Incrémental par ID (WHERE `id > $1`)
    - Combinaison timestamp + ID
    - Pagination offset + cursor
    - Template `{{lastRunTimestamp}}` substitué correctement
    - Format timestamp driver-specific (PostgreSQL vs MySQL)
    - First run (pas de state) → retourne tous les records

2. **Given** `output/database.go` (377 LOC, 258 LOC test, 68 %)
   **When** je mesure après ajout de tests
   **Then** couverture ≥ 75 %
   **And** les chemins suivants sont testés :
    - Mode INSERT simple
    - Mode UPSERT (ON CONFLICT pour PG, ON DUPLICATE KEY pour MySQL, REPLACE pour SQLite)
    - Mode CUSTOM query templated
    - Transaction : commit sur succès
    - Transaction : rollback sur erreur
    - Batch insert vs un-par-un
    - Erreur sur record individuel avec `onError: skip`

3. **Given** les tests ajoutés utilisent SQLite in-memory par défaut
   **When** je les lance
   **Then** ils durent < 15 secondes au total
   **And** ils passent avec `-race`.

4. **Given** un build tag `integration` pour les tests PostgreSQL/MySQL
   **When** je lance `go test -tags=integration ./...`
   **Then** les tests s'exécutent contre un vrai PostgreSQL/MySQL si `DATABASE_URL` est défini
   **And** ils sont skippés sinon avec un message explicite.

## Tasks / Subtasks

- [ ] Task 1 : Tests DatabaseInput incrémental (AC #1)
  - [ ] Setup helper avec table de records avec `id`, `name`, `updated_at`
  - [ ] `TestDatabaseInput_TimestampIncremental` : state avec `lastTimestamp` → WHERE généré correctement
  - [ ] `TestDatabaseInput_IDIncremental` : idem avec `lastID`
  - [ ] `TestDatabaseInput_CombinedIncremental`
  - [ ] `TestDatabaseInput_FirstRun` : pas de state → tout retourné

- [ ] Task 2 : Tests DatabaseInput pagination (AC #1)
  - [ ] `TestDatabaseInput_PaginationOffset` : LIMIT/OFFSET
  - [ ] `TestDatabaseInput_PaginationCursor` : cursor-based avec cursorField/cursorParam

- [ ] Task 3 : Tests DatabaseOutput modes (AC #2)
  - [ ] `TestDatabaseOutput_Insert`
  - [ ] `TestDatabaseOutput_UpsertSQLite` (REPLACE)
  - [ ] `TestDatabaseOutput_CustomQuery` : avec template
  - [ ] `TestDatabaseOutput_Transaction_CommitOnSuccess`
  - [ ] `TestDatabaseOutput_Transaction_RollbackOnError`

- [ ] Task 4 : Tests DatabaseOutput gestion d'erreur (AC #2)
  - [ ] `TestDatabaseOutput_OnErrorSkip` : record invalide skippé, les autres passent
  - [ ] `TestDatabaseOutput_OnErrorFail` : record invalide arrête tout

- [ ] Task 5 : Build tag integration (AC #4)
  - [ ] Ajouter `//go:build integration` en tête des fichiers PG/MySQL
  - [ ] Documenter dans README : `go test -tags=integration ./...` avec `DATABASE_URL=postgres://...`

- [ ] Task 6 : Validation (AC #1, #2, #3)
  - [ ] `go test -cover -race ./internal/modules/input/... ./internal/modules/output/...`
  - [ ] Vérifier les couvertures

## Dev Notes

### Rationale

Audit §4.1. Database modules critiques pour tout pipeline ETL. L'incrémental est complexe (format timestamp driver-spécifique, combinaison ID+TS) et sous-testé.

### Design Decisions

- **SQLite par défaut** : tests unitaires rapides.
- **Integration tests avec build tag** : pattern Go standard (`//go:build integration`).
- **Pas de Docker en test unitaire** : trop lourd, réservé aux integration.

### Out of Scope

- Tests de performance à large volume → autre story
- Tests de failover DB → hors scope

## References

- docs/AUDIT_TECHNIQUE_2026-04-21.md §4.1
- `input/database.go`, `output/database.go`
- `internal/modules/database_integration_test.go` (existant, 747 LOC — à compléter)

## File List

(à compléter)
