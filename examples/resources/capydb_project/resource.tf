# Minimal: CapyDB picks the cluster, environment defaults server-side.
resource "capydb_project" "app" {
  name = "my-app"
}

# Pin the cluster and mark the project as non-production.
data "capydb_clusters" "available" {}

resource "capydb_project" "staging" {
  name        = "my-app-staging"
  cluster_id  = data.capydb_clusters.available.clusters[0].id
  environment = "nonproduction"

  timeouts = {
    create = "30m"
    delete = "30m"
  }
}
