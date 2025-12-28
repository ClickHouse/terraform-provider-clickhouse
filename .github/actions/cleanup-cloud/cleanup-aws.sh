#!/usr/bin/env bash

set -euo pipefail

AWS_REGION="${AWS_REGION:?"AWS_REGION cannot be empty"}"
SERVICE_NAME="${SERVICE_NAME:?"SERVICE_NAME cannot be empty"}"

### VPC Endpoints
echo "::group::Deleting VPC Endpoints..."
ATTEMPTS=10
while [ $ATTEMPTS -gt 0 ]; do
  ATTEMPTS=$((ATTEMPTS - 1))

  mapfile -t ENDPOINT_IDS < <(aws ec2 --region "${AWS_REGION}" describe-vpc-endpoints | jq --arg name "${SERVICE_NAME}" -r '.VpcEndpoints[] | select(.Tags[0].Value == $name) | .VpcENDPOINT_ID')

  if [[ "${#ENDPOINT_IDS[@]}" -eq 0 ]]; then
    echo "No endpoints to delete."
    break
  fi

  echo "Deleting endpoints ${ENDPOINT_IDS[*]}..."
  for ENDPOINT_ID in "${ENDPOINT_IDS[@]}"; do
    echo "Deleting vpc endpoint ${ENDPOINT_ID}..."
    aws ec2 --region "${AWS_REGION}" delete-vpc-endpoints --vpc-endpoint-ids "${ENDPOINT_ID}"
  done

  echo "Waiting 60 seconds..."
  sleep 60
done
echo "::endgroup::"

### Security Groups
echo "::group::Deleting Security Groups..."
mapfile -t SG_IDS < <(aws ec2 --region "${AWS_REGION}" describe-security-groups | jq --arg name "${SERVICE_NAME}" -r '.SecurityGroups[] | select(.Tags[0].Value == $name) | .GroupId')

if [[ "${#SG_IDS[@]}" -eq 0 ]]; then
  echo "No Security Groups to delete."
fi

for SG_ID in "${SG_IDS[@]}"; do
  echo "Deleting SG ${SG_ID}..."
  aws ec2 --region "${AWS_REGION}" delete-security-group --group-id "${SG_ID}"
done
echo "::endgroup::"

### Subnets
echo "::group::Deleting Subnets..."
mapfile -t SUBNET_IDS < <(aws ec2 --region "${AWS_REGION}" describe-subnets | jq --arg name "${SERVICE_NAME}" -r '.Subnets[] | select(.Tags[0].Value == $name) | .SubnetId')

if [[ "${#SUBNET_IDS[@]}" -eq 0 ]]; then
  echo "No Subnets to delete."
fi

for SUBNET_ID in "${SUBNET_IDS[@]}"; do
  echo "Deleting subnet ${SUBNET_ID}..."
  aws ec2 --region "${AWS_REGION}" delete-subnet --subnet-id "${SUBNET_ID}"
done
echo "::endgroup::"

### VPCS
echo "::group::Deleting VPCs..."
mapfile -t VPC_IDS < <(aws ec2 --region "${AWS_REGION}" describe-vpcs | jq --arg name "${SERVICE_NAME}" -r '.Vpcs[] | select(.Tags[0].Value == $name) | .VpcId')

if [[ "${#VPC_IDS[@]}" -eq 0 ]]; then
  echo "No VPCs to delete."
fi

for VPC_ID in "${VPC_IDS[@]}"; do
  echo "Deleting vpc ${VPC_ID}..."
  aws ec2 --region "${AWS_REGION}" delete-vpc --vpc-id "${VPC_ID}"
done
echo "::endgroup::"
