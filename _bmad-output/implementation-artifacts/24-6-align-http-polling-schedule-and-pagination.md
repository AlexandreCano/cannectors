# Story 24.6: Align httpPolling schedule and pagination

Status: ready-for-dev

## Story

En tant qu'utilisateur,
je veux qu'un input `httpPolling` puisse etre lance en one-shot sans schedule ou planifie avec schedule,
et que la pagination HTTP ait une API simple et coherente.

## Acceptance Criteria

1. **Given** un pipeline `httpPolling` sans `schedule`
   **When** il est lance via la CLI
   **Then** il s'execute une seule fois.

2. **Given** un pipeline `httpPolling` avec `schedule`
   **When** il est enregistre dans le scheduler
   **Then** il s'execute selon la planification.

3. **Given** `httpPolling.retry`
   **When** la config est valide
   **Then** le retry est accepte par le schema et applique par le runtime.

4. **Given** une pagination HTTP avec `limit` omis
   **When** le module s'execute
   **Then** le runtime applique son default.

5. **Given** une pagination HTTP avec `limit: 0`
   **When** le schema valide la config
   **Then** la config est rejetee.

6. **Given** une pagination `cursor`, `page` ou `offset`
   **When** le parametre principal est configure
   **Then** le champ canonique est `param`.

7. **Given** une pagination avec `limitParam`
   **When** le type en a besoin
   **Then** `limitParam` reste separe de `param`.

8. **Given** une pagination avec ancien champ `pageParam`, `cursorParam` ou `offsetParam`
   **When** le schema valide la config
   **Then** la config est rejetee, sans compatibilite.

9. **Given** une pagination avec type inconnu
   **When** le module est instancie
   **Then** le runtime renvoie une erreur claire.

10. **Given** `httpPolling.method`
    **When** la methode est une methode HTTP valide
    **Then** le runtime l'utilise au lieu de forcer `GET`.

11. **Given** `httpPolling` avec un body configure
    **When** la requete est envoyee
    **Then** le body est transmis, quelle que soit la methode.

## Tasks / Subtasks

- [ ] Task 1 : Rendre `schedule` optionnel
  - [ ] Schema input
  - [ ] Scheduler/runtime
  - [ ] Tests one-shot et scheduled

- [ ] Task 2 : Ajouter `Schedule string` si utile au parsing typed
  - [ ] `HTTPPollingInputConfig`
  - [ ] Conversion/factory
  - [ ] Tests

- [ ] Task 3 : Revoir pagination HTTP
  - [ ] Remplacer `pageParam`, `cursorParam`, `offsetParam` par `param`
  - [ ] Garder `limitParam`
  - [ ] `limit` optionnel, minimum 1 si fourni
  - [ ] Runtime strict sur type inconnu

- [ ] Task 4 : Aligner methode et body polling
  - [ ] Ne plus forcer `GET`
  - [ ] Supporter toute methode HTTP valide
  - [ ] Supporter un body sur `httpPolling`

## Dev Notes

### Product Rules

Sans schedule, le pipeline est one-shot. Cette regle permet aussi d'appeler un pipeline depuis un autre pipeline via `cannectors le-pipeline` sans introduire un `trigger.type`.

## References

- `internal/config/schema/input-schema.json`
- `internal/config/schema/common-schema.json`
- `internal/modules/input/http_polling*.go`
- `internal/scheduler`
- `cmd/cannectors/main.go`

## File List

- `internal/config/schema/input-schema.json`
- `internal/config/schema/common-schema.json`
- `internal/modules/input/http_polling*.go`
- `internal/scheduler/*.go`
- `cmd/cannectors/main.go`
