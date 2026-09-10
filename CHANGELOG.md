# Changelog

All notable changes to the CapyDB Terraform/OpenTofu provider (`capydatabase/capydb`) are documented
here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow
[SemVer](https://semver.org/).

## [Unreleased]

### Added

- **`capydb_kv_store`** - a project's K/V store (CapyDB Knight/Valkyrie: key-value and rate
  limiting), running in its own KV cell beside the database rather than inside it. `project_id` is
  the only configurable attribute and forces a replacement; the store is sized from the
  organization's plan, so there is nothing to tune. Computed attributes cover `state`,
  `maxmemory_mb` (the storable capacity, not the cell's larger memory ceiling), `maxmemory_policy`,
  `persistence`, `rest_url`, and the sensitive `rest_token` and `redis_url`.

  The plaintext token is captured at create because that is the only response carrying it - the
  control plane keeps its SHA-256 hash - so `Read` deliberately does **not** refresh it from the
  credentials endpoint, which would overwrite the only copy in state with an empty string. Import
  takes the **project** id (every K/V endpoint is addressed by project) and leaves `rest_token`
  empty, because it cannot be recovered. Rotation is not exposed: the response that carries a new
  secret is the only place it exists, which is not a shape Terraform's refresh model can hold.

### Changed

- Project deletion completes the control plane's new approve-then-execute flow: the client mints
  a single-use `project.delete` approval token and presents it to the delete, replacing the old
  `confirm=true` query flag. Terraform's own plan/apply approval remains the human gate; no
  configuration changes.

- `PreviewDatabase`, `UpdateProjectRequest` and `UpdateWebhookEndpointRequest` are now aliases of the
  shared `capydbclient` module rather than provider-local declarations, so the provider and the CLI
  read one definition of each shape. `PreviewDatabase` consequently carries the fields the local copy
  omitted (`direct_port`, `pooled_port`, `ssl_mode`, and the `source_*` provenance fields).
  Tracks `capydbclient` v1.6.0.
- Go directive raised to 1.27.1.
- `capydbclient` bumped to v1.10.0 (approval tokens, read-only SQL, and the import preflight's
  source-provider and replication-readiness fields). This entry previously claimed v1.9.0 while
  `go.mod` still required v1.8.0 - a tag published before the GitHub organisation rename, whose
  `go.mod` still declares the `capydatabase` module path. Any build resolving it failed with "module
  declares its path as github.com/capydatabase/capydbclient", and because the local Go workspace unions
  every module's graph, that one stale requirement broke `go build` in the backend and the CLI too.
  Pre-rename tags cannot be repaired; the fix is to require a version tagged after the rename
  (capydbclient v1.9.0 or later).

## [2026-08-18]

### Changed

- Dependency refresh: `golang.org/x/net` v0.58.0, `golang.org/x/text` v0.41.0.

## [2026-07-22]

### Added

- `postgres_version` on the `capydb_project` resource and data source, for placing a project on
  Postgres 16, 17 or 18. Immutable after creation.

### Changed

- Documentation and attribute descriptions revised across all resources and data sources.

## [2026-07-08]

### Added

- First release. Resources: `capydb_project`, `capydb_preview_database`, `capydb_api_key`,
  `capydb_webhook_endpoint`. Data sources: `capydb_organization`, `capydb_project`,
  `capydb_project_connection`, `capydb_regions`.

### Changed

- The provider reuses the shared `capydbclient` transport instead of its own HTTP client, so retry,
  User-Agent and error decoding behave identically to the CLI.
