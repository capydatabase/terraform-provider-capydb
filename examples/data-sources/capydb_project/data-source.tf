# Look up by slug...
data "capydb_project" "by_slug" {
  slug = "my-app"
}

# ...or by id (exactly one of id / slug must be set).
data "capydb_project" "by_id" {
  id = "prj_0123456789"
}

output "project_state" {
  value = data.capydb_project.by_slug.state
}
