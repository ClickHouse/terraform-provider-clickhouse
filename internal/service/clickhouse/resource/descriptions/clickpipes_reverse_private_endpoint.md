You can use the *clickhouse_clickpipes_reverse_private_endpoint* resource to create and manage reverse private endpoints for secure ClickPipes data source connections in ClickHouse Cloud.

Supported endpoint types: `VPC_ENDPOINT_SERVICE`, `VPC_RESOURCE`, `MSK_MULTI_VPC`, and `GCP_PSC_SERVICE_ATTACHMENT`.

~> **Note:** Endpoint configuration is immutable after creation. Changes force replacement (destroy and recreate), except for `wait_for_ready`, which only updates Terraform state.

Set `wait_for_ready = true` for `VPC_RESOURCE` endpoints when a downstream resource selects an internal DNS target from `dns_names`. Creation waits up to 10 minutes for `Ready` status and non-empty `dns_names`, and fails immediately on `Failed`, `Rejected`, or `Expired`. The default (`false`) waits only until the endpoint leaves `Provisioning`, allowing external acceptance to proceed. Imported endpoints default to `false`.

~> **Warning:** For `VPC_ENDPOINT_SERVICE` with acceptance required, `wait_for_ready = true` combined with an `aws_vpc_endpoint_connection_accepter` referencing this resource's `endpoint_id` deadlocks until the timeout: the accepter cannot run until creation finishes, and creation cannot finish until acceptance. Keep `wait_for_ready = false` in this case.

`private_dns_mappings` exposes the private DNS names and internal targets reported by the endpoint. For a `VPC_RESOURCE` GROUP with one CHILD per MongoDB node, select each CHILD's name from `dns_names` by its resource configuration ID substring (for example, `.rcfg-097648d8068504966.`). **The order of `dns_names` does not match CHILD order.** Use a filtered list with `[0]` so a missing target fails loudly; `one(...)` returns null for an empty list and would silently use the default target.

Use *clickhouse_clickpipes_reverse_private_endpoint_custom_private_dns* to manage custom private DNS mappings for a reverse private endpoint.
