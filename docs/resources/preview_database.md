---
page_title: "capydb_preview_database Resource - capydb"
description: |-
  A disposable CapyDB preview/branch database for a project.
---

# capydb_preview_database (Resource)

A disposable CapyDB preview/branch database for a project. Creation and deletion run as
asynchronous jobs which this resource waits on (bounded by the `timeouts` attribute, 20 minutes by
default).

Previews expire after their TTL. `ttl_hours` is counted from the moment it is set: updating it
resets the expiry in place to that many hours from the time of the update (the API treats the
value as an absolute new TTL, not a delta).

~> Import is not supported for this resource: the plaintext-free preview lookup requires the parent
project id, and the API has no direct preview GET endpoint.

## Example Usage

```terraform
resource "capydb_project" "app" {
  name = "my-app"
}

resource "capydb_preview_database" "pr" {
  project_id = capydb_project.app.id
  name       = "pr-42"
  mode       = "clone" # or "empty"
  ttl_hours  = 48
}
```

## Schema

### Required

- `project_id` (String) Project the preview belongs to. Changing it forces a replacement.
- `ttl_hours` (Number) Time to live in hours (1-168), counted from when it is set. Updating it
  resets the preview's expiry in place to `ttl_hours` from the time of the update.

### Optional

- `name` (String) Preview name. Omit to let CapyDB generate one. Changing it forces a replacement.
- `mode` (String) Data source mode: `clone` (copy of the production database) or `empty`. Changing
  it forces a replacement.
- `timeouts` (Attributes) (see [below for nested schema](#nestedatt--timeouts))

### Read-Only

- `id` (String) Preview database id.
- `state` (String) Lifecycle state of the preview database.
- `database_name` (String) Name of the underlying Postgres database.
- `ttl_expires_at` (String) RFC 3339 timestamp the preview expires at.

<a id="nestedatt--timeouts"></a>

### Nested Schema for `timeouts`

Optional:

- `create` (String) Bound on waiting for the asynchronous create job. Defaults to 20m.
- `delete` (String) Bound on waiting for the asynchronous deletion job. Defaults to 20m.
