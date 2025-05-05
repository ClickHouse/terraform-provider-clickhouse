variable "project_id" {
  type = string
}

provider "google" {
  project = var.project_id
  region = var.region
}

resource "google_kms_key_ring" "key_ring" {
  name     = clickhouse_service.service.id
  project  = var.project_id
  location = var.region
}

resource "google_kms_crypto_key" "key" {
  name                       = clickhouse_service.service.id
  key_ring                   = google_kms_key_ring.key_ring.id
  purpose                    = "ENCRYPT_DECRYPT"
  destroy_scheduled_duration = "86400s"

  version_template {
    algorithm        = "GOOGLE_SYMMETRIC_ENCRYPTION"
    protection_level = "SOFTWARE"
  }

  labels = {
    e2e = true,
    service = lower(replace(var.service_name, "/[^-a-zA-Z0-9_]/", "_"))
  }
}

data "google_client_openid_userinfo" "current" {}

resource "google_kms_crypto_key_iam_binding" "owners" {
  role          = "roles/owner"
  crypto_key_id = google_kms_crypto_key.key.id
  members       = ["user:${data.google_client_openid_userinfo.current.email}"]
}

resource "google_kms_crypto_key_iam_binding" "viewers" {
  role          = "roles/cloudkms.viewer"
  crypto_key_id = google_kms_crypto_key.key.id
  members       = ["serviceAccount:${clickhouse_service.service.transparent_data_encryption.role_id}"]
}

resource "google_kms_crypto_key_iam_binding" "decrypters" {
  role          = "roles/cloudkms.cryptoKeyDecrypter"
  crypto_key_id = google_kms_crypto_key.key.id
  members       = ["serviceAccount:${clickhouse_service.service.transparent_data_encryption.role_id}"]
}

resource "google_kms_crypto_key_iam_binding" "encrypters" {
  role          = "roles/cloudkms.cryptoKeyEncrypter"
  crypto_key_id = google_kms_crypto_key.key.id
  members       = ["serviceAccount:${clickhouse_service.service.transparent_data_encryption.role_id}"]
}
