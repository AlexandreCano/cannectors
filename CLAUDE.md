# Cannectors — AI Agent Instructions

@_bmad-output/project-context.md

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
