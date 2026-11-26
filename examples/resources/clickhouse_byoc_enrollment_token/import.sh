#!/bin/bash
# Import by BYOC infrastructure ID. The API returns a token only when minting
# it, so the imported token is null; replace the resource to mint a new one.
terraform import clickhouse_byoc_enrollment_token.connector 44444444-4444-4444-4444-444444444444
