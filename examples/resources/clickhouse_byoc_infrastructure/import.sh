#!/bin/bash
# Import by BYOC infrastructure ID. Write-only creation parameters
# (external_id, tenant_id, service_principal_client_id, vpc_id, subnet lists)
# are not returned by the API; add them to the configuration after importing.
# They are adopted in place — only changing a recorded value forces replacement.
terraform import clickhouse_byoc_infrastructure.example 44444444-4444-4444-4444-444444444444
