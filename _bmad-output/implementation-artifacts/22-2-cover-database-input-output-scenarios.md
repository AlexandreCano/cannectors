# Story 22.2: Cover database input and output scenarios

Status: ready-for-dev

## Story

En tant que développeur,
je veux tester les modules `database` en input et output contre PostgreSQL local,
afin de valider les lectures, écritures, transactions et erreurs SQL.

## Acceptance Criteria

1. **Given** une table source seedée
   **When** un pipeline utilise `input.type=database`
   **Then** les lignes attendues sont lues et envoyées vers une destination HTTP.

2. **Given** un endpoint HTTP source
   **When** un pipeline utilise `output.type=database`
   **Then** les records sont écrits dans la table destination attendue.

3. **Given** `transaction=true`
   **When** une écriture échoue au milieu du batch
   **Then** la transaction est rollbackée.

4. **Given** `onError=skip` ou `onError=log`
   **When** une écriture échoue sur un record
   **Then** le comportement observé correspond à la stratégie configurée.

## Tasks / Subtasks

- [ ] Task 1 : Ajouter les pipelines database input
  - [ ] Lecture simple
  - [ ] Lecture avec paramètres
  - [ ] Lecture avec `queryFile`

- [ ] Task 2 : Ajouter les pipelines database output
  - [ ] Insert simple
  - [ ] Upsert via `queryFile`
  - [ ] Transaction on/off

- [ ] Task 3 : Ajouter les scénarios d'erreur
  - [ ] Query invalide
  - [ ] Violation de contrainte
  - [ ] Champ record manquant dans template SQL

## Dev Notes

### Rationale

Les modules database touchent des effets de bord persistants. Le lab doit permettre de vérifier l'état final de la base, pas seulement le statut d'exécution.

### Out of Scope

- Tests MySQL
- Tests PostgreSQL avancés hors syntaxe utilisée par Cannectors

## References

- `internal/modules/input/database.go`
- `internal/modules/output/database.go`
- `examples/07-database-input-basic-to-http.yaml`
- `examples/21-database-output-transaction-query-file.yaml`

## File List

- `test-lab/pipelines/database-*.yaml`
- `test-lab/postgres/assertions/*.sql`
