resource "capydb_project" "app" {
  name   = "app"
  region = "eu-central"
}

# A K/V store for rate limiting, sessions, queues and caching. It runs in its
# own KV cell beside the project's database and needs nothing from it - a
# project can have a store and never open the database.
resource "capydb_kv_store" "app" {
  project_id = capydb_project.app.id
}

# The two variables every Upstash-compatible client reads. `Redis.fromEnv()`
# looks for UPSTASH_* names, which CapyDB does not publish, so construct the
# client explicitly or set those names from these values.
output "kv_rest_url" {
  value = capydb_kv_store.app.rest_url
}

output "kv_rest_token" {
  value     = capydb_kv_store.app.rest_token
  sensitive = true
}
