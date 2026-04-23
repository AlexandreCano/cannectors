# Story 17.6: Fix `database.ConvertPlaceholders` for strings containing `?`

Status: backlog

## Story

En tant que développeur,
je veux que la conversion de placeholders SQL (`?` → `$1, $2, ...`) ne remplace pas les `?` contenus dans des strings littérales de la requête,
afin d'éviter des requêtes SQL corrompues quand une query contient un `?` légitime (ex: `WHERE name LIKE '%?%'`).

## Acceptance Criteria

1. **Given** une query `SELECT * FROM t WHERE a = ? AND b LIKE '%?%'` et driver `postgres`
   **When** j'appelle `database.ConvertPlaceholders(query, "postgres")`
   **Then** le résultat est `SELECT * FROM t WHERE a = $1 AND b LIKE '%?%'`
   **And** le `?` dans la string littérale n'est PAS converti.

2. **Given** une query avec `?` dans un commentaire `-- ? is a joke`
   **When** la conversion est faite
   **Then** le `?` dans le commentaire reste inchangé.

3. **Given** les queries sans strings ni commentaires contenant `?` (cas majoritaire)
   **When** la conversion est faite
   **Then** le comportement est identique à aujourd'hui (tous les `?` → `$N`).

4. **Given** les tests existants de `database/database_test.go`
   **When** `go test ./internal/database/...`
   **Then** tous passent + nouveaux cas couverts.

5. **Given** le driver MySQL ou SQLite (style `?`)
   **When** j'appelle `ConvertPlaceholders`
   **Then** la query est retournée inchangée (comportement actuel).

## Tasks / Subtasks

- [ ] Task 1 : Implémenter un parser simple (AC #1, #2)
  - [ ] `database/database.go:ConvertPlaceholders` : remplacer la boucle `for strings.Contains(result, "?")` (O(n²), incorrect) par un parser qui traverse la query char par char
  - [ ] Ignorer les `?` à l'intérieur de :
    - Strings littérales `'...'` (échappement standard SQL `''`)
    - Strings littérales `"..."` (Postgres identifiers, pas strings, mais safe)
    - Commentaires `-- ...` (jusqu'à fin de ligne)
    - Commentaires `/* ... */` (multi-lignes)
  - [ ] Reste des placeholders : incrémenter le compteur `$N`

- [ ] Task 2 : Tests exhaustifs (AC #1, #2, #3, #4)
  - [ ] `TestConvertPlaceholders_StringLiteralsProtected`
  - [ ] `TestConvertPlaceholders_SingleLineComments`
  - [ ] `TestConvertPlaceholders_MultiLineComments`
  - [ ] `TestConvertPlaceholders_NestedEscapes` : `'it''s ? a test'`
  - [ ] `TestConvertPlaceholders_MySQL_NoOp`
  - [ ] Test de performance : 1000 appels sur une query réaliste (< 1ms)

- [ ] Task 3 : Documentation (AC #1)
  - [ ] Ajouter un godoc sur `ConvertPlaceholders` listant les cas supportés et ses limites (ex : dollar-quoted strings Postgres `$$...$$` peut ne pas être géré — à discuter)

## Dev Notes

### Rationale

Bug identifié dans l'audit §3.5. `database/database.go:297` fait :
```go
for strings.Contains(result, "?") {
    result = strings.Replace(result, "?", fmt.Sprintf("$%d", paramIndex), 1)
    paramIndex++
}
```
Deux problèmes :
1. **Correction** : remplace tous les `?` sans distinction, y compris dans strings/commentaires.
2. **Performance** : O(n²) (scan complet à chaque itération).

**Risque sécurité** : possible injection SQL si une valeur utilisateur insérée naïvement contient `?` (peu probable vu l'usage actuel, mais à corriger quand même).

### Design Decisions

- **Parser char-by-char** : suffisant pour SQL standard, pas besoin d'un vrai lexer SQL.
- **Support des dollar-quoted strings Postgres** (`$tag$...$tag$`) : hors scope pour cette story (à documenter comme limite).

### Out of Scope

- Support complet SQL parsing → utiliser une lib dédiée si besoin
- Dollar-quoted strings → amélioration future

## References

- docs/AUDIT_TECHNIQUE_2026-04-21.md §3.5, §5 P3.4
- `database/database.go:286-302`

## File List

(à compléter)
