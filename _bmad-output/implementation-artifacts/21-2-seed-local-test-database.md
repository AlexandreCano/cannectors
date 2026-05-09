# Story 21.2: Seed local test database

Status: ready-for-dev

## Story

En tant que développeur,
je veux une base PostgreSQL locale initialisée avec un schéma et des données déterministes,
afin de tester les modules `database` et `sql_call` de bout en bout.

## Acceptance Criteria

1. **Given** le service PostgreSQL du test lab démarre
   **When** les scripts d'init s'exécutent
   **Then** les tables source, destination et référence sont créées automatiquement.

2. **Given** les données seedées
   **When** je lance plusieurs fois le reset du lab
   **Then** les mêmes lignes sont présentes avec les mêmes IDs et timestamps.

3. **Given** les futurs scénarios database
   **When** ils lisent ou écrivent dans la base
   **Then** ils disposent de tables couvrant input, output, enrichissement et erreurs contrôlées.

4. **Given** un test échoue
   **When** j'inspecte la base
   **Then** les noms de tables et colonnes rendent le scénario compréhensible.

## Tasks / Subtasks

- [ ] Task 1 : Créer le schéma SQL
  - [ ] Tables source `source_customers`, `source_orders`, `source_inventory`
  - [ ] Tables destination `dest_customers`, `dest_orders`, `inventory_snapshot`
  - [ ] Tables référence `customer_reference`, `product_reference`

- [ ] Task 2 : Ajouter les données seedées
  - [ ] Cas nominaux
  - [ ] Cas avec champs manquants ou valeurs nulles
  - [ ] Cas pour pagination et incrémental
  - [ ] Cas pour violation de contrainte contrôlée

- [ ] Task 3 : Ajouter le reset déterministe
  - [ ] Script SQL de truncate/reseed
  - [ ] Commande Make ou script shell dédié

## Dev Notes

### Rationale

Les modules SQL doivent être testés sur une vraie base locale pour couvrir la connexion, les placeholders, les transactions, les erreurs SQL et l'état final des écritures.

### Design Decisions

- Les données doivent rester petites pour garder des tests rapides.
- Les timestamps doivent être fixes pour éviter les assertions fragiles.
- Les tables doivent être orientées scénario plutôt que modèle métier complet.

### Out of Scope

- Support MySQL
- Support SQLite dans le lab Docker
- Migrations applicatives complexes

## References

- `internal/modules/input/database.go`
- `internal/modules/output/database.go`
- `internal/modules/filter/sql_call.go`

## File List

- `test-lab/postgres/init/001_schema.sql`
- `test-lab/postgres/init/002_seed.sql`
- `test-lab/postgres/reset.sql`
