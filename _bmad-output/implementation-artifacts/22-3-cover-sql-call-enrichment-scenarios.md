# Story 22.3: Cover `sql_call` enrichment scenarios

Status: ready-for-dev

## Story

En tant que développeur,
je veux tester `sql_call` dans le lab local,
afin de valider l'enrichissement SQL, le cache et les stratégies de merge avec une vraie base.

## Acceptance Criteria

1. **Given** des records entrants contenant un ID client
   **When** `sql_call` exécute une requête template
   **Then** le résultat SQL enrichit le record attendu.

2. **Given** `mergeStrategy=merge`, `replace` ou `append`
   **When** le pipeline s'exécute
   **Then** la forme du record final correspond à la stratégie.

3. **Given** le cache activé
   **When** plusieurs records partagent la même clé
   **Then** le nombre de requêtes SQL effectives est réduit ou observable.

4. **Given** `queryFile`
   **When** le pipeline démarre
   **Then** la requête est chargée depuis disque et exécutée.

## Tasks / Subtasks

- [ ] Task 1 : Ajouter les pipelines `sql_call`
  - [ ] Merge
  - [ ] Replace
  - [ ] Append avec `resultKey`
  - [ ] Query file

- [ ] Task 2 : Ajouter les fixtures DB de référence
  - [ ] Customer reference
  - [ ] Product reference
  - [ ] Cas résultat vide

- [ ] Task 3 : Ajouter les vérifications
  - [ ] Payload destination enrichi
  - [ ] Résultat append sous la bonne clé
  - [ ] Cache key template

## Dev Notes

### Rationale

`sql_call` combine SQL, templating, cache et mutation de records. Un scénario local réel complète les tests unitaires.

### Out of Scope

- Benchmarks de performance cache
- Tests de concurrence poussés

## References

- `internal/modules/filter/sql_call.go`
- `examples/17-sql-call-merge-cache.yaml`
- `examples/18-sql-call-append-query-file.yaml`

## File List

- `test-lab/pipelines/sql-call-*.yaml`
- `test-lab/postgres/init/002_seed.sql`
