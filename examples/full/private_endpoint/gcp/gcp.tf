variable "gcp_project" {
  type = string
}

variable "gcp_credentials" {
  type      = string
  sensitive = true
  default   = ""
  description = "Service account JSON key content. Leave empty to use Application Default Credentials (e.g. in CI with Workload Identity Federation)."
}

locals {
  # GCP resource names must match [a-z][-a-z0-9]*[a-z0-9].
  # Replace invalid characters with hyphens and strip leading/trailing hyphens.
  gcp_resource_name = replace(
    replace(
      replace(lower(var.service_name), "/[^a-z0-9]+/", "-"),
      "/^-+/", ""
    ),
    "/-+$/", ""
  )
}

provider "google" {
  project     = var.gcp_project
  region      = var.region
  credentials = var.gcp_credentials != "" ? var.gcp_credentials : null
}

resource "google_compute_network" "vpc" {
  name                    = local.gcp_resource_name
  auto_create_subnetworks = false
}

resource "google_compute_subnetwork" "subnet" {
  name          = local.gcp_resource_name
  ip_cidr_range = "10.0.0.0/24"
  region        = var.region
  network       = google_compute_network.vpc.id
}

// Static internal IP address for the PSC endpoint
resource "google_compute_address" "psc_endpoint_ip" {
  name         = local.gcp_resource_name
  address_type = "INTERNAL"
  purpose      = "GCE_ENDPOINT"
  region       = var.region
  subnetwork   = google_compute_subnetwork.subnet.self_link
}

// Private Service Connect forwarding rule targeting the ClickHouse service attachment
resource "google_compute_forwarding_rule" "psc_endpoint" {
  name                  = local.gcp_resource_name
  region                = var.region
  network               = google_compute_network.vpc.self_link
  ip_address            = google_compute_address.psc_endpoint_ip.self_link
  load_balancing_scheme = ""
  target                = "https://www.googleapis.com/compute/v1/${clickhouse_service.this.private_endpoint_config.endpoint_service_id}"
}

// Private DNS zone scoped to the VPC for resolving ClickHouse PSC hostnames
resource "google_dns_managed_zone" "psc_zone" {
  name        = "${local.gcp_resource_name}-clickhouse-psc"
  dns_name    = "${regex("^[^.]+\\.(.+)$", clickhouse_service.this.private_endpoint_config.private_dns_hostname)[0]}."
  description = "Private DNS zone for ClickHouse Private Service Connect"
  visibility  = "private"

  private_visibility_config {
    networks {
      network_url = google_compute_network.vpc.self_link
    }
  }
}

// Wildcard A record resolving all ClickHouse hostnames in the zone to the PSC endpoint IP
resource "google_dns_record_set" "psc_record" {
  name         = "*.${google_dns_managed_zone.psc_zone.dns_name}"
  type         = "A"
  ttl          = 300
  managed_zone = google_dns_managed_zone.psc_zone.name
  rrdatas      = [google_compute_address.psc_endpoint_ip.address]
}
