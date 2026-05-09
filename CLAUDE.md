# Cannectors — AI Agent Instructions

## Fichiers liés

- @_bmad-output/project-context.md
- @test-patterns.md

## Project Maturity

**Le projet n'est pas en production et n'est pas utilisé par des tiers.**

- ❌ Ne pas introduire de shims, d'alias `Deprecated`, ou de code de rétrocompatibilité pour « ne rien casser »
- ✅ Un changement de comportement ou de signature d'API est acceptable si l'alternative est meilleure (plus simple, plus sûre, plus cohérente)
- ✅ Renommer, supprimer, et refactorer librement — corriger les callers plutôt que de maintenir l'ancienne surface
- Les seules limites restent : correction fonctionnelle, tests qui passent, et lint propre

## Validation systématique après modification de code

**Dès que tu modifies du code Go (impl ou tests), tu DOIS exécuter localement, avant d'annoncer la tâche comme terminée :**

1. **Tests** — cible le(s) package(s) modifié(s) par défaut (ex: `go test ./internal/auth/...`) pour garder un feedback loop rapide. Passe à `go test ./...` dès que le changement peut affecter des callers ailleurs : modification de signature/comportement d'une API exportée, renommage de type/fonction, changement de format d'erreur ou de contrat d'interface, retrait ou ajout d'un champ public, refacto de package partagé (`internal/httpclient`, `internal/auth`, `pkg/connector`, etc.).
2. **Lint** — `golangci-lint run ./...` (0 issues attendues). Le lint tourne vite sur tout le repo, pas de raison de cibler un package.

Règles :

- ❌ Ne jamais considérer une tâche comme `completed` sans avoir fait tourner les deux commandes et constaté qu'elles passent
- ❌ Ne pas se contenter de `go build ./...` — le build ne détecte ni les tests cassés ni les lints
- ✅ Dans le doute sur l'impact, élargis à `go test ./...` — coût minime comparé à casser silencieusement un caller
- ✅ Si un test échoue ou qu'un lint remonte, corrige avant de rendre la main
- ✅ Si un diagnostic lint pré-existant est remonté sur du code non touché par la tâche, ignore-le (ne nettoie pas opportunément) sauf si le user le demande
- ✅ En cas d'échec irréductible (ex: test flaky connu, lint qui demande un refacto hors-scope), remonte-le explicitement au user avant de marquer la tâche finie

## Library Usage (override project-context)

**Override de la règle `Library Usage` du `project-context.md`** : sur ce projet, tu **n'as pas besoin de demander l'autorisation** avant d'ajouter ou d'utiliser une librairie.

- ✅ Privilégie systématiquement les librairies maintenues (stdlib, `golang.org/x/...`, ou écosystème Go bien établi) plutôt que de réimplémenter une logique existante (validation, parsing, retry, backoff, LRU, time, URL, etc.)
- ✅ Si un package stdlib ou `golang.org/x/...` couvre déjà le besoin, utilise-le directement — pas de roue réinventée
- ✅ Ajoute la dépendance via `go get` et documente brièvement dans le code *pourquoi* ce package (une ligne de commentaire suffit)
- ❌ N'écris pas de code custom pour quelque chose que Go fait déjà (ex: RFC parsing, encoding, token validation, context cancellation, etc.)
- Critères de sélection : package maintenu activement, API stable, licence compatible, taille raisonnable

### Réflexe « check library first »

**Avant d'écrire ou de valider tout code non-trivial, vérifie systématiquement si une librairie couvre déjà le besoin.** Ce check est obligatoire — pas optionnel.

Déclencheurs (si tu penses/écris l'un de ces patterns, STOP et vérifie d'abord) :

- Parsing d'un format standardisé (RFC, ISO, IETF) : dates HTTP, content-types, URLs, emails, MIME, JWT, UUID, CIDR, etc.
- Validation d'un format standardisé : tokens HTTP, emails, URLs, semver, etc.
- Algorithmes classiques : LRU/LFU, exponential backoff, jitter, circuit breaker, rate limiter, consistent hashing, bloom filter
- Encoding/decoding : base64 variants, hex, percent-encoding, query strings, CSV, TOML
- Concurrence : pool de workers, semaphore, errgroup, singleflight, backpressure
- Cryptographie : jamais d'impl maison, toujours stdlib `crypto/*` ou `golang.org/x/crypto`
- Boucles de `time.Parse` sur plusieurs formats, `strings.Contains/HasPrefix` sur du contenu structuré, compteurs atomiques avec TTL maison → quasi-toujours une lib existe

Ordre de recherche :

1. **stdlib Go** (`net/http`, `net/url`, `mime`, `encoding/*`, `crypto/*`, `strings`, `time`, `errors`, `sync`, `golang.org/x/sync/errgroup`, etc.)
2. **`golang.org/x/...`** (extensions quasi-officielles maintenues par l'équipe Go : `x/net`, `x/crypto`, `x/sync`, `x/text`, `x/time`)
3. **Écosystème Go bien établi** (hashicorp, cenkalti, uber-go, google/uuid, etc.)
4. Réimpl maison uniquement en dernier recours, avec justification explicite (ex: intégration avec un hook spécifique que les libs n'offrent pas)

Quand tu lis du code existant qui fait ce genre de chose à la main (parseur de dates en boucle, validateur ad-hoc, cache maison sans TTL lib, etc.), flagge-le et propose le remplacement par la lib correspondante — même si ce n'était pas l'objet initial de la tâche, c'est de la dette à signaler.



Behavioral guidelines to reduce common LLM coding mistakes. Merge with project-specific instructions as needed.

**Tradeoff:** These guidelines bias toward caution over speed. For trivial tasks, use judgment.

## 1. Think Before Coding

**Don't assume. Don't hide confusion. Surface tradeoffs.**

Before implementing:
- State your assumptions explicitly. If uncertain, ask.
- If multiple interpretations exist, present them - don't pick silently.
- If a simpler approach exists, say so. Push back when warranted.
- If something is unclear, stop. Name what's confusing. Ask.

## 2. Simplicity First

**Minimum code that solves the problem. Nothing speculative.**

- No features beyond what was asked.
- No abstractions for single-use code.
- No "flexibility" or "configurability" that wasn't requested.
- No error handling for impossible scenarios.
- If you write 200 lines and it could be 50, rewrite it.

Ask yourself: "Would a senior engineer say this is overcomplicated?" If yes, simplify.

## 3. Surgical Changes

**Touch only what you must. Clean up only your own mess.**

When editing existing code:
- Don't "improve" adjacent code, comments, or formatting.
- Don't refactor things that aren't broken.
- Match existing style, even if you'd do it differently.
- If you notice unrelated dead code, mention it - don't delete it.

When your changes create orphans:
- Remove imports/variables/functions that YOUR changes made unused.
- Don't remove pre-existing dead code unless asked.

The test: Every changed line should trace directly to the user's request.

## 4. Goal-Driven Execution

**Define success criteria. Loop until verified.**

Transform tasks into verifiable goals:
- "Add validation" → "Write tests for invalid inputs, then make them pass"
- "Fix the bug" → "Write a test that reproduces it, then make it pass"
- "Refactor X" → "Ensure tests pass before and after"

For multi-step tasks, state a brief plan:
```
1. [Step] → verify: [check]
2. [Step] → verify: [check]
3. [Step] → verify: [check]
```

Strong success criteria let you loop independently. Weak criteria ("make it work") require constant clarification.

---

**These guidelines are working if:** fewer unnecessary changes in diffs, fewer rewrites due to overcomplication, and clarifying questions come before implementation rather than after mistakes.


---

## Pièges spécifiques au projet (retours de sessions précédentes)

### Test lab — exécution de pipelines

- **WireMock journal renvoie l'ordre antéchronologique** (le plus récent en `[0]`, le plus ancien en `[-1]`). Si tu cherches « la requête la plus récente », c'est `requests[0]`. Bug typique : itérer en ordre supposé chronologique et vérifier des assertions sur la mauvaise requête.
- **Un binaire `cannectors` laissé sur un port masque les rebuilds.** Toujours vérifier `pgrep -fa cannectors` avant de retester un fix : un vieux process sur le même `listenAddress` répondra avec l'ancien binaire et tu chercheras un bug qui n'existe plus dans le code actuel.
- **`wait` sans argument attend TOUS les enfants en background**, y compris un `cannectors run &` lancé plus tôt qui ne s'arrête jamais. Pour des bursts curl en parallèle dans un script qui a déjà un process long-running, capture les PIDs (`pids+=("$!")`) et attends-les explicitement (`for p in "${pids[@]}"; do wait "$p"; done`).
- **Le contexte HTTP d'une requête est annulé dès que la réponse est écrite.** Si tu enqueues un handler qui s'exécute après la réponse (cas du webhook avec `queueSize`), passer `r.Context()` au handler garantit un `context canceled` côté output. Utilise le contexte du serveur et propage uniquement le trace ID. C'était un vrai bug fixé en 22.7.

### Filtres et input modules

- **Pointeurs sur éléments de slice + `append` = aliasing dangereux.** `&p.Filters[i]` puis `p.Filters = append(p.Filters, ...)` peut invalider le pointeur capturé si la slice se réalloue. Pré-dimensionne (`make([]T, n)`) et assigne par index. Bug fixé en 22.5 dans `internal/config/converter.go`.
- **`statePersistence.storagePath` n'était lu que par l'input.** Avant 22.6, l'executor sauvait dans `./cannectors-data/state` par défaut → l'état ne faisait jamais l'aller-retour. Désormais l'executor partage le même `StateStore` — cf. `internal/runtime/pipeline.go:setupStatePersistence`.
- **`http_call` n'a pas de `resultKey`** (hardcodé à `_response` dans le schema). Il utilise toujours un cache LRU ; sans `keys` ni `cache.key` explicite tous les records partagent le même slot et tu n'observes qu'une seule requête. Pour forcer un appel par record, ajoute `cache.key: <id>` au pipeline.
- **`sql_call` substitue `{{record.x}}` comme paramètre positionnel** (`$1` en postgres), pas comme texte. Ne JAMAIS l'entourer de quotes dans le SQL.

### YAML pipelines

- **Schema requiert toujours `filters:`** au top-level, même vide (`filters: []`). Sans, validation échoue avec `missing required property "filters"`.
- **`schedule` minLength = 9** : le format CRON 5 champs (`* * * * *`) ne passe pas. Utiliser le format 6 champs (`* * * * * *`).
- **Les exemples sous `examples/` peuvent être stale** par rapport au schema actuel — toujours valider avec `./cannectors validate <file> --verbose` avant de partir d'un exemple.
- **Webhook input n'est pas utilisable via `cannectors run`** sans le branchement spécifique (`runWebhookPipeline`) ajouté en 22.7. Si tu écris un nouvel input callback-based (non-`Fetch`), il faut l'aiguillage dans `cmd/cannectors/main.go`.

### Outillage tooling agent

- **Le tool `bash` refuse les commandes contenant la chaîne `kill $VAR`** (variables shell non développées dans certaines positions). Soit substituer le PID littéral via `pgrep -f ... | xargs -I{} kill {}`, soit utiliser `pkill`/`killall` (interdits par config) — en pratique : capture le PID en clair dans un echo puis utilise-le littéralement dans la commande suivante.
- **Les processes longs lancés via le tool `bash` sont tués en fin de session** sauf `mode: async, detach: true`. Pour un webhook ou serveur de test qui doit survivre plusieurs commandes, prévoir le détachement explicite.
