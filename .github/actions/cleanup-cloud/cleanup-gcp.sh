#!/usr/bin/env bash

set -euo pipefail

GCP_PROJECT="${GCP_PROJECT:?"GCP_PROJECT cannot be empty"}"
GCP_REGION="${GCP_REGION:?"GCP_REGION cannot be empty"}"
SERVICE_NAME="${SERVICE_NAME:?"SERVICE_NAME cannot be empty"}"

# Derive the sanitized GCP resource name (must match [a-z][-a-z0-9]*[a-z0-9])
# Uses the same logic as locals.gcp_resource_name in gcp.tf
gcp_name=$(echo "${SERVICE_NAME}" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9]/-/g' | sed 's/-\+/-/g' | sed 's/^-\+//' | sed 's/-\+$//')
dns_zone_name="${gcp_name}-clickhouse-psc"

### DNS record sets
echo "::group::Deleting DNS record sets..."
if gcloud dns managed-zones describe "${dns_zone_name}" --project="${GCP_PROJECT}" >/dev/null 2>&1; then
  mapfile -t RECORD_NAMES < <(gcloud dns record-sets list --zone="${dns_zone_name}" --project="${GCP_PROJECT}" --format="value(name,type)" | grep -v "^${dns_zone_name}\." | awk '{print $1}')
  for name in "${RECORD_NAMES[@]:-}"; do
    [ -z "${name}" ] && continue
    record_type=$(gcloud dns record-sets list --zone="${dns_zone_name}" --project="${GCP_PROJECT}" --format="value(type)" --filter="name=${name}" | head -1)
    echo "Deleting DNS record ${name} (${record_type})..."
    gcloud dns record-sets delete "${name}" --type="${record_type}" --zone="${dns_zone_name}" --project="${GCP_PROJECT}" --quiet || true
  done
else
  echo "DNS zone '${dns_zone_name}' does not exist, skipping record deletion."
fi
echo "::endgroup::"

### DNS managed zone
echo "::group::Deleting DNS managed zone..."
if gcloud dns managed-zones describe "${dns_zone_name}" --project="${GCP_PROJECT}" >/dev/null 2>&1; then
  echo "Deleting DNS zone ${dns_zone_name}..."
  gcloud dns managed-zones delete "${dns_zone_name}" --project="${GCP_PROJECT}" --quiet
else
  echo "DNS zone '${dns_zone_name}' does not exist, skipping."
fi
echo "::endgroup::"

### Forwarding rule (PSC endpoint)
echo "::group::Deleting forwarding rule..."
if gcloud compute forwarding-rules describe "${gcp_name}" --region="${GCP_REGION}" --project="${GCP_PROJECT}" >/dev/null 2>&1; then
  echo "Deleting forwarding rule ${gcp_name}..."
  gcloud compute forwarding-rules delete "${gcp_name}" --region="${GCP_REGION}" --project="${GCP_PROJECT}" --quiet
else
  echo "Forwarding rule '${gcp_name}' does not exist, skipping."
fi
echo "::endgroup::"

### Internal IP address
echo "::group::Deleting internal IP address..."
if gcloud compute addresses describe "${gcp_name}" --region="${GCP_REGION}" --project="${GCP_PROJECT}" >/dev/null 2>&1; then
  echo "Deleting address ${gcp_name}..."
  gcloud compute addresses delete "${gcp_name}" --region="${GCP_REGION}" --project="${GCP_PROJECT}" --quiet
else
  echo "Address '${gcp_name}' does not exist, skipping."
fi
echo "::endgroup::"

### Subnetwork
echo "::group::Deleting subnetwork..."
if gcloud compute networks subnets describe "${gcp_name}" --region="${GCP_REGION}" --project="${GCP_PROJECT}" >/dev/null 2>&1; then
  echo "Deleting subnet ${gcp_name}..."
  gcloud compute networks subnets delete "${gcp_name}" --region="${GCP_REGION}" --project="${GCP_PROJECT}" --quiet
else
  echo "Subnet '${gcp_name}' does not exist, skipping."
fi
echo "::endgroup::"

### VPC network
echo "::group::Deleting VPC network..."
if gcloud compute networks describe "${gcp_name}" --project="${GCP_PROJECT}" >/dev/null 2>&1; then
  echo "Deleting network ${gcp_name}..."
  gcloud compute networks delete "${gcp_name}" --project="${GCP_PROJECT}" --quiet
else
  echo "Network '${gcp_name}' does not exist, skipping."
fi
echo "::endgroup::"
