# Plan de Clean-up — Canectors Runtime

**Date :** 2026-01-23
**Auteur :** Revue technique senior
**Version du code analysé :** Commit actuel
**Lignes de code :** ~31 000 LOC Go

---

## 1. Objectifs du clean-up

### Objectifs

- **Lisibilité** — Réduire la taille de `main.go`, clarifier les responsabilités
- **Cohérence** — Uniformiser le wrapping des erreurs et le nommage
- **Robustesse** — Ajouter la propagation de `context.Context` pour les annulations propres
- **Maintenabilité** — Déplacer les stubs dans leurs packages respectifs, réduire la duplication
- **Préparation aux tests** — Faciliter l'injection de dépendances et les mocks

### Hors scope (explicitement exclus)

- ❌ Nouvelles fonctionnalités (nouveaux modules, nouvelles commandes CLI)
- ❌ Changement de comportement observable (même résultats, mêmes logs, mêmes codes de sortie)
- ❌ Refonte de l'architecture (pas de microservices, pas de plugins dynamiques)
- ❌ Optimisations de performance prématurées
- ❌ Migration vers d'autres librairies (pas de remplacement de cobra, expr, etc.)
- ❌ Persistance OAuth2 (nécessite des décisions produit)
- ❌ Parallélisation du traitement des records

---

## 2. Axes de travail

### A. Structure & organisation du code

Réduire `main.go` de 1141 à ~300-400 lignes en extrayant la logique métier.

### B. Propagation de `context.Context`

Ajouter le support de context dans les modules et le runtime pour permettre l'annulation propre.

### C. Gestion des erreurs

Uniformiser le pattern de wrapping des erreurs à travers tout le codebase.

### D. Déplacement des stubs

Migrer les stubs de `main.go` vers `internal/modules/{input,filter,output}/stub.go`.

### E. Réduction de la duplication

Factoriser les helpers répétés (`getNestedString`, `getNestedMap`, etc.).

### F. Nommage & lisibilité

Renommer les fonctions trop longues, clarifier les noms ambigus.

### G. Validation & robustesse

Renforcer la validation des configs (defaults, expressions CRON).

---

## 3. Liste ordonnée des changements

### Phase 1 — Fondations (changements structurels)

| # | Changement | Pourquoi | Impact | Risque |
|---|------------|----------|--------|--------|
| 1.1 | Extraire les stubs de `main.go` vers `internal/modules/{input,filter,output}/stub.go` | Séparation des responsabilités, testabilité | Les stubs deviennent importables et testables indépendamment | **Faible** — Déplacement mécanique |
| 1.2 | Créer `internal/factory/modules.go` pour la création des modules | Sortir la logique de création de `main.go` | Réduction de ~300 lignes dans main.go | **Faible** — Extraction pure |
| 1.3 | Créer `internal/cli/commands.go` pour les commandes Cobra | Isoler la définition des commandes CLI | Meilleure séparation CLI/logique | **Faible** — Extraction pure |
| 1.4 | Factoriser les helpers `getNestedString`, `getNestedMap` dans `internal/config/helpers.go` | Éliminer la duplication | Code DRY, maintenance simplifiée | **Faible** — Refactoring local |

### Phase 2 — Context propagation

| # | Changement | Pourquoi | Impact | Risque |
|---|------------|----------|--------|--------|
| 2.1 | Ajouter `context.Context` aux interfaces `input.Module.Fetch(ctx)` | Permettre l'annulation des requêtes | Toutes les implémentations devront accepter ctx | **Moyen** — Changement d'interface |
| 2.2 | Ajouter `context.Context` aux interfaces `filter.Module.Process(ctx, records)` | Cohérence, support futur de filtres async | Idem | **Moyen** — Changement d'interface |
| 2.3 | Ajouter `context.Context` aux interfaces `output.Module.Send(ctx, records)` | Permettre l'annulation des requêtes HTTP | Idem | **Moyen** — Changement d'interface |
| 2.4 | Propager le context dans `runtime/pipeline.go` (Executor) | Annulation propre du pipeline | Le scheduler peut annuler un pipeline en cours | **Moyen** — Changement de signature |
| 2.5 | Utiliser `req.WithContext(ctx)` dans `output/http_request.go` | Annulation des requêtes HTTP individuelles | Timeout via context au lieu de client.Timeout | **Moyen** — Changement de comportement interne |

### Phase 3 — Uniformisation des erreurs

| # | Changement | Pourquoi | Impact | Risque |
|---|------------|----------|--------|--------|
| 3.1 | Définir un pattern unique de wrapping : `fmt.Errorf("package.function: %w", err)` | Cohérence, debugging facilité | Toutes les erreurs suivent le même format | **Faible** — Changement cosmétique |
| 3.2 | Auditer et corriger les retours d'erreur sans wrapping dans `config/` | Erreurs actuellement perdues sans contexte | Meilleure traçabilité | **Faible** |
| 3.3 | Auditer et corriger dans `modules/input/` | Idem | Idem | **Faible** |
| 3.4 | Auditer et corriger dans `modules/filter/` | Idem | Idem | **Faible** |
| 3.5 | Auditer et corriger dans `modules/output/` | Idem | Idem | **Faible** |
| 3.6 | Auditer et corriger dans `runtime/` | Idem | Idem | **Faible** |

### Phase 4 — Nommage & lisibilité

| # | Changement | Pourquoi | Impact | Risque |
|---|------------|----------|--------|--------|
| 4.1 | Renommer `parseNestedModuleConfigWithDepth` → `parseModuleConfig` (depth devient interne) | Nom trop long, détail d'implémentation exposé | API plus claire | **Faible** |
| 4.2 | Renommer `resolveAndInjectErrorHandling` → `applyErrorHandling` | Verbeux | Plus concis | **Faible** |
| 4.3 | Simplifier les noms de variables locales trop longs dans `main.go` | Lisibilité | Code plus scannable | **Faible** |
| 4.4 | Ajouter des commentaires de section dans les fichiers > 500 lignes | Navigation | Facilite la lecture | **Faible** |

### Phase 5 — Validation & robustesse

| # | Changement | Pourquoi | Impact | Risque |
|---|------------|----------|--------|--------|
| 5.1 | Valider les expressions CRON à la création du scheduler, pas à l'exécution | Fail-fast | Erreur claire au démarrage | **Faible** |
| 5.2 | Ajouter une limite configurable sur la profondeur de récursion dans `parseModuleConfig` | Prévention stack overflow | Protection contre configs malformées | **Faible** |
| 5.3 | Valider la structure de `defaults` dans le schema JSON | Cohérence | Erreurs de config détectées tôt | **Faible** |
| 5.4 | Augmenter la limite de truncation des body HTTP dans les logs (1000 → 4000 ou configurable) | Debugging | Plus d'info en cas d'erreur | **Faible** |

---

## 4. Ordre d'exécution recommandé

```
┌─────────────────────────────────────────────────────────────┐
│  PHASE 1 — Fondations (peut être parallélisée)              │
│  ├── 1.1 Extraire stubs                                     │
│  ├── 1.2 Créer factory/modules.go                           │
│  ├── 1.3 Créer cli/commands.go                              │
│  └── 1.4 Factoriser helpers                                 │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│  PHASE 2 — Context propagation (séquentielle)               │
│  ├── 2.1 Interface input.Module                             │
│  ├── 2.2 Interface filter.Module                            │
│  ├── 2.3 Interface output.Module                            │
│  ├── 2.4 Executor                                           │
│  └── 2.5 HTTP client                                        │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│  PHASE 3 — Erreurs (parallélisable par package)             │
│  ├── 3.1 Définir pattern                                    │
│  └── 3.2-3.6 Appliquer par package                          │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│  PHASE 4 — Nommage (indépendante, peut être faite tôt)      │
│  └── 4.1-4.4 Renommages et commentaires                     │
└─────────────────────────────────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│  PHASE 5 — Validation (dernière, après stabilisation)       │
│  └── 5.1-5.4 Ajouts de validations                          │
└─────────────────────────────────────────────────────────────┘
```

### Dépendances critiques

- **Phase 2 dépend de Phase 1** — Les interfaces modifiées doivent être dans les bons packages
- **Phase 3 peut commencer pendant Phase 2** — Travail indépendant sur les erreurs
- **Phase 4 peut être faite à tout moment** — Renommages purs
- **Phase 5 doit être dernière** — Validations ajoutées une fois la structure stable

### Ce qui peut être repoussé

- 5.4 (limite truncation) — Nice-to-have, pas critique
- 4.4 (commentaires de section) — Cosmétique

---

## 5. Invariants à respecter

### Comportements à ne pas casser

| Invariant | Vérification |
|-----------|--------------|
| CLI `validate` retourne exit code 0/1/2 selon le résultat | Tests existants + tests manuels |
| CLI `run` retourne exit code 0/1/2/3 selon le résultat | Tests existants + tests manuels |
| Les logs JSON ont le même format (champs, niveaux) | Comparaison de sortie avant/après |
| Les pipelines schedulés tournent indéfiniment jusqu'à Ctrl+C | Test manuel |
| Le dry-run ne fait aucune requête HTTP sortante | Tests existants |
| Le graceful shutdown attend les tâches en cours (30s timeout) | Test manuel |
| Les retries respectent le backoff exponentiel configuré | Tests unitaires existants |

### APIs internes à préserver

| API | Raison |
|-----|--------|
| `pkg/connector.Pipeline` | Type public, potentiellement utilisé par des consommateurs externes |
| `pkg/connector.ExecutionResult` | Idem |
| `pkg/connector.ModuleConfig` | Idem |
| Format des fichiers de config YAML/JSON | Rétrocompatibilité |
| Schema JSON de validation | Rétrocompatibilité |

### Hypothèses du runtime à garder intactes

- Les modules sont exécutés séquentiellement (Input → Filters → Output)
- Un seul pipeline s'exécute à la fois par instance du scheduler
- Les records sont passés par valeur (copie), pas par référence partagée
- Le logger est thread-safe et peut être utilisé depuis n'importe quel goroutine

---

## 6. Critères de succès

À la fin du clean-up :

- [ ] `main.go` < 400 lignes
- [ ] Tous les tests existants passent (go test ./...)
- [ ] Couverture de tests maintenue ≥ 80%
- [ ] Aucun changement de comportement observable (mêmes inputs → mêmes outputs)
- [ ] `go vet ./...` sans warnings
- [ ] `golint ./...` sans nouveaux warnings
- [ ] Les interfaces acceptent `context.Context`
- [ ] Pattern d'erreur uniforme dans tout le codebase

---

## 7. Risques et mitigations

| Risque | Probabilité | Mitigation |
|--------|-------------|------------|
| Régression dans le parsing de config | Moyenne | Tests exhaustifs existants, run avant/après |
| Changement de comportement subtil dans les erreurs | Faible | Comparaison des logs avant/après |
| Oubli d'un appel à context.Cancel() | Moyenne | Code review attentif, tests avec timeout |
| Breaking change dans pkg/connector | Faible | Vérifier qu'aucun consommateur externe n'existe |

---

**Fin du plan de clean-up.**

---

> **Valides-tu ce plan avant que je commence le refactoring ?**
