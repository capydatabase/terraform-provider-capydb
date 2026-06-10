---
page_title: "capydb_preview_database Resource - capydb"
description: |-
  A disposable CapyDB preview/branch database for a project.
---

# capydb_preview_database (Resource)

A disposable CapyDB preview/branch database for a project. Creation and deletion run as
asynchronous jobs which this resource waits on (bounded by the `timeouts` attribute, 20 minutes by
default).

Previews expire after their TTL. Increasing `ttl_hours` extends the expiry in place via the extend
endpoint; decreasing it forces a replacement (the API only supports extending TTLs).

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
- `ttl_hours` (Number) Time to live in hours (minimum 1). Increasing it extends the preview's
  expiry by the difference; decreasing it forces a replacement.

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
