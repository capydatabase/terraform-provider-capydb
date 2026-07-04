---
page_title: "capydb_regions Data Source - capydb"
description: |-
  Lists the available CapyDB placement regions projects can be created in.
---

# capydb_regions (Data Source)

Lists the available CapyDB placement regions projects can be created in. The list is always
non-null (an empty result is an empty list), so `length()` and `for` expressions are safe.

## Example Usage

```terraform
data "capydb_regions" "available" {}

resource "capydb_project" "app" {
  name   = "my-app"
  region = data.capydb_regions.available.regions[0].slug
}
```

## Schema

### Read-Only

- `regions` (Attributes List) Available placement regions. (see
  [below for nested schema](#nestedatt--regions))

<a id="nestedatt--regions"></a>

### Nested Schema for `regions`

Read-Only:

- `slug` (String) Region slug.
