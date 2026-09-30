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

# With split roles enabled, run the application as app_user (RLS applies) and
# keep the owner URL for migrations.
output "app_database_url" {
  value     = coalesce(data.capydb_project_connection.app.app_pooled_url, data.capydb_project_connection.app.pooled_url)
  sensitive = true
}
