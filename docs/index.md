---
page_title: "CapyDB Provider"
description: |-
  Manage CapyDB managed Postgres projects, preview databases, API keys, and webhook endpoints.
---

# CapyDB Provider

The CapyDB provider manages resources on [CapyDB](https://capydb.dev), a managed Postgres hosting
service: projects (logical Postgres databases), disposable preview/branch databases, organization
API keys, and outbound webhook endpoints.

## Authentication

The provider authenticates with a CapyDB **organization API key** (format `capy_live_...`).
Create one in the CapyDB dashboard or via the `capydb_api_key` resource (bootstrapping requires an
existing key). Configure it in the provider block or through the `CAPYDB_API_KEY` environment
variable.

Operations the provider performs require these scopes on the key:

- `projects:read` / `projects:write` - projects and preview databases
- `credentials:read` - the `capydb_project_connection` data source
- `jobs:read` - waiting on asynchronous provision/delete jobs

## Example Usage

```terraform
terraform {
  required_providers {
    capydb = {
      source  = "capy-base/capydb"
      version = "~> 0.1"
    }
  }
}

provider "capydb" {
  # Falls back to the CAPYDB_API_KEY environment variable when omitted.
  api_key = var.capydb_api_key
}

resource "capydb_project" "app" {
  name        = "my-app"
  environment = "production"
}

data "capydb_project_connection" "app" {
  project_id = capydb_project.app.id
}

output "database_url" {
  value     = data.capydb_project_connection.app.pooled_url
  sensitive = true
}
```

## Asynchronous jobs and timeouts

Project provisioning/deletion and preview database creation/deletion run as asynchronous jobs on
the CapyDB control plane. The corresponding resources enqueue the job and poll it until it reaches
a terminal state (`completed` or `failed`). The wait is bounded by the resource's `timeouts`
attribute (`create` / `delete`, both defaulting to 20 minutes):

```terraform
resource "capydb_project" "app" {
  name = "my-app"

  timeouts = {
    create = "30m"
    delete = "30m"
  }
}
```

If a create wait times out or the job fails, the resource is still recorded in state with its id,
so the project/preview is never orphaned - re-run `terraform apply` or inspect the job in the
dashboard.

## Empty lists

CapyDB list endpoints may serialize empty lists as JSON `null`. The provider normalizes every list
it reads to an empty (non-null) list, so expressions like
`length(data.capydb_regions.available.regions)` are always safe.

## Schema

### Optional

- `api_key` (String, Sensitive) CapyDB organization API key (format `capy_live_...`). Falls back
  to the `CAPYDB_API_KEY` environment variable.
- `api_url` (String) CapyDB API base URL. Falls back to the `CAPYDB_API_URL` environment variable,
  then to `https://capydb.dev/api/capydb`.
