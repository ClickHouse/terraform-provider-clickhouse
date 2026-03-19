## ClickPipe CDC with MySQL example

This example demonstrates how to deploy a MySQL CDC ClickPipe with a MySQL database as the input source, including CDC infrastructure configuration for optimal performance.

### What this example creates

- A MySQL CDC ClickPipe that replicates data from MySQL to ClickHouse
- CDC infrastructure resource for scaling the shared CDC compute resources
- Table mappings from MySQL source tables to ClickHouse destination tables

### Prerequisites

- A ClickHouse Cloud service
- A MySQL database with:
  - Binary logging enabled (`log_bin = ON`)
  - Row-based replication format (`binlog_format = ROW`)
  - Network access from ClickHouse Cloud to your MySQL instance

### Using a Reverse Private Endpoint

For secure, private connectivity to your MySQL instance, you can use a [reverse private endpoint](../reverse_private_endpoint_vpce_service). Create the RPE resource first, then use its DNS name as the `host` in your MySQL source configuration:

```hcl
resource "clickhouse_clickpipes_reverse_private_endpoint" "mysql_endpoint" {
  service_id                = var.service_id
  description               = "RPE for MySQL CDC"
  type                      = "VPC_ENDPOINT_SERVICE"
  vpc_endpoint_service_name = var.vpc_endpoint_service_name
}

resource "clickhouse_clickpipe" "mysql_cdc" {
  name       = "MySQL CDC ClickPipe"
  service_id = var.service_id

  depends_on = [clickhouse_clickpipe_cdc_infrastructure.infra]

  source = {
    mysql = {
      host = clickhouse_clickpipes_reverse_private_endpoint.mysql_endpoint.dns_names[0]
      port = var.mysql_port
      # ... remaining configuration
    }
  }

  destination = {
    database = "default"
  }
}
```

## How to run

- Rename `variables.tfvars.sample` to `variables.tfvars` and fill in all needed data.
- Run `terraform init`
- Run `terraform <plan|apply> -var-file=variables.tfvars`

## Notes

- The CDC infrastructure resource is shared across all CDC ClickPipes in the service
- Only one CDC infrastructure resource should be created per service
- Table mappings define which MySQL tables to replicate and their destination in ClickHouse
- The `replication_mode` can be `cdc`, `snapshot`, or `cdc_only`
- The `replication_mechanism` can be `AUTO`, `GTID`, or `FILE_POS`
