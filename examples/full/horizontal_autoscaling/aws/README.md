## AWS Horizontal autoscaling example

The Terraform code deploys the following resources:
- 1 ClickHouse service on AWS configured for horizontal autoscaling (replica count scales between
  `min_replicas` and `max_replicas` at a fixed per-replica memory, `min_replica_memory_gb` ==
  `max_replica_memory_gb`).

Requires an organization with horizontal autoscaling enabled (SCALE/ENTERPRISE tier).

## How to run

- Rename `variables.tfvars.sample` to `variables.tfvars` and fill in all needed data.
- Run `terraform init`
- Run `terraform <plan|apply> -var-file=variables.tfvars`
