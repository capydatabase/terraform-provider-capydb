---
page_title: "capydb_regions Data Source - capydb"
description: |-
  Lists the available CapyDB placement regions projects can be created in.
---

# capydb_regions (Data Source)

Lists the available CapyDB placement regions projects can be created in. Region ids are neutral
(`eu-north-1`); `region_details` carries the same regions, in the same order, with a display label
and location. Both lists are always non-null (an empty result is an empty list), so `length()` and
`for` expressions are safe.

## Example Usage

```terraform
data "capydb_regions" "available" {}

resource "capydb_project" "app" {
  name   = "my-app"
  region = data.capydb_regions.available.regions[0].slug
}

# Human-readable labels, keyed by region id.
output "region_labels" {
  value = {
    for region in data.capydb_regions.available.region_details :
    region.id => "${region.display_name} (${region.location})"
  }
}
```

## Schema

### Read-Only

- `regions` (Attributes List) Available placement regions. (see
  [below for nested schema](#nestedatt--regions))
- `region_details` (Attributes List) The same regions, in the same order, with their display
  labels. (see [below for nested schema](#nestedatt--region_details))

<a id="nestedatt--regions"></a>

### Nested Schema for `regions`

Read-Only:

- `slug` (String) Region id (for example `eu-north-1`), the value `capydb_project.region` takes.

<a id="nestedatt--region_details"></a>

### Nested Schema for `region_details`

Read-Only:

- `id` (String) Region id (for example `eu-north-1`), the value `capydb_project.region` takes.
- `display_name` (String) Display label, for example `EU North`.
- `location` (String) Where the region's nodes run, for example `Helsinki, Finland`.
