variable "service_id" {
  description = "ClickHouse service ID"
}

variable "kafka_brokers" {
  description = "Kafka brokers"
}

variable "kafka_topics" {
  description = "Kafka topics"
}

variable "kafka_username" {
  description = "Username"
  sensitive   = true
}

variable "kafka_password" {
  description = "Password"
  sensitive   = true
}

variable "number_of_pipes" {
  description = "Number of pipes to create"
  default     = 5
}

resource "clickhouse_clickpipe" "multiple" {
  for_each = toset([for i in range(1, var.number_of_pipes + 1) : tostring(i)])

  name        = "📈 multiple pipe ${each.key}"

  service_id = var.service_id

  scaling = {
    replicas = tonumber(each.key)
  }

  state = "Running"

  source = {
    kafka = {
      type    = "confluent"
      format  = "JSONEachRow"
      brokers = var.kafka_brokers
      topics  = var.kafka_topics

      credentials = {
        username = var.kafka_username
        password = var.kafka_password
      }
    }
  }

  destination = {
    table    = "multiple_pipes_example_${each.key}"
    managed_table = true

    table_definition = {
      engine = {
        type = "MergeTree"
      }
    }

    columns = [
      {
        name: "area",
        type: "Int64"
      },
      {
        name: "averageSignal",
        type: "Int64"
      },
      {
        name: "cell",
        type: "Int64"
      },
      {
        name: "changeable",
        type: "Int64"
      },
      {
        name: "created",
        type: "DateTime64(9)"
      },
      {
        name: "lat",
        type: "Float64"
      },
      {
        name: "lon",
        type: "Float64"
      },
      {
        name: "mcc",
        type: "Int64"
      },
      {
        name: "net",
        type: "Int64"
      },
      {
        name: "radio",
        type: "String"
      },
      {
        name: "range",
        type: "Int64"
      },
      {
        name: "samples-renamed",
        type: "Int64"
      },
      {
        name: "unit",
        type: "Int64"
      },
      {
        name: "updated",
        type: "DateTime64(9)"
      }
    ]
  }

  field_mappings = [
    {
      source_field: "averageSignal",
      destination_field: "averageSignal"
    },
    {
      source_field: "cell",
      destination_field: "cell"
    },
    {
      source_field: "changeable",
      destination_field: "changeable"
    },
    {
      source_field: "created",
      destination_field: "created"
    },
    {
      source_field: "lat",
      destination_field: "lat"
    },
    {
      source_field: "lon",
      destination_field: "lon"
    },
    {
      source_field: "mcc",
      destination_field: "mcc"
    },
    {
      source_field: "net",
      destination_field: "net"
    },
    {
      source_field: "radio",
      destination_field: "radio"
    },
    {
      source_field: "range",
      destination_field: "range"
    },
    {
      source_field: "samples",
      destination_field: "samples-renamed"
    },
    {
      source_field: "unit",
      destination_field: "unit"
    },
    {
      source_field: "updated",
      destination_field: "updated"
    }
  ]
}

output "clickpipe_ids" {
  value = {
    for k, v in clickhouse_clickpipe.multiple : k => v.id
  }
}