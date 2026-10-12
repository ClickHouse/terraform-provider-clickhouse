resource "clickhouse_service" "svc" {
  ...
}

resource "clickhouse_service_backup_bucket" "aws" {
  service_id      = clickhouse_service.svc.id
  bucket_provider = "AWS"
  bucket_path     = "https://my-bucket.s3.eu-west-1.amazonaws.com/clickhouse-backups"
  iam_role_arn    = "arn:aws:iam::123456789012:role/clickhouse-backups"
}

# GCP
# resource "clickhouse_service_backup_bucket" "gcp" {
#   service_id        = clickhouse_service.svc.id
#   bucket_provider   = "GCP"
#   bucket_path       = "https://storage.googleapis.com/my-bucket/clickhouse-backups"
#   access_key_id     = var.hmac_access_key_id
#   secret_access_key = var.hmac_secret
# }

# Azure
# resource "clickhouse_service_backup_bucket" "azure" {
#   service_id        = clickhouse_service.svc.id
#   bucket_provider   = "AZURE"
#   container_name    = "clickhouse-backups"
#   connection_string = var.azure_connection_string
# }
