# Story 20.4: Add E2E non-regression tests for example configs

Status: backlog

## Story

En tant que développeur,
je veux au moins un test E2E par combinaison input×output des exemples,
afin de détecter toute régression fonctionnelle pas capturée par les tests unitaires ou de conformité schéma.

## Acceptance Criteria

1. **Given** un nouveau fichier `internal/e2e/e2e_test.go` (package isolé pour éviter impact sur unit tests)
   **When** il est exécuté avec `go test -tags=e2e ./internal/e2e/`
   **Then** il exécute au moins les scénarios suivants :
    - HTTP polling → HTTP request (config 01-simple)
    - HTTP polling avec pagination page → HTTP request (05-pagination)
    - HTTP polling avec pagination offset → HTTP request (06-pagination-offset)
    - HTTP polling avec pagination cursor → HTTP request (07-pagination-cursor)
    - HTTP polling + mapping filter → HTTP request (08-filters-mapping)
    - HTTP polling + condition filter → HTTP request (09-filters-condition)
    - HTTP polling + state persistence (timestamp) → HTTP request (18-timestamp-persistence)
    - HTTP polling + state persistence (ID) → HTTP request (19-id-persistence)
    - Database input → HTTP request (27-database-input)
    - HTTP polling → Database output (30-database-output-insert)
    - Webhook → HTTP request (14-webhook)

2. **Given** chaque scénario
   **When** il s'exécute
   **Then** il monte un serveur HTTP de test (`httptest.NewServer`) comme source / destination
   **And** il vérifie que le bon nombre de records a été transféré et que le contenu est attendu
   **And** il ne touche pas à des ressources externes (pas d'Internet, pas de DB distante).

3. **Given** les tests E2E
   **When** ils passent
   **Then** ils tournent en < 60 secondes total
   **And** ils tournent dans la CI sur chaque PR.

4. **Given** un bug introduit volontairement (ex: casser le parsing de pagination cursor)
   **When** je lance les tests E2E
   **Then** au moins un test échoue avec un message clair identifiant le pipeline en cause.

5. **Given** un développeur ajoute un nouveau module
   **When** il veut valider son travail
   **Then** la checklist documente l'ajout d'un E2E test dans `internal/e2e/`.

## Tasks / Subtasks

- [ ] Task 1 : Setup du package E2E (AC #1)
  - [ ] `internal/e2e/e2e_test.go` avec `//go:build e2e`
  - [ ] Helper `runPipelineFromConfig(t, configPath string, fixtureOverrides map[string]interface{}) *ExecutionResult`
  - [ ] Helper `mockSourceServer(t, records []map[string]interface{}) *httptest.Server` (paginé si besoin)
  - [ ] Helper `mockDestinationServer(t) (*httptest.Server, *[][]map[string]interface{})` (capture le body reçu)

- [ ] Task 2 : Implémenter les scénarios HTTP (AC #1)
  - [ ] 10-12 subtests, un par exemple
  - [ ] Chaque test : setup fixture → appelle runPipelineFromConfig → assert sur la capture

- [ ] Task 3 : Scénarios DB (AC #1)
  - [ ] SQLite in-memory pour source et destination
  - [ ] Insert fixtures → exécuter pipeline → assert

- [ ] Task 4 : Scénario webhook (AC #1)
  - [ ] Démarrer le pipeline en arrière-plan
  - [ ] Envoyer une requête HTTP vers le endpoint webhook
  - [ ] Vérifier la réception côté destination mock
  - [ ] Shutdown gracieux

- [ ] Task 5 : CI (AC #3)
  - [ ] Ajouter une étape `go test -tags=e2e -timeout=2m ./internal/e2e/`
  - [ ] Documenter dans README comment lancer en local

- [ ] Task 6 : Documentation (AC #5)
  - [ ] Paragraphe dans `docs/MODULE_EXTENSIBILITY.md` : "Ajouter un E2E test pour votre module"

## Dev Notes

### Rationale

Audit §4.1 note que les E2E tests sont limités. Plan.md §6.3 recommande d'utiliser `configs/examples/` comme fixtures de non-régression. Cette story rend cela concret.

### Design Decisions

- **Build tag `e2e`** : séparation nette des unit tests rapides et des E2E plus lourds.
- **Pas de réseau externe** : stable, déterministe, rapide.
- **httptest.Server** : idiomatique Go, suffisant pour mocker HTTP.

### Out of Scope

- Tests de charge / stress → autre epic
- Tests contre vraies APIs (Shopify, Stripe, etc.) → hors scope, risqué

## References

- docs/AUDIT_TECHNIQUE_2026-04-21.md §5 P2.6 (complémentaire au schema compliance 17.7), §4.1
- docs/plan.md §6.3
- `configs/examples/` (fixtures)

## File List

(à compléter)
