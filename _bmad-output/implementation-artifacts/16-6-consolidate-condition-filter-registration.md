# Story 16.6: Consolidate `condition` filter registration (remove double registration)

Status: backlog

## Story

En tant que développeur,
je veux que le filter `condition` soit enregistré une seule fois et que la factory utilise exclusivement la registry,
afin d'éliminer le contournement actuel (la factory a un `if cfg.Type == "condition"` qui bypass la registry pour gérer les nested modules).

## Acceptance Criteria

1. **Given** `factory/modules.go`
   **When** je consulte `createSingleFilterModule`
   **Then** il n'y a plus de `if cfg.Type == "condition"` qui contourne la registry
   **And** tous les types filter passent par `registry.GetFilterConstructor(cfg.Type)(cfg, index)`.

2. **Given** `registry/builtins.go`
   **When** je consulte l'enregistrement de `condition`
   **Then** le constructeur enregistré gère correctement les blocs `then`/`else` (nested modules)
   **And** aucune mention de "IMPORTANT: nested modules NOT parsed here" dans les commentaires.

3. **Given** `filter.NestedModuleCreator` (variable globale actuelle)
   **When** je le remplace
   **Then** il est supprimé ou remplacé par un mécanisme plus explicite (injection via registry, ou interface `Resolver` passée au constructeur)
   **And** la dépendance factory ↔ filter est explicite au lieu d'être masquée.

4. **Given** un pipeline avec des `condition` imbriquées (`configs/examples/09-filters-condition.yaml`)
   **When** je l'exécute
   **Then** le comportement est identique à avant.

5. **Given** les tests de `condition`
   **When** `go test ./...`
   **Then** tout passe.

## Tasks / Subtasks

- [ ] Task 1 : Décider du mécanisme (AC #3) - AVANT TOUTE IMPLÉMENTATION
  - [ ] Option A : `registry.RegisterFilter` accepte un `Resolver` optionnel pour les nested modules
  - [ ] Option B : le constructeur de `condition` reçoit un `filterFactory func(cfg, idx) (Module, error)` via closure
  - [ ] Option C : garder `NestedModuleCreator` mais le documenter et rendre l'injection plus visible
  - [ ] Choisir A ou B (préférables à C)

- [ ] Task 2 : Implémenter l'option choisie (AC #2, #3)
  - [ ] Si A ou B : modifier la signature `FilterConstructor` ou la méthode d'enregistrement
  - [ ] Enregistrer une version "complète" de `condition` dans `builtins.go` qui gère les nested modules
  - [ ] Supprimer `filter.NestedModuleCreator` variable globale
  - [ ] Supprimer `factory.CreateFilterModuleFromNestedConfig` ou le faire devenir une méthode interne à la registry

- [ ] Task 3 : Simplifier la factory (AC #1)
  - [ ] Supprimer `factory/modules.go:createConditionFilterModule` (lignes 90-105)
  - [ ] Supprimer le `if cfg.Type == "condition"` dans `createSingleFilterModule` (ligne 76)
  - [ ] `createSingleFilterModule` devient un pur lookup + appel

- [ ] Task 4 : Supprimer le `init()` de factory (AC #3)
  - [ ] `factory/modules.go:29-33` (init qui assigne `filter.NestedModuleCreator`) devient obsolète si option A ou B

- [ ] Task 5 : Tests (AC #4, #5)
  - [ ] Vérifier tous les tests de `condition_test.go` (2218 LOC) passent
  - [ ] Vérifier l'exemple `configs/examples/09-filters-condition.yaml` s'exécute correctement
  - [ ] Ajouter un test : un custom filter type enregistré en test peut apparaître dans un bloc `then`

## Dev Notes

### Rationale

L'audit §2.2 relève deux problèmes dans le registry+factory :

1. **Double registration** : `registry/builtins.go:67-78` enregistre une version "basic" de condition, puis `factory/modules.go:76-78` l'ignore avec un `if cfg.Type == "condition"` pour appeler sa propre version complète.

2. **Variable globale `filter.NestedModuleCreator`** : contournement de cycle d'import. Le commentaire à `filter/condition.go:52-68` dit : *"If factory is not imported, NestedModuleCreator will be nil and nested modules will fall back to hardcoded behavior for built-in types only"*. C'est un piège pour contributeurs.

### Design Decisions

L'option B (closure) est probablement la plus simple :
```go
RegisterFilter("condition", func(cfg ModuleConfig, idx int) (Module, error) {
    return filter.NewConditionFromConfig(cfg, func(nestedCfg, nestedIdx) (Module, error) {
        // Appel récursif à la registry via une référence capturée
        return registry.GetFilterConstructor(nestedCfg.Type)(...)
    })
})
```

### Out of Scope

- Refonte complète du mécanisme de nested modules → garder la sémantique actuelle
- Support de nested dans d'autres filters → pour l'instant, uniquement `condition`

## References

- docs/AUDIT_TECHNIQUE_2026-04-21.md §2.2
- `registry/builtins.go:67-78`
- `factory/modules.go:29-105`
- `filter/condition.go:52-68` (NestedModuleCreator)

## File List

(à compléter)
