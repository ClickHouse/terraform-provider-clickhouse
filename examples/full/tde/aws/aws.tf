variable "aws_key" {
  type = string
}

variable "aws_secret" {
  type = string
}

variable "aws_session_token" {
  type = string
  default = ""
}

locals {
  tags = {
    Role = "terraform-e2e-test"
    ServiceName = replace(var.service_name, "/[^-a-zA-Z0-9_.:/=+@ ]/", "_")
  }
}

provider "aws" {
  region     = var.region
  access_key = var.aws_key
  secret_key = var.aws_secret
  token      = var.aws_session_token
}

data "aws_caller_identity" "current" {}

data "aws_iam_policy_document" "policy" {
  # Allow root user on the account all access.
  statement {
    sid = "AllowRoot"

    actions   = ["kms:*"]
    resources = ["*"]
    principals {
      type        = "AWS"
      identifiers = ["arn:aws:iam::${data.aws_caller_identity.current.account_id}:root"]
    }
  }

  # Allow user that runs terraform to manage the KMS key.
  statement {
    sid = "AllowAdmins"
    actions = [
      "kms:Create*",
      "kms:Describe*",
      "kms:Enable*",
      "kms:List*",
      "kms:Put*",
      "kms:Update*",
      "kms:Revoke*",
      "kms:Disable*",
      "kms:Get*",
      "kms:Delete*",
      "kms:TagResource",
      "kms:UntagResource",
      "kms:ScheduleKeyDeletion",
      "kms:CancelKeyDeletion",
      "kms:RotateKeyOnDemand",
      "kms:Encrypt",
      "kms:Decrypt",
      "kms:ReEncrypt*",
      "kms:DescribeKey",
      "kms:CreateGrant",
      "kms:ListGrants",
      "kms:RevokeGrant"
    ]
    resources = ["*"]

    principals {
      type        = "AWS"
      identifiers = [data.aws_caller_identity.current.arn]
    }
  }

  # Allow clickhouse's accounts to access the KMS key.
  statement {
    sid = "AllowClickHouse"
    actions = [
      "kms:Encrypt",
      "kms:Decrypt",
      "kms:ReEncrypt*",
      "kms:DescribeKey",
    ]
    resources = ["*"]

    principals {
      type        = "AWS"
      identifiers = [clickhouse_service.service.transparent_data_encryption.role_id]
    }
  }
}

resource "aws_kms_key" "enc" {
  customer_master_key_spec = "SYMMETRIC_DEFAULT"
  deletion_window_in_days  = 7
  description              = var.service_name
  enable_key_rotation      = false
  is_enabled               = true
  key_usage                = "ENCRYPT_DECRYPT"
  multi_region             = false

  policy = data.aws_iam_policy_document.policy.json

  tags = local.tags
}
