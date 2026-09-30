Use the *clickhouse_clickpipes_reverse_private_endpoint_custom_private_dns* resource to manage the full set of custom private DNS mappings for an existing ClickPipes reverse private endpoint.

This resource updates only custom private DNS mappings. The mapping list is a full replacement list, and deleting this resource clears all custom private DNS mappings on the reverse private endpoint.

By default, each `private_dns_name` resolves to the endpoint's default target. For `VPC_RESOURCE` endpoints, set `target_id` to a resource configuration ID to resolve the name to that target instead. For a GROUP resource configuration, use the CHILD resource configuration ID, for example to give each database node its own hostname. The reverse private endpoint's `dns_targets` attribute shows which targets are reported.

Because target IDs are known in advance, mappings can be applied right after the reverse private endpoint is created, without waiting for it to become ready. Once the endpoint reports targets, the API rejects a new or changed `target_id` that is not among them. A mapping whose target cannot be resolved is skipped; it never falls back to the default target. `target_id` is rejected for other endpoint types.
