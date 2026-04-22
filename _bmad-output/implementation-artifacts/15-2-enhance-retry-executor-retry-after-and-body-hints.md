# Story 15.2: Enhance `errhandling.RetryExecutor` with Retry-After and body-hint callbacks

Status: review

## Story

En tant que développeur,
je veux que `errhandling.RetryExecutor` supporte nativement le header HTTP `Retry-After` et un callback `retryHintFromBody`,
afin que tous les modules HTTP puissent réutiliser ce retry générique au lieu de réimplémenter leur propre boucle (`output/http_request.go:retryLoop` fait 250 LOC pour combler ce manque).

## Acceptance Criteria

1. **Given** un `errhandling.RetryExecutor` configuré avec `RetryHooks`
   **When** une tentative échoue avec une erreur exposant un header `Retry-After`
   **Then** si `cfg.UseRetryAfterHeader` est vrai, le prochain délai utilise la valeur du header (borné par `MaxDelayMs`, HTTP-date dans le passé ⇒ retry immédiat)
   **And** sinon le délai calculé par l'exponential backoff est utilisé.

2. **Given** un `RetryExecutor` configuré avec un callback `ShouldRetryBody(body []byte) (retry, hinted bool)`
   **When** une réponse HTTP contient un body et le callback retourne `(hinted=true, retry=true)`
   **Then** la décision de retry override la classification automatique (si le status code est dans `retryableStatusCodes`)
   **And** si `ShouldRetryBody` retourne `(hinted=true, retry=false)`, le retry est annulé même sur un status retryable.

3. **Given** le `RetryExecutor` existant
   **When** j'ajoute les nouvelles capacités
   **Then** l'API publique actuelle (`Execute`, `ExecuteWithCallback`, `GetRetryInfo`) reste rétrocompatible
   **And** les tests existants dans `errhandling/retry_test.go` passent sans modification.

4. **Given** le nouveau callback `ShouldRetryBody`
   **When** il n'est pas fourni (nil)
   **Then** le comportement est identique à aujourd'hui (décision par classification d'erreur uniquement).

5. **Given** la configuration `RetryConfig.RetryHintFromBody`
   **When** c'est une expression `expr-lang` non vide
   **Then** `httpclient.DoWithRetry` compile l'expression une fois au setup puis l'évalue à chaque échec
   **And** la longueur d'expression > `MaxRetryHintExpressionLength` (10 000) est rejetée au setup.

## Tasks / Subtasks

- [x] Task 1 : Étendre `RetryHooks` (AC #1, #2, #4)
  - [x] `ShouldRetryBody func([]byte) (retry, hinted bool)` déjà exposé dans `httpclient.RetryHooks` (Story 15.1) — consommé par `DoWithRetry` dans 15.2
  - [x] `errhandling.Hooks.ExtractRetryAfter func(err error) (time.Duration, bool)` ajouté (errhandling reste indépendant de `httpclient.Error`)
  - [x] `httpclient.RetryHooks.OnAttemptFailure` ajouté (indispensable pour OAuth2 401, consommé par 15.3/15.4)

- [x] Task 2 : Modifier `RetryExecutor.ExecuteWithCallback` (AC #1, #3)
  - [x] Nouvelle méthode `RetryExecutor.ExecuteWithHooks(ctx, fn, hooks, callback)` — retour-compatible
  - [x] `hooks.ExtractRetryAfter` remplace le délai exponential-backoff (capé par MaxDelayMs, clamp 0 pour négatifs)
  - [x] `ExecuteWithCallback(ctx, fn, cb)` conservé comme wrapper (délègue à `ExecuteWithHooks`)

- [x] Task 3 : Compilation différée de `retryHintFromBody` (AC #5)
  - [x] `httpclient.CompileRetryHint(expr string) (*vm.Program, error)` — valide longueur + compile
  - [x] `httpclient.EvalRetryHint(program *vm.Program, body []byte) (retry, hinted bool)` — JSON parse + expr.Run, (false,false) sur échec

- [x] Task 4 : Tests (AC #1-#5)
  - [x] `errhandling/retry_hooks_test.go` : ExtractRetryAfter override, cap MaxDelayMs, clamp négatif → 0, nil hook = comportement actuel
  - [x] `httpclient/retry_hint_test.go` : nil program, empty body, JSON invalide, résultat non-booléen, expression trop longue
  - [x] `httpclient/doretry_test.go` : Retry-After réel, ShouldRetryBody hint=false → pas de retry, OnAttemptFailure force-retry 401

## Dev Notes

### Rationale

L'audit §3.1 D4 note que `output/http_request.go:656-693` (`retryLoop`) réimplémente ~250 LOC alors que `errhandling.RetryExecutor` existe déjà. La seule raison est que l'exécuteur actuel ne connaît pas `Retry-After` ni `retryHintFromBody`. En injectant ces deux capacités par callbacks, on débloque la migration des 3 modules HTTP.

### Design Decisions

- **Callbacks plutôt qu'intégration dure** : garde `errhandling` indépendant du type `httpclient.Error`.
- **Rétrocompatibilité stricte** : pas de breaking change sur `Execute`/`ExecuteWithCallback` existants.
- **Compilation au setup** : la compilation d'une expression `expr-lang` est coûteuse, doit être faite une fois à la création du module.

### Out of Scope

- Migration effective des modules HTTP → 15.3, 15.4, 15.5
- Circuit breaker → future story

## References

- docs/AUDIT_TECHNIQUE_2026-04-21.md §3.1 D4, §5 P1.1
- `errhandling/retry.go:232-390` (implémentation actuelle)
- `output/http_request.go:656-693` (retryLoop à remplacer)
- `output/http_request.go:810-840` (waitForRetry, intégration Retry-After à extraire)
- `output/http_request.go:761-805` (evaluateRetryHintFromBody à extraire)

## File List

Modifiés :

- `internal/errhandling/retry.go` : ajout du type `Hooks`, méthode
  `RetryExecutor.ExecuteWithHooks`, helper `clampDelay` ;
  `ExecuteWithCallback` devient wrapper sans changement de signature.

Nouveaux :

- `internal/httpclient/retry_hint.go` : `MaxRetryHintExpressionLength`,
  `CompileRetryHint`, `EvalRetryHint`.
- `internal/httpclient/retry_hint_test.go` : tests de compilation
  (valide, vide, trop longue, syntaxe invalide) et d'évaluation (nil
  program, body vide, JSON invalide, résultat non-booléen, true/false).
- `internal/errhandling/retry_hooks_test.go` : tests de `ExecuteWithHooks`
  (override delay via `ExtractRetryAfter`, cap `MaxDelayMs`, clamp
  négatif → 0, hook nil = no-op, compat `ExecuteWithCallback`).

Mis à jour (Story 15.1) :

- `internal/httpclient/doretry.go` : `DoWithRetry` consomme maintenant
  `errhandling.Hooks.ExtractRetryAfter` et exploite `ShouldRetryBody` +
  `OnAttemptFailure` ; ajoute la relecture du body (rewind via
  `io.NopCloser(bytes.NewReader)`) et la réémission du body de requête
  entre tentatives (GetBody).

Vérifications :

- `go test ./internal/httpclient/... ./internal/errhandling/...` : OK
  (couverture respectivement 92,8 % et 94,8 %).
- `go test ./...` : OK.
- `golangci-lint run ./...` : 0 issue.
