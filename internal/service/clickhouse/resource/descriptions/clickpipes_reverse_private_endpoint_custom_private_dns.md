Use the *clickhouse_clickpipes_reverse_private_endpoint_custom_private_dns* resource to manage the full set of custom private DNS mappings for an existing ClickPipes reverse private endpoint.

This resource updates only custom private DNS mappings. The mapping list is a full replacement list, and deleting this resource clears all custom private DNS mappings on the reverse private endpoint.

By default, each `private_dns_name` resolves to the endpoint's default target. Set `target_id` to resolve the name to a specific target instead. The reverse private endpoint's `dns_targets` attribute shows which targets are reported.

- **`VPC_RESOURCE`:** set `target_id` to a resource configuration ID. For a GROUP resource configuration, use the CHILD resource configuration ID, for example to give each database node its own hostname.
- **Same-region `VPC_ENDPOINT_SERVICE`:** set `target_id` to an AWS availability zone ID that the endpoint service offers, for example `use1-az1`. Use this for providers that address nodes through per-zone hostnames, such as Confluent Cloud brokers. If the endpoint is not in that zone yet, ClickHouse Cloud adds it, and the mapping is served once the zone appears in `dns_targets`.
- Cross-region `VPC_ENDPOINT_SERVICE` endpoints and other endpoint types do not support `target_id`.
