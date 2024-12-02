#!/bin/bash

set -e

api_url=""
organization_id=""
api_key_id=""
api_key_secret=""
region=""

cloud="$1"

case "${api_env}" in
Production)
api_url="$(echo "${api_env_production}" | jq -r .api_url)"
organization_id="$(echo "${api_env_production}" | jq -r .organization_id)"
api_key_id="$(echo "${api_env_production}" | jq -r .api_key_id)"
api_key_secret="$(echo "${api_env_production}" | jq -r .api_key_secret)"
region="$(echo "${api_env_production}" | jq -rc --arg cloud $cloud '.regions[$cloud]' | jq -c '.[]' | shuf -n 1 |jq -r .)"
;;

Staging)
api_url="$(echo "${api_env_staging}" | jq -r .api_url)"
organization_id="$(echo "${api_env_staging}" | jq -r .organization_id)"
api_key_id="$(echo "${api_env_staging}" | jq -r .api_key_id)"
api_key_secret="$(echo "${api_env_staging}" | jq -r .api_key_secret)"
region="$(echo "${api_env_staging}" | jq -rc --arg cloud $cloud '.regions[$cloud]' | jq -c '.[]' | shuf -n 1 |jq -r .)"
;;

Development)
api_url="$(echo "${api_env_development}" | jq -r .api_url)"
organization_id="$(echo "${api_env_development}" | jq -r .organization_id)"
api_key_id="$(echo "${api_env_development}" | jq -r .api_key_id)"
api_key_secret="$(echo "${api_env_development}" | jq -r .api_key_secret)"
region="$(echo "${api_env_development}" | jq -rc --arg cloud $cloud '.regions[$cloud]' | jq -c '.[]' | shuf -n 1 |jq -r .)"
;;

Custom)
if [ "${api_url}" == "" ]; then
  echo "api_url input must be set when api_env is set to 'Custom'"
  exit 1
fi

if [ "${organization_id}" == "" ]; then
  echo "organization_id input must be set when api_env is set to 'Custom'"
  exit 1
fi

if [ "${api_key_id}" == "" ]; then
  echo "api_key_id input must be set when api_env is set to 'Custom'"
  exit 1
fi

if [ "${api_key_secret}" == "" ]; then
  echo "api_key_secret input must be set when api_env is set to 'Custom'"
  exit 1
fi
;;
esac

echo "api_url=${api_url}" >> $GITHUB_OUTPUT
echo "organization_id=${organization_id}" >> $GITHUB_OUTPUT
echo "api_key_id=${api_key_id}" >> $GITHUB_OUTPUT
echo "api_key_secret=${api_key_secret}" >> $GITHUB_OUTPUT
echo "region=${region}" >> $GITHUB_OUTPUT
