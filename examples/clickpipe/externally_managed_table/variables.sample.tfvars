# these keys are for example only and won't work when pointed to a deployed ClickHouse OpenAPI server
organization_id = "aee076c1-3f83-4637-95b1-ad5a0a825b71"
token_key       = "avhj1U5QCdWAE9CA9"
token_secret    = "4b1dROiHQEuSXJHlV8zHFd0S7WQj7CGxz5kGJeJnca"
service_id      = "aee076c1-3f83-4637-95b1-ad5a0a825b71"

kafka_brokers = "broker.us-east-2.aws.confluent.cloud:9092"
kafka_topics = "cell_towers"

kafka_username =  ""
kafka_password = ""

table_name = "externally_managed_table"

table_columns = [
  {
    name: "id",
    type: "UInt64"
  },
  {
    name: "name",
    type: "String"
  }
]

field_mappings = [
  {
    source_field      = "bar",
    destination_field = "id"
  },
  {
    source_field      = "foo",
    destination_field = "name"
  }
]
