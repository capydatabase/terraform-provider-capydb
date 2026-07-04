# Minimal: CapyDB picks the region, environment defaults server-side.
resource "capydb_project" "app" {
  name = "my-app"
}

# Pin the region and mark the project as non-production.
data "capydb_regions" "available" {}

resource "capydb_project" "staging" {
  name        = "my-app-staging"
  region      = data.capydb_regions.available.regions[0].slug
  environment = "nonproduction"

  timeouts = {
    create = "30m"
    delete = "30m"
  }
}
