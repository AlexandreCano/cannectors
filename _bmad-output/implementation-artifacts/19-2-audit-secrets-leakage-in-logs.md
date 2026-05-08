# Story 19.2: Audit and fix secret leakage in logs

Status: review

## Story

En tant qu'opérateur responsable de la sécurité,
je veux qu'aucun secret (token OAuth2, API key, password, connection string complète) n'apparaisse dans les logs,
afin de ne pas exposer de credentials dans les agrégateurs de logs, disques, traces d'erreur.

## Acceptance Criteria

1. **Given** un pipeline avec authentification OAuth2
   **When** il s'exécute
   **Then** aucun log ne contient `clientSecret`, `token`, ni les query params contenant `access_token`, `api_key`, `password`
   **And** un test automatisé scanne les logs générés pour détecter ces patterns.

2. **Given** un pipeline avec authentification Basic
   **When** il s'exécute
   **Then** le log de la config initiale ne loggue ni `username` ni `password`
   **And** si une erreur survient, le `Authorization: Basic ...` header est masqué.

3. **Given** un module database avec connection string
   **When** il s'exécute et logue sa config
   **Then** le password dans la connection string est masqué (via `database.SanitizeConnectionString`, déjà présent — à vérifier appliqué partout)
   **And** le même helper est utilisé dans tous les logs DB.

4. **Given** le fichier `output/http_request.go:285-292` qui log `logger.Debug("http request output module created", slog.String("endpoint", cfg.Endpoint), ...)`
   **When** l'endpoint contient un token dans l'URL (ex: `https://api.com?api_key=SECRET`)
   **Then** l'URL loggée est sanitisée (query params masqués)
   **And** `sanitizeURL` actuellement dans `filter/http_call.go:129` est promu en helper partagé `httpclient.SanitizeURL` ou `logger.SanitizeURL`.

5. **Given** un test qui fournit des configs avec faux secrets
   **When** je capture les logs via `slog.NewTextHandler` avec un buffer
   **Then** aucun des secrets n'apparaît dans le buffer final
   **And** le test échoue si un pattern suspect est détecté.

## Tasks / Subtasks

- [ ] Task 1 : Audit complet des logs (AC #1, #2, #3, #4)
  - [ ] Grep `slog.String|logger.Debug|logger.Info|logger.Error` dans tout le code
  - [ ] Pour chaque site, vérifier qu'aucun argument ne provient d'un champ de credentials ou d'une URL brute
  - [ ] Lister les sites problématiques dans un doc temporaire (peut être annexé à cette story)

- [ ] Task 2 : Extraire `SanitizeURL` en helper partagé (AC #4)
  - [ ] Déplacer `filter/http_call.go:sanitizeURL:129-139` vers un package `logger` ou `httpclient`
  - [ ] Exporter comme `SanitizeURL(url string) string`
  - [ ] Utiliser partout où une URL est loggée avec potentiellement des query params

- [ ] Task 3 : Corriger les sites problématiques (AC #1, #2, #3, #4)
  - [ ] URLs : utiliser `SanitizeURL` systematiquement
  - [ ] Configs d'auth : logger seulement `type` (bearer/basic/oauth2), jamais le contenu
  - [ ] Connection strings : utiliser `database.SanitizeConnectionString`
  - [ ] Response bodies en erreur : tronquer à 500 char max (déjà fait dans `output/http_request.go:1039-1041`, à vérifier ailleurs)
  - [ ] Ne jamais logger `req.Header["Authorization"]` ou `req.Header["X-API-Key"]`

- [ ] Task 4 : Ajouter le test automatisé (AC #5)
  - [ ] `internal/logger/secrets_test.go` : capture la sortie du logger pendant un pipeline avec faux secrets
  - [ ] Regex scan pour patterns `api_key=`, `access_token=`, `password=`, `Bearer [A-Za-z0-9\._-]+`, etc.
  - [ ] Échec si match

## Dev Notes

### Rationale

Plan.md §7.1 mentionne ce risque. L'audit §3 sécurité confirme : OAuth2 `tokenUrl` parfois loggé (cf. `auth/oauth2.go`). Fuites de secrets = incident de sécurité.

### Design Decisions

- **Masquage plutôt que blocage** : logger `[REDACTED]` permet de garder la forme du message pour debug.
- **Test automatisé** : garde-fou contre la régression.
- **Pas de liste "deny" exhaustive** : préférer une liste "allow" (champs safe à logger).

### Out of Scope

- Chiffrement des logs → responsabilité infra
- Rotation/rétention des logs → responsabilité infra

## References

- docs/AUDIT_TECHNIQUE_2026-04-21.md §7.1 (prérequis implicite)
- docs/plan.md §7.1
- `database/database.go:SanitizeConnectionString` (déjà fait)
- `filter/http_call.go:129-139` (sanitizeURL à promouvoir)

## File List

- `internal/logger/secrets_test.go` (new) — automated regression test scanning logs for `api_key=`, `access_token=`, `token=`, `password=`, `Bearer …`, `Basic …`.
- `internal/modules/output/http_request.go`, `http_request_send.go`, `http_request_url.go` — every `slog.String("endpoint", …)` / `slog.String("url", …)` wrapped with `httpclient.SanitizeURL(...)`.
- `internal/modules/output/stub.go`, `internal/modules/input/stub.go` — endpoint logs sanitized for consistency.
- `internal/modules/input/http_polling.go`, `http_polling_request.go` — endpoint logs sanitized.
- `internal/modules/filter/http_call.go` — endpoint logs sanitized.
- `internal/auth/oauth2.go` — `token_url` logged via `httpclient.SanitizeURL`.
- `httpclient.SanitizeURL` was already promoted in epic 17; no relocation required.
