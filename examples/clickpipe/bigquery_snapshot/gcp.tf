resource "random_id" "suffix" {
  byte_length = 4
}

locals {
  dataset_name    = split("/", data.google_bigquery_dataset.dataset.id)[length(split("/", data.google_bigquery_dataset.dataset.id)) - 1]
  staging_bucket_name     = "${var.gcp_project_id}-clickpipe-staging-${random_id.suffix.hex}"
  sa_name         = "clickpipe-bigquery-${random_id.suffix.hex}"
  sa_display_name = "ClickPipe BigQuery Service Account"
}

// Ensures the BigQuery dataset and tables exist
data "google_bigquery_dataset" "dataset" {
  project    = var.gcp_project_id
  dataset_id = var.bigquery_dataset_id
}

data "google_bigquery_table" "table" {
  for_each = toset(var.bigquery_table_names)

  dataset_id = var.bigquery_dataset_id
  table_id   = each.value
}

// This bucket is used by ClickPipe to stage data during BigQuery exports
resource "google_storage_bucket" "clickpipes_staging_bucket" {
  name          = local.staging_bucket_name
  location      = var.gcp_region
  project       = var.gcp_project_id
  force_destroy = true // do not use in production

  uniform_bucket_level_access = true

  lifecycle_rule {
    condition {
      age = 1
    }
    action {
      type = "Delete"
    }
  }
}

// Service account for ClickPipe to access BigQuery and GCS
resource "google_service_account" "clickpipes" {
  project      = var.gcp_project_id
  account_id   = local.sa_name
  display_name = local.sa_display_name
  description  = "Service account for ClickPipe to access BigQuery and GCS"
}

// Service account key for ClickPipe
resource "google_service_account_key" "clickpipes_key" {
  service_account_id = google_service_account.clickpipes.name
  public_key_type    = "TYPE_X509_PEM_FILE"
  private_key_type   = "TYPE_GOOGLE_CREDENTIALS_FILE"
}

// Allows to view BigQuery datasets and tables with dataset-level condition
resource "google_project_iam_member" "bigquery_data_viewer" {
  project = var.gcp_project_id
  role    = "roles/bigquery.dataViewer"
  member  = "serviceAccount:${google_service_account.clickpipes.email}"

  condition {
    title       = "Restrict access to specific dataset"
    description = "Allow access only to the designated BigQuery dataset"
    expression  = "resource.name.startsWith(\"projects/${var.gcp_project_id}/datasets/${local.dataset_name}\")"
  }
}

// This allows ClickPipes to run BigQuery export jobs
resource "google_project_iam_member" "bigquery_job_user" {
  project = var.gcp_project_id
  role    = "roles/bigquery.jobUser"
  member  = "serviceAccount:${google_service_account.clickpipes.email}"
}

// GCS Object Admin role with bucket-level condition
resource "google_project_iam_member" "storage_object_admin" {
  project = var.gcp_project_id
  role    = "roles/storage.objectAdmin"
  member  = "serviceAccount:${google_service_account.clickpipes.email}"

  condition {
    title       = "Restrict access to staging bucket"
    description = "Allow access only to the ClickPipe staging bucket"
    expression  = "resource.name.startsWith(\"projects/_/buckets/${local.staging_bucket_name}\")"
  }
}

// GCS Bucket Viewer role with bucket-level condition
resource "google_project_iam_member" "storage_bucket_viewer" {
  project = var.gcp_project_id
  role    = "roles/storage.bucketViewer"
  member  = "serviceAccount:${google_service_account.clickpipes.email}"

  condition {
    title       = "Restrict access to staging bucket"
    description = "Allow access only to the ClickPipe staging bucket"
    expression  = "resource.name.startsWith(\"projects/_/buckets/${local.staging_bucket_name}\")"
  }
}

output "clickpipes_bigquery_service_account_email" {
  value = google_service_account.clickpipes.email
}
