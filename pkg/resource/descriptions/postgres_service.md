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
  map; to drop one, remove its key. Writing `pg_config = {}` clears all declared
  parameters (same as `tags = {}`).
- **Values are strings** — quote numbers (`"200"`).
- **Restarts are not automatic.** Some parameter changes require a database
  restart; the provider surfaces the server's restart-required hint as a
  warning during apply. Restart is not exposed by this resource — restart
  out-of-band.

## Passwords

The superuser password can be managed three ways:

- **Omit both `password` and `password_wo`** — the server generates one,
  captured into (sensitive) state as `password`.
- **`password`** — a value you supply; changing it rotates the password.
  Stored in (sensitive) state.
- **`password_wo` + `password_wo_version`** — a write-only password. The
  `password` attribute is **kept null in state** even though the server echoes
  the password on `GET`. Rotation is triggered by **incrementing
  `password_wo_version`** (write-only values can't be diffed). Bumping the
  version without supplying a `password_wo` value is a no-op.

> **`connection_string` still embeds the active password.** Write-only keeps
> the value out of the dedicated `password` attribute, but the server returns a
> `connection_string` with the credential in the URI. It is marked `Sensitive`,
> but the state file stores it in plaintext — there is no way to suppress it.

Rules: `password` and `password_wo` are mutually exclusive; both require
**≥12 chars with at least one lowercase, one uppercase, and one digit**
(enforced at plan time). Rotation is a `PATCH /password` — it does not resize
or restart the instance, and there is a brief (~1–2s) server-side propagation
window before a new password becomes active.

## Read replicas and point-in-time restore

- **`read_replica_of`** — set to a primary's ID to create a streaming read
  replica. Immutable (`RequiresReplace`); mutually exclusive with
  `restore_to_point_in_time` and with `password` / `password_wo` (a replica
  inherits the primary's superuser).
- **`restore_to_point_in_time = { source_id, restore_target }`** — create
  this instance by restoring another instance's backup to an RFC3339
  timestamp. Immutable (`RequiresReplace`); the restored instance's name is
  this resource's top-level `name`. A backup must exist at or before
  `restore_target` (the first automatic backup is taken ~10 minutes after the
  source is created).

```hcl
restore_to_point_in_time = {
  source_id      = clickhouse_postgres_service.primary.id
  restore_target = "2026-06-01T12:00:00Z"
}
```

## Out-of-band changes

- **Password rotated externally**: the next `terraform refresh` syncs
  the new value into state from the GET response.
- **Replica promoted externally**: the resource will detect the change
  (`is_primary` flips), but recovery requires `terraform state rm` and
  re-importing as a fresh primary.

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
- The connection string and password are visible in plan output even
  though both are marked `Sensitive`. The Terraform CLI renders
  `Sensitive` attributes as `(sensitive value)` in human-readable
  output but the underlying state file is plaintext — ensure your
  state backend is configured for at-rest encryption.
