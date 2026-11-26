# A restricted-access BYOC infrastructure registers an EKS cluster, IAM roles
# and S3 buckets provisioned by your own configuration (not shown here).
resource "clickhouse_byoc_infrastructure" "restricted" {
  region_id    = "us-east-1"
  account_id   = var.aws_account_id
  access_model = "restricted"

  restricted_facts = {
    cluster_name       = module.eks.cluster_name
    availability_zones = ["us-east-1a", "us-east-1b", "us-east-1c"]
    image_mode         = "direct"
    executor_role_arn  = module.connector.executor_role_arn
    node_role_arns     = module.eks.node_role_arns
    data_bucket        = module.storage.data_bucket
    backup_bucket      = module.storage.backup_bucket
    storage_role_arn   = module.storage.role_arn
    enable_monitoring  = false
  }
}

# Minted once, after ClickHouse Cloud admits the cluster trust.
resource "clickhouse_byoc_enrollment_token" "connector" {
  byoc_id     = clickhouse_byoc_infrastructure.restricted.id
  ttl_seconds = 14400
}

# Install the connector with the token, for example with the Helm provider.
resource "helm_release" "connector" {
  name      = "clicklink"
  namespace = "clicklink"
  chart     = var.connector_chart

  set_sensitive {
    name  = "enrollment.token"
    value = clickhouse_byoc_enrollment_token.connector.token
  }
}

# Wait until the connector has enrolled and the infrastructure is ready.
resource "clickhouse_byoc_infrastructure_ready" "restricted" {
  byoc_id    = clickhouse_byoc_infrastructure.restricted.id
  depends_on = [helm_release.connector]
}
