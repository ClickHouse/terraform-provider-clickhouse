# these keys are for example only and won't work when pointed to a deployed ClickHouse OpenAPI server
organization_id = "aee076c1-3f83-4637-95b1-ad5a0a825b71"
token_key       = "avhj1U5QCdWAE9CA9"
token_secret    = "4b1dROiHQEuSXJHlV8zHFd0S7WQj7CGxz5kGJeJnca"
service_id      = "aee076c1-3f83-4637-95b1-ad5a0a825b71"

msk_cluster_arn = "arn:aws:kafka:us-east-2:123456789012:cluster/ExampleCluster/12345678-1234-1234-1234-123456789012-1"
msk_authentication = "SASL_SCRAM" # or SASL_IAM
msk_scram_user = "example_user"
msk_scram_password = "example_password"
kafka_topic = "example_topic"