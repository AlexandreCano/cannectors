# Story 17.2: Fix OAuth2 `scope` vs `scopes` parsing

Status: backlog

## Story

En tant qu'utilisateur,
je veux que les scopes OAuth2 définis dans ma config soient effectivement envoyés à l'endpoint de token,
afin que mes appels API ne soient plus rejetés pour manque de permission.

## Acceptance Criteria

1. **Given** un pipeline avec `authentication.credentials.scope: "read write"`
   **When** le module polling obtient un token OAuth2
   **Then** la requête POST vers `tokenUrl` inclut `scope=read write` dans le body x-www-form-urlencoded
   **And** un test vérifie ce comportement.

2. **Given** une config legacy avec `credentials.scopes: "read,write"` (comma-separated string)
   **When** le module obtient un token
   **Then** le parsing convertit en "read write" (séparé par espaces, selon RFC 6749)
   **And** un warning log indique que `scopes` est déprécié.

3. **Given** une config avec `credentials.scopes: ["read", "write"]` (array, selon schéma)
   **When** le module obtient un token
   **Then** la requête inclut `scope=read write`.

4. **Given** aucun scope configuré
   **When** le module obtient un token
   **Then** le paramètre `scope` est omis de la requête (pas envoyé vide).

5. **Given** les tests OAuth2
   **When** `go test ./internal/auth/...`
   **Then** tous passent incluant les nouveaux cas.

## Tasks / Subtasks

- [ ] Task 1 : Parser les deux formats (AC #1, #2, #3)
  - [ ] Dans `pkg/connector/types.go:CredentialsOAuth2`, conserver `Scopes []string`
  - [ ] Dans `auth/oauth2.go:newOAuth2Handler`, accepter les deux formats d'unmarshal :
    - `scope` (string, singulier, selon schéma)
    - `scopes` (string CSV ou array, legacy)
  - [ ] Joindre avec espace (RFC 6749) pour l'envoi

- [ ] Task 2 : Adapter le schéma (AC #3)
  - [ ] Vérifier `internal/config/schema/auth-schema.json` pour `oauth2.credentials`
  - [ ] S'assurer que `scope` (string) est documenté comme champ officiel, `scopes` comme legacy

- [ ] Task 3 : Tests (AC #1, #2, #3, #4)
  - [ ] Test `scope` string → envoyé tel quel
  - [ ] Test `scopes` array → joint avec espaces
  - [ ] Test `scopes` CSV string → converti en "a b"
  - [ ] Test aucun scope → paramètre absent du body
  - [ ] Mock du server OAuth2 pour vérifier la requête

## Dev Notes

### Rationale

Bug critique P0 de `docs/plan.md` §3 "Bug 2". Le schéma utilise `scope` (singulier, conforme RFC 6749), le code lit `scopes` (pluriel). Résultat : **les scopes configurés ne sont jamais envoyés**, les tokens obtenus peuvent échouer silencieusement à l'usage.

### Design Decisions

- **Rétrocompatibilité** : accepter `scope` ET `scopes`, avec priorité à `scope`.
- **RFC 6749 compliance** : le format envoyé est toujours une string space-separated, même si l'utilisateur fournit un array.

### Out of Scope

- Support de scopes dynamiques par record → hors périmètre
- Refresh token → autre story

## References

- docs/AUDIT_TECHNIQUE_2026-04-21.md §5 P1.2
- docs/plan.md §3 "Bug 2"
- `internal/auth/oauth2.go:52` (bug site)
- `internal/config/schema/auth-schema.json`
- RFC 6749 §3.3

## File List

(à compléter)
