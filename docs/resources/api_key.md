---
page_title: "capydb_api_key Resource - capydb"
description: |-
  A CapyDB organization API key.
---

# capydb_api_key (Resource)

A CapyDB organization API key. The plaintext key is returned by the API exactly once at creation
and is stored in Terraform state as the sensitive `token` attribute - the API never returns it
again, so drift on the secret itself cannot be detected (only metadata such as revocation is).

All configurable attributes force a replacement; deleting the resource revokes the key. A key that
is revoked outside Terraform is dropped from state on the next refresh so Terraform plans a
recreation.

~> The plaintext key lives in Terraform state. Protect the state file accordingly. Import is not
supported because the secret cannot be recovered after creation.

## Example Usage

```terraform
# Org-wide read-only key.
resource "capydb_api_key" "ci_readonly" {
  name   = "ci-readonly"
  scopes = ["projects:read", "jobs:read"]
}

# Project-scoped key that can read connection credentials for one project only.
resource "capydb_api_key" "app_credentials" {
  name       = "app-credentials"
  scopes     = ["projects:read", "credentials:read"]
  project_id = capydb_project.app.id
}

output "ci_readonly_token" {
  value     = capydb_api_key.ci_readonly.token
  sensitive = true
}
```

## Schema

### Required

- `name` (String) Human-readable key name. Changing it forces a replacement.
- `scopes` (List of String) Scopes granted to the key (at least one), e.g. `projects:read`,
  `projects:write`, `credentials:read`, `backups:read`, `backups:write`, `jobs:read`. Changing it
  forces a replacement.

### Optional

- `organization_id` (String) Organization the key belongs to. Defaults to the authenticated
  principal's organization. Changing it forces a replacement.
- `project_id` (String) Restricts the key to a single project. Project-scoped keys cannot touch
  sibling projects, manage other keys, or mutate the organization. Changing it forces a
  replacement.

### Read-Only

- `id` (String) API key id.
- `token` (String, Sensitive) The plaintext API key (format `capy_live_...`). Returned exactly
  once at creation and stored in state; rotation of the secret outside Terraform cannot be
  detected.
- `key_prefix` (String) Non-secret key prefix used to identify the key.
- `is_active` (Boolean) Whether the key is active (not revoked or expired).
