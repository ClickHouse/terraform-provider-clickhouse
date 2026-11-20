#!/bin/bash
# The allowed VPC endpoints can be imported by specifying the Postgres service ID.
terraform import clickhouse_postgres_private_endpoint_attachment.example xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
