resource "capydb_webhook_endpoint" "ops" {
  url         = "https://hooks.example.com/capydb"
  description = "Ops notifications"

  # Omit (or set to []) to subscribe to all events.
  event_types = ["project.provisioned", "backup.completed"]

  # Bump to rotate the HMAC signing secret in place; the previous secret stops
  # validating immediately. The value itself is never sent to the API.
  secret_version = 1
}

output "webhook_signing_secret" {
  value     = capydb_webhook_endpoint.ops.signing_secret
  sensitive = true
}
