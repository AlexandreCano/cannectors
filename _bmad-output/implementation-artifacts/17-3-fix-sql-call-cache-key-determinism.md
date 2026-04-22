# Story 17.3: Fix `sql_call` cache key non-determinism

Status: backlog

## Story

En tant qu'utilisateur,
je veux que le cache du filtre `sql_call` produise toujours la même clé pour un record donné,
afin que des cache misses inattendus ne dégradent pas les performances quand l'ordre des clés JSON varie.

## Acceptance Criteria

1. **Given** deux records sémantiquement identiques mais avec des clés JSON dans un ordre différent (`{"a":1,"b":2}` vs `{"b":2,"a":1}`)
   **When** le filtre `sql_call` calcule la cache key
   **Then** la même clé est produite dans les deux cas.

2. **Given** `sql_call.buildCacheKey` actuel (utilise `json.Marshal` sur un map Go, dont l'ordre n'est pas garanti par spec)
   **When** je le remplace
   **Then** le nouveau calcul utilise des clés triées alphabétiquement avant sérialisation.

3. **Given** des valeurs imbriquées dans la cache key (`{"user":{"id":1,"name":"x"}}`)
   **When** le calcul est fait
   **Then** le tri est récursif (imbrications profondes respectent l'ordre).

4. **Given** un test qui stocke une valeur en cache avec record A puis tente un Get avec record A' (même contenu, clés permutées)
   **When** il s'exécute
   **Then** le cache hit est systématique (100 % sur 1000 itérations).

5. **Given** le cache existant rempli avant le fix
   **When** on déploie
   **Then** les anciennes clés deviennent obsolètes (cold start tolérable) — **ne pas** ajouter de logique de compatibilité ancienne/nouvelle.

## Tasks / Subtasks

- [ ] Task 1 : Implémenter le tri récursif (AC #1, #2, #3)
  - [ ] Dans `filter/sql_call.go:buildCacheKey` (ligne ~351) : avant `json.Marshal`, convertir la map en structure triée
  - [ ] Option : utiliser `encoding/json` avec `MarshalIndent` puis hash, ou implémenter `sortedMarshal(m)` qui écrit récursivement les clés triées

- [ ] Task 2 : Alternative plus simple : hash des champs de clé extraits (AC #1, #2)
  - [ ] Si `cacheKey` template configuré (`{{record.user.id}}`), évaluer le template et utiliser le résultat comme clé — déjà déterministe
  - [ ] Si pas de template : trier alphabétiquement les clés extraites de `keyValues map[string]string` avant concaténation

- [ ] Task 3 : Tests (AC #1, #4)
  - [ ] Test `TestSQLCallCacheKey_Deterministic` : 1000 itérations avec ordre permuté → toujours la même clé
  - [ ] Test `TestSQLCallCacheKey_Nested` : maps imbriquées, tri récursif
  - [ ] Test avec le vrai `Cache.Get` / `Cache.Set` pour vérifier hits

- [ ] Task 4 : Appliquer le même fix à `http_call` (AC #1)
  - [ ] Vérifier si `filter/http_call.go:buildCacheKey` a le même bug
  - [ ] Si oui, appliquer le même correctif

## Dev Notes

### Rationale

Bug moyen P1 de `docs/plan.md` §3 "Bug 5". Les maps Go n'ont pas d'ordre d'itération garanti, et `json.Marshal` sur un map sérialise dans l'ordre d'itération. Donc **pour un même record, des appels successifs peuvent produire des clés JSON différentes** → cache miss, re-query SQL inutile.

### Design Decisions

- **Tri alphabétique** récursif : simple, deterministic, pas de perte d'info.
- **Pas de compat ancienne cache** : les entrées en cache sont éphémères (TTL), un cold start est acceptable.
- **Même bug dans `http_call`** (probablement) : à vérifier et corriger dans le même PR.

### Out of Scope

- Changer le TTL par défaut → garder 5 min
- Ajouter une option `cacheKeyStrategy: "template"|"full"|"hash"` → potentielle évolution future

## References

- docs/AUDIT_TECHNIQUE_2026-04-21.md §5 P1.2
- docs/plan.md §3 "Bug 5"
- `filter/sql_call.go:565-587` (buildCacheKey, compositeKeyString)
- `filter/http_call.go:565-587` (équivalent)

## File List

(à compléter)
