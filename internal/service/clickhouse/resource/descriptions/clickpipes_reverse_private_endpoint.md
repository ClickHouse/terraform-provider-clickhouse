You can use the *clickhouse_clickpipes_reverse_private_endpoint* resource to create and manage reverse private endpoints for secure ClickPipes data source connections in ClickHouse Cloud.

Supported endpoint types: `VPC_ENDPOINT_SERVICE`, `VPC_RESOURCE`, `MSK_MULTI_VPC`, and `GCP_PSC_SERVICE_ATTACHMENT`.

~> **Note:** All fields on this resource are immutable after creation. Any change will force replacement (destroy and recreate).

Use *clickhouse_clickpipes_reverse_private_endpoint_custom_private_dns* to manage custom private DNS mappings for a reverse private endpoint.

`dns_targets` lists the DNS targets the endpoint reports, which custom private DNS mappings can reference by `target_id`. It is populated only for `VPC_RESOURCE` endpoints, with one entry per VPC resource configuration association; for a GROUP resource configuration, each entry's `id` is a CHILD resource configuration ID. Targets appear once the endpoint has provisioned them, so the list can be empty right after creation.
