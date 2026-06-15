#!/bin/bash
# Managed Postgres services can be imported by specifying the service ID.
# The password is recovered on import (the server echoes it on GET) and stored
# in state.
terraform import clickhouse_postgres_service.example xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
