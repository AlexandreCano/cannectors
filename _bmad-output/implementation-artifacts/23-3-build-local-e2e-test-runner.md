# Story 23.3: Build local E2E test runner

Status: ready-for-dev

## Story

En tant que développeur,
je veux un runner local qui orchestre le lab, lance les pipelines et vérifie les résultats,
afin d'exécuter les scénarios d'intégration avec une seule commande fiable.

## Acceptance Criteria

1. **Given** le lab Docker est disponible
   **When** je lance le runner
   **Then** il reset WireMock, la base et le state local avant chaque scénario ou suite.

2. **Given** un scénario HTTP
   **When** le pipeline se termine
   **Then** le runner vérifie les requêtes WireMock reçues et les payloads attendus.

3. **Given** un scénario database
   **When** le pipeline se termine
   **Then** le runner vérifie l'état final PostgreSQL via assertions SQL.

4. **Given** un scénario échoue
   **When** le runner se termine
   **Then** il affiche le nom du scénario, la commande exécutée et les logs utiles.

5. **Given** un développeur veut lancer une seule catégorie
   **When** il passe un filtre
   **Then** seuls les scénarios correspondants s'exécutent.

## Tasks / Subtasks

- [ ] Task 1 : Définir le format des scénarios
  - [ ] Nom
  - [ ] Pipeline
  - [ ] Préparation
  - [ ] Assertions HTTP
  - [ ] Assertions SQL

- [ ] Task 2 : Implémenter le runner
  - [ ] Reset WireMock
  - [ ] Reset DB
  - [ ] Exécution `cannectors run`
  - [ ] Collecte logs
  - [ ] Résumé final

- [ ] Task 3 : Ajouter les assertions
  - [ ] HTTP requests count
  - [ ] JSON body matching
  - [ ] SQL row count / content

- [ ] Task 4 : Ajouter les commandes
  - [ ] `make test-lab-run`
  - [ ] `make test-lab-run SCENARIO=...`

## Dev Notes

### Rationale

Les epics 21 et 22 donnent les briques et scénarios. Cette story les transforme en suite exécutable et répétable.

### Design Decisions

- Le runner doit rester simple et lisible.
- Les assertions doivent produire des messages actionnables.
- Le lab local reste la source de vérité; pas de réseau externe.

### Out of Scope

- Exécution CI
- Reporting HTML
- Tests de performance

## References

- `cmd/cannectors/main.go`
- `test-lab/`
- `examples/`

## File List

- `test-lab/run.sh`
- `test-lab/scenarios/*.yaml`
- `Makefile`
