# Notes de Refactorisation - Story 4.4

## Visibilité Go (Exports)

En Go, la visibilité est déterminée par la **première lettre** du nom :

### Règles de visibilité

- **Majuscule (exporté/public)** : `NewHTTPPolling`, `ConvertToPipeline`, `DefaultMaxAttempts`
  - Accessible depuis d'autres packages
  - Utilisé pour l'API publique du package
  - Exemple : `runtime.NewExecutor()` peut être appelé depuis `cmd/canectors`

- **Minuscule (non-exporté/privé)** : `doRequest`, `convertErrorHandling`, `resolveOnError`
  - Accessible uniquement dans le même package
  - Utilisé pour les détails d'implémentation internes
  - Exemple : `input.doRequest()` ne peut être appelé que depuis `package input`

### Exemples dans le code

**Package `internal/config` :**
- `ConvertToPipeline` (exporté) - API publique pour convertir config → Pipeline
- `convertModuleConfig` (privé) - Détail d'implémentation
- `resolveAndInjectErrorHandling` (privé) - Détail d'implémentation

**Package `internal/modules/input` :**
- `NewHTTPPollingFromConfig` (exporté) - Constructeur public
- `doRequest` (privé) - Méthode interne
- `extractTimeout` (privé) - Helper interne

**Package `internal/runtime` :**
- `NewExecutor` (exporté) - API publique
- `executeInput` (privé) - Détail d'implémentation

### Bonnes pratiques

1. **Exporter le minimum nécessaire** : Seulement ce qui doit être utilisé par d'autres packages
2. **Préfixer les helpers internes** : `extract*`, `parse*`, `build*`, `handle*` sont souvent privés
3. **Constructeurs publics** : `New*` est généralement exporté
4. **Types publics** : Si un type est utilisé dans l'API publique, il doit être exporté

## Constantes runtime/retry.go

Les constantes dans `internal/runtime/retry.go` sont des **ré-exports** depuis `internal/errhandling` :

```go
const (
	DefaultMaxAttempts       = errhandling.DefaultMaxAttempts
	DefaultDelayMs           = errhandling.DefaultDelayMs
	DefaultBackoffMultiplier = errhandling.DefaultBackoffMultiplier
	// ...
)
```

### Pourquoi elles ne sont pas utilisées ?

1. **Package `runtime` est un wrapper** : Il ré-exporte pour compatibilité, mais le code utilise directement `errhandling.*`
2. **Pas de dépendance circulaire** : Les modules (`input`, `output`) importent `errhandling` directement, pas `runtime`
3. **Séparation des responsabilités** : 
   - `errhandling` = implémentation
   - `runtime` = ré-export pour compatibilité (peut être supprimé si non utilisé)

### Recommandation

**Vérification effectuée :**
```bash
grep -r "runtime\.Default" canectors-runtime/
# Résultat : Aucune utilisation trouvée
```

**Conclusion :** Les constantes dans `runtime/retry.go` ne sont **pas utilisées** dans le codebase. Elles sont des ré-exports pour compatibilité, mais le code utilise directement `errhandling.*`.

**Options :**
1. **Garder** : Si prévu pour une future API publique du package `runtime`
2. **Supprimer** : Si non nécessaire (réduit la duplication)

**Recommandation :** Les garder pour l'instant car elles font partie de l'API de ré-export du package `runtime`, même si non utilisées actuellement. Elles peuvent être supprimées plus tard si l'API publique du package `runtime` n'en a pas besoin.

## Refactorisations effectuées

### 1. Constantes de log
- `logMsgPaginationStarted`, `logMsgPaginationPageFetched`, `logMsgPaginationCompleted`
- Évite les typos et facilite la maintenance

### 2. `resolveAndInjectErrorHandling`
- Extraite en 3 fonctions : `resolveOnError`, `resolveTimeout`, `resolveRetry`
- Complexité cognitive réduite : chaque fonction a une responsabilité unique

### 3. `convertErrorHandling`
- Extraite en 5 fonctions : `parseLegacyRetryFields`, `extractStringField`, `extractTimeoutMs`, `extractRetryConfig`, `buildRetryFromLegacy`
- Séparation claire entre parsing legacy et nouveau format

### 4. `ConvertToPipeline`
- Extraite en 4 fonctions : `newPipeline`, `extractPipelineMetadata`, `extractModules`, `extractDefaultsAndErrorHandling`
- Flux linéaire plus facile à suivre

### 5. `NewHTTPPollingFromConfig`
- Extraite en 8 fonctions : `extractEndpoint`, `extractTimeout`, `extractHeaders`, `extractDataField`, `extractPagination`, `extractRetryConfig`, `createHTTPClient`, `createAuthHandler`, `logModuleCreation`
- Chaque extraction est isolée et testable

### 6. `doRequest`
- Extraite en 8 fonctions : `buildRequest`, `setRequestHeaders`, `executeRequest`, `closeResponseBody`, `readResponseBody`, `handleHTTPError`, `truncateBodyForLogging`, `handleOAuth2Unauthorized`, `logRequestStart`, `logRequestSuccess`
- Séparation claire des responsabilités (construction, exécution, gestion d'erreur)

### 7. `doRequestWithHeaders`
- Extraite en 6 fonctions : `retryLoop`, `waitForRetry`, `shouldRetryOAuth2`, `handleRetrySuccess`, `handleRetryFailure`, `logNonRetryableError`, `logTransientError`
- Logique de retry isolée et plus facile à comprendre

## Métriques de complexité

| Fonction | Avant (lignes) | Après (lignes max/fonction) | Réduction |
|----------|----------------|----------------------------|-----------|
| `resolveAndInjectErrorHandling` | 54 | ~15 | 72% |
| `convertErrorHandling` | 38 | ~10 | 74% |
| `ConvertToPipeline` | 97 | ~25 | 74% |
| `NewHTTPPollingFromConfig` | 75 | ~15 | 80% |
| `doRequest` | 64 | ~20 | 69% |
| `doRequestWithHeaders` | 96 | ~25 | 74% |

**Complexité cognitive réduite** : Chaque fonction a maintenant une responsabilité unique et claire.
