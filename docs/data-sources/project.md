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
- `primary_instance_id` (String) Identifier of the database cell the project runs in.
- `environment` (String) Environment label.
- `plan` (String) Billing-derived project plan.
- `postgres_version` (String) Postgres major version of the database.
- `postgres_channel` (String) Release channel of the Postgres major: `previous`, `stable`, `current`
  or `beta`. Null while the database is still provisioning.
- `postgres_warning` (String) What CapyDB does not guarantee for this database, set only when its
  Postgres major is not production ready (the beta channel). Null otherwise.
- `region` (String) Region the project lives in.
- `state` (String) Lifecycle state.
- `organization_id` (String) Owning organization id.
- `database_name` (String) Underlying Postgres database name.
