# Epic 24: Schema/runtime contract alignment

Status: in-progress

## Goal

Aligner le contrat produit des pipelines Cannectors entre les schemas JSON, le parsing Go, la conversion de config et le runtime reel.

Le schema devient la porte d'entree obligatoire pour toute execution utilisateur. Le runtime garde des validations defensives pour les appels directs internes, mais le chemin normal ne doit plus accepter une config que le schema refuserait, ni ignorer silencieusement une config que le schema accepte.

## Product Decisions

- Le projet n'etant pas release, aucune retrocompatibilite n'est requise pour les anciens champs.
- Toute execution de pipeline doit passer par la validation schema.
- Les champs inconnus restent ignores par le code Go si le runtime est appele directement, mais les configs utilisateur passent par un schema strict.
- `enabled` et `onError` doivent etre applicables sur tous les modules via `ModuleBase`.
- Sans `schedule`, un input polling/database s'execute une seule fois. Avec `schedule`, il est planifie.
- Tous les modules HTTP sortants doivent accepter toute methode HTTP valide et pouvoir envoyer un body.
- Les champs SQL exclusifs restent en `oneOf`: `connectionString` xor `connectionStringRef`, `query` xor `queryFile`.
- Les paginations HTTP et database utilisent `param` comme champ canonique pour le parametre principal; pas d'anciens aliases.
- `tags` reste accepte pour l'experience utilisateur, meme si le runtime l'ignore pour l'instant.
- Pas de limites artificielles de profondeur pour les conditions imbriquees.
- `success.expression` est une feature produit importante et doit etre implementee.
- `cache.defaultTTL` est remplace par `cache.ttlSeconds` partout, sans compatibilite.
- Les defaults `retry` ne s'appliquent qu'aux modules HTTP retry-aware: `httpPolling`, `http_call`, `httpRequest`.

## Stories

- Story 24.1: Enforce schema validation before pipeline execution
- Story 24.2: Align root pipeline and ModuleBase contracts
- Story 24.3: Align common HTTP contract
- Story 24.4: Align common SQL contract and database modules
- Story 24.5: Align authentication config contract
- Story 24.6: Align httpPolling schedule and pagination
- Story 24.7: Align webhook input contract
- Story 24.8: Align mapping filter contract
- Story 24.9: Simplify condition branching and add drop filter
- Story 24.10: Align script, set and remove filter contracts
- Story 24.11: Align http_call filter contract
- Story 24.12: Align httpRequest output success and request contract

## Cross-Cutting Acceptance Criteria

1. **Given** une config pipeline chargee depuis un fichier
   **When** elle est executee via le chemin CLI/runtime utilisateur
   **Then** elle est validee par les JSON Schemas avant instanciation des modules.

2. **Given** une config validee par schema
   **When** le runtime l'execute
   **Then** aucun champ accepte par le schema n'est ignore silencieusement s'il porte une semantique runtime.

3. **Given** une config invalide mais appelee directement depuis un test ou code Go interne
   **When** elle atteint un constructeur de module
   **Then** le constructeur renvoie une erreur claire pour les contraintes runtime critiques.

4. **Given** un ancien champ remplace par une decision produit plus propre
   **When** la story correspondante est implementee
   **Then** l'ancien champ est retire sans alias de compatibilite.

## References

- `docs/SCHEMA_RUNTIME_DRIFT.md`
- `internal/config/schema/*.json`
- `internal/config/converter.go`
- `internal/config/validator.go`
- `internal/registry/builtins.go`
- `internal/moduleconfig`
- `internal/modules/input`
- `internal/modules/filter`
- `internal/modules/output`
- `pkg/connector/types.go`
