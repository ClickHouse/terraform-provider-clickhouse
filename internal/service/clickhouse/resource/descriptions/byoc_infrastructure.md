Provision a BYOC (Bring Your Own Cloud) infrastructure: a ClickHouse Cloud
data plane deployed into your own AWS account, GCP project or Azure
subscription. Once the infrastructure reaches the 'infra-ready' state,
`clickhouse_service` resources can be deployed into it via their `byoc_id`
attribute.

Before creating, the provider runs the preflight validation endpoint to
simulate the cloud permissions the deployment needs and fails the apply with
the failing checks if any are denied. Set `skip_preflight_validation = true`
to create without this gate (for example if validation is not yet supported
for your cloud provider).

Creation provisions cloud infrastructure in your account and typically takes
30-60 minutes. Deletion triggers an asynchronous termination that removes the
provisioned cloud resources.

The cloud-side onboarding (IAM role or service principal trusted by
ClickHouse Cloud, and for BYO-VPC topologies the VPC and subnets) must exist
before creating this resource. See the [BYOC onboarding
documentation](https://clickhouse.com/docs/cloud/reference/byoc) for details.

~> **Note:** This resource is in beta. Its behavior may change in future provider versions.

Known limitations:

- Write-only creation parameters (`external_id`, `tenant_id`, `service_principal_client_id`, `availability_zone_suffixes`, `public_subnet_ids`, `vpc_id`, `private_subnet_ids`, `gcp_pod_cidr_range_names`, `gcp_shared_vpc_host_project_id`) are not returned by the API. After `terraform import` they are unset in state; set them in the configuration to match what the infrastructure was created with.
