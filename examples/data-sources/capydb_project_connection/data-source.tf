# Requires an API key with the credentials:read scope. The URLs are sensitive
# and end up in Terraform state.
data "capydb_project" "app" {
  slug = "my-app"
}

data "capydb_project_connection" "app" {
  project_id = data.capydb_project.app.id
}

output "database_url" {
  value     = data.capydb_project_connection.app.pooled_url
  sensitive = true
}
