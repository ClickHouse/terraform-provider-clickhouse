variable "organization_id" {
  description = "ClickHouse Cloud organization ID"
}
variable "token_key" {
  description = "ClickHouse Cloud API token key"
}
variable "token_secret" {
  description = "ClickHouse Cloud API token secret"
}

variable "service_id" {
  description = "ClickHouse ClickPipe service ID"
}

variable "gcp_project_id" {
  description = "GCP project ID where the BigQuery dataset is located"
}

variable "gcp_region" {
  description = "GCP region for the BigQuery dataset"
}

variable "bigquery_dataset_id" {
  description = "Source BigQuery dataset ID"
}

variable "bigquery_table_names" {
  description = "Source BigQuery table names"
  type        = list(string)
}

variable "bigquery_watermark_column" {
  description = "TIMESTAMP column present in every source table, used as the CDC watermark"
  type        = string
  default     = "updated_at"
}

variable "sync_interval_seconds" {
  description = "Interval in seconds between CDC syncs"
  type        = number
  default     = 60
}
