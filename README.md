# Terraform Provider for CapyDB

The official Terraform/OpenTofu provider for [CapyDB](https://capydb.dev) — simple managed Postgres
hosting. Manage projects (logical Postgres databases), disposable preview/branch databases,
organization API keys, and webhook endpoints as code.

Built on [terraform-plugin-framework](https://github.com/hashicorp/terraform-plugin-framework)
(protocol v6; Terraform >= 1.0 / OpenTofu).

## Installation

### From the registry

```terraform
terraform {
  required_providers {
    capydb = {
      source  = "capy-base/capydb"
      version = "~> 0.1"
    }
  }
}
```

### Local development override

Build the provider and point Terraform at the binary with a
[`dev_overrides`](https://developer.hashicorp.com/terraform/cli/config/config-file#development-overrides-for-provider-developers)
block — no registry, version pin, or `terraform init` needed for the overridden provider:

```bash
make install   # go install -> $GOPATH/bin/terraform-provider-capydb
```

`~/.terraformrc`:

```hcl
provider_installation {
  dev_overrides {
    "capy-base/capydb" = "/Users/you/go/bin"   # directory containing the binary
  }
  direct {}
}
```

Then run `terraform plan` / `apply` directly (skip `terraform init` for the overridden provider —
Terraform prints a warning that dev overrides are in effect).

## Provider configuration

```terraform
provider "capydb" {
  api_key = var.capydb_api_key # sensitive; falls back to CAPYDB_API_KEY
  # api_url = "https://capydb.dev/api/capydb"  # falls back to CAPYDB_API_URL, then this default
}
```

| Attribute | Type | Required | Environment fallback | Notes |
| --- | --- | --- | --- | --- |
| `api_key` | string, sensitive | yes (via block or env) | `CAPYDB_API_KEY` | Organization API key, format `capy_live_...` |
| `api_url` | string | no | `CAPYDB_API_URL` | Defaults to `https://capydb.dev/api/capydb` |

The API key needs `projects:read`/`projects:write` for project and preview management,
`credentials:read` for the connection data source, and `jobs:read` to wait on asynchronous jobs.

## Quickstart

```terraform
resource "capydb_project" "app" {
  name        = "my-app"
  environment = "production"
}

# Ephemeral clone of the production database for a pull request.
resource "capydb_preview_database" "pr" {
  project_id = capydb_project.app.id
  name       = "pr-42"
  mode       = "clone"
  ttl_hours  = 48
}

data "capydb_project_connection" "app" {
  project_id = capydb_project.app.id
}

output "database_url" {
  value     = data.capydb_project_connection.app.pooled_url
  sensitive = true
}
```

## Resources and data sources

| Resource | Purpose |
| --- | --- |
| [`capydb_project`](docs/resources/project.md) | A managed logical Postgres database. Async provision/delete; `environment` updatable in place; importable by id. |
| [`capydb_preview_database`](docs/resources/preview_database.md) | Disposable preview/branch database with a TTL. Raising `ttl_hours` extends in place; lowering forces replacement. |
| [`capydb_api_key`](docs/resources/api_key.md) | Organization (or project-scoped) API key. Plaintext key captured once into the sensitive `token` attribute; delete revokes. |
| [`capydb_webhook_endpoint`](docs/resources/webhook_endpoint.md) | Outbound webhook receiver. Bump `secret_version` to rotate the HMAC signing secret in place. |

| Data source | Purpose |
| --- | --- |
| [`capydb_clusters`](docs/data-sources/clusters.md) | Active clusters (regions) projects can be placed on. |
| [`capydb_project`](docs/data-sources/project.md) | Look up a project by `id` or `slug` (exactly one). |
| [`capydb_project_connection`](docs/data-sources/project_connection.md) | Pooled/direct connection URLs with credentials embedded (sensitive). |
| [`capydb_organization`](docs/data-sources/organization.md) | The organization of the configured API key, including billing plan/status. |

Runnable HCL for every resource and data source lives under [`examples/`](examples/).

## Asynchronous jobs and timeouts

CapyDB executes lifecycle operations (project provision/delete, preview create/delete) as
asynchronous jobs. The provider enqueues the job and polls `/v1/jobs/{id}` every 5 seconds until
it completes or fails. Each affected resource exposes a `timeouts` attribute bounding the wait:

```terraform
resource "capydb_project" "app" {
  name = "my-app"

  timeouts = {
    create = "30m" # default 20m
    delete = "30m" # default 20m
  }
}
```

On create, the resource id is written to state *before* waiting, so a timed-out or failed job
never orphans the project/preview — re-run `terraform apply` or inspect the job in the dashboard.

## Null lists

CapyDB list endpoints may serialize empty lists as JSON `null`. The provider's API client
normalizes every decoded list to an empty, non-nil list, so `clusters`, `scopes`, `event_types`,
etc. are always safe to iterate.

## Secrets in state

`capydb_api_key.token`, `capydb_webhook_endpoint.signing_secret`, and the
`capydb_project_connection` URLs are sensitive values that the API returns once (or that embed
live credentials) and that are persisted in Terraform state. Protect the state file (encrypted
remote backend, restricted access).

## Development

```bash
make build             # build ./bin/terraform-provider-capydb
make test              # go test -race ./...
make check             # fmt + vet + lint + test
make release-snapshot  # local GoReleaser snapshot
```

The test suite runs the provider's CRUD logic against an in-memory mock of the CapyDB control
plane (`internal/provider/mock_server_test.go`) — no real infrastructure needed.

Releases are published with GoReleaser following the
[Terraform Registry publishing layout](https://developer.hashicorp.com/terraform/registry/providers/publishing):
zip archives named `terraform-provider-capydb_VERSION_OS_ARCH.zip`, a `_SHA256SUMS` file with a
detached GPG signature (`GPG_FINGERPRINT` env), and the registry manifest uploaded as
`terraform-provider-capydb_VERSION_manifest.json`.
