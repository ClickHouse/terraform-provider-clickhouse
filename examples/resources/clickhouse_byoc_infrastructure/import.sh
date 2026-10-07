#!/bin/bash
# Import by BYOC infrastructure ID. Write-only creation parameters
# (external_id, tenant_id, service_principal_client_id,
# availability_zone_suffixes, public_subnet_ids) are not returned by the API;
# add them to the configuration after importing — they are adopted in place,
# and only changing a recorded value forces replacement. The BYO-VPC
# attributes (vpc_id, private_subnet_ids, gcp_pod_cidr_range_names,
# gcp_shared_vpc_host_project_id) are restored from the API during import;
# configuring a different value plans a replacement.
terraform import clickhouse_byoc_infrastructure.example 44444444-4444-4444-4444-444444444444
