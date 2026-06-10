---
page_title: "capydb_project Data Source - capydb"
description: |-
  Looks up a CapyDB project by id or by slug.
---

# capydb_project (Data Source)

Looks up a CapyDB project by `id` or by `slug`. Exactly one of the two must be set.

## Example Usage

```terraform
data "capydb_project" "by_slug" {
  slug = "my-app"
}

data "capydb_project" "by_id" {
  id = "prj_0123456789"
}
```

## Schema

### Optional

- `id` (String) Project id. Exactly one of `id` or `slug` must be set.
- `slug` (String) Project slug. Exactly one of `id` or `slug` must be set.

### Read-Only

- `name` (String) Project name.
- `cluster_id` (String) Cluster the project lives on.
- `environment` (String) Environment label.
- `plan` (String) Billing-derived project plan.
- `region` (String) Region the project's cluster lives in.
- `state` (String) Lifecycle state.
- `organization_id` (String) Owning organization id.
- `database_name` (String) Underlying Postgres database name.
