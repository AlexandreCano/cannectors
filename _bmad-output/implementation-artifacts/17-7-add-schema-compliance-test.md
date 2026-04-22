# Story 17.7: Add automated schema/runtime compliance test

Status: backlog

## Story

En tant que développeur,
je veux un test automatisé qui vérifie que chaque config d'exemple est parseable, convertible en Pipeline et instantiable par la factory,
afin que les divergences schéma/runtime (comme webhook `path` vs `endpoint`) soient détectées en CI au lieu d'être découvertes par les utilisateurs.

## Acceptance Criteria

1. **Given** un nouveau fichier `internal/config/schema_compliance_test.go`
   **When** il est exécuté
   **Then** il itère sur tous les fichiers `configs/examples/*.yaml` et `*.json`
   **And** pour chacun : parse → valide contre le schéma → convertit en Pipeline → instantie tous les modules via la factory → ferme les modules
   **And** toute étape qui échoue fait échouer le test avec le nom du fichier en cause.

2. **Given** l'ajout du bug "webhook path/endpoint" (Story 17.1) dans un fichier d'exemple
   **When** le test est exécuté avant le fix
   **Then** il échoue explicitement
   **And** après le fix, il passe.

3. **Given** les 36 fichiers d'exemple actuels
   **When** le test est exécuté en CI
   **Then** tous passent
   **And** le test total s'exécute en < 5 secondes.

4. **Given** un nouveau fichier d'exemple qui référence un `type` non enregistré
   **When** le test est exécuté
   **Then** il échoue avec un message clair `"module type 'xyz' not registered in registry"`.

5. **Given** des exemples qui font du SQL ou du HTTP
   **When** le test les instantie
   **Then** l'instanciation n'effectue **pas** de requête réseau / DB (utiliser le mode "validation seulement" ou mocker `NewHTTPClient` avec un transport no-op).

## Tasks / Subtasks

- [ ] Task 1 : Écrire le squelette du test (AC #1)
  - [ ] `internal/config/schema_compliance_test.go`
  - [ ] `func TestExampleConfigs(t *testing.T)` avec `filepath.Walk("../../configs/examples")` + subtests par fichier

- [ ] Task 2 : Implémenter le flow complet (AC #1, #5)
  - [ ] Parse : `config.ParseConfig(path)` → vérifier pas d'erreurs
  - [ ] Valider : vérifier `result.ValidationErrors` vide
  - [ ] Convertir : `config.ConvertToPipeline(result.Data)` → vérifier pas d'erreur
  - [ ] Instancier : `factory.CreateInputModule(p.Input)`, `factory.CreateFilterModules(p.Filters)`, `factory.CreateOutputModule(p.Output)` → vérifier pas d'erreur
  - [ ] Pour DB : injecter un stub driver qui ne tente pas de connexion (via `database.SetDriverFactory` si nécessaire, ou skip ces tests en short mode)
  - [ ] Fermer : appeler `Close()` sur chaque module fermable

- [ ] Task 3 : Gérer les cas particuliers (AC #4, #5)
  - [ ] Si l'exemple utilise `${ENV_VAR}` : définir des valeurs par défaut dans `os.Setenv` avant test
  - [ ] Si l'exemple utilise `scheduleInput` : vérifier `scheduler.ValidateCronExpression`, mais ne pas démarrer le scheduler
  - [ ] Si l'exemple utilise webhook : ne pas binder le port (utiliser un config `listenAddress: "127.0.0.1:0"` ou skip le listen)

- [ ] Task 4 : Garde-fou pour nouveaux exemples (AC #3)
  - [ ] Vérifier que chaque fichier YAML a son pendant JSON ou vice-versa (convention actuelle) — optionnel, seulement si c'est une règle du projet

- [ ] Task 5 : CI (AC #3)
  - [ ] S'assurer que `make test` ou la CI inclut ce test
  - [ ] Ajouter au `Makefile` si nécessaire : `test-compliance: go test -run TestExampleConfigs ./internal/config/...`

## Dev Notes

### Rationale

L'audit §5 P2.6 recommande ce filet de sécurité avant tout gros chantier HTTP (Epic 15). Plan.md §6 le liste aussi en priorité P0.

### Design Decisions

- **Test data-driven** : un subtest par fichier (`t.Run(filename, ...)`) pour des erreurs localisables.
- **Pas de mocks lourds** : factory crée les modules, mais `Close()` immédiatement — pas de HTTP réel.
- **Skippable en short mode** si besoin : `if testing.Short() { t.Skip() }` pour les exemples DB lourds.

### Out of Scope

- Exécution réelle des pipelines (E2E) → Story 20.4
- Validation croisée multi-pipelines → non pertinent actuellement

## References

- docs/AUDIT_TECHNIQUE_2026-04-21.md §5 P2.6
- docs/plan.md §6.2
- `configs/examples/` (36+ fichiers)

## File List

(à compléter)
