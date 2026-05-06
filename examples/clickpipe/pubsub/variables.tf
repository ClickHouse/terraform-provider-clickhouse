variable "token_key" {
  description = "ClickHouse Cloud API Key ID"
  type        = string
}

variable "token_secret" {
  description = "ClickHouse Cloud API Key Secret"
  type        = string
  sensitive   = true
}

variable "organization_id" {
  description = "ClickHouse Cloud Organization ID"
  type        = string
}

variable "service_id" {
  description = "The ID of the ClickHouse Cloud service"
  type        = string
}

variable "pipe_name" {
  description = "The name of the ClickPipe"
  type        = string
  default     = "pubsub-example"
}

variable "database" {
  description = "The database to store the data"
  type        = string
  default     = "default"
}

variable "table" {
  description = "The table to store the data"
  type        = string
  default     = "pubsub_data"
}

variable "gcp_project_id" {
  description = "The GCP project ID that owns the Pub/Sub topic"
  type        = string
}

variable "pubsub_topic" {
  description = "The Pub/Sub topic name (not the fully-qualified path)"
  type        = string
}

variable "gcp_service_account_b64" {
  description = "Base64-encoded GCP service account JSON key file contents"
  type        = string
  sensitive   = true
}
