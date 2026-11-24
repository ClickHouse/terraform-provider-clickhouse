~> **Note:** This resource is in beta and its behavior may change in future provider versions.

Creates a private endpoint service for [ClickHouse Cloud Managed Postgres](https://clickhouse.com/cloud/postgres),
so VPC endpoints in your AWS account can connect over AWS PrivateLink:

1. Create this resource. It waits until the service is available, which takes about 5 to 10 minutes.
2. Create an `aws_vpc_endpoint` with `service_name = endpoint_service_name` in the Postgres service's region.
3. Allow the VPC endpoint with `clickhouse_postgres_private_endpoint_attachment`.
4. Once AWS shows the VPC endpoint as available, turn on its private DNS. The VPC also needs DNS hostnames and
   DNS resolution turned on.
5. Connect to `private_hostname` from that VPC.

~> **Note:** Postgres private endpoints are available only for AWS Postgres services, and only once they are enabled
for your organization. Contact ClickHouse support to enable access.

## Supported lifecycle

- Create
- Read
- Delete. Deleting waits until the private endpoint service is gone.
- Import by Postgres service ID

Changing `postgres_id` replaces the private endpoint service.
