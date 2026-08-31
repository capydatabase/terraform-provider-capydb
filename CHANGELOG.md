# Changelog

All notable changes to the CapyDB Terraform/OpenTofu provider (`capy-base/capydb`) are documented
here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow
[SemVer](https://semver.org/).

## [Unreleased]

### Changed

- `PreviewDatabase`, `UpdateProjectRequest` and `UpdateWebhookEndpointRequest` are now aliases of the
  shared `capydbclient` module rather than provider-local declarations, so the provider and the CLI
  read one definition of each shape. `PreviewDatabase` consequently carries the fields the local copy
  omitted (`direct_port`, `pooled_port`, `ssl_mode`, and the `source_*` provenance fields).
  Tracks `capydbclient` v1.6.0.
- Go directive raised to 1.27.0.

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
