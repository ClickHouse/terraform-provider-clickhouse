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
