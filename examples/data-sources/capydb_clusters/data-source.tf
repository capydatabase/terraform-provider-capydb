data "capydb_clusters" "available" {}

output "cluster_regions" {
  value = [for c in data.capydb_clusters.available.clusters : c.region]
}
