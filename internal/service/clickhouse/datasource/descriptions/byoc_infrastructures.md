List all BYOC (Bring Your Own Cloud) infrastructures of the organization as
summaries: ID, provisioning state, cloud provider, region, cloud account and
display name.

Use it to discover the ID of a console-created infrastructure (for example by
display name or region) instead of copying it manually, then read the full
configuration with the `clickhouse_byoc_infrastructure` data source or attach
services via `byoc_id`.

~> **Note:** This data source is in beta. Its behavior may change in future provider versions.

Read-only.
