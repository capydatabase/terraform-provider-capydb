resource "capydb_project" "app" {
  name = "my-app"
}

# Disposable clone of the production database, expiring after 48 hours.
# Raising ttl_hours later extends the expiry in place; lowering it forces a
# replacement.
resource "capydb_preview_database" "pr" {
  project_id = capydb_project.app.id
  name       = "pr-42"
  mode       = "clone" # or "empty"
  ttl_hours  = 48
}
