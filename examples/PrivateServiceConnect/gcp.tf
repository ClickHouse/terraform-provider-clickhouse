variable "gcp_project_id" {
  type = string
}

variable "gcp_region" {
  type = string
}

variable "gcp_subnetwork" {
  type = string
}

variable "gcp_network" {
  type = string
}


provider "google" {
  project = var.gcp_project_id
  region  = var.gcp_region
}


resource "google_compute_address" "psc_endpoint_ip" {
  // if you want to assign specific address, uncomment the line below
  # address      = "10.148.0.2"
  address_type = "INTERNAL"
  name         = "clickhouse-cloud-psc-${var.gcp_region}"
  purpose      = "GCE_ENDPOINT"
  region       = var.gcp_region
  subnetwork   = var.gcp_subnetwork
}

resource "google_compute_forwarding_rule" "clickhouse_cloud_psc" {
  ip_address            = google_compute_address.psc_endpoint_ip.self_link
  name                  = "ch-cloud-${var.gcp_region}"
  network               = var.gcp_network
  region                = var.gcp_region
  load_balancing_scheme = ""
  target                = "https://www.googleapis.com/compute/v1/${data.clickhouse_private_endpoint_config.gcp_regional_endpoint_config.endpoint_service_id}"
  // uncomment next line to allow connections via this link from other than ${var.gcp_region} regions
  # allow_psc_global_access = true
}

// Service attachment for Private Service Connect 
data "clickhouse_private_endpoint_config" "gcp_regional_endpoint_config" {
  cloud_provider = "gcp"
  region         = var.gcp_region
}

// PSC uses ${var.gcp_region}.p.gcp.clickhouse.cloud. domain
resource "google_dns_managed_zone" "clickhouse_cloud_private_service_connect" {
  description   = "Private DNS zone for accessing ClickHouse Cloud using Private Service Connect"
  dns_name      = "${var.gcp_region}.p.gcp.clickhouse.cloud."
  force_destroy = false
  name          = "clickhouse-cloud-private-service-connect-${var.gcp_region}"
  visibility    = "private"

  // associate private DNS zone with network
  private_visibility_config {
    networks {
      network_url = var.gcp_network
    }
  }
}

// create a wildcard and point to google_compute_address.psc_endpoint_ip.address address
// any connections to "*.${var.gcp_region}.p.gcp.clickhouse.cloud." will be routed to PSC link 
resource "google_dns_record_set" "psc-wildcard" {
  managed_zone = google_dns_managed_zone.clickhouse_cloud_private_service_connect.name
  name         = "*.${var.gcp_region}.p.gcp.clickhouse.cloud."
  type         = "A"
  rrdatas      = [google_compute_address.psc_endpoint_ip.address]
  ttl          = 3600
}
