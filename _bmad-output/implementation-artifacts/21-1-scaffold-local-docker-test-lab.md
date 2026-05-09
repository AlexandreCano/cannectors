# Story 21.1: Scaffold local Docker test lab

Status: ready-for-dev

## Story

En tant que développeur,
je veux un lab de test local Dockerisé,
afin de pouvoir lancer des tests d'intégration réalistes sans dépendre de services externes.

## Acceptance Criteria

1. **Given** le repository Cannectors
   **When** je lance la commande du lab local
   **Then** Docker Compose démarre au minimum WireMock et PostgreSQL
   **And** les ports exposés sont documentés et stables.

2. **Given** le lab est démarré
   **When** je vérifie les healthchecks
   **Then** WireMock et PostgreSQL sont prêts avant l'exécution de scénarios.

3. **Given** un développeur veut réinitialiser le lab
   **When** il relance la commande de reset
   **Then** les stubs, volumes temporaires et données de test reviennent à un état déterministe.

4. **Given** le lab n'est pas destiné à la production
   **When** je lis les fichiers créés
   **Then** les credentials, ports et volumes sont clairement isolés pour les tests locaux.

## Tasks / Subtasks

- [ ] Task 1 : Ajouter l'infrastructure Docker
  - [ ] Créer `test-lab/docker-compose.yml`
  - [ ] Ajouter un service WireMock
  - [ ] Ajouter un service PostgreSQL
  - [ ] Ajouter les healthchecks nécessaires

- [ ] Task 2 : Ajouter les commandes développeur
  - [ ] Ajouter `make test-lab-up`
  - [ ] Ajouter `make test-lab-down`
  - [ ] Ajouter `make test-lab-reset`

- [ ] Task 3 : Documenter le bootstrap
  - [ ] Décrire les prérequis Docker
  - [ ] Décrire les ports utilisés
  - [ ] Décrire comment vérifier que le lab est prêt

## Dev Notes

### Rationale

Le projet a besoin d'un environnement local contrôlable pour tester les pipelines contre de vrais endpoints HTTP mockés et une vraie base SQL locale. Le lab doit être reproductible et utilisable ensuite par les stories de couverture.

### Design Decisions

- Docker Compose est le point d'entrée pour éviter une installation manuelle de dépendances.
- WireMock est utilisé pour les APIs HTTP mockées.
- PostgreSQL est utilisé pour les modules database afin de couvrir un comportement proche production.

### Out of Scope

- Scénarios de test complets
- Assertions automatisées
- CI

## References

- `examples/`
- `internal/modules/input`
- `internal/modules/output`
- `internal/modules/filter`

## File List

- `test-lab/docker-compose.yml`
- `Makefile`
- `test-lab/README.md`
