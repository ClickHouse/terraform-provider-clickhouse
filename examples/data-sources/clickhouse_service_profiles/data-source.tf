# Step 1: discover the profiles available to your organization.
# Apply this alone first and inspect the output to pick a profile:
#
#   terraform output available_profiles
data "clickhouse_service_profiles" "byoc" {
  region_id = "us-east-1"
  byoc_id   = var.byoc_id # optional; BYOC profiles are only returned when set
}

output "available_profiles" {
  value = data.clickhouse_service_profiles.byoc.profiles
}

# Step 2: pin the chosen profile name literally in the service config.
# The profile name is the only unique key — do not select dynamically
# (e.g. profiles[0]): the list can change and 'profile' forces replacement.
resource "clickhouse_service" "byoc" {
  name           = "byoc-service"
  cloud_provider = "aws"
  region         = "us-east-1"
  byoc_id        = var.byoc_id

  profile               = "v1-standard-byoc-4"
  min_replica_memory_gb = 8 # must equal the profile's memory_gi
  max_replica_memory_gb = 8

  lifecycle {
    # Fail at plan time with a clear message if the pinned profile is no
    # longer available for this BYOC infra/region.
    precondition {
      condition     = contains([for p in data.clickhouse_service_profiles.byoc.profiles : p.profile], "v1-standard-byoc-4")
      error_message = "Profile v1-standard-byoc-4 is not available for this BYOC infra/region."
    }
    prevent_destroy = true
  }
}
