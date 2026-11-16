You can use the *clickhouse_clickpipes_reverse_private_endpoint* resource to create and manage reverse private endpoints for secure ClickPipes data source connections in ClickHouse Cloud.

Supported endpoint types: `VPC_ENDPOINT_SERVICE`, `VPC_RESOURCE`, `MSK_MULTI_VPC`, and `GCP_PSC_SERVICE_ATTACHMENT`.

~> **Note:** All fields on this resource are immutable after creation. Any change will force replacement (destroy and recreate).

Use *clickhouse_clickpipes_reverse_private_endpoint_custom_private_dns* to manage custom private DNS mappings for a reverse private endpoint.

`dns_targets` lists the DNS targets the endpoint reports, which custom private DNS mappings can reference by `target_id`:

- `VPC_RESOURCE` endpoints report one `RESOURCE_CONFIGURATION` entry per VPC resource configuration association. For a GROUP resource configuration, each entry's `id` is a CHILD resource configuration ID.
- Same-region `VPC_ENDPOINT_SERVICE` endpoints report one `AVAILABILITY_ZONE` entry per zonal endpoint DNS name. Each entry's `id` is an AWS availability zone ID, for example `use1-az1`. The regional DNS name is the default target and is not listed.
- Cross-region `VPC_ENDPOINT_SERVICE` endpoints and other endpoint types report no targets.

Targets appear once the endpoint has provisioned them, so the list can be empty right after creation. New target kinds may be added.
