Read the full configuration of a BYOC (Bring Your Own Cloud) infrastructure
of the organization: provisioning state, connectivity toggles (private link
and load balancers), network layout, BYO-VPC settings, tags, and — when
private link is enabled — the private link endpoint service details.

Use it to reference a console-created BYOC infrastructure from a
`clickhouse_service` resource via its `byoc_id` attribute, or to verify the
state and flags of an infrastructure managed outside Terraform.

~> **Note:** This data source is in beta. Its behavior may change in future provider versions.

Read-only.
