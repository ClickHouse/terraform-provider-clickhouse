Manages a ClickHouse Cloud managed Postgres instance.

This resource provisions and manages Postgres instances running on ClickHouse Cloud infrastructure (powered by Ubicloud). It supports creating, reading, updating, and deleting instances.

## Immutable Fields

The following fields cannot be changed after creation and will force a replacement if modified:
- `name`
- `cloud_provider`
- `region`
- `postgres_version`
- `pg_config`
- `pg_bouncer_config`

## Mutable Fields

The following fields can be updated in-place:
- `size` (may cause brief service disruption during resize)
- `storage_size` (can only be increased, not decreased)
- `ha_type` ('none' = 0 standbys, 'async' = 1 standby with async replication, 'sync' = 2 standbys with synchronous replication)
- `tags`

## Prerequisites

The organization must have the `FT_ORG_MANAGED_POSTGRES_SERVICES` feature flag enabled. Currently only AWS is supported as a cloud provider.
