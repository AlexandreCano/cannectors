# Cannectors — AI Agent Instructions

@_bmad-output/project-context.md

## Project Maturity

**Le projet n'est pas en production et n'est pas utilisé par des tiers.**

- ❌ Ne pas introduire de shims, d'alias `Deprecated`, ou de code de rétrocompatibilité pour « ne rien casser »
- ✅ Un changement de comportement ou de signature d'API est acceptable si l'alternative est meilleure (plus simple, plus sûre, plus cohérente)
- ✅ Renommer, supprimer, et refactorer librement — corriger les callers plutôt que de maintenir l'ancienne surface
- Les seules limites restent : correction fonctionnelle, tests qui passent, et lint propre
