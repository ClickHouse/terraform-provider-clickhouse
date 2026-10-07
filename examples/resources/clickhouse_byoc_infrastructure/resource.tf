variable "aws_account_id" {
  description = "AWS account to deploy the BYOC infrastructure into."
  type        = string
}

variable "external_id" {
  description = "ExternalID of the BYOC onboarding IAM role trust policy."
  type        = string
}

resource "clickhouse_byoc_infrastructure" "example" {
  region_id   = "us-east-1"
  account_id  = var.aws_account_id
  external_id = var.external_id

  display_name   = "Production BYOC"
  vpc_cidr_range = "10.0.0.0/16"

  tags = {
    environment = "production"
  }
}

# Deploy a service into the BYOC infrastructure once it is ready.
resource "clickhouse_service" "byoc" {
  name           = "byoc service"
  password_hash  = "n4bQgYhMfWWaL+qgxVrQFaO/TxsrC4Is0V1sFbDwCgg=" # base64 encoded sha256 hash of "test"
  cloud_provider = "aws"
  region         = clickhouse_byoc_infrastructure.example.region_id
  byoc_id        = clickhouse_byoc_infrastructure.example.id

  # BYOC services use custom instance profiles; discover the available
  # profiles with the clickhouse_service_profiles data source.
  profile               = "v1-standard-byoc-4"
  min_replica_memory_gb = 8 # must equal the profile's memory_gi
  max_replica_memory_gb = 8
}
