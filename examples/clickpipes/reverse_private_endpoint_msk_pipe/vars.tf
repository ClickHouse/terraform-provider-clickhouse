variable "organization_id" {}
variable "token_key" {}
variable "token_secret" {}

variable "service_id" {
  description = "ClickHouse service ID"
}

variable "msk_cluster_arn" {
  description = "MSK cluter ARN"
}

variable "msk_authentication" {
  description = "MSK authentication"
  default     = "SCRAM-SHA-512" # or IAM_USER or IAM_ROLE
}

variable "msk_scram_user" {
  description = "MSK scram user"
  default     = "scram_user"
}

variable "msk_scram_password" {
  description = "MSK scram password"
  default     = "scram_password"
}

variable "kafka_topic" {
  description = "Kafka topic"
  default     = "my_topic"
}
