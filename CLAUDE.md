# Cannectors — AI Agent Instructions

@_bmad-output/project-context.md

## Project Maturity

**Le projet n'est pas en production et n'est pas utilisé par des tiers.**

- ❌ Ne pas introduire de shims, d'alias `Deprecated`, ou de code de rétrocompatibilité pour « ne rien casser »
- ✅ Un changement de comportement ou de signature d'API est acceptable si l'alternative est meilleure (plus simple, plus sûre, plus cohérente)
- ✅ Renommer, supprimer, et refactorer librement — corriger les callers plutôt que de maintenir l'ancienne surface
- Les seules limites restent : correction fonctionnelle, tests qui passent, et lint propre

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
