# Story 17.5: Protect HTTP 401 retry from infinite loop with bad OAuth2 credentials

Status: backlog

## Story

En tant qu'opérateur,
je veux qu'un pipeline avec des credentials OAuth2 révoqués ou expirés échoue rapidement au lieu de boucler sur le retry 401,
afin de ne pas consommer inutilement de CPU/bande passante ni de déclencher des rate limits côté API.

## Acceptance Criteria

1. **Given** un module HTTP avec auth OAuth2
   **When** l'API retourne 401 systématiquement (credentials invalides)
   **Then** le module tente au maximum **2 fois** l'invalidation + refresh du token
   **And** après 2 échecs consécutifs avec 401, le module échoue avec `ErrOAuth2InvalidCredentials` (nouveau)
   **And** aucune tentative supplémentaire n'est faite (pas de rate limit accidentel).

2. **Given** un 401 suivi d'un refresh réussi qui retourne 200
   **When** la deuxième tentative réussit
   **Then** le module continue normalement.

3. **Given** un 401 suivi d'un refresh qui retourne encore 401
   **When** la deuxième tentative échoue aussi
   **Then** le compteur `oauth2401Consecutive` atteint 2 → fail.

4. **Given** un `oauth2Retried bool` actuel (output/http_request.go, input/http_polling.go)
   **When** je le remplace
   **Then** il devient un `oauth2401Count int` borné par `MaxOAuth2Retries` (=2)
   **And** cette constante est documentée.

5. **Given** un test avec un serveur qui retourne toujours 401
   **When** le module est exécuté
   **Then** exactement 3 requêtes sont faites (1 initiale + 2 retries) puis échec
   **And** le test vérifie les 3 requêtes et l'erreur finale.

## Tasks / Subtasks

- [ ] Task 1 : Définir la constante (AC #4)
  - [ ] Dans `auth/auth.go` ou `httpclient` : `const MaxOAuth2Retries = 2`
  - [ ] Exposer `ErrOAuth2InvalidCredentials = errors.New("OAuth2 credentials appear invalid after N retries")`

- [ ] Task 2 : Modifier `handleOAuth2Unauthorized` (AC #1, #3)
  - [ ] Dans `output/http_request.go:604` et `input/http_polling.go:408` : remplacer `oauth2Retried bool` par un compteur
  - [ ] Si `count >= MaxOAuth2Retries` : retourner false + logger une fois en WARN
  - [ ] Sinon : invalider le token et retry

- [ ] Task 3 : Si stories 15.3/15.4/15.5 faites : déplacer dans `httpclient` (AC #1)
  - [ ] La logique OAuth2 reste module-spécifique (nécessite accès au `authHandler`)
  - [ ] Mais le compteur et la constante peuvent être dans `httpclient.OAuth2RetryGuard`

- [ ] Task 4 : Tests (AC #5)
  - [ ] `TestHTTP401_InfiniteLoopProtection` : serveur retourne toujours 401 → exactement N requêtes puis échec
  - [ ] `TestHTTP401_SuccessAfterRefresh` : premier 401, refresh, puis 200 → succès
  - [ ] `TestHTTP401_TwoFailures` : deux 401 consécutifs → échec avec ErrOAuth2InvalidCredentials

## Dev Notes

### Rationale

Bug moyen P1 de `docs/plan.md` §3 "Bug 7". Le code actuel a un flag `oauth2Retried bool` qui empêche un deuxième refresh **dans la même requête**, mais quand la prochaine requête arrive (pipeline bouclé ou retry du runtime), le flag est remis à zéro → boucle infinie possible.

### Design Decisions

- **Compteur borné** : 2 tentatives suffisent (le token a échoué, on re-fetch, ça échoue encore → credentials définitivement invalides).
- **Erreur typée `ErrOAuth2InvalidCredentials`** : permet au runtime de classifier ça en `CategoryAuthentication` → fatal, pas de retry outer.

### Out of Scope

- Détection de "account locked" spécifique fournisseur → générique suffit
- Notification ops (email, Slack) → responsabilité de l'ops tooling en amont

## References

- docs/AUDIT_TECHNIQUE_2026-04-21.md §5 P1.2
- docs/plan.md §3 "Bug 7"
- `output/http_request.go:604-634` (handleOAuth2Unauthorized)
- `input/http_polling.go:408-453`

## File List

(à compléter)
