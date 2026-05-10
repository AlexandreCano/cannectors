# Story 24.10: Align script, set and remove filter contracts

Status: ready-for-dev

## Story

En tant qu'utilisateur des filters de transformation,
je veux que `script`, `set` et `remove` aient une configuration stricte mais ergonomique,
afin d'exprimer les changements de record sans ambiguite.

## Acceptance Criteria

1. **Given** un filter `script`
   **When** `script` et `scriptFile` sont fournis ensemble
   **Then** la config est rejetee.

2. **Given** un filter `script`
   **When** aucun des deux champs n'est fourni
   **Then** la config est rejetee.

3. **Given** `script` ou `scriptFile` vide
   **When** le schema valide la config
   **Then** la config est rejetee.

4. **Given** un script whitespace-only, un fichier invalide, un JS invalide ou sans `transform(record)`
   **When** le module est cree
   **Then** le runtime renvoie une erreur claire.

5. **Given** un filter `set` avec `value: null`
   **When** le module s'execute
   **Then** le champ cible est mis a la vraie valeur null.

6. **Given** un filter `set` sans `value`
   **When** le schema ou runtime valide la config
   **Then** la config est rejetee.

7. **Given** un filter `set` avec `value: "null"`
   **When** le module s'execute
   **Then** le champ cible est mis a la string `"null"`.

8. **Given** un filter `remove`
   **When** `target` est une string non vide
   **Then** le champ cible est supprime.

9. **Given** un filter `remove`
   **When** `target` est une liste non vide de strings non vides
   **Then** tous les champs cibles sont supprimes.

10. **Given** l'ancien champ `targets`
    **When** le schema valide la config
    **Then** la config est rejetee, sans compatibilite.

## Tasks / Subtasks

- [ ] Task 1 : Aligner `script`
  - [ ] Garder `oneOf` entre `script` et `scriptFile`
  - [ ] Ajouter `minLength: 1`
  - [ ] Ne pas ajouter `maxLength` au schema
  - [ ] Garder limite runtime 100KB
  - [ ] Pas de champ `language`

- [ ] Task 2 : Aligner `set`
  - [ ] `value` requis dans le schema
  - [ ] Parser map pour distinguer absent de null
  - [ ] Accepter null explicite
  - [ ] `target` non vide
  - [ ] Integrer ModuleBase

- [ ] Task 3 : Aligner `remove`
  - [ ] Supprimer `targets`
  - [ ] `target` requis
  - [ ] `target` oneOf string ou liste de strings
  - [ ] Normaliser en interne vers `[]string`
  - [ ] Integrer ModuleBase

## Dev Notes

### Product Rules

Pas de retrocompatibilite pour `targets`. `target` devient le champ unique et peut porter une ou plusieurs cibles.

## References

- `internal/config/schema/filter-schema.json`
- `internal/modules/filter/script.go`
- `internal/modules/filter/set.go`
- `internal/modules/filter/remove.go`

## File List

- `internal/config/schema/filter-schema.json`
- `internal/modules/filter/script*.go`
- `internal/modules/filter/set*.go`
- `internal/modules/filter/remove*.go`
