---
page_title: "capydb_organization Data Source - capydb"
description: |-
  Returns the organization the configured API key belongs to.
---

# capydb_organization (Data Source)

Returns the organization the configured API key belongs to (the authenticated viewer). Fails with
a clear error when the credential is not bound to an organization (e.g. a platform admin token).

## Example Usage

```terraform
data "capydb_organization" "current" {}

output "billing_plan" {
  value = data.capydb_organization.current.billing_plan
}
```

## Schema

### Read-Only

- `id` (String) Organization id.
- `name` (String) Organization name.
- `slug` (String) Organization slug.
- `billing_plan` (String) Current billing plan (e.g. `vibe`, `ship`, `business`).
- `billing_status` (String) Billing subscription status.
