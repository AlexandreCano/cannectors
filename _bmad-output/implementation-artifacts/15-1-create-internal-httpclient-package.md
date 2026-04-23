# Story 15.1: Create `internal/httpclient` shared package

Status: review

## Story

En tant que développeur,
je veux un package `internal/httpclient` qui factorise la création de client HTTP, le type d'erreur, la boucle de retry et la validation RFC 7230 des headers,
afin que les trois modules HTTP (polling input, request output, http_call filter) puissent s'appuyer sur une implémentation commune au lieu de dupliquer ~1500 LOC.

## Acceptance Criteria

1. **Given** un nouveau package `internal/httpclient`
   **When** je consulte son API publique
   **Then** il expose au minimum : `Client` (wrapper `*http.Client` avec timeout + transport optimisé), `Error` (remplaçant les deux `HTTPError` dupliqués), `DoWithRetry(ctx, req, cfg)`, `ValidateHeaderName/Value` (RFC 7230), `TryAddValidHeader(headers, name, value)`.
   **And** le package ne dépend d'aucun module (input/filter/output) — uniquement de `errhandling`, `logger`, `pkg/connector`.

2. **Given** le package `httpclient`
   **When** j'appelle `httpclient.NewClient(timeout)`
   **Then** il retourne un `*http.Client` avec `MaxIdleConns=100`, `MaxIdleConnsPerHost=10`, `IdleConnTimeout=90s` et le timeout demandé
   **And** le comportement est identique à l'actuel `createHTTPClient` dupliqué dans `input/http_polling.go` et `output/http_request.go`.

3. **Given** le type `httpclient.Error`
   **When** une erreur HTTP est construite
   **Then** elle expose `StatusCode`, `Status`, `Endpoint`, `Method`, `Message`, `ResponseBody`, `ResponseHeaders`
   **And** elle implémente `GetRetryAfter() string` pour l'extraction du header `Retry-After`
   **And** elle est compatible avec `errors.As` pour les conversions.

4. **Given** `httpclient` est publié
   **When** j'exécute `go test ./internal/httpclient/...`
   **Then** la couverture de ses fonctions est ≥ 85 %
   **And** les tests couvrent : création client, validation headers (valide, invalide, caractères contrôles, CRLF), extraction Retry-After (secondes, HTTP-date RFC1123/RFC850/ANSI).

5. **Given** les modules HTTP existants n'ont pas encore migré
   **When** j'exécute `go test ./...`
   **Then** tous les tests existants continuent de passer (ce story crée le package sans refacto des modules).

## Tasks / Subtasks

- [x] Task 1 : Créer la structure du package (AC #1)
  - [x] `internal/httpclient/client.go` : `type Client`, `NewClient(timeout time.Duration) *Client`
  - [x] `internal/httpclient/error.go` : `type Error` unifié (fusion des champs des deux HTTPError actuels)
  - [x] `internal/httpclient/headers.go` : `ValidateHeaderName`, `ValidateHeaderValue`, `TryAddValidHeader`
  - [x] Godoc complet sur chaque export

- [x] Task 2 : Extraire les helpers Retry-After (AC #3)
  - [x] Déplacer `parseRetryAfterValue` depuis `output/http_request.go:863` vers `httpclient/retry_after.go`
  - [x] Exposer `Error.GetRetryAfter() string`
  - [x] Exposer `httpclient.ParseRetryAfter(value string) (time.Duration, bool)`

- [x] Task 3 : Implémenter `DoWithRetry` (AC #1)
  - [x] Signature : `DoWithRetry(ctx context.Context, req *http.Request, cfg RetryConfig, hooks RetryHooks) (*http.Response, error)`
  - [x] `RetryHooks` : `OnRetry(attempt int, err error, delay time.Duration)`, `ShouldRetryBody(body []byte) (retry bool, hinted bool)`
  - [x] Déléguer le calcul du delay à `errhandling.RetryExecutor` (Story 15.2)

- [x] Task 4 : Tests unitaires (AC #4)
  - [x] `client_test.go` : timeout respecté, transport settings, close idle connections
  - [x] `error_test.go` : `errors.As`, `GetRetryAfter` (présent/absent/vide)
  - [x] `headers_test.go` : chaque cas RFC 7230 (alphanumeric, symboles valides, :, CR, LF, control chars, espaces, UTF-8)
  - [x] `retry_after_test.go` : seconds 0/120/-1, RFC1123, RFC850, ANSI C, format invalide
  - [x] `doretry_test.go` : success first attempt, retry sur 5xx, stop sur 4xx, exhaustion, nil client

- [x] Task 5 : Non-régression (AC #5)
  - [x] `go test ./...` et `golangci-lint run` passent
  - [x] Les modules HTTP existants ne sont **pas** modifiés dans ce story

## Dev Notes

### Rationale

L'audit technique (docs/AUDIT_TECHNIQUE_2026-04-21.md §3.1) identifie 10 duplications majeures dont D1 (`HTTPError` × 2), D2 (`createHTTPClient` × 2), D4 (retry loop réimplémentée), D5 (`keyEntry` × 2). L'effet combiné : `output/http_request.go` 1641 LOC + `input/http_polling.go` 1057 LOC + `filter/http_call.go` 894 LOC ≈ 3600 LOC de HTTP, dont ~40 % serait factorisable.

Ce story pose la **fondation** ; les migrations des modules sont dans 15.3/15.4/15.5.

### Design Decisions

- **Un seul `Error` partagé** : les deux `HTTPError` actuels ont 4 champs en commun et diffèrent uniquement sur `ResponseBody`/`ResponseHeaders` (absents de `input/http_polling.go`). La version unifiée prend le sur-ensemble.
- **`DoWithRetry` reçoit `RetryConfig` + `RetryHooks`** plutôt qu'un module — garde le package indépendant.
- **Pas de masking auth ici** : la logique de preview reste dans `output` (sera traitée via une évolution de `auth.Handler` dans un autre story).

### Out of Scope

- Migration réelle des modules → 15.3, 15.4, 15.5
- Extension de `RetryExecutor` → 15.2
- Unification `keyEntry` / `moduleconfig.KeyConfig` → 16.x

## References

- docs/AUDIT_TECHNIQUE_2026-04-21.md §3.1 D1, D2, D3, D4, D5
- docs/plan.md §5.1 "Centraliser extraction timeout/retry"
- `output/http_request.go:77-97` (HTTPError v2)
- `input/http_polling.go:53-62` (HTTPError v1)
- `output/http_request.go:1217-1268` (validateHeader*, tryAddValidHeader)
- `output/http_request.go:860-887` (parseRetryAfterValue)

## File List

Fichiers créés :

- `internal/httpclient/client.go`
- `internal/httpclient/error.go`
- `internal/httpclient/headers.go`
- `internal/httpclient/retry_after.go`
- `internal/httpclient/doretry.go`
- `internal/httpclient/client_test.go`
- `internal/httpclient/error_test.go`
- `internal/httpclient/headers_test.go`
- `internal/httpclient/retry_after_test.go`
- `internal/httpclient/doretry_test.go`

Aucun fichier existant modifié (les modules input/filter/output ne sont pas
touchés par cette story — migrations prévues en 15.3 / 15.4 / 15.5).

Vérifications :

- `go build ./...` : OK
- `go vet ./...` : OK
- `go test ./...` : OK (tous les packages)
- `go test ./internal/httpclient/... -cover` : **89.7 %** (≥ 85 % exigé)
- `golangci-lint run ./...` : 0 issue
