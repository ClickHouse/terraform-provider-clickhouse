# ClickPipe Kafka Confluent E2E Test

This example creates a ClickHouse service and a ClickPipe that ingests data from Confluent Kafka using SASL/PLAIN authentication.

## What Gets Created

1. **ClickHouse Service**: A basic ClickHouse Cloud service in AWS
2. **ClickPipe**: A Kafka ClickPipe that:
   - Connects to an existing Confluent Kafka cluster using SASL/PLAIN authentication
   - Consumes from specified Kafka topics
   - Ingests data into a managed ClickHouse table

## Prerequisites

### Confluent Kafka Cluster Requirements

You need an existing Confluent Kafka cluster with:
- SASL/PLAIN authentication enabled
- At least one topic with data (or empty topic for testing)
- Network connectivity allowing ClickHouse Cloud to reach the brokers

### Authentication

The ClickPipe uses SASL/PLAIN authentication with API key and secret:
- `kafka_username`: Your Confluent Kafka API key
- `kafka_password`: Your Confluent Kafka API secret

You can create API keys in the Confluent Cloud console under your cluster settings.

## Variables

| Variable | Type | Required | Description |
|----------|------|----------|-------------|
| `organization_id` | string | Yes | ClickHouse Cloud organization ID |
| `token_key` | string | Yes | ClickHouse Cloud API key |
| `token_secret` | string | Yes | ClickHouse Cloud API secret |
| `service_name` | string | No | Name for the ClickHouse service (default: "My ClickPipe Kafka Confluent Test") |
| `region` | string | No | AWS region (default: "us-east-1") |
| `kafka_brokers` | list(string) | Yes | List of Confluent Kafka broker endpoints (e.g., ["pkc-xxxxx.us-east-1.aws.confluent.cloud:9092"]) |
| `kafka_topics` | list(string) | Yes | List of Kafka topics to consume from |
| `kafka_username` | string | Yes | Confluent Kafka API key |
| `kafka_password` | string | Yes | Confluent Kafka API secret |

## Usage

1. Copy `variables.tfvars.sample` to `variables.tfvars`
2. Fill in your ClickHouse Cloud credentials and Confluent Kafka details
3. Initialize and apply:
   ```bash
   terraform init
   terraform plan -var-file=variables.tfvars
   terraform apply -var-file=variables.tfvars
   ```

## Expected Data Format

This example expects Kafka messages in JSON format with the following schema (cell tower data):
```json
{
  "radio": "CDMA",
  "mcc": "250",
  "cell": "25456",
  "lat": "55.332283",
  "lon": "85.899208",
  "created": "2017-09-13 15:03:32"
}
```

The ClickPipe is configured with `format = "JSONEachRow"` which expects one JSON object per message. Only the fields specified in the field mappings (radio, mcc, cell, lat, lon, created) will be ingested from the full cell tower data schema.

## Outputs

- `service_id`: The ID of the created ClickHouse service
- `service_endpoints`: Connection endpoints for the ClickHouse service
- `clickpipe_id`: The ID of the created ClickPipe
- `clickpipe_state`: Current state of the ClickPipe (should be "running" after successful creation)

## E2E Testing

This example is designed for automated e2e testing in CI/CD. The test validates:
- ClickHouse service creation
- ClickPipe resource creation with Confluent Kafka source
- State transitions (provisioning → running)
- Resource cleanup (terraform destroy)

The test does NOT validate actual data ingestion (would require producing messages to Kafka).

## Cleanup

To destroy all resources:
```bash
terraform destroy -var-file=variables.tfvars
```

## Troubleshooting

### ClickPipe stuck in "provisioning"
- Verify Kafka API key and secret are correct
- Ensure Kafka cluster is accessible from ClickHouse Cloud
- Check that the Kafka API key has permissions to read from the topics

### Connection errors
- Verify broker endpoints are correct (use port 9092 for SASL)
- Check network connectivity between ClickHouse Cloud and Confluent
- Ensure SASL/PLAIN authentication is enabled on the cluster

### Data not ingesting
- Verify topics exist and have data
- Check JSON format matches expected schema
- Review ClickPipe logs in ClickHouse Cloud console
- Ensure the API key has ACLs allowing topic read access
