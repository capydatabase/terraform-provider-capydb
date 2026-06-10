# Org-wide read-only key. The plaintext key is exposed once via the sensitive
# `token` attribute and stored in Terraform state.
resource "capydb_api_key" "ci_readonly" {
  name   = "ci-readonly"
  scopes = ["projects:read", "jobs:read"]
}

# Project-scoped key that can read connection credentials for one project only.
resource "capydb_project" "app" {
  name = "my-app"
}

resource "capydb_api_key" "app_credentials" {
  name       = "app-credentials"
  scopes     = ["projects:read", "credentials:read"]
  project_id = capydb_project.app.id
}

output "ci_readonly_token" {
  value     = capydb_api_key.ci_readonly.token
  sensitive = true
}
