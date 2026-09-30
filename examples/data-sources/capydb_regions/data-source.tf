data "capydb_regions" "available" {}

output "region_slugs" {
  value = [for r in data.capydb_regions.available.regions : r.slug]
}

# Human-readable labels, keyed by region id.
output "region_labels" {
  value = {
    for region in data.capydb_regions.available.region_details :
    region.id => "${region.display_name} (${region.location})"
  }
}
