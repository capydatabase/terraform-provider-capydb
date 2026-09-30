---
page_title: "capydb_postgres_versions Data Source - capydb"
description: |-
  Lists the Postgres major versions a new CapyDB database can be created on, oldest first.
---

# capydb_postgres_versions (Data Source)

Lists the Postgres major versions a new CapyDB database can be created on, oldest first, with the
release channel each is offered on and which one is the default. A `beta` major appears only while
CapyDB offers it; it is an upstream PostgreSQL beta and has `production_ready = false`. `versions`
is always non-null (an empty result is an empty list), so `length()` and `for` expressions are
safe. Requires the `projects:read` scope.

## Example Usage

```terraform
data "capydb_postgres_versions" "available" {}

output "default_postgres_version" {
  value = data.capydb_postgres_versions.available.default_version
}

# Majors fit for production data (excludes a beta major while one is offered).
output "production_postgres_versions" {
  value = [
    for v in data.capydb_postgres_versions.available.versions : v.version
    if v.production_ready
  ]
}
```

Pin `capydb_project.postgres_version` to a literal rather than to `default_version`: changing
`postgres_version` forces a replacement, so following the default would replace the database when
the platform default moves.

## Schema

### Read-Only

- `versions` (Attributes List) Postgres majors open for new databases, oldest first. (see
  [below for nested schema](#nestedatt--versions))
- `default_version` (String) The `version` of the entry marked `default`. Null if no entry is
  marked.

<a id="nestedatt--versions"></a>

### Nested Schema for `versions`

Read-Only:

- `version` (String) Major version (for example `17`), the value `capydb_project.postgres_version`
  takes.
- `channel` (String) Release channel: `previous`, `stable`, `current` or `beta`.
- `default` (Boolean) Whether a new database gets this major when `postgres_version` is not set.
- `production_ready` (Boolean) Whether this major is fit for production data. False for a `beta`
  major, an upstream PostgreSQL beta offered for evaluation only.
