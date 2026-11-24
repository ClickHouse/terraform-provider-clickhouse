~> **Note:** This resource is in beta and its behavior may change in future provider versions.

Manages the VPC endpoints allowed to connect to [ClickHouse Cloud Managed Postgres](https://clickhouse.com/cloud/postgres)
over AWS PrivateLink. First create the private endpoint service with `clickhouse_postgres_private_endpoint`.

This resource owns the allow-list: VPC endpoints missing from `allowed_vpc_endpoints` can no longer connect, including
ones added in the ClickHouse Cloud console. Use one `clickhouse_postgres_private_endpoint_attachment` per
Postgres service; multiple attachments for the same service overwrite each other.

## Supported lifecycle

- Create and update replace the whole allow-list.
- Read
- Delete removes every allowed VPC endpoint. AWS then rejects their connections. If the `aws_vpc_endpoint` is
  destroyed in the same run, AWS can refuse to delete it with "The existing processing REJECT is too recent"; run
  `terraform destroy` again.
- Import by Postgres service ID
