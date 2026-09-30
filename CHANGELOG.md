# Changelog

All notable changes to the CapyDB Terraform/OpenTofu provider (`capydatabase/capydb`) are documented
here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow
[SemVer](https://semver.org/).

## [Unreleased]

## [0.3.0] - 2026-09-30

### Added

- `capydb_project.postgres_version` accepts `19`, the beta channel. It is an upstream PostgreSQL
  beta and not production ready, and the control plane accepts it only while CapyDB offers it (off
  by default); otherwise the apply fails with the API's error.
- `postgres_channel` and `postgres_warning` (read-only) on the `capydb_project` resource and data
  source. The channel is `previous`, `stable`, `current` or `beta` and is refreshed on every read,
  so it follows a major upgrade; the warning is set only for a major that is not production ready
  and is null otherwise.
- `capydb_regions.region_details`: the same regions as `regions`, in the same order, each with its
  `id`, `display_name` and `location`. `regions[*].slug` is unchanged.
- `capydb_project_connection.app_username`, `app_pooled_url` and `app_direct_url` (URLs
  sensitive): the connection strings of the project's runtime login `app_user`, a role that owns
  nothing and cannot bypass row-level security. Null unless the project has enabled its app role;
  enabling it is not a Terraform operation.
- `capydb_api_key.manager` (read-only): whether the key may perform organization admin actions
  (key management, project deletion, production-overwrite restores, webhook and K/V management).
- `capydb_kv_store.stopped_reason` (read-only), and `state` documents `stopped`: the platform
  stopped the store (`org_suspended`), keeps its data and starts it again when the reason clears. A
  stopped store stays in state and plans no change.

### Changed

- Examples and docs name regions by their neutral ids (`eu-north-1`). `hel1` still works as a
  deprecated alias of `eu-north-1`.

## [0.2.1] - 2026-09-30

### Fixed

- **A configuration naming `region = "hel1"` no longer replaces the database.** The control plane
  now reports neutral region ids (`eu-north-1`) and accepts `hel1` only as a deprecated alias. The
  provider keeps the configured alias in state instead of the reported id, and moving a
  configuration from `hel1` to `eu-north-1` updates state in place; any other region change still
  forces a replacement. Upgrade the provider before the control plane reports the new ids, and
  switch configurations to `eu-north-1` (or `data.capydb_regions`) within the deprecation release.

## [0.2.0] - 2026-09-26

### Changed

- **Deleting a production project takes an approval a person created.** The control plane no
  longer lets an API key mint the `project.delete` approval, so the provider stops minting one: set
  `CAPYDB_APPROVAL_TOKEN` to an approval an organization admin created on the project's settings
  page in the dashboard (valid 10 minutes). Without it, destroying or replacing a `production`
  project fails before any API call and says where to get one; `non_production` projects are
  unaffected.

## [0.1.1] - 2026-09-24

### Changed

- The README and registry index list the `capydb_kv_store` resource and describe a project as a
  dedicated database cell, not a "logical Postgres database".
- `capydb_api_key.scopes` documents the full grantable set (the eleven tenant scopes or `*`). The
  control plane now rejects any other scope, including the platform's service capabilities, so a
  configuration naming one fails at apply instead of creating an over-privileged key.

## [0.1.0] - 2026-09-22

First tagged release. Everything under the date-keyed sections below shipped before the provider had
a release pipeline; from here on `release.yml` publishes every `vX.Y.Z` tag as a signed GitHub
release in the Terraform Registry layout.

### Security

- **`google.golang.org/grpc` moved past GO-2026-6443** (server panic on a request missing the
  `:authority`/`Host` header, reachable through the plugin server every provider binary runs).
  The fix is only on grpc `master` so far — there is no tagged release carrying it — so the
  indirect requirement is pinned to the post-fix pseudo-version until one exists.

### Added

- `capydb_project.always_on` (optional, computed): whether the database is exempt from pausing
  when idle. Defaults server-side to `true` for `production` and `false` for `non_production`; a
  config that pins it keeps its value across an environment change. Updatable in place.

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

- `capydb_kv_store.redis_url` documents its published environment name, `CAPYKV_REDIS_URL`.

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
