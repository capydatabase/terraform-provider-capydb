---
page_title: "capydb_project_connection Data Source - capydb"
description: |-
  Fetches a project's connection strings with the current credentials embedded.
---

# capydb_project_connection (Data Source)

Fetches a project's connection strings with the current credentials embedded. Requires an API key
with the `credentials:read` scope.

~> The URLs embed live database credentials and end up in Terraform state — treat the state file
accordingly.

## Example Usage

```terraform
data "capydb_project" "app" {
  slug = "my-app"
}

data "capydb_project_connection" "app" {
  project_id = data.capydb_project.app.id
}

output "database_url" {
  value     = data.capydb_project_connection.app.pooled_url
  sensitive = true
}
```

## Schema

### Required

- `project_id` (String) Project to fetch connection strings for.

### Read-Only

- `pooled_url` (String, Sensitive) Pooled (PgBouncer) connection URL — the default for
  applications.
- `direct_url` (String, Sensitive) Direct Postgres connection URL for migrations and long-lived
  sessions.
- `username` (String) Database role the URLs authenticate as.
