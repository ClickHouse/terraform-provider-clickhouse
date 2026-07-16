~> **Note:** This resource is in alpha and its behavior may change in future provider versions.

Manages a [ClickHouse Cloud Managed Postgres](https://clickhouse.com/cloud/postgres)
service. A Managed Postgres service is a fully-managed Postgres instance
provisioned in the ClickHouse Cloud control plane.

## Supported lifecycle

- Create — standard, as a read replica (`read_replica_of`), or by
  point-in-time restore (`restore_to_point_in_time`)
- Read
- Update — `size`, `ha_type`, `tags`, `pg_config`, `pgbouncer_config`,
  `password` rotation
- Delete
- Import

Three companion data sources are also provided (alpha):
`clickhouse_postgres_service`, `clickhouse_postgres_services`, and
`clickhouse_postgres_service_ca_certificates`.

## Unsupported attributes

The following are intentionally absent from the schema:

- Operational commands (restart / promote / switchover). See
  "Operational commands" below for the rationale.
- IP allowlist, private endpoints, backup configuration, maintenance
  windows, customer-managed encryption keys, BYOC. These depend on
  server-side endpoint additions.
- Configurable lifecycle timeouts — there is no `timeouts {}` block; the
  provider uses fixed internal poll/retry budgets.

## Tag semantics

Tags are a `map(string → string)` — same shape as `clickhouse_service`.
Values must be non-empty alphanumeric / `.` / `-` / `_` strings (server
regex `^[a-zA-Z0-9._-]+$`); the server's PATCH endpoint returns `400
BAD_REQUEST` on omitted values, so the schema rejects empty values at
plan time.

Setting `tags = {}` clears all user-controlled tags. Omitting the
attribute entirely preserves the prior state value (`Optional +
Computed + UseStateForUnknown`).

The Postgres PATCH endpoint has PUT-like semantics specifically for the
`tags` field: omitting it from the request body clears all tags
server-side. The provider works around this by re-asserting the current
state tags in every PATCH that mutates `size` or `ha_type`, so users
won't lose tags when they resize or change HA mode. This is invisible
end-to-end but worth knowing if you inspect `TF_LOG=DEBUG` request
bodies — you'll see tags repeated on non-tag mutations.

## Runtime configuration (`pg_config` / `pgbouncer_config`)

Postgres server parameters and PgBouncer pooler parameters are managed as
`map(string → string)` (same shape as `tags`):

```hcl
pg_config = {
  max_connections = "200"
  work_mem        = "8MB"
}
pgbouncer_config = {
  pool_mode = "transaction"
}
```

- **Full replacement of declared parameters.** The declared map is the desired
  state. Every apply sends the full map via `POST /config`, so removing a key
  from the map removes it server-side. Out-of-band changes to declared
  parameters are reverted on the next apply.
- **`Optional + Computed` (like `tags`).** Omitting the attribute preserves the
  prior state rather than clearing it — a read replica inherits its primary's
  parameters, and the server may surface values the configuration never
  declared, so those must be allowed into state. To change parameters, edit the
  map; to drop one, remove its key. Writing `pg_config = {}` on an existing
  instance clears all declared parameters (same as `tags = {}`). An empty
  `pg_config = {}` is **only valid on update** — the server rejects it on create,
  so on a create (including a read replica / restore) omit the attribute to use
  the default / inherit, or set at least one parameter (the provider blocks an
  empty map at plan time with a clear error).
- **Values are strings** — quote numbers (`"200"`).
- **Restarts are not automatic.** Some parameter changes require a database
  restart; the provider surfaces the server's restart-required hint as a
  warning during apply. Restart is not exposed by this resource — restart
  out-of-band.

## Credentials

Credentials are **config-owned**, matching `clickhouse_service`: the
ClickHouse Cloud API does not return the Postgres superuser password (or a
password-bearing connection string), so Terraform manages exactly the
credential you declare and never reads one back.

- A **standard service** must declare `password` or `password_wo` (write-only,
  never stored in state — requires Terraform >= 1.11, and increment
  `password_wo_version` to rotate).
- A **read replica** must not declare either — it inherits the primary's
  superuser. Its password is not tracked in state.
- A **point-in-time restore** may declare one (rotated in after the restore);
  if omitted, the restored service keeps the source backup's password,
  untracked by Terraform.
- **Rotation:** change `password`, or bump `password_wo_version`, and apply.
  Out-of-band rotations (UI/API) are invisible to Terraform — the next
  Terraform-driven rotation re-asserts the declared value.
- **Import** cannot recover the live password. After `terraform import`, the
  first apply rotates to the configured credential.
- **Connection details:** compose the URI from `hostname`, `port`, `username`,
  and your declared password, e.g.
  `postgres://${username}:${password}@${hostname}:${port}/postgres?sslmode=require`.

## Read replicas and point-in-time restore

Both create the instance from a **source**, so the create-time attributes that
define where it runs and how big it is are **inherited from the source** — omit
them. The provider reads the source at plan time and fills them in:

- `cloud_provider`, `region`, `postgres_version` (and, for a **replica**,
  `size`) are reproduced verbatim. Omit them, or set them to **match** the
  source — a mismatch is a plan-time error ("conflicts with the source
  instance"). The provider pins these into the plan so it shows real values.
- `size` on a **restore** (the restored instance comes up at the **backup's**
  size) and `ha_type` (server-assigned for a new replica/restore) are **not**
  taken from the source — they must be **omitted**; setting them is a plan-time
  error. They show as "(known after apply)".

If the source ID doesn't exist, the plan errors. (A standalone primary or a
restored instance can be resized or have its `ha_type` changed in place — those
are normal in-place updates. A **live read replica cannot** — see below.)

- **`read_replica_of`** — set to a primary's ID to create a streaming read
  replica. Mutually exclusive with `restore_to_point_in_time` and with
  `password`/`password_wo` (a replica inherits the primary's superuser).
  Changing or removing it **destroys and recreates** the instance as a
  standalone primary — a live replica can't be converted in place (see
  "Out-of-band changes" for the promotion exception).
  A **live read replica cannot be modified directly**: changing `size`,
  `ha_type`, or `tags` is a **plan-time error** ("read replica cannot be
  modified directly"), because the server rejects any such change on a replica.
  Resize/retag the **parent** instead. (Removing `read_replica_of` turns this
  into a standalone primary, but — as noted above — that **destroys and
  recreates** a live replica; it is not an in-place detach.) `pg_config` /
  `pgbouncer_config` **are** changeable on a replica — they use a separate
  endpoint that allows per-replica values.
- **`restore_to_point_in_time = { source_id, restore_target }`** — create
  this instance by restoring another instance's backup to an RFC3339
  timestamp. The restored instance's name is this resource's top-level `name`
  and it is independent of its source. A backup must exist at or before
  `restore_target` (the first automatic backup is taken ~10 minutes after the
  source is created). The block is create-time only: changing `source_id` /
  `restore_target` **or removing** it **destroys and recreates** the instance.

```hcl
restore_to_point_in_time = {
  source_id      = clickhouse_postgres_service.primary.id
  restore_target = "2026-06-01T12:00:00Z"
}
```

## Out-of-band changes

- **Password rotated externally**: invisible to Terraform — the API does not
  return credentials, so refresh cannot detect it. The next Terraform-driven
  rotation (changing `password` or bumping `password_wo_version`) re-asserts
  the declared value.
- **Replica promoted externally**: the next refresh surfaces `is_primary`
  flipping true, and the plan then **errors** ("read replica has been promoted
  to a primary"), directing you to remove `read_replica_of` from the
  configuration. Doing so reconciles the instance **in place** (no destroy),
  adopting it as a standalone primary — precisely because `is_primary` is true.
  (Removing `read_replica_of` from a *non-promoted* replica instead destroys and
  recreates it as a standalone primary.)

## Operational commands

Restart, promote, and switchover are not exposed as Terraform
attributes. Terraform describes infrastructure shape; operational
state changes (restart, promote, switchover) go through the API,
UI, or CLI directly.

## Known limitations

- The `size` attribute is not validated client-side beyond non-empty.
  Invalid sizes surface as an HTTP 400 at apply time rather than a
  plan-time error. Pinning the list to a compile-time snapshot would
  mean new AWS instance families require a provider patch release
  before users can adopt them; `size` is the most frequently changed
  attribute, so the trade-off goes the other way here. The
  `cloud_provider`, `ha_type`, and `postgres_version` attributes
  remain client-side validated because they churn rarely.
- `name` is immutable post-create. The server's PATCH body has no
  `name` field, so changing it forces destroy-and-recreate via
  `RequiresReplace`.
- `password` is marked `Sensitive`, so the Terraform CLI renders it as
  `(sensitive value)` in human-readable output, but the underlying
  state file is plaintext — ensure your state backend is configured
  for at-rest encryption, or use `password_wo` to keep the credential
  out of state entirely.
