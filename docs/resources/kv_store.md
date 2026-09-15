---
page_title: "capydb_kv_store Resource - capydb"
description: |-
  A CapyDB K/V store: managed key-value and rate limiting in its own KV cell.
---

# capydb_kv_store (Resource)

A CapyDB K/V store (CapyDB Knight/Valkyrie): a managed key-value and rate-limiting service running
in its own KV cell, beside the project's database rather than inside it. A project has at most one,
and may have one with or without ever using its database.

The store is sized from the organization's plan, so there is no size attribute to set. `project_id`
is the only configurable attribute and it forces a replacement; destroying the resource deletes the
store and its data.

~> The plaintext token is returned by the API exactly once at creation and lives in Terraform state
as the sensitive `rest_token`. Protect the state file accordingly. After an `import` it is empty,
because the control plane keeps only its SHA-256 hash - a lost token can be replaced but never
recovered.

-> A K/V store keeps only a periodic snapshot: **no backups, no point-in-time recovery**, and keys
carrying a TTL are evicted once it is full. Treat it as fast, expendable state - counters, sessions,
caches, locks - and keep anything you cannot reconstruct in the database cell next door.

## Example Usage

```terraform
resource "capydb_project" "app" {
  name   = "app"
  region = "eu-central"
}

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
```

## Import

Import takes the **project** id, not the store id: every K/V endpoint is addressed by project, and a
project has at most one store.

```shell
terraform import capydb_kv_store.app project_01H8XYZ
```

`rest_token` is empty on an imported resource and cannot be refreshed. Rotate in the dashboard or
with `capydb kv rotate-token` to obtain a new one; rotation is not a Terraform operation, because
the response that carries the new secret is the only place it ever exists.

~> Rotating outside Terraform leaves `rest_token` and `redis_url` stale in state, and Terraform
cannot detect it: a refresh preserves both rather than overwriting the only copy the state file
holds, and the control plane has nothing to compare them against. Take the new token from the rotate
output and feed it to whatever consumes it. Replacing the resource would resynchronize state, but it
destroys the store and its data.

## Schema

### Required

- `project_id` (String) Project the store belongs to. Changing it forces a replacement.

### Read-Only

- `id` (String) K/V store id.
- `state` (String) Lifecycle state: `provisioning`, `running`, `error` or `destroying`. Creation is
  asynchronous, so a freshly created store reads as `provisioning`.
- `maxmemory_mb` (Number) Storable capacity in MB, derived from the organization's plan. This is
  what the store will hold - the KV cell's own memory ceiling is larger so a snapshot fork has
  headroom, and that difference is not usable capacity.
- `maxmemory_policy` (String) Eviction policy, managed by CapyDB. `volatile-lru`: only keys carrying
  a TTL are evictable, so a store filled with non-expiring keys rejects writes rather than
  discarding data.
- `persistence` (String) Durability mode, managed by CapyDB. `rdb` is a periodic snapshot.
- `rest_url` (String) Upstash-compatible REST endpoint. Set it as `CAPYKV_REST_URL`.
- `rest_token` (String, Sensitive) The plaintext K/V token (format `capy_kv_...`). Returned exactly
  once at creation; set it as `CAPYKV_REST_TOKEN`. Empty on an imported resource.
- `redis_url` (String, Sensitive) RESP endpoint for ordinary Redis® OSS clients. Set it as
  `CAPYKV_REDIS_URL`. Carries the token as its password at creation; after an import it has no
  password.
