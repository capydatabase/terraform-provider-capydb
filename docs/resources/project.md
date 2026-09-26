---
page_title: "capydb_project Resource - capydb"
description: |-
  A CapyDB project (a dedicated Postgres database cell).
---

# capydb_project (Resource)

A CapyDB project (a dedicated Postgres database cell). Provisioning and deletion run as
asynchronous jobs which this resource waits on (bounded by the `timeouts` attribute, 20 minutes by
default).

The project **plan** is derived from the organization's billing state and cannot be configured
here. The CapyDB API does not support renaming projects, so changing `name` forces a replacement.

**Deleting a production project needs an approval from a person.** The control plane does not let
an API key approve its own production delete, so `terraform destroy` (or a replacement) of a
project whose `environment` is `production` stops before calling the API until an organization
admin creates a delete approval on the project's settings page in the CapyDB dashboard. Set the
token as `CAPYDB_APPROVAL_TOKEN` and run apply again within 10 minutes. `non_production` projects
delete without one.

## Example Usage

```terraform
# Minimal: CapyDB picks the region, environment defaults server-side.
resource "capydb_project" "app" {
  name = "my-app"
}

# Pin the region and mark the project as non-production.
data "capydb_regions" "available" {}

resource "capydb_project" "staging" {
  name        = "my-app-staging"
  region      = data.capydb_regions.available.regions[0].slug
  environment = "non_production"

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

- `region` (String) Region to place the project in. Omit to let CapyDB pick. Changing it
  forces a replacement.
- `environment` (String) Environment label, either `production` or `non_production`. Updatable in
  place.
- `always_on` (Boolean) Whether the database is exempt from pausing when idle. Defaults to `true` for `production` and `false` for `non_production`; changing the environment without setting this re-derives it. Updatable in place.
- `timeouts` (Attributes) (see [below for nested schema](#nestedatt--timeouts))

### Read-Only

- `id` (String) Project id.
- `slug` (String) URL-safe project slug derived from the name.
- `plan` (String) Project plan (e.g. `vibe`, `ship`, `business`). Derived from the organization's
  billing state; never configurable.
- `primary_instance_id` (String) Identifier of the database cell the project runs in.
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
