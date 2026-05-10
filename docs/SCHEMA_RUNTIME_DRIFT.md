# Schema / Runtime Drift

Ce document capture les ecarts identifies entre les schemas JSON sous
`internal/config/schema` et le comportement reel du code, puis formalise les
decisions produit validees pour les corriger.

Il sert de source de reference pour l'Epic 24:

- `_bmad-output/implementation-artifacts/epic-24-schema-runtime-contract-alignment.md`
- `_bmad-output/implementation-artifacts/24-*.md`

## Perimetre verifie

- Schemas: `internal/config/schema/*.json`
- Conversion: `internal/config/converter.go`
- Types publics: `pkg/connector/types.go`
- Registry/factory: `internal/registry`, `internal/factory`
- Configs partagees: `internal/moduleconfig`
- Modules runtime: `internal/modules/input`, `internal/modules/filter`, `internal/modules/output`
- Auth, database, scheduler, HTTP client, CLI lorsque la config est consommee hors module

## Regles globales validees

- Toute execution utilisateur de pipeline doit passer par la validation JSON
  Schema avant conversion et runtime.
- Le schema est le contrat produit utilisateur.
- Le runtime Go conserve des validations defensives si un constructeur est
  appele directement par du code interne ou des tests.
- Les champs inconnus doivent etre rejetes par le schema utilisateur. Le code Go
  peut continuer a les ignorer lorsqu'il est appele directement, car ce n'est
  pas le chemin produit.
- Le projet n'etant pas release, il ne faut pas ajouter de shims, alias
  deprecated ou compatibilite avec les anciens champs.
- Les contraintes statiques et produit vont dans le schema: enum, minimum,
  oneOf, champs requis, forme des champs.
- Les contraintes dynamiques restent runtime: existence de fichier, syntaxe de
  template, regex Go, JS Goja, expression `expr`, SQL, URL finale, headers HTTP
  apres resolution.
- `enabled` et `onError` doivent etre applicables sur tous les modules via
  `connector.ModuleBase`.
- `onError` accepte un casing flexible en entree, puis est normalise vers
  `fail`, `skip` ou `log`. Toute valeur inconnue doit produire une erreur
  claire.

## Pipeline racine

### Ecarts constates

- `id` est lu par le convertisseur mais rejete par le schema racine.
- `tags` est accepte par le schema mais ignore par le runtime.
- `filters` est requis par le schema, mais optionnel cote convertisseur.
- `enabled` existe dans `connector.Pipeline` et est teste par CLI/runtime, mais
  n'est pas accepte/lu correctement partout.
- `dryRunOptions.showCredentials` est utilise par le dry-run, mais n'est pas
  accepte/converti correctement.
- `version` est trop strict dans le schema avec un pattern semver `x.y.z`.

### Decisions

- `pipeline.id` doit etre accepte par le schema et mappe vers `Pipeline.ID`.
- `pipeline.tags` reste accepte pour l'experience utilisateur. Le runtime peut
  l'ignorer tant qu'il n'a pas d'usage produit.
- `pipeline.filters` devient optionnel. `filters: []` reste valide.
- `pipeline.enabled` doit etre accepte, mappe vers `Pipeline.Enabled`, et avoir
  `true` comme default produit si omis.
- `pipeline.dryRunOptions.showCredentials` doit etre accepte et mappe vers
  `Pipeline.DryRunOptions`.
- `pipeline.name` reste requis, non vide, max 128.
- `pipeline.version` devient une string optionnelle non vide, sans pattern
  semver strict.
- `pipeline.description` garde `maxLength: 1024` cote schema.

## ModuleBase

### Ecarts constates

- `moduleBase` est permis par le schema pour tous les modules, mais certains
  structs ne l'embarquent pas ou n'appliquent pas `enabled`/`onError`.
- `onError` fallback silencieusement vers `fail` dans plusieurs chemins runtime.

### Decisions

- Integrer `connector.ModuleBase` dans tous les structs de modules.
- `enabled` et `onError` doivent etre applicables partout.
- `id`, `name`, `description`, `enabled`, `tags` restent des metadata module
  acceptees par le schema.
- `tags` peut rester ignore par le runtime si non utile.
- `onError` doit etre strict:
  - valeurs valides: `fail`, `skip`, `log`
  - casing flexible a l'entree
  - normalisation en lowercase
  - valeur inconnue = erreur claire

## Validation schema obligatoire

### Ecarts constates

- Plusieurs chemins runtime peuvent instancier/executer une config qui n'a pas
  ete validee par les schemas.
- Cela permet au code Go d'ignorer des champs inconnus ou d'appliquer des
  fallbacks qui contredisent le contrat schema.

### Decisions

- Toute execution de pipeline via le chemin utilisateur doit valider le schema
  avant conversion et execution.
- Si la validation schema echoue, le pipeline ne demarre pas.
- Les commandes CLI de run, dry-run, validation, execution planifiee et
  execution callback doivent respecter cette regle.
- Les constructeurs Go gardent des validations defensives pour les appels
  directs internes.

## Retry

### Ecarts constates

- `retry` est supporte par `httpPolling`, mais absent du schema.
- `retry` est supporte par `http_call`, mais absent du schema.
- Les bornes du schema ne sont pas toujours validees runtime.
- Les defaults retry peuvent etre injectes dans des modules qui ne les lisent
  pas.

### Decisions

- Ajouter `retry` au schema `httpPolling`.
- Ajouter `retry` au schema `http_call`.
- `retry` reste supporte sur `httpRequest`.
- Les defaults `retry` ne s'appliquent qu'aux modules HTTP retry-aware:
  - `httpPolling`
  - `http_call`
  - `httpRequest`
- Pas de retry DB pour l'instant.
- Garder les bornes produit:
  - `maxAttempts <= 10`
  - `backoffMultiplier <= 10`
  - status codes entre `100` et `599`
- Appliquer une validation runtime defensive apres resolution des defaults.

## HTTP commun

### Ecarts constates

- Les methodes HTTP sont incoherentes selon les modules:
  - `httpPolling` force `GET`
  - `http_call` accepte seulement `GET|POST|PUT`
  - `httpRequest` accepte seulement `POST|PUT|PATCH`
- Le schema commun limite les methodes via enum.
- `endpoint` a `format: uri`, incompatible avec les endpoints templatises.
- Les headers invalides sont parfois ignores silencieusement.
- Le body est limite a certains modules ou certaines methodes.

### Decisions

- Tous les modules HTTP sortants acceptent toute methode HTTP valide.
- Sont concernes:
  - `httpPolling`
  - `http_call`
  - `httpRequest`
- `DELETE` doit etre possible partout, y compris sur `httpPolling`.
- Un body doit pouvoir etre configure sur tous les modules HTTP.
- Un body doit pouvoir etre envoye quelle que soit la methode.
- Retirer `format: uri` du schema `endpoint`.
- `endpoint` devient une string non vide, compatible avec templates.
- L'URL finale doit etre validee au runtime apres resolution.
- Les headers restent `map[string]string` dans le schema.
- Les headers finaux doivent etre valides au runtime apres resolution de
  template. Un header invalide doit produire une erreur, pas etre ignore.
- `timeoutMs` est optionnel; si fourni, `minimum: 1`.
- Si `timeoutMs` est omis, le runtime recoit `0` et applique son default.

## SQL commun

### Ecarts constates

- Le schema impose `connectionString` xor `connectionStringRef`, mais le code
  accepte parfois les deux.
- Le schema impose `query` xor `queryFile`, mais le code accepte parfois les
  deux et garde `query`.
- `connectionStringRef` est plus strict cote schema que cote runtime.
- Plusieurs champs numeriques acceptent ou interpretent `0` differemment.

### Decisions

- Garder `oneOf` pour `connectionString` / `connectionStringRef`.
- Runtime strict: fournir les deux est une erreur.
- Garder `oneOf` pour `query` / `queryFile`.
- Runtime strict: fournir les deux est une erreur.
- `connectionStringRef` doit respecter `${ENV_VAR_NAME}` avec nom d'env en
  uppercase/underscore selon le pattern schema.
- `query` et `queryFile` doivent etre des strings non vides.
- Les champs suivants sont optionnels, mais si fournis doivent avoir
  `minimum: 1`:
  - `maxOpenConns`
  - `maxIdleConns`
  - `connMaxLifetimeSeconds`
  - `connMaxIdleTimeSeconds`
  - `timeoutMs`
- Si ces champs sont omis, le runtime recoit `0` et applique ses defaults.
- Les validations de fichier, SQL et templates restent runtime.

## Auth

### Ecarts constates

- Le schema auth est strict sur les champs inconnus, mais Go les ignore si decode
  directement.
- `api-key.location` inconnu est ignore runtime.
- OAuth2 expose `scope` string en schema, mais `CredentialsOAuth2` expose
  `Scopes []string`.

### Decisions

- Garder le schema auth strict avec `additionalProperties: false`.
- Les champs inconnus sont rejetes dans le chemin utilisateur par validation
  schema.
- Le code Go peut continuer a ignorer les champs inconnus si appele
  directement.
- `api-key.location`:
  - absent = default `header`
  - `header` valide
  - `query` valide
  - valeur inconnue = erreur runtime claire
- Remplacer `connector.CredentialsOAuth2.Scopes []string` par `Scope string`.
- Le handler OAuth2 split localement `Scope` avec `strings.Fields`.
- Ne pas ajouter `scopes` au schema.

## Input: httpPolling

### Ecarts constates

- `schedule` est requis par le schema, mais la CLI peut executer un pipeline
  sans schedule en one-shot.
- `schedule` n'est pas dans `HTTPPollingInputConfig`.
- `retry` est supporte runtime mais absent du schema.
- La pagination exige des champs que le runtime n'utilise pas toujours.
- Type de pagination inconnu tombe sur fetch simple cote code.
- `method` est ignore et `GET` est force.

### Decisions

- `schedule` est optionnel.
- Absence de `schedule` = execution one-shot.
- Presence de `schedule` = execution planifiee.
- Ajouter `Schedule string` a `HTTPPollingInputConfig` si utile au parsing typed.
- Ajouter `retry` au schema et l'appliquer runtime.
- `httpPolling` accepte toute methode HTTP valide.
- `httpPolling` peut envoyer un body.
- Pagination HTTP:
  - `limit` optionnel
  - si `limit` est fourni, `minimum: 1`
  - `limit: 0` est invalide
  - omission = default runtime
  - remplacer `pageParam`, `cursorParam`, `offsetParam` par un champ canonique
    `param`
  - garder `limitParam` separe
  - pas de compatibilite avec les anciens champs
  - type inconnu = erreur schema et runtime
- Exemples cibles:

```yaml
pagination:
  type: page
  param: page
  totalPagesField: meta.totalPages
  limit: 100
  limitParam: limit
```

```yaml
pagination:
  type: cursor
  param: cursor
  nextCursorField: meta.nextCursor
```

## Input: webhook

### Ecarts constates

- Le code supporte `timeoutMs`, absent du schema.
- Ce timeout controle en realite `ReadTimeout` et `WriteTimeout` par requete,
  pas la duree de vie du serveur.
- `signature.header` est requis par le schema mais optionnel cote code.
- `queueSize` / `maxConcurrent` acceptent `0` runtime comme default.
- `rateLimit` n'est pas aligne entre schema et code.
- `ModuleBase` n'est pas applique partout.

### Decisions

- Renommer le champ config webhook en `requestTimeoutMs`.
- Ne pas exposer `timeoutMs` sur webhook.
- `requestTimeoutMs` controle le timeout par requete HTTP entrante.
- Le webhook continue d'ecouter indefiniment tant que le pipeline tourne.
- Si `requestTimeoutMs` est omis, le runtime applique le default actuel.
- `signature.header` est optionnel.
- Default `signature.header`: `X-Hub-Signature-256`.
- `signature.secret` reste obligatoire quand `signature` est present.
- `queueSize` et `maxConcurrent`:
  - optionnels
  - `minimum: 1` si fournis
  - omission = default runtime
  - `0` explicite invalide via schema
- `rateLimit`:
  - optionnel
  - si present, `requestsPerSecond` est obligatoire, integer, `minimum: 1`
  - `burst` est optionnel, integer, `minimum: 1` si fourni
  - si `burst` est omis, runtime met `burst = requestsPerSecond`
  - si `rateLimit` est absent, pas de rate limit
- Integrer `ModuleBase` dans `WebhookInputConfig`.

## Input: database

### Ecarts constates

- `schedule` est supporte de fait par le scheduler mais absent/incoherent dans
  le schema database input.
- La pagination database exige des champs differents du modele HTTP.
- Type de pagination inconnu peut tomber sur fetch simple.
- Herite des ecarts SQL communs.

### Decisions

- `schedule` est optionnel.
- Absence de `schedule` = execution one-shot.
- Presence de `schedule` = execution planifiee.
- Appliquer toutes les decisions SQL communes.
- Pagination database:
  - utiliser `param` comme champ canonique
  - remplacer `cursorParam` et `offsetParam` par `param`
  - pas de compatibilite avec les anciens champs
  - `limit` optionnel
  - si `limit` est fourni, `minimum: 1`
  - omission = default runtime
  - type inconnu = erreur schema et runtime
- Exemples cibles:

```yaml
pagination:
  type: cursor
  param: last_id
  cursorField: id
  limit: 100
```

```yaml
pagination:
  type: offset
  param: offset
  limit: 100
```

## Filter: mapping

### Ecarts constates

- `target` n'a pas `minLength` cote schema.
- `onMissing` inconnu n'est pas strict runtime.
- `transform.op` n'est pas un enum schema et devient no-op si inconnu runtime.
- `replace` sans pattern ne remplace rien silencieusement.

### Decisions

- `mapping.rules[].target` ou `mappings[].target` doit etre une string non
  vide.
- Garder les valeurs `onMissing` existantes:
  - `setNull`
  - `skipField`
  - `useDefault`
  - `fail`
- Default `onMissing`: `setNull`.
- `onMissing` inconnu = erreur runtime claire.
- Si `onMissing: useDefault`, `defaultValue` doit etre present.
- `transforms[].op` devient un enum strict:
  - `trim`
  - `lowercase`
  - `uppercase`
  - `dateFormat`
  - `replace`
  - `split`
  - `join`
  - `toString`
  - `toInt`
  - `toFloat`
  - `toBool`
  - `toArray`
  - `toObject`
- Op inconnu = erreur, pas no-op.
- Pour `op: replace`:
  - `pattern` obligatoire
  - `pattern` string non vide
  - `replacement` optionnel, default `""`
  - regex invalide = erreur runtime claire

## Filter: condition

### Ecarts constates

- `lang` expose des langages non supportes.
- `onTrue` / `onFalse` dupliquent partiellement `then` / `else`.
- Le runtime applique des defaults silencieux pour des valeurs inconnues.
- Le code a une limite de profondeur artificielle.

### Decisions

- Supprimer `condition.lang` du schema et du code.
- Supprimer `condition.onTrue` du schema et du code.
- Supprimer `condition.onFalse` du schema et du code.
- Garder uniquement:
  - `expression`
  - `then`
  - `else`
- `expression` est obligatoire, string non vide.
- Runtime rejette `expression` vide ou whitespace-only.
- Runtime compile/valide l'expression au demarrage.
- `then` est applique quand l'expression est true.
- `else` est applique quand l'expression est false.
- Si la branche correspondante est absente, le record est garde inchange.
- Pas de limite artificielle de profondeur pour les conditions imbriquees.
- Ajouter un filter `drop`.
- Pour filtrer, l'utilisateur met explicitement `drop` dans une branche:

```yaml
filters:
  - type: condition
    expression: "status == 'active'"
    else:
      - type: drop
```

## Filter: script

### Ecarts constates

- Le schema verifie seulement la presence exclusive de `script` ou
  `scriptFile`.
- Le runtime rejette les strings vides/whitespace, les fichiers invalides, les
  scripts trop longs, le JS invalide et l'absence de `transform(record)`.

### Decisions

- Garder les deux modes exclusifs:
  - `script`
  - `scriptFile`
- Garder `oneOf`, exactement un des deux.
- Ajouter `minLength: 1` a `script`.
- Ajouter `minLength: 1` a `scriptFile`.
- Ne pas ajouter de `maxLength` dans le schema.
- Garder la limite runtime actuelle de `100KB`.
- Ne pas ajouter de champ `language`.
- Garder les validations runtime:
  - whitespace-only refuse
  - fichier invalide/refuse
  - fichier > 100KB refuse
  - JS invalide refuse
  - `transform(record)` obligatoire

## Filter: set

### Ecarts constates

- `value` doit etre requis, mais le code peut confondre champ absent et valeur
  `null` si le parsing ne passe pas par la map brute.
- `ModuleBase` n'est pas applique partout.

### Decisions

- `target` requis, string non vide.
- `value` requis.
- `value` accepte tous les types, y compris `null`.
- Runtime doit distinguer:
  - champ `value` absent = erreur
  - champ `value` present avec `null` = valide, set vrai null
- En YAML/JSON:
  - `value: null` devient une vraie valeur Go `nil`
  - `value: "null"` devient la string `"null"`
- Integrer `ModuleBase`.

## Filter: remove

### Ecarts constates

- Le schema et le code exposent `target` et `targets`, ce qui cree de
  l'ambiguite.
- Le code accepte certaines combinaisons que le schema rejette ou inversement.
- `ModuleBase` n'est pas applique partout.

### Decisions

- Supprimer `targets`.
- Garder uniquement `target`.
- `target` requis.
- `target` accepte:
  - une string non vide
  - une liste non vide de strings non vides
- Runtime normalise en interne vers `[]string`.
- Pas de compatibilite avec `targets`.
- Integrer `ModuleBase`.

## Filter: http_call

### Ecarts constates

- `retry` existe cote code mais pas schema.
- `method` est limite a `GET|POST|PUT`.
- Body seulement via `bodyTemplateFile` et seulement pour `POST|PUT`.
- `cache.enabled` est ignore, cache actif par defaut.
- `defaultTTL` est le nom actuel du TTL cache.
- `mergeStrategy` inconnue fallback vers `merge`.
- Headers invalides peuvent etre ignores.

### Decisions

- Toute methode HTTP valide est acceptee.
- Body possible sur toutes les methodes HTTP.
- Ajouter `retry` au schema et l'appliquer.
- `cache.enabled` doit etre honore.
- Cache desactive par defaut sauf `enabled: true`.
- Renommer `cache.defaultTTL` en `cache.ttlSeconds`.
- Pas de compatibilite avec `defaultTTL`.
- `cache.maxSize` optionnel, `minimum: 1` si fourni.
- `cache.ttlSeconds` optionnel, `minimum: 1` si fourni.
- `cache.key` optionnel, string non vide si fourni, template valide runtime.
- `mergeStrategy` strict:
  - `merge`
  - `replace`
  - `append`
- Garder `append`, ne pas renommer.
- Valeur inconnue = erreur.
- Endpoint, headers, body et cache key templates valides runtime.
- Header final invalide = erreur.
- Keys:
  - `field` non vide
  - `paramType` strict `query|path|header`
  - `paramName` non vide
  - champ record absent/null/vide = erreur claire quand la key est utilisee

## Filter: sql_call

### Ecarts constates

- Herite des ecarts SQL communs.
- `mergeStrategy` inconnue fallback vers merge.
- `append` utilise `_sql_result` si `resultKey` absent.
- Cache utilise `defaultTTL`.

### Decisions

- Pas de retry sur `sql_call`.
- Appliquer les decisions SQL communes.
- `mergeStrategy` strict:
  - `merge`
  - `replace`
  - `append`
- Garder le nom `append`.
- Valeur inconnue = erreur.
- Si `mergeStrategy: append`, `resultKey` est obligatoire.
- Supprimer le default implicite `_sql_result`.
- Cache aligne avec `http_call`:
  - `enabled`
  - `maxSize`
  - `ttlSeconds`
  - `key`
- `cache.enabled` honore.
- Cache desactive par defaut sauf `enabled: true`.
- Supprimer `defaultTTL`, sans compatibilite.
- `cache.maxSize` et `cache.ttlSeconds` optionnels, `minimum: 1` si fournis.
- `cache.key` valide runtime.

## Output: httpRequest

### Ecarts constates

- `method` est optionnel schema mais requis runtime.
- `method` est limite runtime a `POST|PUT|PATCH`.
- `success.lang` et `success.expression` sont acceptes par le schema mais
  ignores par le runtime.
- `success.statusCodes` est borne par le schema mais pas runtime.
- `requestMode` inconnu devient batch silencieusement.
- `keys` invalides sont ignorees silencieusement.
- Headers invalides peuvent etre ignores.

### Decisions

- `method` devient optionnel avec default produit `POST`.
- Toute methode HTTP valide est acceptee.
- Body possible sur toutes les methodes.
- Si body omis:
  - `POST`, `PUT`, `PATCH`: envoyer le record en JSON par defaut
  - `GET`, `DELETE`: pas de body par defaut
- Endpoint templating accepte par schema, URL finale validee runtime.
- `retry` accepte et applique.
- `defaults.retry` applicable a `httpRequest`.
- Headers invalides apres template = erreur.
- `success.lang` est supprime.
- `success.expression` est implemente.
- Langage implicite unique: `expr`.
- Variables disponibles dans `success.expression`:
  - `statusCode`
  - `headers`
  - `body`
- Regles de succes:
  - `success` absent = default status codes `201, 202, 203, 204`
  - expression seule = succes selon expression uniquement
  - status codes seuls = succes selon status code uniquement
  - expression + status codes = les deux doivent etre vrais
- Si l'expression utilise `body` et que le body n'est pas JSON exploitable,
  erreur claire.
- Expression invalide = erreur au chargement.
- `success.statusCodes`:
  - optionnel
  - si fourni, array non vide
  - codes entre `100` et `599`
  - doublons refuses
  - liste vide invalide, pas fallback vers default
- `requestMode`:
  - absent = `batch`
  - `batch` valide
  - `single` valide
  - autre valeur = erreur
- `keys`:
  - optionnel
  - `field` non vide
  - `paramType` strict `query|path|header`
  - `paramName` non vide
  - valeur record absente/null/vide = erreur quand utilisee
  - header final invalide = erreur

## Output: database

### Ecarts constates

- Herite des ecarts SQL communs.
- `transaction` est deja coherent.
- Query template et `queryFile` sont valides runtime.

### Decisions

- Appliquer les decisions SQL communes:
  - `connectionString` xor `connectionStringRef`
  - `query` xor `queryFile`
  - `connectionStringRef` strict
  - champs numeriques `minimum: 1` si fournis
  - `query` / `queryFile` non vides
- `transaction` reste optionnel, default `false`.
- `transaction: true` = batch en transaction.
- `transaction: false` = execution record par record.
- `queryFile` valide et lu runtime.
- Templates SQL valides runtime.
- Placeholder SQL vers champ absent continue a produire `NULL`.
- `onError` strict et normalise via `ModuleBase`.

## Documentation BMAD

Les decisions ci-dessus sont decoupees dans l'Epic 24:

- Story 24.1: validation schema obligatoire avant execution
- Story 24.2: pipeline root et ModuleBase
- Story 24.3: contrat HTTP commun
- Story 24.4: contrat SQL commun et modules database
- Story 24.5: authentification
- Story 24.6: httpPolling schedule et pagination
- Story 24.7: webhook
- Story 24.8: mapping
- Story 24.9: condition et drop
- Story 24.10: script, set, remove
- Story 24.11: http_call
- Story 24.12: httpRequest

## Verification

Ce document est decisionnel. L'implementation doit verifier chaque story avec:

- tests cibles sur les packages modifies
- `go test ./...` si un contrat partage, un type public ou un schema commun est
  modifie
- `golangci-lint run ./...`
