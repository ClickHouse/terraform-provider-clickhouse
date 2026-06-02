> **Alpha resource.** This resource is exposed only in alpha builds of the
> provider (`-tags alpha`). The backing ClickHouse Cloud Managed Postgres API
> is marked `beta` server-side. Expect breaking changes between alpha
> releases. Do **not** use this resource for production workloads until it is
> promoted to stable.

Manages a [ClickHouse Cloud Managed Postgres](https://clickhouse.com/cloud/postgres)
service. A Managed Postgres service is a fully-managed Postgres instance
provisioned in the ClickHouse Cloud control plane.

## Supported lifecycle

- Create
- Read
- Update — `size`, `ha_type`, `tags`
- Delete
- Import

## Unsupported attributes

The following are intentionally absent from the schema:

- Postgres / PgBouncer runtime parameters (`pg_config` /
  `pgbouncer_config`).
- User-supplied passwords (`password`, `password_wo`). The server
  generates the password; the resource exposes it as a sensitive
  computed attribute and persists it in state from the create response.
- Point-in-time restore (`restore_to_point_in_time`).
- Read replicas (`read_replica_of`).
- CA certificate data source.
- Operational commands (restart / promote / switchover). See
  "Operational commands" below for the rationale.
- Configurable lifecycle timeouts (`timeouts {}` block). Create / update
  / delete budgets are hardcoded to 30m / 30m / 10m.
- IP allowlist, private endpoints, backup configuration, maintenance
  windows, customer-managed encryption keys, BYOC. These depend on
  server-side endpoint additions.

## Tag semantics

Tags are a `map(string → string)` — same shape as `clickhouse_service`.
Values must be non-empty alphanumeric / `.` / `-` / `_` strings (server
regex `^[a-zA-Z0-9._-]+$`); the server's PATCH endpoint returns `400
BAD_REQUEST` on omitted values, so the schema rejects empty values at
plan time. Tag keys starting with `chc_` are reserved by the server
and also rejected at plan time.

Writing `tags = {}` is rejected at plan time. To express "no tags,"
omit the attribute entirely — `Optional + Computed + UseStateForUnknown`
then carries the prior state forward without spurious diffs.

The Postgres PATCH endpoint has PUT-like semantics specifically for the
`tags` field: omitting it from the request body clears all tags
server-side. The provider works around this by re-asserting the current
state tags in every PATCH that mutates `size` or `ha_type`, so users
won't lose tags when they resize or change HA mode. This is invisible
end-to-end but worth knowing if you inspect `TF_LOG=DEBUG` request
bodies — you'll see tags repeated on non-tag mutations.

## Out-of-band changes

- **Password rotated externally**: the server does not echo the password
  on `GET`, so a rotation done outside Terraform cannot be detected.
  Terraform will continue to hold the old value in state.
- **Replica promoted externally**: the resource will detect the change
  (`is_primary` flips), but recovery requires `terraform state rm` and
  re-importing as a fresh primary.

## Operational commands

Restart, promote, and switchover are deliberately not exposed as
Terraform attributes. They are state transitions that don't map to a
declarative resource. Use the API, UI, or CLI directly.

Rationale: industry survey across AWS RDS (silent attribute removal),
GCP Cloud SQL (coordinated attribute flip), Azure Postgres Flexible
(explicit `replication_role`), Aiven (explicitly excluded), and
DigitalOcean (also excluded) showed real disagreement and real
footguns. ClickHouse Cloud follows the Aiven model: Terraform describes
infrastructure shape; operational state changes are API calls.

## Import

```
terraform import clickhouse_postgres_service.example <postgres-instance-id>
```

Post-import, every attribute except `password` is hydrated from the
server.

> **Password is unrecoverable after import.**
> The server does not echo the superuser password on `GET`, so
> `terraform import` cannot retrieve the value the instance was created
> with. After import:
>
> - `password` will be null in state.
> - `connection_string` will contain the password embedded in the URI
>   (the server includes it in the GET response), so the credential is
>   not lost — but Terraform cannot manage it as a standalone attribute
>   without user-supplied password support. Imported instances are
>   effectively read-only via Terraform with respect to the password:
>   any apply that needs to surface the password as the standalone
>   `password` attribute will show drift that cannot be reconciled.
> - Workaround: parse the password out of `connection_string` externally
>   and store it where your CI/automation needs it. Don't try to set it
>   back into Terraform state by hand — there is no
>   `terraform import`-time hook to do this safely.

## Known limitations

- The `size` attribute is not validated client-side beyond non-empty.
  Invalid sizes surface as an HTTP 400 at apply time rather than a
  plan-time error. Pinning the list to a compile-time snapshot would
  mean new AWS instance families require a provider patch release
  before users can adopt them; `size` is the most frequently changed
  attribute, so the trade-off goes the other way here. The
  `cloud_provider`, `ha_type`, and `postgres_version` attributes
  remain client-side validated because they churn rarely.
- Lifecycle timeouts are not user-configurable.
- `name` is immutable post-create. The server's PATCH body has no
  `name` field, so changing it forces destroy-and-recreate via
  `RequiresReplace`.
- The connection string and password are visible in plan output even
  though both are marked `Sensitive`. The Terraform CLI renders
  `Sensitive` attributes as `(sensitive value)` in human-readable
  output but the underlying state file is plaintext — ensure your
  state backend is configured for at-rest encryption.
