#!/bin/bash
# Managed Postgres services can be imported by specifying the service ID.
# terraform import cannot recover the live password; after import, the first
# apply rotates to the configured password / password_wo.
terraform import clickhouse_postgres_service.example xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
