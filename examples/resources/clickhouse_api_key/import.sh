#!/bin/bash
# API keys can be imported by specifying the key ID.
# Note: key_secret is only returned at creation, so it will be empty after import.
terraform import clickhouse_api_key.example xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
