The *clickhouse_api_key_id* data source can be used to retrieve the UUID of a ClickHouse cloud API key.
It is meant to be used in the *clickhouse_service* resource to set the `query_api_endpoints` attribute.

It can be used in two ways:

1) To retrieve information about an API Key, by providing its name
2) To retrieve information about the API Key currently configured for running the terraform provider

In both cases the data source will contain the `id` and `name` attributes.
