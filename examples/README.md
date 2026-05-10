# Maintained Examples

`examples/` is the canonical examples directory. These files are validated by
the test suite and should be used as templates for new pipelines.

Validate all examples:

```bash
make validate-examples
```

Validate one example:

```bash
./cannectors validate ./examples/01-http-polling-basic-to-http-batch.yaml
```

## Index

| File | Covers |
| --- | --- |
| [01-http-polling-basic-to-http-batch.yaml](01-http-polling-basic-to-http-batch.yaml) | HTTP polling input to batched HTTP output |
| [02-http-polling-page-pagination.yaml](02-http-polling-page-pagination.yaml) | Page-based HTTP pagination |
| [03-http-polling-offset-pagination-state.yaml](03-http-polling-offset-pagination-state.yaml) | Offset pagination with state |
| [04-http-polling-cursor-oauth2.yaml](04-http-polling-cursor-oauth2.yaml) | Cursor pagination with OAuth2 |
| [05-webhook-hmac-to-http-single.yaml](05-webhook-hmac-to-http-single.yaml) | HMAC webhook to single HTTP requests |
| [06-webhook-queue-rate-limit-to-database.yaml](06-webhook-queue-rate-limit-to-database.yaml) | Queued webhook to database output |
| [07-database-input-basic-to-http.yaml](07-database-input-basic-to-http.yaml) | Database input to HTTP output |
| [08-database-input-limit-offset-to-database.yaml](08-database-input-limit-offset-to-database.yaml) | Database limit/offset input to database output |
| [09-database-input-cursor-incremental.yaml](09-database-input-cursor-incremental.yaml) | Incremental database input |
| [10-mapping-transforms-all.yaml](10-mapping-transforms-all.yaml) | Mapping transforms |
| [11-condition-nested-routing.yaml](11-condition-nested-routing.yaml) | Nested condition routing |
| [12-script-inline-transform.yaml](12-script-inline-transform.yaml) | Inline JavaScript transform |
| [13-script-file-transform.yaml](13-script-file-transform.yaml) | JavaScript transform file |
| [14-http-call-get-merge-cache.yaml](14-http-call-get-merge-cache.yaml) | HTTP enrichment with merge and cache |
| [15-http-call-query-header-append.yaml](15-http-call-query-header-append.yaml) | HTTP enrichment with query/header keys and append |
| [16-http-call-post-template-replace.yaml](16-http-call-post-template-replace.yaml) | HTTP enrichment POST template and replace |
| [17-sql-call-merge-cache.yaml](17-sql-call-merge-cache.yaml) | SQL enrichment with merge and cache |
| [18-sql-call-append-query-file.yaml](18-sql-call-append-query-file.yaml) | SQL enrichment with query file and append |
| [19-http-output-single-template.yaml](19-http-output-single-template.yaml) | Single-record HTTP output templating |
| [20-http-output-retry-auth-api-key.yaml](20-http-output-retry-auth-api-key.yaml) | HTTP output retry and API key auth |
| [21-database-output-transaction-query-file.yaml](21-database-output-transaction-query-file.yaml) | Transactional database output with query file |
| [22-defaults-inheritance.yaml](22-defaults-inheritance.yaml) | Top-level defaults and module overrides |
| [23-auth-basic-bearer-query-key.yaml](23-auth-basic-bearer-query-key.yaml) | Bearer, basic, and query API key auth |
| [24-empty-filter-pass-through.yaml](24-empty-filter-pass-through.yaml) | Empty filter chain pass-through |

## Assets

Some examples reference reusable files under `examples/assets/`:

- `assets/scripts/` for JavaScript transforms
- `assets/sql/` for SQL query files
- `assets/templates/` for HTTP body templates
