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
  default     = "kinesis-iam-user-example"
}


variable "database" {
  description = "The database to store the data"
  type        = string
  default     = "default"
}

variable "table" {
  description = "The table to store the data"
  type        = string
  default     = "kinesis_data"
}

variable "kinesis_stream_name" {
  description = "The name of the Kinesis stream"
  type        = string
}

variable "aws_region" {
  description = "The AWS region where the Kinesis stream is located"
  type        = string
  default     = "us-east-1"
}

variable "aws_access_key" {
  description = "AWS Access Key ID with permissions to read from Kinesis"
  type        = string
  sensitive   = true
}

variable "aws_secret_key" {
  description = "AWS Secret Access Key with permissions to read from Kinesis"
  type        = string
  sensitive   = true
}
