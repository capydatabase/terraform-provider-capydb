---
page_title: "capydb_project Resource - capydb"
description: |-
  A CapyDB project (a managed logical Postgres database).
---

# capydb_project (Resource)

A CapyDB project (a managed logical Postgres database). Provisioning and deletion run as
asynchronous jobs which this resource waits on (bounded by the `timeouts` attribute, 20 minutes by
default).

The project **plan** is derived from the organization's billing state and cannot be configured
here. The CapyDB API does not support renaming projects, so changing `name` forces a replacement.

## Example Usage

```terraform
# Minimal: CapyDB picks the cluster, environment defaults server-side.
resource "capydb_project" "app" {
  name = "my-app"
}

# Pin the cluster and mark the project as non-production.
data "capydb_clusters" "available" {}

resource "capydb_project" "staging" {
  name        = "my-app-staging"
  cluster_id  = data.capydb_clusters.available.clusters[0].id
  environment = "nonproduction"

  timeouts = {
    create = "30m"
    delete = "30m"
  }
}
```

## Schema

### Required

- `name` (String) Project name. The CapyDB API does not support renaming projects, so changing the
  name forces a replacement.

### Optional

- `cluster_id` (String) Cluster to place the project on. Omit to let CapyDB pick. Changing it
  forces a replacement.
- `environment` (String) Environment label, either `production` or `nonproduction`. Updatable in
  place.
- `timeouts` (Attributes) (see [below for nested schema](#nestedatt--timeouts))

### Read-Only

- `id` (String) Project id.
- `slug` (String) URL-safe project slug derived from the name.
- `plan` (String) Project plan (e.g. `free`, `launch`, `scale`). Derived from the organization's
  billing state; never configurable.
- `region` (String) Region the project's cluster lives in.
- `state` (String) Lifecycle state of the project.
- `organization_id` (String) Owning organization id.
- `database_name` (String) Name of the underlying Postgres database.

<a id="nestedatt--timeouts"></a>

### Nested Schema for `timeouts`

Optional:

- `create` (String) Bound on waiting for the asynchronous provision job. Defaults to 20m.
- `delete` (String) Bound on waiting for the asynchronous deletion job. Defaults to 20m.

## Import

Projects can be imported by id:

```shell
terraform import capydb_project.app <project-id>
```
