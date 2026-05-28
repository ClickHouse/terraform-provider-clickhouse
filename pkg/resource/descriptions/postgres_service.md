> **Alpha resource.** This resource is exposed only in alpha builds of the
> provider (`-tags alpha`). The backing ClickHouse Cloud Managed Postgres API
> is marked `beta` server-side. Expect breaking changes between alpha
> releases. Do **not** use this resource for production workloads until it is
> promoted to stable.

Manages a [ClickHouse Cloud Managed Postgres](https://clickhouse.com/cloud/postgres)
service. A Managed Postgres service is a fully-managed Postgres instance
provisioned in the ClickHouse Cloud control plane.

## Phase 2 scope

This release ships the minimum useful surface: create, read, update
(`size`, `ha_type`, `tags`), delete, and import. The following are
intentionally deferred:

- **`pg_config` / `pgbouncer_config`** — Phase 3.
- **User-supplied passwords (`password`, `password_wo`)** — Phase 4. Today
  the server always generates the password; it is exposed as a sensitive
  computed attribute and persisted in state from the create response.
- **Point-in-time restore (`restore_to_point_in_time`)** — Phase 5.
- **Read replicas (`read_replica_of`)** — Phase 5.
- **CA certificate data source** — Phase 5.
- **Operational commands (restart / promote / switchover)** — out of scope
  for v1. Use the ClickHouse Cloud UI or API directly. See "Operational
  commands" below.
- **`timeouts {}` block** — Phase 5. Create/update/delete budgets are
  currently hardcoded to 30m / 30m / 10m.

## Differences from `clickhouse_service`

- **Name is immutable.** The server's PATCH body has no `name` field
  (`PostgresInstancePatchRequestV1`). Changing `name` triggers
  destroy-and-recreate via `RequiresReplace`. `clickhouse_service` allows
  in-place rename.
- **Tags are a set of nested `{ key, value }` objects, not a flat map.** The
  server's `ResourceTagV1` shape is an array of objects with optional
  `value`. Using nested attributes preserves that distinction. Tags whose
  key starts with `chc_` are reserved by the server and rejected at plan
  time. **Tag values cannot be explicit empty strings** — the server
  normalizes `""` to no-value, which would cause perpetual plan/state
  drift; omit the `value` attribute or set it to `null` instead.
  **Each tag must include a non-null `value` on update.** Even though
  the server's create endpoint accepts no-value tags, the PATCH endpoint
  returns `400 BAD_REQUEST` if any tag entry omits `value`. Always set
  a non-empty alphanumeric/`.`/`-`/`_` value (server regex
  `^[a-zA-Z0-9._-]+$`) when declaring tags. The provider rejects empty
  strings at plan time; the no-value case is a server-side 400.
- **No support for explicit empty tag lists.** Writing `tags = []` is
  rejected at plan time. To express "no tags," omit the attribute
  entirely — `Optional + Computed + UseStateForUnknown` then carries
  the prior state forward without spurious diffs. The constraint
  exists because the server-side round-trip of an empty array
  collapses to no-value on read, which a literal `tags = []` in `.tf`
  would diff against forever.
- **No IP allowlist, private endpoints, backup configuration, maintenance
  windows, customer-managed encryption keys, or BYOC support.** All blocked
  on server-side endpoint additions; tracked in the project plan as Phases
  8–14.

## Out-of-band changes

- **`pg_config` / `pgbouncer_config`** (Phase 3+): once shipped, any
  change made via the ClickHouse Cloud UI or API will be reverted on the
  next `terraform apply`.
- **Password** (Phase 4+): the server does not echo the password on `GET`,
  so a rotation done outside Terraform cannot be detected. Terraform will
  continue to hold the old value in state.
- **`is_primary` flip**: if a user promotes a replica via the API, this
  resource will detect the change but the recovery path is destructive in
  v1 (`terraform state rm` and re-import). Phase 18 addresses this
  gracefully.

## Operational commands

Restart, promote, and switchover are deliberately not exposed as Terraform
attributes. They are state transitions that don't map to a declarative
resource. Use the API, UI, or CLI directly.

Rationale: industry survey across AWS RDS (silent attribute removal), GCP
Cloud SQL (coordinated attribute flip), Azure Postgres Flexible (explicit
`replication_role`), Aiven (explicitly excluded), and DigitalOcean (also
excluded) showed real disagreement and real footguns. ClickHouse Cloud
follows the Aiven model: Terraform describes infrastructure shape;
operational state changes are API calls.

## Import

```
terraform import clickhouse_postgres_service.example <postgres-instance-id>
```

Post-import: every attribute except `password` is hydrated from the server.

> **⚠ Password is unrecoverable after import.**
> The server does not echo the superuser password on `GET`, so `terraform
> import` cannot retrieve the value the instance was created with. After
> import:
>
> - `password` will be null in state.
> - `connection_string` will contain the password embedded in the URI
>   (the server includes it in the GET response), so the credential is
>   not lost — but Terraform itself cannot manage it until Phase 4 ships
>   user-supplied passwords. In Phase 2, an imported instance is
>   effectively **read-only**: any future apply that needs to surface
>   the password as the standalone `password` attribute will show drift
>   that cannot be reconciled.
> - Workaround in Phase 2: parse the password out of `connection_string`
>   externally and store it where your CI/automation needs it. Don't try
>   to set it back into Terraform state by hand — there is no
>   `terraform import`-time hook to do this safely.
> - Phase 4 will add an explicit password-rotation flow
>   (`password_wo_version` bump) that makes imported instances fully
>   manageable. Until then, treat imported services as observable but
>   not mutable.

## Known limitations (alpha)

- The `size` attribute is validated against a compile-time snapshot of
  `VM_SPECS`. New AWS instance families added server-side require a
  provider patch release before they are usable in `.tf`.
- Lifecycle timeouts are not user-configurable in Phase 2 (see "Phase 2
  scope" above).
- The connection string and password are visible in plan output even
  though both are marked `Sensitive`. Terraform CLI treats `Sensitive`
  attributes as `(sensitive value)` in human-readable output but the
  underlying state file is plaintext — ensure your state backend is
  configured for at-rest encryption.
