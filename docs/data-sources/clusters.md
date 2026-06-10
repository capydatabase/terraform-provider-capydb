---
page_title: "capydb_clusters Data Source - capydb"
description: |-
  Lists the active CapyDB clusters (regions) projects can be created on.
---

# capydb_clusters (Data Source)

Lists the active CapyDB clusters (regions) projects can be created on. The list is always non-null
(an empty result is an empty list), so `length()` and `for` expressions are safe.

## Example Usage

```terraform
data "capydb_clusters" "available" {}

resource "capydb_project" "app" {
  name       = "my-app"
  cluster_id = data.capydb_clusters.available.clusters[0].id
}
```

## Schema

### Read-Only

- `clusters` (Attributes List) Active clusters. (see
  [below for nested schema](#nestedatt--clusters))

<a id="nestedatt--clusters"></a>

### Nested Schema for `clusters`

Read-Only:

- `id` (String) Cluster id.
- `name` (String) Cluster name.
- `provider` (String) Infrastructure provider.
- `region` (String) Region identifier.
- `public_host` (String) Public database host.
- `direct_port` (Number) Direct Postgres port.
- `pooled_port` (Number) Pooled (PgBouncer) port.
- `postgres_version` (String) Postgres major version.
- `extensions` (List of String) Postgres extensions installed on the cluster.
- `state` (String) Cluster state.
- `supports_point_in_time_recovery` (Boolean) Whether the cluster supports point-in-time recovery.
