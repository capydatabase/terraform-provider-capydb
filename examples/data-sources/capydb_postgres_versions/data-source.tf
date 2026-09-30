data "capydb_postgres_versions" "available" {}

output "default_postgres_version" {
  value = data.capydb_postgres_versions.available.default_version
}

# Majors fit for production data (excludes a beta major while one is offered).
output "production_postgres_versions" {
  value = [
    for v in data.capydb_postgres_versions.available.versions : v.version
    if v.production_ready
  ]
}
