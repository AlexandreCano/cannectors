# Story 23.4: Add test lab CI workflow

Status: ready-for-dev

## Story

En tant que mainteneur,
je veux exécuter une partie du test lab en CI,
afin de détecter les régressions d'intégration avant merge.

## Acceptance Criteria

1. **Given** une Pull Request
   **When** le workflow CI s'exécute
   **Then** il démarre les services Docker nécessaires au test lab.

2. **Given** les services sont prêts
   **When** le runner local est lancé
   **Then** les scénarios critiques passent sans accès à Internet.

3. **Given** un scénario échoue
   **When** le job CI se termine
   **Then** les logs Cannectors, WireMock et PostgreSQL sont disponibles en artifacts ou dans la sortie du job.

4. **Given** la suite complète devient trop longue
   **When** le workflow est configuré
   **Then** les scénarios rapides tournent sur chaque PR et les scénarios longs peuvent être isolés.

## Tasks / Subtasks

- [ ] Task 1 : Ajouter le workflow CI
  - [ ] Setup Go
  - [ ] Build du CLI
  - [ ] Docker Compose up
  - [ ] Healthcheck services
  - [ ] Runner test lab

- [ ] Task 2 : Sélectionner les scénarios critiques
  - [ ] HTTP polling vers HTTP output
  - [ ] Database input/output
  - [ ] Auth simple
  - [ ] Retry simple

- [ ] Task 3 : Ajouter les logs de debug
  - [ ] Logs Cannectors
  - [ ] WireMock request journal
  - [ ] PostgreSQL logs ou dump ciblé

- [ ] Task 4 : Documenter le workflow
  - [ ] Comment lancer localement la même suite
  - [ ] Comment ajouter un scénario CI-safe

## Dev Notes

### Rationale

Une fois le runner stable, la CI garantit que les scénarios locaux restent exécutés régulièrement et évitent les régressions silencieuses.

### Design Decisions

- Les scénarios CI doivent être courts et déterministes.
- Les scénarios longs ou flaky doivent rester hors PR par défaut.

### Out of Scope

- Déploiement d'environnements distants
- Tests contre services tiers publics

## References

- `test-lab/run.sh`
- `Makefile`
- `.github/workflows`

## File List

- `.github/workflows/test-lab.yml`
- `test-lab/README.md`
