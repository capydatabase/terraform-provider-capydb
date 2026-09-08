# Changelog

All notable changes to the CapyDB Terraform/OpenTofu provider (`capydatabase/capydb`) are documented
here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow
[SemVer](https://semver.org/).

## [Unreleased]

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
  `go.mod` still declares the `capy-base` module path. Any build resolving it failed with "module
  declares its path as github.com/capy-base/capydbclient", and because the local Go workspace unions
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
