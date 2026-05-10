# Story 24.8: Align mapping filter contract

Status: ready-for-dev

## Story

En tant qu'utilisateur du filter `mapping`,
je veux que les mappings et transforms invalides echouent clairement,
afin qu'une faute de config ne devienne jamais un no-op silencieux.

## Acceptance Criteria

1. **Given** un mapping avec `target: ""`
   **When** le schema valide la config
   **Then** la config est rejetee.

2. **Given** `onMissing` omis
   **When** le module est cree
   **Then** le default reste `setNull`.

3. **Given** `onMissing` avec valeur inconnue
   **When** le module est cree
   **Then** le runtime renvoie une erreur claire.

4. **Given** `onMissing: useDefault`
   **When** `defaultValue` est absent
   **Then** la config est rejetee.

5. **Given** un transform op inconnu
   **When** le schema ou runtime valide la config
   **Then** la config est rejetee.

6. **Given** `op: replace`
   **When** `pattern` est absent ou vide
   **Then** la config est rejetee.

7. **Given** `op: replace` avec regex invalide
   **When** le module est cree
   **Then** le runtime renvoie une erreur claire.

8. **Given** un transform supporte
   **When** il est configure
   **Then** l'op est dans l'enum schema et executee par le runtime.

## Tasks / Subtasks

- [ ] Task 1 : Renforcer le schema mapping
  - [ ] `target.minLength: 1`
  - [ ] Enum `onMissing`: `setNull|skipField|useDefault|fail`
  - [ ] Regle `useDefault` implique `defaultValue`
  - [ ] Enum `transforms[].op`
  - [ ] Regle `replace.pattern` requis non vide

- [ ] Task 2 : Renforcer le runtime mapping
  - [ ] Valider `onMissing`
  - [ ] Valider op inconnu
  - [ ] Garder compilation regex runtime
  - [ ] Tests pour chaque erreur

## Dev Notes

### Supported Transform Ops

`trim`, `lowercase`, `uppercase`, `dateFormat`, `replace`, `split`, `join`, `toString`, `toInt`, `toFloat`, `toBool`, `toArray`, `toObject`.

## References

- `internal/config/schema/filter-schema.json`
- `internal/modules/filter/mapping.go`
- `internal/modules/filter/mapping_parse.go`
- `internal/modules/filter/mapping_transforms.go`

## File List

- `internal/config/schema/filter-schema.json`
- `internal/modules/filter/mapping*.go`
