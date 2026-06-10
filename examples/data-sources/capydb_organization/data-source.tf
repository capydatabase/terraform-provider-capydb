# The organization the configured API key belongs to.
data "capydb_organization" "current" {}

output "billing_plan" {
  value = data.capydb_organization.current.billing_plan
}
