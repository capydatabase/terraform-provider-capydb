data "capydb_regions" "available" {}

output "region_slugs" {
  value = [for r in data.capydb_regions.available.regions : r.slug]
}
