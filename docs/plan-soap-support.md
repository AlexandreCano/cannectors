# Plan d'implémentation — Support SOAP (input, filter, output)

**Statut** : Plan validé, prêt pour exécution
**Date** : 2026-05-12
**Périmètre** : Ajout du support SOAP aux modules HTTP (`input`, `filter`, `output`)

---

## 1. Contexte et scope

Trois nouveaux modules SOAP en parallèle des modules HTTP existants, réutilisant l'infrastructure interne (HTTP client, auth, registry, cache, pagination) :

- `soapPolling` (input) — calque de `httpPolling`
- `soap_call` (filter) — calque de `http_call`
- `soapRequest` (output) — calque de `httpRequest`

**Contraintes fonctionnelles fermes :**

- **MTOM bidirectionnel** (émission ET réception) — encoding XOP en envoi, parsing multipart/related en réception
- **WS-Security UsernameToken** uniquement (PasswordText et PasswordDigest). Pas de X.509, pas de signing/chiffrement.
- **SOAP 1.1 et SOAP 1.2** — les deux versions supportées nativement
- **Pas de gestion WSDL** — ni parsing, ni codegen, ni validation. L'utilisateur fournit le body XML directement.
- **Parser de réponse XML** : `github.com/clbanning/mxj` (XML → `map[string]any` natif)
- **Injection XML prévenue** : substitution des `{{record.x}}` dans le body wrappée par `xml.EscapeText`

---

## 2. Librairie SOAP : `github.com/hooklift/gowsdl/soap`

On utilise le **sous-package `soap`** de `hooklift/gowsdl` (sans le générateur de code) comme client SOAP générique pour XML raw.

**Features réutilisées en natif :**

| Feature | Mécanisme |
|---|---|
| MTOM émission | `soap.WithMTOM()` + type `Binary` (XOP encoding) |
| MIME multipart | `soap.WithMIMEMultipartAttachments()`, `AddMIMEMultipartAttachment` |
| WS-Security UsernameToken | Types `WSSSecurityHeader`, `WSSUsernameToken` |
| Injection du HTTP client | `soap.WithHTTPClient(c HTTPClient)` → branche notre `internal/httpclient.Client` |
| SOAP Fault parsing | Type `SOAPFault`, interface `FaultError` |
| TLS | `soap.WithTLS(*tls.Config)` |
| Envoi XML raw | `Call(action, request, response)` accepte `interface{}` |

**Adoption** : 349 importeurs sur pkg.go.dev (Google Ads API, FedEx, Salesforce, SAP, ONVIF, Terraform providers, services financiers). Code éprouvé sur ~10 ans de production contre des services SOAP très variés.

**Licence** : MPL-2.0 (compatible, vendoring autorisé).

**Limitation à patcher** : la lib cible SOAP 1.1. Le support SOAP 1.2 demande un patch (voir §10).

---

## 3. Gestion des WSDL : aucune

L'utilisateur fournit directement le XML du `<Body>` SOAP dans son YAML (templating `{{record.x}}` standard). Cannectors construit l'enveloppe, ajoute les headers WS-Security/MTOM si demandés, et parse la réponse XML.

Le WSDL reste un artefact de design-time, consulté hors-bande par l'utilisateur (SoapUI, Postman, doc vendeur).

---

## 4. Architecture cible

```
┌─────────────────────────────────────────────────┐
│  YAML pipeline (utilisateur)                    │
│  - type: soap_polling | soap_call | soap_request │
│  - body: <XML raw avec templating>              │
│  - soapVersion: "1.1" | "1.2"                   │
│  - mtom.enabled / attachments                   │
│  - authentication (WS-Security ou Basic HTTP)   │
└─────────────────┬───────────────────────────────┘
                  │
                  v
┌─────────────────────────────────────────────────┐
│  Modules SOAP (internal/modules/{input,filter,  │
│  output}/soap_*.go)                             │
│  Calque des modules HTTP, mêmes patterns :      │
│  cache, pagination, retry, merge strategies     │
└─────────────────┬───────────────────────────────┘
                  │
                  v
┌─────────────────────────────────────────────────┐
│  internal/soapclient/                           │
│  Interface SOAPClient (notre abstraction)       │
│  + envelope, mtom, security, fault, response    │
└─────────────────┬───────────────────────────────┘
                  │
                  v
┌─────────────────────────────────────────────────┐
│  third_party/hooklift_soap/                     │
│  hooklift/gowsdl/soap vendoré + patch SOAP 1.2  │
│  configuré avec WithHTTPClient(notre client)    │
└─────────────────┬───────────────────────────────┘
                  │
                  v
┌─────────────────────────────────────────────────┐
│  internal/httpclient.Client (existant)          │
│  retry, timeouts, pooling                       │
└─────────────────────────────────────────────────┘
```

**Réutilisations clés :**

| Existant | Réutilisé par |
|---|---|
| `internal/httpclient.Client` | Injecté dans `soap.NewClient` via `WithHTTPClient` |
| `internal/httpclient.DoWithRetry` | Hérité automatiquement via le client injecté |
| `internal/auth` (Handler, AuthConfig) | Credentials WS-Security UsernameToken + auth HTTP transport |
| Cache LRU de `http_call` | Réutilisé dans `soap_call` |
| Pagination de `http_polling` | Réutilisée dans `soapPolling` (cursor injecté dans le body via templating) |
| Merge strategies de `http_call` | Réutilisées dans `soap_call` |
| `PreviewableModule` | Implémentée par `soapRequest` |
| `StatePersistentInput` | Implémentée par `soapPolling` |
| Pattern registry/factory | Enregistrement des 3 nouveaux types |

---

## 5. Phasage

### Phase 1 — Socle `internal/soapclient/` + vendoring lib SOAP

**Effort** : 4-6 jours

**Fichiers à créer :**

| Fichier | Rôle |
|---|---|
| `third_party/hooklift_soap/` | Vendoring de `hooklift/gowsdl/soap` |
| `third_party/hooklift_soap/README.md` | Provenance, version vendorée, diff appliqué, raison |
| `third_party/hooklift_soap/patch-soap12.diff` | Patch ajoutant `WithSOAPVersion(SOAP11\|SOAP12)` au builder (content-type, namespace, action, fault 1.2) |
| `internal/soapclient/version.go` | Constantes `SOAPVersion11`, `SOAPVersion12` + helpers |
| `internal/soapclient/client.go` | Interface `SOAPClient`, struct `Client`, méthode `Call(ctx, op SOAPOperation) (SOAPResponse, error)` wrap de la lib vendorée avec `WithHTTPClient(httpclient.Client)` |
| `internal/soapclient/envelope.go` | Construction de l'enveloppe paramétrée par version. Substitution des `{{record.x}}` dans le body via `xml.EscapeText` (sécurité injection) |
| `internal/soapclient/mtom.go` | Wiring `WithMTOM()` côté émission + parsing multipart/related entrant pour MTOM réception (utilise `mime/multipart` stdlib pour la réception) |
| `internal/soapclient/security.go` | Construction de `WSSSecurityHeader` + `WSSUsernameToken` (PasswordText et PasswordDigest). Credentials résolues via `internal/auth`. |
| `internal/soapclient/fault.go` | Parsing structuré `SOAPFault` 1.1 (`<faultcode>` non-namespaced) et 1.2 (`<env:Code>`, `<env:Reason>`, etc. namespaced) → erreur typée `*SOAPFaultError` exploitable pour retry hints |
| `internal/soapclient/response.go` | Décodage XML → `map[string]any` via `github.com/clbanning/mxj` |

**Tests Phase 1 :**

- `client_test.go` — `httptest.Server` simulant un endpoint SOAP, version 1.1 ET 1.2
- `mtom_test.go` — émission : vérifie content-type `multipart/related; type="application/xop+xml"`, parts MIME bien formées. Réception : parsing d'un multipart/related entrant, reconstruction des binaires depuis les parts XOP
- `security_test.go` — vérifie header `wsse:Security` + `wsse:UsernameToken` (PasswordText et PasswordDigest, nonce + created)
- `fault_test.go` — fault 1.1 et fault 1.2 parsés correctement
- `response_test.go` — round-trip XML → `map[string]any` via mxj, namespaces préservés
- `envelope_test.go` — vérifie échappement XML : un record contenant `</Body>` ne casse pas l'enveloppe

**Critères de complétion :**

- `go test ./internal/soapclient/... ./third_party/hooklift_soap/...` passe à 100%
- `golangci-lint run ./...` passe sans nouvelle issue
- Démo manuelle d'un appel SOAP+MTOM bidirectionnel contre un endpoint mock WireMock, en 1.1 et 1.2

### Phase 2 — Config commune et schémas JSON

**Effort** : 1,5 jour

**Modifications :**

`internal/moduleconfig/shared.go` — ajout de `SOAPRequestBase` :

```go
type SOAPRequestBase struct {
    Endpoint       string                  `json:"endpoint"`
    SOAPVersion    string                  `json:"soapVersion,omitempty"` // "1.1" (défaut) | "1.2"
    SOAPAction     string                  `json:"soapAction,omitempty"`
    Operation      string                  `json:"operation"`
    Body           string                  `json:"body"`                  // XML raw, templatisable
    Headers        []SOAPHeaderTemplate    `json:"headers,omitempty"`
    Authentication AuthConfig              `json:"authentication,omitempty"`
    MTOM           MTOMConfig              `json:"mtom,omitempty"`
    HTTPHeaders    map[string]string       `json:"httpHeaders,omitempty"`
    TimeoutMs      int                     `json:"timeoutMs,omitempty"`
}

type MTOMConfig struct {
    Enabled     bool                  `json:"enabled"`
    Attachments []AttachmentTemplate  `json:"attachments,omitempty"` // émission
}

type AttachmentTemplate struct {
    ContentID   string `json:"contentId"`   // ex: "{{record.id}}-attachment"
    ContentType string `json:"contentType"` // ex: "application/pdf"
    SourceField string `json:"sourceField"` // chemin dans le record vers les bytes
}
```

`internal/config/schema/common-schema.json` — nouveau bloc `soapRequestBase` avec :

- `soapVersion` : `"enum": ["1.1", "1.2"]`, défaut `"1.1"`
- Réutilisation des blocs `authentication`, `retry`, `cacheConfig` existants
- Validation cross-field : si `soapVersion: "1.2"`, refuser un `httpHeaders.SOAPAction` (1.2 le porte dans le content-type)

`internal/config/schema/{input,filter,output}-schema.json` — ajout des 3 types SOAP dans les `oneOf`.

**Tests Phase 2 :**

- Tests de contrat (`*_contract_test.go`) sur les exemples YAML
- Tests unitaires struct ↔ JSON

### Phase 3 — Modules SOAP

**Effort** : 4-5 jours

#### 3.1 — `internal/modules/input/soap_polling.go`

Calque de `http_polling.go`. Spécificités :

- Polling périodique d'une opération SOAP
- Pagination cursor/page/offset injectée dans le `body` XML via templating depuis l'état
- Extraction des records via path-like sur la map mxj (ex: `dataField: "soap:Body.GetOrdersResponse.Orders.Order"`)
- État persistant via `StatePersistentInput`

#### 3.2 — `internal/modules/filter/soap_call.go`

Calque de `http_call.go`. Spécificités :

- Appel SOAP synchrone par record pour enrichissement
- Cache LRU réutilisé
- Merge strategies réutilisées (`merge` / `replace` / `append`)
- `resultKey` configurable (ne pas hardcoder)

#### 3.3 — `internal/modules/output/soap_request.go`

Calque de `http_request.go`. Spécificités :

- Mode `single` (un appel par record) ou `batch` (un appel pour N records)
- Retry hérité via le HTTP client injecté
- Implémente `PreviewableModule` pour le dry-run
- Support MTOM émission : YAML déclare quels champs du record sont des binaires à attacher

**Enregistrement** dans `internal/registry/registry.go` :

```go
RegisterInput("soap_polling", NewSOAPPollingInput)
RegisterFilter("soap_call",   NewSOAPCallFilter)
RegisterOutput("soap_request", NewSOAPRequestOutput)
```

**Tests Phase 3 :**

- `soap_polling_test.go`, `soap_call_test.go`, `soap_request_test.go` avec `httptest.Server`
- Cache, merge strategies, pagination — variantes XML
- Erreurs : SOAP Fault (1.1 et 1.2), timeout, retry, retry hint depuis le fault

### Phase 4 — Exemples YAML et tests d'intégration

**Effort** : 3 jours

**Nouveaux exemples (`examples/`) :**

| Fichier | Démontre |
|---|---|
| `40-soap-polling-basic-v11.yaml` | Polling SOAP 1.1 simple |
| `40b-soap-polling-basic-v12.yaml` | Polling SOAP 1.2 simple |
| `41-soap-polling-cursor.yaml` | Polling avec pagination cursor |
| `42-soap-call-enrichment.yaml` | Filter SOAP avec cache et merge |
| `43-soap-output-batch.yaml` | Output SOAP en mode batch |
| `44-soap-output-mtom-emission.yaml` | Output avec attachement binaire MTOM (envoi PDF/image) |
| `44b-soap-input-mtom-reception.yaml` | Input recevant un MTOM (extraction binaire d'une part multipart) |
| `45-soap-output-wssecurity-passwordtext.yaml` | WS-Security UsernameToken PasswordText |
| `45b-soap-output-wssecurity-passworddigest.yaml` | WS-Security UsernameToken PasswordDigest |

**Tests test lab :**

- Round-trip MTOM bidirectionnel : envoi avec attachement → réception côté mock → mock répond avec attachement → input parse et extrait le binaire
- WS-Security : header `wsse:UsernameToken` présent et bien formé (PasswordText et PasswordDigest)
- SOAP Fault 1.1 et 1.2 : fault provoqué par le mock, erreur typée remontée
- Retry sur fault : retry hint déclenché par un détail spécifique du fault

⚠️ **Piège WireMock** : journal antéchronologique, `requests[0]` = plus récent.

### Phase 5 — Documentation

**Effort** : 1,5 jour

**Documents :**

- `docs/MODULES.md` — section SOAP avec les 3 modules
- `docs/SOAP.md` (nouveau) — guide utilisateur :
  - Vue d'ensemble (XML raw, pas de WSDL runtime)
  - Versions SOAP 1.1 et 1.2 (différences pratiques)
  - Templating XML et échappement automatique
  - MTOM émission et réception
  - WS-Security UsernameToken (PasswordText/PasswordDigest)
  - Gestion des namespaces dans les réponses
  - Parsing avec mxj (chemins `soap:Body.X.Y`)
  - SOAP Faults et retry hints
- `examples/README.md` (si présent) — référencer les nouveaux exemples
- Mise à jour de `_bmad-output/project-context.md` si patterns nouveaux

---

## 6. Estimation globale

| Phase | Effort |
|---|---|
| 1 — Socle `internal/soapclient/` + vendoring + patch 1.2 | 4-6 jours |
| 2 — Config + schémas JSON | 1,5 jour |
| 3 — 3 modules SOAP | 4-5 jours |
| 4 — Exemples + tests lab | 3 jours |
| 5 — Documentation | 1,5 jour |
| **Total** | **14-17 jours** |

---

## 7. Points d'attention

### 7.1 Validation systématique (rappel CLAUDE.md)

Pour chaque phase, avant de marquer la tâche complète :

```bash
go test ./...
golangci-lint run ./...
```

### 7.2 Sécurité — injection XML

Le templating `{{record.x}}` substitue des valeurs texte. Toute substitution dans le body SOAP est **systématiquement wrappée par `xml.EscapeText`** au moment de la construction. Un record contenant `</Body>` ou tout caractère XML spécial est échappé, l'enveloppe reste intègre. Test dédié dans `envelope_test.go`.

### 7.3 MTOM bidirectionnel

- **Émission** : `soap.WithMTOM()` natif de la lib hooklift, type `Binary` avec XOP encoding
- **Réception** : `internal/soapclient/mtom.go` implémente le parsing du `multipart/related` entrant via `mime/multipart` stdlib, matching des `Content-ID` avec les références `xop:Include` dans le body XML, reconstruction des binaires en `[]byte`

### 7.4 Pièges remontés (CLAUDE.md)

- **WireMock journal antéchronologique** — `requests[0]` est le plus récent (tests Phase 4)
- **Processus `cannectors` orphelins** — `pgrep -fa cannectors` avant chaque relance test lab
- **`wait` sans argument** — attendre les enfants par PID explicite
- **Contexte HTTP annulé après réponse** — utiliser le contexte du serveur pour webhook, pas `r.Context()`
- **Pointeurs sur slice + append** — pré-dimensionner les slices, assigner par index

---

## 8. Critères de succès

- [ ] 3 modules SOAP (`soap_polling`, `soap_call`, `soap_request`) enregistrés et fonctionnels
- [ ] Au moins un exemple YAML par module passe `cannectors validate --verbose`
- [ ] Exemple MTOM bidirectionnel passe en test lab e2e
- [ ] Exemple WS-Security UsernameToken (PasswordText et PasswordDigest) passe en test lab e2e
- [ ] Exemples SOAP 1.1 et SOAP 1.2 passent tous les deux
- [ ] SOAP Fault 1.1 et 1.2 parsés en erreur typée
- [ ] `go test ./...` passe à 100%
- [ ] `golangci-lint run ./...` passe sans nouvelle issue
- [ ] `docs/SOAP.md` rédigé et référencé depuis `docs/MODULES.md`
- [ ] `internal/soapclient/` reste sous 1 500 LOC (hors tests)

---

## 9. Spécifications techniques SOAP 1.1 / 1.2

### 9.1 Différences à gérer

| Aspect | SOAP 1.1 | SOAP 1.2 |
|---|---|---|
| Content-Type HTTP | `text/xml; charset=utf-8` | `application/soap+xml; charset=utf-8; action="..."` |
| `SOAPAction` | Header HTTP séparé (`SOAPAction: "..."`) | Paramètre `action` du content-type |
| Namespace enveloppe | `http://schemas.xmlsoap.org/soap/envelope/` | `http://www.w3.org/2003/05/soap-envelope` |
| Fault | `<faultcode>` / `<faultstring>` / `<detail>` non-namespaced | `<env:Code><env:Value>` / `<env:Reason><env:Text>` / `<env:Detail>` namespaced |
| Fault codes standards | `Client`, `Server`, `VersionMismatch`, `MustUnderstand` | `env:Sender`, `env:Receiver`, `env:VersionMismatch`, `env:MustUnderstand`, `env:DataEncodingUnknown` |
| MTOM / XOP | Compatible | Compatible (namespace XOP identique) |
| WS-Security | Identique | Identique |

### 9.2 Patch SOAP 1.2 appliqué à la lib vendorée

Localisation : `third_party/hooklift_soap/patch-soap12.diff`

Le patch ajoute :

1. Option `WithSOAPVersion(SOAP11 | SOAP12)` au builder
2. Sélection du namespace de l'enveloppe selon la version
3. Sélection du content-type émis (`text/xml` vs `application/soap+xml`)
4. Sélection de l'emplacement du `SOAPAction` (header séparé vs paramètre content-type)
5. Type `SOAPFault12` parsé pour les réponses 1.2
6. Tests unitaires version-aware ajoutés

Volume estimé : ~150 LOC de patch + ~100 LOC de tests.

### 9.3 Tests version-aware

Chaque scénario testé dans les deux versions :

- Envelope construction (namespace, attributs)
- Fault parsing (`<faultcode>` 1.1 ET `<env:Code><env:Value>` 1.2)
- Content-Type émis (`text/xml` 1.1 ET `application/soap+xml; action="..."` 1.2)
- `SOAPAction` (header 1.1 ET paramètre content-type 1.2)
- MTOM émission et réception en 1.1 et en 1.2
- WS-Security UsernameToken en 1.1 et en 1.2

---

## Annexes

### A. Références librairies

- [hooklift/gowsdl/soap (349 importeurs, MPL-2.0)](https://pkg.go.dev/github.com/hooklift/gowsdl/soap)
- [clbanning/mxj — XML → map[string]any (parser de réponse)](https://pkg.go.dev/github.com/clbanning/mxj)

### B. Fichiers du codebase à étudier avant implémentation

- `internal/modules/input/http_polling.go` — calque pour `soap_polling.go`
- `internal/modules/filter/http_call.go` — calque pour `soap_call.go`
- `internal/modules/output/http_request.go` — calque pour `soap_request.go`
- `internal/httpclient/client.go` + `doretry.go` — client à injecter dans `soap.NewClient`
- `internal/auth/auth.go` — handlers réutilisables
- `internal/moduleconfig/shared.go` — `HTTPRequestBase` à miroir
- `internal/config/schema/common-schema.json` — schémas à étendre
- `internal/registry/registry.go` + `internal/factory/modules.go` — enregistrement
