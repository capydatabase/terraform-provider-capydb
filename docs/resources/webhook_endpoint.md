---
page_title: "capydb_webhook_endpoint Resource - capydb"
description: |-
  An outbound CapyDB webhook endpoint for the organization.
---

# capydb_webhook_endpoint (Resource)

An outbound CapyDB webhook endpoint for the organization. The HMAC signing secret is returned by
the API exactly once at creation (and again on rotation) and is stored in Terraform state as the
sensitive `signing_secret` attribute; drift on the secret itself cannot be detected.

To rotate the secret, bump `secret_version` (e.g. `1` → `2`). The endpoint is updated in place and
the new secret is stored in state; the previous secret stops validating immediately. The
`secret_version` value itself is never sent to the API — it is purely a rotation trigger.

## Example Usage

```terraform
resource "capydb_webhook_endpoint" "ops" {
  url         = "https://hooks.example.com/capydb"
  description = "Ops notifications"

  # Omit (or set to []) to subscribe to all events.
  event_types = ["project.provisioned", "backup.completed"]

  # Bump to rotate the signing secret in place.
  secret_version = 1
}

output "webhook_signing_secret" {
  value     = capydb_webhook_endpoint.ops.signing_secret
  sensitive = true
}
```

## Schema

### Required

- `url` (String) HTTPS receiver URL deliveries are POSTed to.

### Optional

- `organization_id` (String) Organization the endpoint belongs to. Defaults to the authenticated
  principal's organization. Changing it forces a replacement.
- `description` (String) Free-form endpoint description.
- `event_types` (List of String) Event types to deliver. An empty (or omitted) list subscribes to
  all events.
- `active` (Boolean) Whether deliveries are enabled. Defaults to `true`.
- `secret_version` (Number) Rotation trigger for the signing secret. Bump this value (e.g. 1 → 2)
  to rotate the secret in place; the value itself is never sent to the API. Defaults to `1`.

### Read-Only

- `id` (String) Webhook endpoint id.
- `signing_secret` (String, Sensitive) HMAC-SHA256 signing secret used for the
  `X-CapyDB-Signature` header. Returned exactly once at creation/rotation and stored in state;
  out-of-band rotation cannot be detected.
