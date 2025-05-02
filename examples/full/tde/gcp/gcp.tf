variable "project_id" {
  type = string
}

provider "google" {
  project = var.project_id
  region = var.region
}

resource "google_kms_key_ring" "key_ring" {
  name     = replace(var.service_name, "/[^-a-zA-Z0-9_]/", "_")
  project  = var.project_id
  location = var.region
}

resource "google_kms_crypto_key" "key" {
  name                       = replace(var.service_name, "/[^-a-zA-Z0-9_]/", "_")
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
